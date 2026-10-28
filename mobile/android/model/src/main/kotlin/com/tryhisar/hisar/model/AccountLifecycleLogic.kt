package com.tryhisar.hisar.model

import java.text.BreakIterator
import java.util.Locale

// Not regex \X: Android 10's ICU regex splits flags and joined emoji, unlike its BreakIterator and Swift's String.count.
private fun String.characterCount(): Int {
    val boundaries = BreakIterator.getCharacterInstance(Locale.ROOT)
    boundaries.setText(this)
    var count = 0
    while (boundaries.next() != BreakIterator.DONE) count++
    return count
}

/** The rules and sentences of changing the master password, replacing the Secret Key and deleting the account. */
object AccountLifecycleLogic {
    const val MINIMUM_PASSWORD_LENGTH = 10
    const val PASSWORD_CHANGE_WARNING = "Your other devices will be signed out. Sign in on them with the new master password."
    const val SECRET_KEY_REPLACEMENT_WARNING =
        "Your other devices will be signed out. Sign in on them with your master password and the new Secret Key from the new Emergency Kit. Your old Emergency Kit stops working."
    const val EXPORT_ADVICE = "Export your items first in Settings → Account → Export data."
    const val SIGN_OUT_TITLE = "Sign out of Hisar on this device?"
    const val SIGN_OUT_MESSAGE = "Hisar removes this device's saved Secret Key and offline copy of your vaults. To sign in again, you need your master password and the Secret Key from your Emergency Kit or another signed-in device."
    const val LOCKED_SIGN_OUT_MESSAGE = "$SIGN_OUT_MESSAGE If you have not saved an Emergency Kit, cancel, unlock Hisar and save one in Settings, Account, Secret Key and Emergency Kit."

    fun signOutMessage(kit: EmergencyKitStatus?, isLocked: Boolean): String {
        if (kit == null) return if (isLocked) LOCKED_SIGN_OUT_MESSAGE else SIGN_OUT_MESSAGE
        val prefix = if (kit.savedBy.isEmpty()) "You haven't saved this account's Emergency Kit yet."
        else "You haven't confirmed this account's Emergency Kit yet."
        val instruction = if (isLocked) " Cancel, unlock Hisar and finish saving or confirming your Emergency Kit." else ""
        return "$prefix $SIGN_OUT_MESSAGE The protected unfinished setup stays on this device. Don't rely on it as your only recovery copy.$instruction"
    }

    /**
     * The sentence that refuses a new master password: fewer than 10 characters, counted as the user sees them, or core's
     * rule ([MasterPasswordFeedback]) blocks it. [feedback] is core's rating of this password, null while it is on its way.
     */
    fun describeMasterPasswordProblem(password: String, feedback: MasterPasswordFeedback?): String? = when {
        password.isEmpty() -> null
        password.characterCount() < MINIMUM_PASSWORD_LENGTH -> "Use at least $MINIMUM_PASSWORD_LENGTH characters."
        feedback != null && feedback.isBlocking -> feedback.message
        else -> null
    }

    /** The hint under a Weak master password that core allows. */
    fun describeMasterPasswordHint(feedback: MasterPasswordFeedback?): String? =
        feedback?.takeIf { !it.isBlocking && it.level == "weak" }?.message

    fun isMasterPasswordAccepted(password: String, feedback: MasterPasswordFeedback?): Boolean =
        password.isNotEmpty() && feedback != null && describeMasterPasswordProblem(password, feedback) == null

    fun describeNewPasswordProblem(currentPassword: String, newPassword: String, repeatedPassword: String, feedback: MasterPasswordFeedback?): String? =
        describeMasterPasswordProblem(newPassword, feedback) ?: when {
            newPassword.isNotEmpty() && newPassword == currentPassword -> "The new master password is the same as the current one."
        repeatedPassword.isNotEmpty() && repeatedPassword != newPassword -> "The passwords do not match."
        else -> null
    }

    fun canChangePassword(currentPassword: String, newPassword: String, repeatedPassword: String, feedback: MasterPasswordFeedback?): Boolean =
        currentPassword.isNotEmpty() && newPassword == repeatedPassword && isMasterPasswordAccepted(newPassword, feedback) &&
            describeNewPasswordProblem(currentPassword, newPassword, repeatedPassword, feedback) == null

