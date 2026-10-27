package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportProgress
import com.tryhisar.hisar.model.ImportVaultGroup
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultSummary
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** What another password manager handed over through Android's credential transfer. */
sealed interface CredentialExchangeResult {
    /** The Credential Exchange (CXF) JSON; the model clears it once the backend has read it. */
    class Received(val credentialData: ByteArray) : CredentialExchangeResult

    /** The user closed the system's list of apps, or the other app: there is nothing to say. */
    data object Cancelled : CredentialExchangeResult

    /** A sentence a user can read. */
    data class Failed(val message: String) : CredentialExchangeResult
}

/** Asks the system for another app's credentials: it lists the apps that can export, and the user picks one. */
fun interface CredentialExchangeReceiver {
    suspend fun receive(): CredentialExchangeResult
}

/** The import sheet: null while closed. */
sealed interface CredentialImportSheet {
    /** The system's list or the other app is open, or Hisar is reading what it handed over. */
    data object Receiving : CredentialImportSheet

    data class Failed(val message: String) : CredentialImportSheet

    data class Previewing(val preview: ImportPreview) : CredentialImportSheet

    /** An encrypted Hisar export is open: the user enters its export passphrase and openImportFile opens it. */
    data class EnteringPassphrase(val fileName: String) : CredentialImportSheet

    data class Importing(val progress: ImportProgress) : CredentialImportSheet
}

/**
 * Credential Exchange: another password manager on this device hands its logins and passkeys to Hisar, the port of the
 * iPhone app's import from another app. The user starts it from Settings; Android cannot hold the transfer back, so what
 * arrives while Hisar is locked waits in memory for the unlock. The backend keeps the parsed items until the import starts,
 * and cancelImport drops them. Locking closes a previewed or running import; signing out drops everything.
 */
class CredentialImportModel(private val context: ModelContext) {
    private val mutableSheet = MutableStateFlow<CredentialImportSheet?>(null)
    private val mutableIsImporting = MutableStateFlow(false)
    private var pendingCredentialData: ByteArray? = null
    private var receiveJob: Job? = null

    /** Grows when the sheet closes, so work a closed sheet started leaves the next one alone. */
    private var session = 0

    val sheet: StateFlow<CredentialImportSheet?> = mutableSheet.asStateFlow()

    /** A batch is being saved. */
    val isImporting: StateFlow<Boolean> = mutableIsImporting.asStateFlow()

    /** A sentence for an alert, when there is no sheet to show it in. */
    val message = MutableStateFlow<String?>(null)

    val isShown: Boolean get() = sheet.value != null
    var sourceFileName: String? = null
        private set
    val preview: ImportPreview? get() = (sheet.value as? CredentialImportSheet.Previewing)?.preview
    val progress: ImportProgress? get() = (sheet.value as? CredentialImportSheet.Importing)?.progress

    /** Whether credentials that arrived while Hisar was locked wait for the unlock. */
    val isWaitingForUnlock: Boolean get() = pendingCredentialData != null

    /** The vaults an import can go to: the personal vault and those the user may edit. */
    val allVault: List<VaultSummary> get() = context.unlockedData.value.allVault.filter(::isWritableVault)
    val allDestinationGroup: List<ImportVaultGroup>
        get() = OrganizationLogic.importVaultGroups(allVault, context.unlockedData.value.allOrganization)

    fun destinationLabel(vaultID: String, fallback: String = vaultID): String =
        allDestinationGroup.asSequence().flatMap { it.allChoice.asSequence() }.firstOrNull { it.id == vaultID }?.label ?: fallback

    init {
        context.addStatusListener(::handleStatus)
    }

    /** Opens the sheet and asks [receiver] for the credentials. */
    fun start(receiver: CredentialExchangeReceiver) {
        val status = context.status.value
        if (!status.isSignedIn) {
            message.value = SIGNED_OUT_MESSAGE
            return
        }
        if (!status.isUnlocked || isShown) return
        sourceFileName = null
        context.errorMessage.value = null
        val current = session
        mutableSheet.value = CredentialImportSheet.Receiving
        receiveJob = context.scope.launch { receive(current, receiver) }
    }

