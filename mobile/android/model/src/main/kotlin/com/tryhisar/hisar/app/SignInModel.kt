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

/** Signing in, creating an account, the links that fill those forms in, and the screens right after sign-up. */
class SignInModel(private val context: ModelContext) {
    private val mutableNewAccountStep = MutableStateFlow<NewAccountStep?>(null)
    val newAccountStep: StateFlow<NewAccountStep?> = mutableNewAccountStep.asStateFlow()

    /** A link the sign-in form takes in, then clears: from the Emergency Kit's QR code or a hisar://sign-in link. */
    val pendingSignInLink = MutableStateFlow<SignInLink?>(null)

    /** An invite the create-account form takes in, then clears. */
    val pendingInviteLink = MutableStateFlow<InviteLink?>(null)

    /** Why an invite link was not used: someone is already signed in. */
    val inviteLinkMessage = MutableStateFlow<String?>(null)

    suspend fun signIn(server: String, email: String, secretKey: String, password: String): Boolean {
        val formattedSecretKey = VaultLogic.formatSecretKey(secretKey)
        val trimmedServer = server.trimmingWhitespacesAndNewlines()
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        val newStatus = context.perform { it.signIn(trimmedServer, trimmedEmail, formattedSecretKey, password) } ?: return false
        context.applyStatus(newStatus)
        context.refreshAll()
        return true
    }

    suspend fun signUp(inviteLink: String, email: String, name: String, password: String): Boolean {
        val trimmedInviteLink = inviteLink.trimmingWhitespacesAndNewlines()
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        val result = context.perform { it.signUp(trimmedInviteLink, trimmedEmail, trimmedName, password) } ?: return false
        mutableNewAccountStep.value = NewAccountStep.SaveSecretKey(result.secretKey)
        context.applyStatus(result.status)
        context.refreshAll()
        return true
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

    suspend fun turnOnBiometricUnlockFromOffer(): Boolean {
        val newStatus = context.perform { it.enableNewAccountBiometricUnlock() } ?: return false
        context.applyStatus(newStatus)
        dismissNewSecretKey()
        context.showNotice("Biometric unlock is on")
        return true
    }

    /** Opens a hisar:// link from outside the app, or the text of a scanned QR code; false when it is neither link. */
    fun openLink(text: String): Boolean {
        SignInLink.parse(text)?.let {
            pendingSignInLink.value = it
            return true
        }
        InviteLink.parse(text)?.let {
            openInviteLink(it)
            return true
        }
        return false
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

    companion object {
        const val DEFAULT_SERVER = "https://api.tryhisar.com"

        fun isDefaultServer(server: String): Boolean {
            val trimmedServer = server.trimmingWhitespacesAndNewlines()
            return trimmedServer.isEmpty() || trimmedServer.trim('/') == DEFAULT_SERVER
        }

        fun describeServer(server: String): String = server.replace("https://", "")
    }
}