    fun isEmailMatching(typedEmail: String, accountEmail: String): Boolean {
        val trimmedEmail = typedEmail.trimmingWhitespacesAndNewlines()
        return trimmedEmail.isNotEmpty() && trimmedEmail.equals(accountEmail, ignoreCase = true)
    }

    /**
     * A sign-up that finished and waits only for its Emergency Kit, as the iPhone decides: Save your Secret Key saves and
     * acknowledges it. Every other pending result opens account recovery.
     */
    fun isWaitingOnlyForKit(setupState: String, secretKey: String, problems: List<String>): Boolean =
        setupState == "delivery-pending" && secretKey.isNotEmpty() && problems.isEmpty()

    fun describeSignedOutDevices(count: Int): String = when (count) {
        in Int.MIN_VALUE..-1 -> "Any other devices were signed out; their count could not be recovered."
        0 -> "No other devices were signed in."
        1 -> "1 other device was signed out."
        else -> "$count other devices were signed out."
    }

    private fun describePeople(count: Int): String = if (count == 1) "1 person" else "$count people"

    fun describeOwnedVault(vault: DeletionVault): String = when {
        vault.type == VaultSummary.TYPE_PERSONAL -> "Deleted with its items."
        vault.otherMemberCount == 0 -> "Deleted with its items. Nobody else has access."
        else -> "Deleted with its items. ${describePeople(vault.otherMemberCount)} lose access."
    }

    fun describeMemberVault(vault: DeletionVault): String =
        if (vault.role == Member.VAULT_ROLE_OWNER) "Stays with its other owners. Your access is removed." else "Stays with its owners. Your access is removed."

    fun describeOrganization(organization: DeletionOrganization): String = when {
        organization.isDeleted -> "${organization.name}, its vaults, and their items are deleted with your account"
        organization.newAdminName.isNotEmpty() -> "${organization.newAdminName} becomes an admin of ${organization.name}"
        else -> "You leave ${organization.name}"
    }

    fun describeAllOtherConsequence(preview: AccountDeletionPreview): List<String> = buildList {
        if (preview.shareLinkCount > 0) {
            add(if (preview.shareLinkCount == 1) "1 share link stops working." else "${preview.shareLinkCount} share links stop working.")
        }
        val invitationCount = preview.sentInvitationCount + preview.receivedInvitationCount
        if (invitationCount > 0) {
            add(if (invitationCount == 1) "1 invitation is withdrawn." else "$invitationCount invitations are withdrawn.")
        }
        add(
            when {
                preview.deviceCount <= 1 -> "This device is signed out."
                preview.deviceCount == 2 -> "This device and 1 other device are signed out."
                else -> "This device and ${preview.deviceCount - 1} other devices are signed out."
            },
        )
    }
}

object ExportLogic {
    const val PLAIN_WARNING =
        "Plain JSON is not encrypted. Anyone who gets the file can read every password, one-time code secret and SSH private key in it. Delete it when you are done."

    fun isPasswordRequested(error: String?): Boolean = error?.contains("master password", ignoreCase = true) == true

    /** What is missing before an export can start, or null. */
    fun checkInput(format: ExportFormat, passphrase: String, repeatedPassphrase: String, minPassphraseLength: Int): String? = when {
        format != ExportFormat.ENCRYPTED -> null
        passphrase.characterCount() < minPassphraseLength -> "Use at least $minPassphraseLength characters for the export passphrase."
        passphrase != repeatedPassphrase -> "The passphrases don't match."
        else -> null
    }
}

/** Settings → Help: where to write, and what the copied diagnostics hold. */
object HelpLogic {
    const val SUPPORT_EMAIL = "support@tryhisar.com"

    fun buildDiagnostics(appVersion: String, systemVersion: String, deviceModel: String, server: String, deviceID: String?): String = buildList {
        add("Hisar $appVersion")
        add("Android $systemVersion on $deviceModel")
        add("Server ${server.ifEmpty { "not signed in" }}")
        if (!deviceID.isNullOrEmpty()) add("Device ID $deviceID")
    }.joinToString("\n")
}
