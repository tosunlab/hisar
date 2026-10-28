package com.tryhisar.hisar

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.service.credentials.BeginGetCredentialOption
import android.service.credentials.BeginGetCredentialRequest
import android.service.credentials.CallingAppInfo
import android.service.credentials.CredentialProviderService
import android.view.WindowManager
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.isOn
import androidx.credentials.PasswordCredential
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.autofill.AutofillActivity
import com.tryhisar.hisar.autofill.AutofillRequest
import com.tryhisar.hisar.autofill.FillTarget
import com.tryhisar.hisar.autofill.RequestKind
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.passkey.CredentialActivity
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PrivacyUITests : HisarUITestCase() {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()

    @Test
    fun testAppSwitcherHidesAnOpenSheet() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_PRIVACY_INVITE_LINK"), readArgument("HISAR_UI_TEST_PRIVACY_EMAIL"), "Android Privacy")
        openSettings("open-security")
        val isAllowed = hasTestTag("allow-screenshots-switch") and isOn()
        if (!exists(isAllowed)) tap("allow-screenshots-switch")
        waitFor(isAllowed)
        goBack()
        openTab("Vaults")
        tap("vault-Personal")
        choose("new-login", "new-menu")
        tap("editor-generate-password")
        waitFor(hasTestTag("generator-use") and isEnabled())
        val password = readText("generated-password")
        val passwordBounds = findOnScreen(password)
        val inFront = countInk(captureScreen("privacy-sheet"), passwordBounds)
        assertTrue("a screenshot shows the sheet's password while Hisar is in front ($inFront marked pixels)", inFront > 0)

        device.pressHome()
        device.pressRecentApps()
        val card = device.findRecentsCard { it == "Hisar" }
        val inRecents = countInk(captureScreen("privacy-recents"), card)
        assertTrue("Recents shows Hisar's card with the sheet's password on it ($inRecents marked pixels of ${card.width() * card.height()})", inRecents * 1000 < card.width() * card.height())

        device.click(card.centerX(), card.centerY())
        waitFor(hasTestTag("generated-password") and hasText(password))
        composeRule.waitUntil("back in front, a screenshot shows the sheet's password again", 10_000) {
            countInk(takeScreenshot(), findOnScreen(password)) > 0
        }
        captureScreen("privacy-sheet-again")
    }

    /** An AutoFill or passkey provider window, over another app, follows Allow screenshots while it is open. */
    private fun <T : Activity> checkWindow(intent: Intent) {
        application.installFakeBackend(FakeBackend().apply { seedSignedIn(state = VaultStatus.STATE_LOCKED) })
        val setting = ScreenshotSetting(application)
        val original = setting.isAllowed
        try {
            setting.isAllowed = true
            ActivityScenario.launch<T>(intent).use { scenario ->
                fun awaitSecure(expected: Boolean) {
                    val deadline = SystemClock.uptimeMillis() + 5_000
                    var secure = !expected
                    while (secure != expected && SystemClock.uptimeMillis() < deadline) {
                        scenario.onActivity { secure = it.window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE != 0 }
                        if (secure != expected) SystemClock.sleep(10)
                    }
                    assertEquals("the open provider window follows Allow screenshots", expected, secure)
                }
                awaitSecure(false)
                setting.isAllowed = false
                awaitSecure(true)
                setting.isAllowed = true
                awaitSecure(false)
            }
        } finally { setting.isAllowed = original }
    }

    @Test
    @SdkSuppress(minSdkVersion = 34)
    fun testAutoFillWindowFollowsScreenshotSettingWhileOpen() {
        val request = AutofillRequest(null, null, null, RequestKind.PASSWORD, FillTarget.Websites(listOf("github.com")))
        checkWindow<AutofillActivity>(request.writeTo(Intent(application, AutofillActivity::class.java)))
    }

    @Test
    @SdkSuppress(minSdkVersion = 34)
    fun testPasskeyProviderWindowFollowsScreenshotSettingWhileOpen() {
        val signingInfo = checkNotNull(application.packageManager.getPackageInfo(application.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo)
        val request = BeginGetCredentialRequest.Builder()
            .setCallingAppInfo(CallingAppInfo(application.packageName, signingInfo))
            .addBeginGetCredentialOption(BeginGetCredentialOption("password", PasswordCredential.TYPE_PASSWORD_CREDENTIAL, Bundle()))
            .build()
        val intent = Intent(application, CredentialActivity::class.java)
            .putExtra("com.tryhisar.hisar.passkey.PICK", "unlock")
            .putExtra(CredentialProviderService.EXTRA_BEGIN_GET_CREDENTIAL_REQUEST, request)
        checkWindow<CredentialActivity>(intent)
    }

    private fun findOnScreen(text: String): Rect =
        checkNotNull(device.wait(Until.findObject(By.text(text)), 10_000)) { "\"$text\" is not on the screen" }.visibleBounds

    private fun captureScreen(name: String): Bitmap {
        val screenshot = takeScreenshot()
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
        return screenshot
    }
}
