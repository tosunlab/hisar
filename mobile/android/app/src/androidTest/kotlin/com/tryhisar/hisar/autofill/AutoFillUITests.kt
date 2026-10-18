package com.tryhisar.hisar.autofill

import android.content.Intent
import androidx.compose.ui.test.hasTestTag
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.debug.AutoFillPreview
import com.tryhisar.hisar.debug.PreviewActivity
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import org.junit.runner.RunWith

/** Drives the AutoFill screens through the debug preview, as the iPhone's AutoFillUITests do, against a real server. */
@RunWith(AndroidJUnit4::class)
class AutoFillUITests : HisarUITestCase() {
    private var preview: ActivityScenario<PreviewActivity>? = null

    @After
    fun closePreview() {
        preview?.close()
    }

    @Test
    fun testAutoFillListsMatchingLoginsAndFills() {
        signUpWithLogins(
            "AUTOFILL_",
            login("Example", "autofill-user", "autofill-password-1", "accounts.example.com"),
            login("Other site", "other-user", "other-password-1", "other.example.org"),
        )

        launchAutoFillAndUnlock()
        waitFor("autofill-login-Example")
        assertFalse("a login for another site is not suggested", exists(hasTestTag("autofill-login-Other site")))
        saveScreenshot("autofill-list")
        tap("autofill-login-Example")
        checkResult("Filled autofill-user with a password of 19 characters")

        launchAutoFillAndUnlock()
        type("other", "autofill-search")
        waitFor("autofill-login-Other site")
        saveScreenshot("autofill-search")
        tap("autofill-login-Other site")
        checkResult("Filled other-user with a password of 16 characters")
    }

    // The app and AutoFill share one unlock on Android, so locking stands in for the iPhone extension's fresh process.
    private fun launchAutoFillAndUnlock() {
        preview?.close()
        testBackend.lock()
        val intent = Intent(ApplicationProvider.getApplicationContext(), PreviewActivity::class.java)
            .putExtra(AutoFillPreview.PASSWORD_EXTRA, "https://example.com/login")
        preview = ActivityScenario.launch(intent)
        waitFor("unlock-password")
        saveScreenshot("autofill-lock")
        type(MASTER_PASSWORD, "unlock-password")
        tap("unlock-submit")
        waitFor("autofill-search")
        saveScreenshot("autofill-unlocked")
    }

    private fun checkResult(expectedMessage: String) {
        assertEquals(expectedMessage, readText("autofill-result"))
    }
}
