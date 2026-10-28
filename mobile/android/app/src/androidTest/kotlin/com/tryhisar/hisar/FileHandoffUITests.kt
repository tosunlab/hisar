package com.tryhisar.hisar

import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.hasTestTag
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import com.tryhisar.hisar.app.SettingsModel
import com.tryhisar.hisar.ui.settings.SHARED_FOLDER_NAME
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.regex.Pattern
import org.junit.Assert.assertFalse
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class FileHandoffUITests : HisarUITestCase() {
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    private val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
    private val handoffFolder = File(application.cacheDir, SettingsModel.HANDOFF_FOLDER_NAME)
    private val sharedFolder = File(application.cacheDir, SHARED_FOLDER_NAME)
    private val shareSheet = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))

    @Test
    fun testEmergencyKitShareCopyGoesWhenTheScreenCloses() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_FILE_HANDOFF_KIT_INVITE_LINK"), readArgument("HISAR_UI_TEST_FILE_HANDOFF_KIT_EMAIL"), "Android Kit")
        openSettings("open-account")
        tap("open-account-secret-key")
        type(MASTER_PASSWORD, "secret-key-password")
        tap("secret-key-reveal")
        waitFor("share-emergency-kit", ACCOUNT_KEY_TIMEOUT)
        shareWhileClosing("share-emergency-kit") { goBack() }

        exportPlainData()
        val export = findHandoffFile()
        tap("export-done")
        composeRule.waitUntil("the plain export is deleted with its screen", 10_000) { !export.exists() }
    }

    private fun exportPlainData() {
        tap("open-export")
        tap("export-format-plain")
        if (exists(hasTestTag("export-password"))) type(MASTER_PASSWORD, "export-password")
        tapWhenEnabled("export-start")
        waitFor("share-export", ACCOUNT_KEY_TIMEOUT)
    }

    private fun findHandoffFile(): File = handoffFolder.walk().single { it.isFile }

    private fun listSharedFolder(): List<String> = sharedFolder.list().orEmpty().toList()

    /** The sharer's coroutines run on the Compose test clock, which waitUntil advances while it waits. */
    private fun waitForShareSheet(timeoutMillis: Long): Boolean = try {
        composeRule.waitUntil(timeoutMillis = timeoutMillis) { device.hasObject(shareSheet) }
        true
    } catch (_: ComposeTimeoutException) {
        false
    }

    private fun shareWhileClosing(shareTag: String, close: () -> Unit) {
        val source = findHandoffFile()
        val isCopied = CountDownLatch(1)
        val release = CountDownLatch(1)
        val isFinished = CountDownLatch(1)
        application.shareCopy = { from, target ->
            try {
                val copy = from.copyTo(target, overwrite = true)
                val folder = checkNotNull(target.parentFile)
                folder.setWritable(false)
                isCopied.countDown()
                release.await(60, TimeUnit.SECONDS)
                folder.setWritable(true)
                copy
            } finally {
                isFinished.countDown()
            }
        }
        try {
            tap(shareTag)
            composeRule.waitUntil("the share copy is being made", 10_000) { isCopied.count == 0L }
            close()
            composeRule.waitUntil("the screen closes and deletes its own file", 10_000) { !source.exists() }
        } finally {
            release.countDown()
        }
        composeRule.waitUntil("the share copy is finished", 10_000) { isFinished.count == 0L }
        composeRule.waitUntil("the share copy is deleted once it is finished: ${listSharedFolder()}", 10_000) { listSharedFolder().isEmpty() }
        assertFalse("no share sheet opens for a screen that closed", waitForShareSheet(2_000))
    }
}
