package com.tryhisar.hisar.ui

import com.tryhisar.hisar.HisarUITestCase
import org.junit.Assert.assertTrue
import org.junit.Test

class AccountLifecycleUITests : HisarUITestCase() {
    @Test
    fun testChangePasswordReplaceSecretKeyAndDeleteAccount() {
        val inviteLink = readArgument("HISAR_UI_TEST_LIFECYCLE_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_LIFECYCLE_EMAIL")
        launch()
        signUp(inviteLink, email, "Lifecycle")

        openSettings("open-security")
        tap("open-change-master-password")
        type(MASTER_PASSWORD, "change-password-current")
        type(NEW_MASTER_PASSWORD, "change-password-new")
        type(NEW_MASTER_PASSWORD, "change-password-repeated")
        saveScreenshot("account-life-change-master-password")
        tap("change-password-submit")
        tap("change-password-confirm")
        waitFor("open-replace-secret-key", ACCOUNT_KEY_TIMEOUT)

        tap("open-replace-secret-key")
        type(NEW_MASTER_PASSWORD, "replace-secret-key-password")
        saveScreenshot("account-life-replace-secret-key")
        tap("replace-secret-key-submit")
        tap("replace-secret-key-confirm")
        val newSecretKey = finishPendingRecovery(email, NEW_MASTER_PASSWORD)
        assertTrue(newSecretKey, newSecretKey.startsWith("H1-"))
        saveScreenshot("account-life-new-secret-key")

        goBack()
        tap("open-account")
        tap("open-delete-account")
        waitFor("delete-account-password")
        saveScreenshot("account-life-delete-account-preview")
        type(NEW_MASTER_PASSWORD, "delete-account-password")
        type(email, "delete-account-email")
        saveScreenshot("account-life-delete-account")
        tap("delete-account-submit")
        tap("delete-account-confirm")
        waitFor("sign-in-mode", ACCOUNT_KEY_TIMEOUT)
        saveScreenshot("account-life-deleted")
    }

    private companion object {
        const val NEW_MASTER_PASSWORD = "ui test new master password"
    }
}
