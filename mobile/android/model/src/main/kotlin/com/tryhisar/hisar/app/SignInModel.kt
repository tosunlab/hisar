package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.EmergencyKitExport
import com.tryhisar.hisar.model.InviteLink
import com.tryhisar.hisar.model.SignInLink
import com.tryhisar.hisar.model.SignUpResult
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.SignUpPolicy
import com.tryhisar.hisar.model.trimmingWhitespacesAndNewlines
import java.io.File
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** The account's kit presentation contains no portable secret. Core owns its authorization. */
sealed interface NewAccountStep {
    data object SaveSecretKey : NewAccountStep

    data object BiometricOffer : NewAccountStep
}

sealed interface SignUpPolicyState {
    data object Checking : SignUpPolicyState
    data class Ready(val policy: SignUpPolicy) : SignUpPolicyState
    data class Failed(val server: String, val message: String) : SignUpPolicyState

    val requiresInvitation: Boolean get() = this is Ready && policy.mode in listOf("invite", "setup")
    val showsInviteField: Boolean get() = this is Ready && policy.mode != "open"
    fun showsAccountFields(hasInvitation: Boolean): Boolean = hasInvitation || this is Ready && !requiresInvitation
}

/**
 * Signing in, creating an account, the links that fill those forms in, and the screens right after sign-up. Unless
 * [isPlainHttpAllowed], a server or invite link that starts with http:// is refused before the backend is asked.
 */
class SignInModel(private val context: ModelContext, private val isPlainHttpAllowed: Boolean) {
    private val mutableNewAccountStep = MutableStateFlow<NewAccountStep?>(null)
    val newAccountStep: StateFlow<NewAccountStep?> = mutableNewAccountStep.asStateFlow()
    val signUpPolicy = MutableStateFlow<SignUpPolicyState>(SignUpPolicyState.Checking)
    val isInvitationRequiredAfterSubmit = MutableStateFlow(false)
    val interruptedKitOperationID = MutableStateFlow<String?>(null)
    private var policyGeneration = 0L
    private var policyServer = ""
    private var kitEpoch = 0L

    /** A link the sign-in form takes in, then clears: from the Emergency Kit's QR code or a hisar://sign-in link. */
    val pendingSignInLink = MutableStateFlow<SignInLink?>(null)

    /** An invite the create-account form takes in, then clears. */
    val pendingInviteLink = MutableStateFlow<InviteLink?>(null)

    /** Why an invite link was not used: someone is already signed in. */
    val inviteLinkMessage = MutableStateFlow<String?>(null)

    /** Whether the sign-in screen shows the camera that scans an Emergency Kit's QR code. */
    val isScannerShown = MutableStateFlow(false)

    /** Why a scanned QR code filled nothing in. */
    val scannerMessage = MutableStateFlow<String?>(null)

    init {
        context.addStatusListener { previous, current ->
            val sameAccount = previous.email == current.email && previous.server == current.server
            if (previous.isUnlocked && !current.isUnlocked || !sameAccount) {
                kitEpoch++
                interruptedKitOperationID.value = if (sameAccount && current.isSignedIn &&
                    mutableNewAccountStep.value == NewAccountStep.SaveSecretKey
                ) previous.emergencyKit?.operationId else null
                mutableNewAccountStep.value = null
            }
            if (!current.isSignedIn) interruptedKitOperationID.value = null
            if (!previous.isUnlocked && current.isUnlocked) {
                val interrupted = interruptedKitOperationID.value
                if (interrupted != null && interrupted == current.emergencyKit?.operationId) {
                    mutableNewAccountStep.value = NewAccountStep.SaveSecretKey
                }
                interruptedKitOperationID.value = null
            }
        }
    }

    suspend fun checkSignUpPolicy(server: String) {
        val canonical = canonicalServer(server)
        policyServer = canonical
        val generation = ++policyGeneration
        signUpPolicy.value = SignUpPolicyState.Checking
        context.errorMessage.value = null
        val answer = context.attempt {
            try {
                Result.success(it.getSignUpPolicy(canonical))
            } catch (error: kotlinx.coroutines.CancellationException) {
                throw error
            } catch (error: Exception) {
                Result.failure(error)
            }
        } ?: return
        if (generation != policyGeneration || canonical != policyServer) return
        signUpPolicy.value = answer.fold(
            onSuccess = {
                if (canonicalServer(it.server) != canonical) return
                SignUpPolicyState.Ready(it)
            },
            onFailure = {
                SignUpPolicyState.Failed(canonical, it.message ?: "Hisar can't reach ${describeServer(canonical)}. Check your internet connection and the server address.")
            },
        )
        if (answer.isFailure) context.refreshStatus()
    }

    fun invalidateSignUpPolicy() {
        policyGeneration++
        signUpPolicy.value = SignUpPolicyState.Checking
        isInvitationRequiredAfterSubmit.value = false
        context.errorMessage.value = null
    }

