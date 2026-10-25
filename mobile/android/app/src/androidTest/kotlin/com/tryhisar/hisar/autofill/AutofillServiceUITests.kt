package com.tryhisar.hisar.autofill

import android.content.ClipboardManager
import android.content.ComponentName
import android.content.Intent
import android.graphics.Bitmap
import android.os.Build
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
import com.tryhisar.hisar.model.ItemDetail
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

        openPage(AutofillTestPageActivity.MIXED_PAGE)
        chooseSuggestion("Example", By.res("username"))
        waitForReport("username=code-user; password=13 characters; code=")
        clearClipboard()
        waitForCode("username=code-user; password=13 characters; code=") { chooseSuggestion("Example", By.res("code")) }

        testBackend.lock()
        openPage(AutofillTestPageActivity.MIXED_PAGE)
        findSuggestion("Unlock Hisar", By.res("username"))
        dismissKeyboard("Unlock Hisar")
        unlockAndFillCode()
        chooseSuggestion("Unlock Hisar", By.res("username"))
        waitForObject(By.res("autofill-login-Example")).click()
        assertTrue("the locked username response still fills the login", device.wait(Until.hasObject(By.text(Regex("username=code-user; password=13 characters; code=\\d{6}").toPattern())), 10_000))
        clearClipboard()

        withoutPageLoadRequest {
            testBackend.lock()
            openPage(AutofillTestPageActivity.MIXED_PAGE)
            unlockAndFillCode()
            chooseSuggestion("Example", By.res("username"))
            assertTrue("after a code-first unlock, username focus offers the unlocked login", device.wait(Until.hasObject(By.text(Regex("username=code-user; password=13 characters; code=\\d{6}").toPattern())), 10_000))
        }

        clearClipboard()
        val personal = testBackend.listAllVault().single { it.name == "Personal" }
        val saved = testBackend.listAllItem(personal.id).single { it.title == "Example" }
        val caller = checkNotNull(InstalledApps.readAndroidApp(instrumentation.targetContext, testPackage))
        testBackend.addAndroidApp(personal.id, saved.id, caller.packageName, caller.certificateSHA256)
        open(AutofillTestFormActivity::class.java.name, By.desc("report")) { putExtra(AutofillTestFormActivity.EXTRA_CODE, true) }
        chooseSuggestion("Example", By.hint("Email"))
        waitForReport("username=code-user; password=13 characters; code=")
        clearClipboard()
        waitForCode("username=code-user; password=13 characters; code=") { chooseSuggestion("Example", By.hint("Code")) }
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
        type("new-user", "username=new-user; password=0 characters")
        waitForObject(By.res("password")).click()
        type("new-password", "username=new-user; password=12 characters")
        val detail = saveTypedLogin("new-user", "new-password")

        openPage(AutofillTestPageActivity.LOGIN_PAGE)
        chooseSuggestion(detail.title, By.res("username"))
        assertNotNull("a later visit receives the exact saved login", device.wait(Until.findObject(By.text("username=new-user; password=new-password")), 20_000))
    }

    // The system decides whether to save from the latest fill response only, and a field no earlier response covers asks
    // for a new one, so the code field's response comes between typing a login and signing in.
    @Test
    fun testMixedFormSavesTheTypedLoginAroundTheCodeField() {
        signUpWithLogins("AUTOFILL_SERVICE_SAVE_CODE_")

        openPage(AutofillTestPageActivity.MIXED_PAGE)
        findSuggestion("Search Hisar", By.res("username"))
        type("first-user", "username=first-user; password=0 characters; code=")
        waitForObject(By.res("password")).click()
        type("first-password", "username=first-user; password=14 characters; code=")
        findSuggestion("Search Hisar", By.res("code"))
        val first = saveTypedLogin("first-user", "first-password")

        withoutPageLoadRequest {
            openPage(AutofillTestPageActivity.MIXED_PAGE)
            findSuggestion("Search Hisar", By.res("code"))
            chooseSuggestion(first.title, By.res("username"))
            assertNotNull("a login field focused after the code field fills", device.wait(Until.findObject(By.text("username=first-user; password=first-password; code=")), 20_000))

            openPage(AutofillTestPageActivity.MIXED_PAGE)
            findSuggestion("Search Hisar", By.res("code"))
            findSuggestion("Search Hisar", By.res("username"))
            type("second-user", "username=second-user; password=0 characters; code=")
            waitForObject(By.res("password")).click()
            type("second-password", "username=second-user; password=15 characters; code=")
            saveTypedLogin("second-user", "second-password")
        }
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

    // Hiding the keyboard takes the focused field's suggestions with it, so a later field's suggestion is not a stale one.
    private fun dismissKeyboard(title: String) {
        device.pressBack()
        assertTrue("the suggestions close", device.wait(Until.gone(By.text(title)), 10_000) && device.wait(Until.gone(By.descStartsWith(title)), 10_000))
    }

    private fun unlockAndFillCode() {
        chooseSuggestion("Unlock Hisar", By.res("code"))
        waitForObject(By.res("unlock-password")).text = MASTER_PASSWORD
        waitForObject(By.res("unlock-submit")).click()
        val codeLogin = waitForObject(By.res("autofill-login-Example"))
        waitForCode("username=; password=0 characters; code=") { codeLogin.click() }
        clearClipboard()
    }

    // On Android 13 and later a page with a password field asks for a fill as it loads, before any field is focused, when
    // the device's autofill_dialog_hints name password, as Google Play's do, so the login fields always answer first.
    // Without that request the code field asks first, as it does on devices without those hints.
    private fun withoutPageLoadRequest(block: () -> Unit) {
        val valuesByKey = listOf("autofill_dialog_enabled", "autofill_dialog_hints").associateWith { shell("device_config get autofill $it").trim() }
        for (key in valuesByKey.keys) shell("device_config delete autofill $key")
        try {
            block()
        } finally {
            for ((key, value) in valuesByKey) if (value != "null") shell("device_config put autofill $key $value")
        }
    }

    /** Signs in with what was typed, accepts the system's offer to save and saves Hisar's editor; returns the saved login. */
    private fun saveTypedLogin(username: String, password: String): ItemDetail {
        waitForObject(By.text("Sign in")).click()
        val save = device.wait(Until.findObject(By.text("Save")), 15_000) ?: error("the system offered no save sheet")
        saveScreenshot("autofill-service-save-prompt-$username")
        save.click()
        if (!saveSheetOpensHisar) {
            device.wait(Until.gone(By.text("Save")), 10_000)
            instrumentation.targetContext.startActivity(checkNotNull(instrumentation.targetContext.packageManager.getLaunchIntentForPackage(appPackage)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        }
        assertEquals(username, waitForObject(By.res("editor-username")).text)
        saveScreenshot("autofill-service-save-editor-$username")
        assertEquals("https://accounts.example.com", waitForObject(By.res("editor-website-0")).text)
        waitForObject(By.res("editor-save")).click()
        assertTrue("the editor closes after saving", device.wait(Until.gone(By.res("editor-save")), 20_000))
        val personal = testBackend.listAllVault().single { it.name == "Personal" }
        val saved = testBackend.listAllItem(personal.id).single { it.username == username }
        val detail = testBackend.getItem(personal.id, saved.id)
        assertEquals(username, detail.username)
        assertEquals(password, detail.password)
        assertEquals(listOf("https://accounts.example.com"), detail.allURL)
        return detail
    }

    private fun waitForCode(prefix: String, fill: () -> Unit) {
        val personal = testBackend.listAllVault().single { it.name == "Personal" }
        val login = testBackend.listAllItem(personal.id).single { it.title == "Example" }
        val before = testBackend.getOneTimeCode(personal.id, login.id).code
        fill()
        val report = device.wait(Until.findObject(By.text(Regex("${Regex.escape(prefix)}\\d{6}").toPattern())), 10_000)
        assertNotNull("only the code is filled, preserving the login fields", report)
        val after = testBackend.getOneTimeCode(personal.id, login.id).code
        assertTrue("the field receives the current code", report!!.text.removePrefix(prefix) in setOf(before, after))
    }

    private fun waitForReport(expected: String) {
        val isFilled = device.wait(Until.hasObject(By.text(expected)), 20_000)
        assertTrue("the form reads ${device.findObject(By.textStartsWith("username="))?.text}, not $expected", isFilled)
    }

    // The first key filters out every suggestion. On API 29 the dropdown then closes, and keys that arrive while its
    // window goes away find the page without focus and are dropped, so the rest is typed once it has closed.
    private fun type(text: String, expectedReport: String) {
        shell("input text ${text.first()}")
        device.wait(Until.gone(By.text("Search Hisar")), 10_000)
        shell("input text ${text.drop(1)}")
        waitForReport(expectedReport)
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

        // Android 14 before QPR2 (March 2024) starts the save sheet's intent without letting the page's app lend its
        // right to open an activity, so it never opens Hisar; the user then finds the login when they open Hisar.
        val saveSheetOpensHisar = Build.VERSION.SDK_INT != Build.VERSION_CODES.UPSIDE_DOWN_CAKE || Build.VERSION.SECURITY_PATCH >= "2024-03-01"
    }
}
