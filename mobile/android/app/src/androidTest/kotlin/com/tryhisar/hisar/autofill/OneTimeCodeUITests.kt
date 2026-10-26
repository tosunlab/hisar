package com.tryhisar.hisar.autofill

import android.content.Intent
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.debug.AutoFillPreview
import com.tryhisar.hisar.debug.PreviewActivity
import java.nio.ByteBuffer
import java.time.Instant
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec
import org.junit.After
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** A setup key typed into the editor gives the item a code that the detail copies and AutoFill fills, as on the iPhone. */
@RunWith(AndroidJUnit4::class)
class OneTimeCodeUITests : HisarUITestCase() {
    private var preview: ActivityScenario<PreviewActivity>? = null

    @After
    fun closePreview() {
        preview?.close()
    }

    @Test
    fun testSetupKeyShowsCodeThatCopiesAndFills() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_ONE_TIME_CODE_INVITE_LINK"), readArgument("HISAR_UI_TEST_ONE_TIME_CODE_EMAIL"), "Android One-Time Code")

        tap("vault-Personal")
        choose("new-login", "new-menu")
        type("Example", "editor-title")
        type("code-user", "editor-username")
        type("code-password-1", "editor-password")
        type("not a setup key!", "editor-one-time-password")
        tap("editor-save")
        waitFor(hasText("The one-time password is not a valid secret or otpauth:// link"))
        saveScreenshot("otp-editor-invalid")
        composeRule.onNode(hasTestTag("editor-one-time-password")).performTextReplacement(SETUP_KEY)
        type("accounts.example.com", "editor-website-0")
        saveScreenshot("otp-editor")
        tap("editor-save")
        waitFor("item-Example")

        val startedAt = Instant.now()
        tap("item-Example")
        checkCurrentCode(readText("one-time-code-value").replace(" ", ""), startedAt)
        saveScreenshot("otp-item-detail")
        tap("copy-one-time-code")
        saveScreenshot("otp-copied")

        launchAutoFill(AutoFillPreview.ONE_TIME_CODE_EXTRA)
        waitFor("autofill-login-Example")
        saveScreenshot("otp-autofill-list")
        val fillStartedAt = Instant.now()
        tap("autofill-login-Example")
        checkCurrentCode(readText("autofill-result").removePrefix("Filled the one-time code "), fillStartedAt)

        launchAutoFill(AutoFillPreview.PASSWORD_EXTRA)
        waitFor("autofill-login-Example")
        val passwordStartedAt = Instant.now()
        tap("autofill-login-Example")
        val prefix = "Filled code-user with a password of 15 characters and copied the one-time code "
        val passwordResult = readText("autofill-result")
        assertTrue(passwordResult, passwordResult.startsWith(prefix))
        checkCurrentCode(passwordResult.removePrefix(prefix), passwordStartedAt)
    }

    // The app and AutoFill share one unlock on Android, so locking stands in for the iPhone extension's fresh process.
    private fun launchAutoFill(extra: String) {
        preview?.close()
        testBackend.lock()
        val intent = Intent(ApplicationProvider.getApplicationContext(), PreviewActivity::class.java)
            .putExtra(extra, "https://accounts.example.com/two-factor")
        preview = ActivityScenario.launch(intent)
        type(MASTER_PASSWORD, "unlock-password")
        tap("unlock-submit")
    }

    // A code read across a 30 s boundary may be the earlier one.
    private fun checkCurrentCode(code: String, startedAt: Instant) {
        assertTrue("$code is not the current code", code in setOf(computeCode(startedAt), computeCode(Instant.now())))
    }

    private companion object {
        // RFC 6238's test secret "12345678901234567890", typed the way a site shows a setup key.
        const val SETUP_KEY = "gezd gnbv gy3t qojq gezd gnbv gy3t qojq"
        val SECRET = "12345678901234567890".toByteArray()

        fun computeCode(at: Instant): String {
            val mac = Mac.getInstance("HmacSHA1").apply { init(SecretKeySpec(SECRET, "HmacSHA1")) }
            val digest = mac.doFinal(ByteBuffer.allocate(8).putLong(at.epochSecond / 30).array())
            val offset = digest[19].toInt() and 0x0f
            val binary = ByteBuffer.wrap(digest, offset, 4).int and 0x7fffffff
            return "%06d".format(binary % 1_000_000)
        }
    }
}