    suspend fun signIn(server: String, email: String, secretKey: String, password: String): Boolean = context.runInModel {
        val formattedSecretKey = VaultLogic.formatSecretKey(secretKey)
        val trimmedServer = server.trimmingWhitespacesAndNewlines()
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        if (isRefusedAsPlainHttp(trimmedServer)) {
            context.errorMessage.value = PLAIN_HTTP_SERVER_REFUSAL
            return@runInModel false
        }
        val newStatus = context.perform { it.signIn(trimmedServer, trimmedEmail, formattedSecretKey, password) } ?: return@runInModel false
        context.applyStatus(newStatus)
        context.refreshAll()
        context.refreshPendingRecovery(isOpening = false)
        true
    }

    suspend fun signUp(inviteLink: String, email: String, name: String, password: String): Boolean = context.runInModel {
        val trimmedInviteLink = inviteLink.trimmingWhitespacesAndNewlines()
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        if (isRefusedAsPlainHttp(trimmedInviteLink)) {
            context.errorMessage.value = PLAIN_HTTP_INVITE_REFUSAL
            return@runInModel false
        }
        val result = context.perform { it.signUp(trimmedInviteLink, trimmedEmail, trimmedName, password) } ?: return@runInModel false
        finishSignUp(result)
    }

    /**
     * Creates an account on a server whose admin opened registration, with no invite; the backend answers with its own
     * sentence when sign-ups are closed.
     */
    suspend fun signUpWithoutInvite(server: String, email: String, name: String, password: String): Boolean = context.runInModel {
        val trimmedServer = server.trimmingWhitespacesAndNewlines()
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        if (isRefusedAsPlainHttp(trimmedServer)) {
            context.errorMessage.value = PLAIN_HTTP_SERVER_REFUSAL
            return@runInModel false
        }
        val result = context.perform { it.signUpWithoutInvite(trimmedServer, trimmedEmail, trimmedName, password) }
        if (result == null) {
            if (context.errorMessage.value?.contains("Sign-ups are not open") == true) {
                isInvitationRequiredAfterSubmit.value = true
                checkSignUpPolicy(trimmedServer)
                context.errorMessage.value = "Sign-ups on ${describeServer(trimmedServer)} need an invitation now. Ask someone with an account there to invite you."
            }
            return@runInModel false
        }
        finishSignUp(result)
    }

    private suspend fun finishSignUp(result: SignUpResult): Boolean {
        if (result.pendingOperationId.isNotEmpty() && !AccountLifecycleLogic.isWaitingOnlyForKit(result.setupState, result.secretKey, result.problems)) {
            context.refreshStatus()
            // The backend's first sync can report its change before this status arrives, when it is ignored.
            if (context.status.value.isUnlocked) context.refreshAll()
            context.refreshPendingRecovery(operationID = result.pendingOperationId)
            context.errorMessage.value = result.problems.takeIf { it.isNotEmpty() }?.joinToString("\n")
            return true
        }
        context.applyStatus(result.status)
        mutableNewAccountStep.value = NewAccountStep.SaveSecretKey
        context.refreshAll()
        context.refreshPendingRecovery(isOpening = false)
        return true
    }

    suspend fun exportNewAccountEmergencyKit(): EmergencyKitExport? {
        val directory = File(System.getProperty("java.io.tmpdir"), "${SettingsModel.HANDOFF_FOLDER_NAME}/EmergencyKit-${UUID.randomUUID()}")
        val epoch = kitEpoch
        var published = false
        try {
            val result = context.perform(isBusyShown = false) { it.exportNewAccountEmergencyKit(directory) } ?: return null
            if (epoch != kitEpoch || !context.status.value.isUnlocked) return null
            published = true
            return result
        } finally {
            if (!published) directory.deleteRecursively()
        }
    }

    fun openEmergencyKit() {
        if (!context.status.value.isUnlocked || context.status.value.emergencyKit == null) return
        mutableNewAccountStep.value = NewAccountStep.SaveSecretKey
    }

    fun closeEmergencyKitGate() {
        mutableNewAccountStep.value = null
        interruptedKitOperationID.value = null
    }

    suspend fun authorizeEmergencyKit(password: String? = null): Boolean {
        val epoch = kitEpoch
        context.perform {
            if (password == null) it.authorizeNewAccountEmergencyKitWithBiometric()
            else it.authorizeNewAccountEmergencyKit(password)
        } ?: return false
        if (epoch != kitEpoch) return false
        context.refreshStatus()
        return context.status.value.emergencyKit?.isAuthorized == true
    }

    suspend fun recordEmergencyKitDelivery(export: EmergencyKitExport, method: String): Boolean {
        context.perform(isBusyShown = false) { it.recordNewAccountEmergencyKitDelivery(export.exportId, method) } ?: return false
        context.refreshStatus()
        context.refreshPendingRecovery(isOpening = false)
        return true
    }

    suspend fun hideEmergencyKitReminder() {
        context.attempt { it.hideEmergencyKitReminder() }
        context.refreshStatus()
    }

