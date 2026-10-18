package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.InviteLink
import com.tryhisar.hisar.model.SignInLink
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.trimmingWhitespacesAndNewlines
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** What covers the app right after sign-up, until the new account's owner has saved the Secret Key. */
sealed interface NewAccountStep {
    data class SaveSecretKey(val secretKey: String) : NewAccountStep

    data object BiometricOffer : NewAccountStep
}

/**
 * Signing in, creating an account, the links that fill those forms in, and the screens right after sign-up. Unless
 * [isPlainHttpAllowed], a server or invite link that starts with http:// is refused before the backend is asked.
 */
class SignInModel(private val context: ModelContext, private val isPlainHttpAllowed: Boolean) {
    private val mutableNewAccountStep = MutableStateFlow<NewAccountStep?>(null)
    val newAccountStep: StateFlow<NewAccountStep?> = mutableNewAccountStep.asStateFlow()

    /** A link the sign-in form takes in, then clears: from the Emergency Kit's QR code or a hisar://sign-in link. */
    val pendingSignInLink = MutableStateFlow<SignInLink?>(null)

    /** An invite the create-account form takes in, then clears. */
    val pendingInviteLink = MutableStateFlow<InviteLink?>(null)

    /** Why an invite link was not used: someone is already signed in. */
    val inviteLinkMessage = MutableStateFlow<String?>(null)

    /** Why a scanned QR code filled nothing in. */
    val scannerMessage = MutableStateFlow<String?>(null)

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
        mutableNewAccountStep.value = NewAccountStep.SaveSecretKey(result.secretKey)
        context.applyStatus(result.status)
        context.refreshAll()
        true
    }

    /** "I saved my Secret Key": offers biometric unlock when the device has it and it is off, else closes the cover. */
    fun finishNewSecretKey() {
        val status = context.status.value
        if (status.biometricAvailable && !status.biometricEnabled) {
            context.errorMessage.value = null
            mutableNewAccountStep.value = NewAccountStep.BiometricOffer
        } else {
            dismissNewSecretKey()
        }
    }

    /** Closes the cover and has the backend forget the new account's Secret Key, which it kept for this offer. */
    fun dismissNewSecretKey() {
        mutableNewAccountStep.value = null
        context.scope.launch { context.attempt { it.forgetNewAccountSecretKey() } }
    }

    suspend fun turnOnBiometricUnlockFromOffer(): Boolean = context.runInModel {
        val newStatus = context.perform { it.enableNewAccountBiometricUnlock() } ?: return@runInModel false
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

    /** Takes the text of a QR code scanned on the sign-in screen, which only an Emergency Kit's sign-in link fills in. */
    fun openScannedEmergencyKit(text: String) {
        val link = SignInLink.parse(text)
        if (link == null) {
            scannerMessage.value = "This QR code is not from a Hisar Emergency Kit."
            return
        }
        pendingSignInLink.value = link
    }

    fun openInviteLink(link: InviteLink) {
        val status = context.status.value
        if (status.isSignedIn) {
            inviteLinkMessage.value = "You're signed in as ${status.email}. Sign out to create an account with this invite."
            return
        }
        pendingInviteLink.value = link
    }

    suspend fun dismissSignedOutReason() {
        context.attempt { it.dismissSignedOutReason() }?.let(context::applyStatus)
    }

    private fun isRefusedAsPlainHttp(address: String): Boolean = !isPlainHttpAllowed && address.startsWith("http://", ignoreCase = true)

    companion object {
        const val DEFAULT_SERVER = "https://api.tryhisar.com"
        const val PLAIN_HTTP_SERVER_REFUSAL = "This server's address starts with http://, so the connection would not be encrypted. Hisar signs in only over https://."
        const val PLAIN_HTTP_INVITE_REFUSAL = "This invite link starts with http://, so the connection would not be encrypted. Hisar creates accounts only over https://."

        fun isDefaultServer(server: String): Boolean {
            val trimmedServer = server.trimmingWhitespacesAndNewlines()
            return trimmedServer.isEmpty() || trimmedServer.trim('/') == DEFAULT_SERVER
        }

        fun describeServer(server: String): String = server.replace("https://", "")
    }
}
