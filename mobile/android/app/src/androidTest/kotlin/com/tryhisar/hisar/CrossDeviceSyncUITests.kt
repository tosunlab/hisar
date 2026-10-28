package com.tryhisar.hisar

import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.model.ItemDetail
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import org.junit.runner.RunWith

/**
 * This phone and the user's other device, the command line that scripts/test-android-ui.sh runs as
 * answer_as_other_device, on one account through the harness server. The phone never pulls to refresh: what the other
 * device changes has to arrive through the server's event stream.
 */
@RunWith(AndroidJUnit4::class)
class CrossDeviceSyncUITests : HisarUITestCase() {
    @Test
    fun testChangesOnEitherDeviceReachTheOther() {
        launch()
        signIn(
            readArgument("HISAR_UI_TEST_CROSS_DEVICE_EMAIL"),
            readArgument("HISAR_UI_TEST_CROSS_DEVICE_SECRET_KEY"),
            readArgument("HISAR_UI_TEST_CROSS_DEVICE_PASSWORD"),
        )
        waitFor("vaults-list")
        assertFalse(exists(hasText("Favorites")))

        addLogin(title = LOGIN, username = "android-user", password = "android-login-password", website = "https://cross-device.example.com")
        waitFor("item-Server lists android/mobile", SYNC_TIMEOUT)
        waitFor(hasTestTag("item-$LOGIN") and hasText("cli-user"), SYNC_TIMEOUT)
        saveScreenshot("cross-device-edited-on-the-command-line")
        tap("item-$LOGIN")
        tap("reveal-password")
        assertEquals("cli-login-password", readText("password-value"))
        goBack()
        goBack()

        waitFor("vaults-list")
        assertFalse(exists(hasText("Favorites")))
        askOtherDevice("Ready for a favorite")
        waitFor("favorite-$LOGIN", SYNC_TIMEOUT)
        saveScreenshot("cross-device-starred-on-the-command-line")

        openSettings("open-devices")
        val commandLine = "Command line on ${readArgument("HISAR_UI_TEST_CROSS_DEVICE_CLI_SYSTEM")}"
        waitFor(hasTestTag("device-${readArgument("HISAR_UI_TEST_CROSS_DEVICE_CLI_NAME")}") and hasText(commandLine, substring = true))
        waitFor(hasText("Android app · Active now"))
        saveScreenshot("cross-device-devices")
        goBack()

        val newPassword = readArgument("HISAR_UI_TEST_CROSS_DEVICE_NEW_PASSWORD")
        tap("open-account")
        tap("open-change-master-password")
        type(readArgument("HISAR_UI_TEST_CROSS_DEVICE_PASSWORD"), "change-password-current")
        type(newPassword, "change-password-new")
        type(newPassword, "change-password-repeated")
        tap("change-password-submit")
        tap("change-password-confirm")
        waitFor(hasText("Master password changed. 1 other device was signed out.") and hasAnyAncestor(hasTestTag("notice")), ACCOUNT_KEY_TIMEOUT)
        openTab("Vaults")
        tap("vault-Personal")
        waitFor("item-Signed out on the command line", SYNC_TIMEOUT)

        askOtherDevice("Ready to be signed out")
        waitFor(hasTestTag("signed-out-reason") and hasAnyDescendant(hasText(SIGNED_OUT_REASON)), SYNC_TIMEOUT)
        waitFor("sign-in-mode")
        saveScreenshot("cross-device-signed-out-by-the-command-line")
    }

    /** Saves a note answer_as_other_device waits for, once this phone shows the screen the next step changes. */
    private fun askOtherDevice(title: String) {
        val backend = ApplicationProvider.getApplicationContext<HisarTestApplication>().backend
        val personalVault = backend.listAllVault().first { it.type == "personal" }
        backend.saveItem(personalVault.id, ItemDetail(category = ItemDetail.CATEGORY_SECURE_NOTE, title = title))
    }

    private companion object {
        const val LOGIN = "Cross-device login"
        const val SYNC_TIMEOUT = 60_000L
        const val SIGNED_OUT_REASON = "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one."
    }
}