    /** Opens the sheet with a Hisar export [content] the user picked ([fileName] names it), read by the backend. */
    fun startHisarExport(fileName: String, content: ByteArray) {
        val status = context.status.value
        if (!status.isSignedIn) {
            message.value = SIGNED_OUT_MESSAGE
            content.fill(0)
            return
        }
        if (!status.isUnlocked || isShown) {
            content.fill(0)
            return
        }
        context.errorMessage.value = null
        sourceFileName = fileName
        val current = session
        mutableSheet.value = CredentialImportSheet.Receiving
        context.scope.launch { prepareHisarExport(current, fileName, content) }
    }

    private suspend fun prepareHisarExport(current: Int, fileName: String, content: ByteArray) {
        val allKnownVault = context.unlockedData.value.allVault
        val preview = context.perform { backend ->
            val vault = chooseVault(allKnownVault.ifEmpty { backend.listAllVault() })
                ?: throw BackendError("You have no vault you can add items to.")
            backend.prepareHisarExportImport(vault.id, fileName, content)
        }
        content.fill(0)
        if (session != current) {
            // Closed or signed out while reading: drop what the backend prepared, and any sentence about it.
            if (preview == null) context.errorMessage.value = null
            cancelBackendImport()
            return
        }
        mutableSheet.value = when {
            preview == null -> CredentialImportSheet.Failed(context.errorMessage.value ?: UNREADABLE_MESSAGE).also { context.errorMessage.value = null }
            preview.isPassphraseRequired -> CredentialImportSheet.EnteringPassphrase(preview.fileName)
            else -> CredentialImportSheet.Previewing(preview)
        }
    }

    /** Opens the waiting encrypted Hisar export with its export passphrase; a wrong one keeps the passphrase step. */
    suspend fun openImportFile(passphrase: String) {
        if (sheet.value !is CredentialImportSheet.EnteringPassphrase) return
        val current = session
        val preview = context.perform { it.openImportFile(passphrase) }
        if (session != current || sheet.value !is CredentialImportSheet.EnteringPassphrase) return
        // A wrong passphrase leaves its sentence in errorMessage and keeps the passphrase step.
        if (preview != null) {
            context.errorMessage.value = null
            mutableSheet.value = CredentialImportSheet.Previewing(preview)
        }
    }

    private suspend fun receive(current: Int, receiver: CredentialExchangeReceiver) {
        val result = receiver.receive()
        if (session != current) {
            (result as? CredentialExchangeResult.Received)?.credentialData?.fill(0)
            return
        }
        when (result) {
            is CredentialExchangeResult.Received -> {
                pendingCredentialData = result.credentialData
                handlePendingCredentials()
            }
            CredentialExchangeResult.Cancelled -> close()
            is CredentialExchangeResult.Failed -> mutableSheet.value = CredentialImportSheet.Failed(result.message)
        }
    }

    private fun handleStatus(previous: VaultStatus, current: VaultStatus) {
        if (!current.isSignedIn && (isShown || pendingCredentialData != null)) {
            if (sheet.value == CredentialImportSheet.Receiving) message.value = SIGNED_OUT_MESSAGE
            close()
            return
        }
        // Locking drops the prepared import in the backend too; a transfer still under way waits for the unlock.
        if (previous.isUnlocked && !current.isUnlocked && isShown && sheet.value != CredentialImportSheet.Receiving) close()
        handlePendingCredentials()
    }

    private fun handlePendingCredentials() {
        val credentialData = pendingCredentialData ?: return
        val status = context.status.value
        if (!status.isUnlocked || status.updateRequired != null) return
        pendingCredentialData = null
        val current = session
        context.scope.launch { prepare(current, credentialData) }
    }

    private suspend fun prepare(current: Int, credentialData: ByteArray) {
        val allKnownVault = context.unlockedData.value.allVault
        val preview = context.perform { backend ->
            val vault = chooseVault(allKnownVault.ifEmpty { backend.listAllVault() })
                ?: throw BackendError("You have no vault you can add items to.")
            backend.prepareCredentialExchangeImport(vault.id, credentialData)
        }
        if (preview == null && session == current) {
            // The backend may have locked before its Locked event arrived: then the credentials wait for the unlock.
            context.refreshStatus()
            val status = context.status.value
            if (session == current && status.isSignedIn && !status.isUnlocked) {
                context.errorMessage.value = null
                pendingCredentialData = credentialData
                return
            }
        }
        credentialData.fill(0)
        if (session != current) {
            // Closed or signed out while preparing: drop what the backend just prepared, and any sentence about it.
            if (preview == null) context.errorMessage.value = null
            cancelBackendImport()
            return
        }
        mutableSheet.value = if (preview != null) {
            CredentialImportSheet.Previewing(preview)
        } else {
            CredentialImportSheet.Failed(context.errorMessage.value ?: UNREADABLE_MESSAGE).also { context.errorMessage.value = null }
        }
    }