    /** "I saved my Emergency Kit": ends the sign-up's pending record, then offers biometric unlock. */
    suspend fun finishNewSecretKey(): Boolean = context.runInModel {
        context.perform { it.acknowledgeNewAccountRecovery() } ?: return@runInModel false
        context.refreshStatus()
        context.refreshPendingRecovery(isOpening = false)
        offerBiometricUnlock()
        true
    }

    suspend fun postponeNewSecretKey() {
        context.perform { it.postponeNewAccountEmergencyKit() } ?: return
        context.refreshStatus()
        context.refreshPendingRecovery(isOpening = false)
        offerBiometricUnlock()
    }

    /** Offers biometric unlock when the device has it and it is off, else closes the cover. */
    fun offerBiometricUnlock() {
        val status = context.status.value
        if (status.biometricOfferAvailable) {
            context.errorMessage.value = null
            mutableNewAccountStep.value = NewAccountStep.BiometricOffer
        } else {
            dismissNewSecretKey()
        }
    }

    /** Ends the offer without ending an owed kit's authorization. */
    fun dismissNewSecretKey() {
        mutableNewAccountStep.value = null
        context.scope.launch { context.attempt { it.forgetNewAccountSecretKey() }; context.refreshStatus() }
    }

    suspend fun turnOnBiometricUnlockFromOffer(): Boolean = context.runInModel {
        val epoch = kitEpoch
        val newStatus = context.perform { it.enableNewAccountBiometricUnlock() } ?: return@runInModel false
        if (epoch != kitEpoch || !context.status.value.isUnlocked) return@runInModel false
        context.applyStatus(newStatus)
        dismissNewSecretKey()
        context.showNotice("Biometric unlock is on")
        true
    }

    /** Opens a hisar://sign-in or hisar://invite link from outside the app; other links change nothing. */
    fun openLink(text: String) {
        val signInLink = SignInLink.parse(text)
        if (signInLink != null) {
            pendingSignInLink.value = signInLink
            return
        }
        InviteLink.parse(text)?.let(::openInviteLink)
    }

    /** Keeps an invalid code in the scanner until the user explicitly tries again. */
    fun openScannedEmergencyKit(text: String) {
        if (scannerMessage.value != null) return
        val link = SignInLink.parse(text)
        if (link == null) {
            scannerMessage.value = "This QR code is not from a Hisar Emergency Kit."
            return
        }
        isScannerShown.value = false
        pendingSignInLink.value = link
    }

    /**
     * Follows Open in Hisar on an invite page. Signed out, Create account takes the invite. Signed in to the account an
     * organization or vault share invitation is for, Settings shows it, after the next unlock when Hisar is locked; any
     * other account is told who the invitation is for.
     */
    fun openInviteLink(link: InviteLink) {
        val status = context.status.value
        if (!status.isSignedIn) {
            pendingInviteLink.value = link
            return
        }
        if (link.invitation == null) {
            inviteLinkMessage.value = "You're signed in as ${status.email}. Sign out to create an account with this invite."
            return
        }
        // Both servers went through resolveServer, so they compare as written, the host's case aside.
        if (link.email != status.email.trim().lowercase() || !link.server.equals(status.server, ignoreCase = true)) {
            inviteLinkMessage.value =
                "This invitation is for ${link.email} on ${link.server.removePrefix("https://")}. You're signed in as ${status.email}."
            return
        }
        val destination = if (link.invitation == InviteLink.INVITATION_ORGANIZATION) SettingsDestination.ORGANIZATIONS else SettingsDestination.PEOPLE
        context.openInvitationSettings(destination)
    }

    suspend fun dismissSignedOutReason() {
        context.attempt { it.dismissSignedOutReason() }?.let(context::applyStatus)
    }

    private fun isRefusedAsPlainHttp(address: String): Boolean = !isPlainHttpAllowed && address.startsWith("http://", ignoreCase = true)

    companion object {
        fun shouldShowWelcome(status: com.tryhisar.hisar.model.VaultStatus, hasPendingRecovery: Boolean, hasIncomingLink: Boolean): Boolean =
            !status.isSignedIn && status.server.isEmpty() && !hasPendingRecovery && !hasIncomingLink
        fun canonicalServer(server: String): String {
            val value = server.trim().trimEnd('/').ifEmpty { DEFAULT_SERVER }
            return if ("://" in value) value else "https://$value"
        }
        const val DEFAULT_SERVER = "https://eu.tryhisar.com"
        const val PLAIN_HTTP_SERVER_REFUSAL = "This server's address starts with http://, so the connection would not be encrypted. Hisar signs in only over https://."
        const val PLAIN_HTTP_INVITE_REFUSAL = "This invite link starts with http://, so the connection would not be encrypted. Hisar creates accounts only over https://."

        fun isDefaultServer(server: String): Boolean {
            val trimmedServer = server.trimmingWhitespacesAndNewlines()
            return trimmedServer.isEmpty() || trimmedServer.trim('/') == DEFAULT_SERVER
        }

        fun describeServer(server: String): String = server.replace("https://", "")
    }
}
