package com.tryhisar.hisar.autofill

import android.content.ClipboardManager
import android.content.ComponentName
import android.content.Intent
import android.graphics.Bitmap
import android.os.ParcelFileDescriptor
import android.view.accessibility.AccessibilityEvent
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.UiObject2
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.HisarUITestCase.Companion.MASTER_PASSWORD
import java.util.concurrent.TimeoutException
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The real AutoFill service filling pages and fields of another package (the test APK's), against a real server. The
 * system shows the suggestions above Gboard on API 30 and later, or in a dropdown.
 *
 * It drives everything through UiAutomator and has no Compose test rule: a rule takes over every composition in the
 * process, Hisar's AutoFill screens included, and recomposes them only while the test waits through the rule.
 */
@RunWith(AndroidJUnit4::class)
class AutofillServiceUITests {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val testPackage = instrumentation.context.packageName
    private val appPackage = instrumentation.targetContext.packageName

    @Before
    fun turnOnHisarAutoFill() {
        shell("settings put secure autofill_service $appPackage/${HisarAutofillService::class.java.name}")
        clearClipboard()
    }

    @After
    fun closeTestPages() {
        shell("am force-stop $testPackage")
        shell("settings delete secure autofill_service")
        device.pressHome()
    }

    // Gboard shows a recent clip, such as a copied code, in place of the AutoFill suggestions.
    private fun clearClipboard() = instrumentation.targetContext.getSystemService(ClipboardManager::class.java).clearPrimaryClip()

    @Test
    fun testWebViewFormFillsAfterUnlock() {
        signUpWithLogins("AUTOFILL_SERVICE_WEB_", login("Example", "web-user", "web-password-12", "accounts.example.com", ONE_TIME_PASSWORD))
        testBackend.lock()

        openPage(AutofillTestPageActivity.LOGIN_PAGE)
        chooseSuggestion("Unlock Hisar", By.res("username"))
        saveScreenshot("autofill-service-locked")
        waitForObject(By.res("unlock-password")).text = MASTER_PASSWORD
        waitForObject(By.res("unlock-submit")).click()
        val login = waitForObject(By.res("autofill-login-Example"))
        waitForNotice("One-time code copied. It clears in about 2 minutes.") { login.click() }
        waitForReport("username=web-user; password=15 characters")
        saveScreenshot("autofill-service-filled")

        clearClipboard()
        openPage(AutofillTestPageActivity.LOGIN_PAGE)
        chooseSuggestion("Example", By.res("username"))
        waitForReport("username=web-user; password=15 characters")
        assertNull("an unlocked suggestion fills without Hisar's screens", device.findObject(By.res("autofill-cancel")))
    }

    @Test
    fun testOneTimeCodeFieldGetsTheCode() {
        signUpWithLogins("AUTOFILL_SERVICE_CODE_", login("Example", "code-user", "code-password", "accounts.example.com", ONE_TIME_PASSWORD))

        openPage(AutofillTestPageActivity.ONE_TIME_CODE_PAGE)
        chooseSuggestion("Example", By.res("code"))
        assertNotNull("a six-digit code is filled", device.wait(Until.findObject(By.text(Regex("code=\\d{6}").toPattern())), 10_000))
    }

    @Test
    fun testNativeAppSearchLinksTheLoginAndFills() {
        signUpWithLogins("AUTOFILL_SERVICE_APP_", login("Example Bank", "app-user", "app-password-1", "bank.example.com"))

        openForm()
        chooseSuggestion("Search Hisar", By.hint("Email"))
        waitForObject(By.res("autofill-search")).text = "bank"
        waitForObject(By.res("autofill-login-Example Bank")).click()
        saveScreenshot("autofill-service-link-app")
        waitForObject(By.res("autofill-link-always")).click()
        waitForReport("username=app-user; password=14 characters")

        openForm()
        chooseSuggestion("Example Bank", By.hint("Email"))
        waitForReport("username=app-user; password=14 characters")
    }

    @Test
    fun testSavePromptOpensTheEditorWithTheTypedLogin() {
        signUpWithLogins("AUTOFILL_SERVICE_SAVE_")

        openPage(AutofillTestPageActivity.LOGIN_PAGE)
        findSuggestion("Search Hisar", By.res("username"))
        // Typed key by key, as a person types: WebView reports a value set through accessibility only when the form is
        // submitted, after the system has already decided there is nothing to save.
        shell("input text new-user")
        waitForObject(By.res("password")).click()
        shell("input text new-password")
        waitForObject(By.text("Sign in")).click()
        val save = device.wait(Until.findObject(By.text("Save")), 15_000) ?: error("the system offered no save sheet")
        saveScreenshot("autofill-service-save-prompt")
        save.click()
        assertEquals("new-user", waitForObject(By.res("editor-username")).text)
        saveScreenshot("autofill-service-save-editor")
    }

    private fun openPage(page: String) = open(AutofillTestPageActivity::class.java.name, By.res("result")) { putExtra(AutofillTestPageActivity.EXTRA_PAGE, page) }

    private fun openForm() = open(AutofillTestFormActivity::class.java.name, By.desc("report"))

    private fun open(activity: String, loaded: BySelector, extras: Intent.() -> Unit = {}) {
        shell("am force-stop $testPackage")
        val intent = Intent().setComponent(ComponentName(testPackage, activity)).apply(extras)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
        instrumentation.targetContext.startActivity(intent)
        waitForObject(loaded)
    }

    // Gboard can miss the very first suggestions after it starts, so the field is focused again until they show. A
    // dropdown suggestion shows its title as text; one above the keyboard reports it in its content description.
    private fun chooseSuggestion(title: String, field: BySelector) = findSuggestion(title, field).click()

    private fun findSuggestion(title: String, field: BySelector): UiObject2 {
        repeat(4) {
            waitForObject(field).click()
            val suggestion = device.wait(Until.findObject(By.text(title)), 6_000) ?: device.findObject(By.descStartsWith(title))
            if (suggestion != null) return suggestion
            device.pressBack()
        }
        error("no AutoFill suggestion \"$title\" for $field")
    }

    private fun waitForReport(expected: String) {
        val isFilled = device.wait(Until.hasObject(By.text(expected)), 10_000)
        assertTrue("the form reads ${device.findObject(By.textStartsWith("username="))?.text}, not $expected", isFilled)
    }

    // A toast is announced to accessibility but never appears in the window hierarchy UiAutomator searches.
    private fun waitForNotice(text: String, action: () -> Unit) {
        try {
            instrumentation.uiAutomation.executeAndWaitForEvent(
                action,
                { it.eventType == AccessibilityEvent.TYPE_NOTIFICATION_STATE_CHANGED && it.text.any { line -> line.toString() == text } },
                10_000,
            )
        } catch (error: TimeoutException) {
            throw AssertionError("no notice \"$text\"", error)
        }
    }

    private fun waitForObject(selector: BySelector): UiObject2 = device.wait(Until.findObject(selector), 30_000) ?: error("$selector did not appear")

    private fun saveScreenshot(name: String) {
        val screenshot = checkNotNull(instrumentation.uiAutomation.takeScreenshot()) { "no screenshot of $name" }
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

    private fun shell(command: String): String =
        ParcelFileDescriptor.AutoCloseInputStream(instrumentation.uiAutomation.executeShellCommand(command)).use { it.readBytes().decodeToString() }

    private companion object {
        const val ONE_TIME_PASSWORD = "otpauth://totp/Example:user?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // gitleaks:allow, the RFC 6238 test secret
    }
}
