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
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Sharing a plain export or an Emergency Kit on a real account: Hisar copies the file into the folder its FileProvider
 * serves, and that copy goes when its screen closes, also when the screen closes while the copy is still being made.
 */
@RunWith(AndroidJUnit4::class)
class FileHandoffUITests : HisarUITestCase() {
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    private val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
    private val handoffFolder = File(application.cacheDir, SettingsModel.HANDOFF_FOLDER_NAME)
    private val sharedFolder = File(application.cacheDir, SHARED_FOLDER_NAME)
    private val shareSheet = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))

    @Test
    fun testExportShareCopyGoesWhenTheScreenCloses() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_FILE_HANDOFF_EXPORT_INVITE_LINK"), readArgument("HISAR_UI_TEST_FILE_HANDOFF_EXPORT_EMAIL"), "Android Export")
        openSettings("open-account")

        exportPlainData()
        val export = findHandoffFile()
        tap("share-export")
        assertTrue("the share sheet opens", waitForShareSheet(10_000))
        assertArrayEquals("the share sheet gets the plain export", export.readBytes(), findSharedFiles().single().readBytes())
        device.pressBack()
        composeRule.waitUntil("the share sheet closes", 10_000) { !device.hasObject(shareSheet) }
        tap("export-done")
        composeRule.waitUntil("the export is deleted", 10_000) { !export.exists() }
        assertEquals("the share copy is deleted with its screen", emptyList<File>(), findSharedFiles())

        exportPlainData()
        shareWhileClosing("share-export") { tap("export-done") }
    }

    @Test
    fun testEmergencyKitShareCopyGoesWhenTheScreenCloses() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_FILE_HANDOFF_KIT_INVITE_LINK"), readArgument("HISAR_UI_TEST_FILE_HANDOFF_KIT_EMAIL"), "Android Kit")
        openSettings("open-secret-key")
        type(MASTER_PASSWORD, "secret-key-password")
        tap("secret-key-reveal")
        waitFor("share-emergency-kit", ACCOUNT_KEY_TIMEOUT)
        shareWhileClosing("share-emergency-kit") { goBack() }
    }

    private fun exportPlainData() {
        tap("open-export")
        tap("export-format-plain")
        if (exists(hasTestTag("export-password"))) type(MASTER_PASSWORD, "export-password")
        tapWhenEnabled("export-start")
        waitFor("share-export", ACCOUNT_KEY_TIMEOUT)
    }

    private fun findHandoffFile(): File = handoffFolder.walk().single { it.isFile }

    private fun findSharedFiles(): List<File> = sharedFolder.walk().filter { it.isFile }.toList()

    /** The sharer's coroutines run on the Compose test clock, which waitUntil advances while it waits. */
    private fun waitForShareSheet(timeoutMillis: Long): Boolean = try {
        composeRule.waitUntil(timeoutMillis = timeoutMillis) { device.hasObject(shareSheet) }
        true
    } catch (_: ComposeTimeoutException) {
        false
    }

    /**
     * Taps [shareTag] and closes the screen with [close] while the share copy is being made. The copy waits in its folder,
     * which nothing can delete from meanwhile, as when the screen's deletion listed the folder an instant before the copy
     * appeared in it: the deletion fails, and the copy is finished after its screen has gone.
     */
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
        composeRule.waitUntil("the share copy is deleted once it is finished: ${findSharedFiles()}", 10_000) { findSharedFiles().isEmpty() }
        assertFalse("no share sheet opens for a screen that closed", waitForShareSheet(2_000))
    }
}
