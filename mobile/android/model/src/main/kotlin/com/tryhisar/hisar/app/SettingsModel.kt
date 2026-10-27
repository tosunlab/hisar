package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.CredentialChangeResult
import com.tryhisar.hisar.model.Device
import com.tryhisar.hisar.model.EmergencyKitExport
import com.tryhisar.hisar.model.ExportFormat
import com.tryhisar.hisar.model.ExportOptions
import com.tryhisar.hisar.model.ExportResult
import com.tryhisar.hisar.model.VaultTheme
import java.io.File
import java.util.UUID
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * Settings and the account's lifecycle: the theme, Mac approvals, devices, the Emergency Kit, changing the master
 * password, replacing the Secret Key, deleting the account and exporting data.
 *
 * Files the backend writes (Emergency Kits, exports) go into a folder of their own under [HANDOFF_FOLDER_NAME] in the
 * temporary directory, which on Android is the app's cache, and leaving their screen deletes that folder.
 */
class SettingsModel(private val context: ModelContext) {
    private val temporaryDirectory = File(System.getProperty("java.io.tmpdir"), HANDOFF_FOLDER_NAME)
    val deviceOperationErrorMessage = MutableStateFlow<String?>(null)
    val macApprovalErrorMessage = MutableStateFlow<String?>(null)

    private fun createFolder(prefix: String) = File(temporaryDirectory, "$prefix-${UUID.randomUUID()}")

    private fun removeFolder(path: String) {
        if (path.isNotEmpty()) File(path).parentFile?.deleteRecursively()
    }

    /** Shows the theme at once, and goes back to the one in use when it cannot be saved. */
    suspend fun setTheme(newTheme: VaultTheme) {
        val previousTheme = context.theme.value
        context.theme.value = newTheme
        context.theme.value = context.perform(isBusyShown = false) { it.setTheme(newTheme) } ?: previousTheme
    }

    suspend fun setMacApprovalAllowed(isAllowed: Boolean): Boolean {
        macApprovalErrorMessage.value = null
        context.errorMessage.value = null
        if (context.perform(isBusyShown = false) { it.setMacApprovalAllowed(isAllowed) } == null) {
            macApprovalErrorMessage.value = context.errorMessage.value
            context.errorMessage.value = null
            return false
        }
        context.isMacApprovalAllowed.value = isAllowed
        return true
    }

    suspend fun isExactHostMatch(): Boolean? = context.perform(isBusyShown = false) { it.isExactHostMatch() }

    suspend fun setExactHostMatch(isExact: Boolean): Boolean =
        context.perform(isBusyShown = false) { it.setExactHostMatch(isExact) } != null

    suspend fun loadAllDevice(): List<Device>? = context.perform(isBusyShown = false) { it.listAllDevice() }

    suspend fun revokeDevice(device: Device): Boolean = context.runInModel {
        deviceOperationErrorMessage.value = null
        context.errorMessage.value = null
        if (context.perform { it.revokeDevice(device.id) } == null) {
            deviceOperationErrorMessage.value = context.errorMessage.value
            context.errorMessage.value = null
            return@runInModel false
        }
        context.showNotice("${device.name} is signed out.")
        true
    }

    suspend fun exportEmergencyKit(password: String): EmergencyKitExport? =
        context.perform { it.exportEmergencyKit(password, createFolder("EmergencyKit")) }

    fun removeEmergencyKit(export: EmergencyKitExport) = removeFolder(export.emergencyKitPath)

    suspend fun changeMasterPassword(currentPassword: String, newPassword: String): CredentialChangeResult? = context.runInModel {
        val result = context.perform { it.changeMasterPassword(currentPassword, newPassword) } ?: return@runInModel null
        if (result.pendingOperationId.isNotEmpty()) {
            context.refreshStatus()
            context.refreshPendingRecovery(operationID = result.pendingOperationId)
            context.errorMessage.value = (result.problems + listOf(result.localError, result.keychainError))
                .filter { it.isNotEmpty() }.distinct().takeIf { it.isNotEmpty() }?.joinToString("\n")
            return@runInModel result
        }
        context.applyStatus(result.status)
        val warning = result.localError.takeIf { it.isNotEmpty() }?.let { " $it" }.orEmpty()
        context.showNotice("Master password changed. ${AccountLifecycleLogic.describeSignedOutDevices(result.signedOutDeviceCount)}$warning")
        result
    }

    suspend fun replaceSecretKey(password: String): CredentialChangeResult? {
        val result = context.perform { it.replaceSecretKey(password, createFolder("EmergencyKit")) } ?: return null
        if (result.pendingOperationId.isNotEmpty()) {
            removeFolder(result.emergencyKitPath)
            context.refreshStatus()
            context.refreshPendingRecovery(operationID = result.pendingOperationId)
            context.errorMessage.value = (result.problems + listOf(result.localError, result.keychainError))
                .filter { it.isNotEmpty() }.distinct().takeIf { it.isNotEmpty() }?.joinToString("\n")
            return result
        }
        context.applyStatus(result.status)
        return result
    }

    /** Deletes the new Emergency Kit and lets the backend forget the new Secret Key it kept to show once. */
    fun finishSecretKeyReplacement(result: CredentialChangeResult) {
        removeFolder(result.emergencyKitPath)
        context.scope.launch { context.attempt { it.forgetNewAccountSecretKey() } }
    }

    suspend fun loadAccountDeletionPreview(): AccountDeletionPreview? = context.perform(isBusyShown = false) { it.previewAccountDeletion() }

    suspend fun deleteAccount(password: String, email: String, planVersion: String): Boolean = context.runInModel {
        val newStatus = context.perform { it.deleteAccountWithPlan(password, email, planVersion) }
        // A deletion the server made can still fail to clean up this phone; the status read again shows it signed out.
        if (newStatus == null) {
            context.refreshStatus()
            return@runInModel false
        }
        context.applyStatus(newStatus)
        context.showNotice("Your Hisar account was deleted.")
        true
    }

    suspend fun loadExportOptions(): ExportOptions? = context.perform(isBusyShown = false) { it.getExportOptions() }

    suspend fun exportAllData(format: ExportFormat, passphrase: String, password: String): ExportResult? {
        val folder = createFolder("Export")
        val usedPassphrase = if (format == ExportFormat.ENCRYPTED) passphrase else ""
        val result = context.perform { it.exportAllData(format, usedPassphrase, password, folder) }
        if (result == null) folder.deleteRecursively()
        return result
    }

    fun removeExport(result: ExportResult) = removeFolder(result.path)

    companion object {
        const val HANDOFF_FOLDER_NAME = "handoff"
    }
}