    /** Shows the import again for another vault, with that vault's duplicates. */
    suspend fun previewCredentialImport(vaultID: String) {
        if (preview == null) return
        val current = session
        val newPreview = context.perform { it.previewImport(vaultID) } ?: return
        if (session == current && preview != null) mutableSheet.value = CredentialImportSheet.Previewing(newPreview)
    }

    /** Sends one source group, such as a collection of the other app, to another vault; the other groups stay where they are. */
    suspend fun mapCredentialImportGroup(groupID: String, vaultID: String) {
        if (preview == null) return
        val current = session
        val newPreview = context.perform { it.mapImportGroup(groupID, vaultID) } ?: return
        if (session == current && preview != null) mutableSheet.value = CredentialImportSheet.Previewing(newPreview)
    }

    suspend fun startCredentialImport(isSkippingDuplicate: Boolean) {
        val preview = preview ?: return
        if (mutableIsImporting.value) return
        mutableIsImporting.value = true
        try {
            val current = session
            val progress = context.perform(isBusyShown = false) { it.startImport(preview.vaultID, isSkippingDuplicate) } ?: return
            if (session != current) return
            mutableSheet.value = CredentialImportSheet.Importing(progress)
            continueUntilDone(current)
        } finally {
            mutableIsImporting.value = false
        }
    }

    /** Retry: an error pauses the import where it was, and the backend continues from there. */
    suspend fun continueCredentialImport() {
        if (progress?.isDone != false || mutableIsImporting.value) return
        mutableIsImporting.value = true
        try {
            continueUntilDone(session)
        } finally {
            mutableIsImporting.value = false
        }
    }

    // The backend saves a batch per call. The finished import lists the vaults again.
    private suspend fun continueUntilDone(current: Int) {
        context.errorMessage.value = null
        while (session == current) {
            val progress = progress ?: return
            if (progress.isDone) {
                context.refreshAll()
                return
            }
            val nextProgress = context.perform(isBusyShown = false) { it.continueImport() }
            if (session != current) {
                context.errorMessage.value = null
                return
            }
            mutableSheet.value = CredentialImportSheet.Importing(nextProgress ?: return)
        }
    }

    /** Cancel and Done: drops the import in the backend. Items a cancelled import already saved stay, so the vaults are listed again. */
    fun closeCredentialImport() {
        if (!isShown) return
        val isPartlySaved = progress?.isDone == false
        close()
        context.errorMessage.value = null
        if (isPartlySaved) context.scope.launch { context.refreshAll() }
    }

    private fun close() {
        session += 1
        receiveJob?.cancel()
        receiveJob = null
        pendingCredentialData?.fill(0)
        pendingCredentialData = null
        // A previewed, running, or waiting (encrypted, not yet opened) import is dropped in the backend too.
        if (preview != null || progress != null || sheet.value is CredentialImportSheet.EnteringPassphrase) cancelBackendImport()
        sourceFileName = null
        mutableSheet.value = null
    }

    // A batch being saved holds the backend until it ends, so the import is dropped in the background.
    private fun cancelBackendImport() {
        context.scope.launch { context.attempt { it.cancelImport() } }
    }

    companion object {
        const val SIGNED_OUT_MESSAGE = "Sign in to Hisar, then import from the other app again."
        const val UNREADABLE_MESSAGE = "Hisar couldn't read the items from the other app. Export them again."

        fun isWritableVault(vault: VaultSummary): Boolean = vault.type == VaultSummary.TYPE_PERSONAL || vault.role != Member.VAULT_ROLE_READER

        /** The vault an import starts in: the personal vault, else the first the user may edit. */
        fun chooseVault(allVault: List<VaultSummary>): VaultSummary? =
            allVault.firstOrNull { it.type == VaultSummary.TYPE_PERSONAL } ?: allVault.firstOrNull(::isWritableVault)
    }
}
