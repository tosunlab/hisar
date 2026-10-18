package com.tryhisar.hisar.platform

import android.content.ClipData
import android.content.ClipboardManager
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import kotlin.concurrent.thread
import kotlin.time.Duration.Companion.seconds
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ClipboardAdapterTests {
    private val clipboardManager = targetContext.getSystemService(ClipboardManager::class.java)
    private val scenario = launchFocusedActivity()

    private val clipboardText: String?
        get() = clipboardManager.primaryClip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.text?.toString()

    @Before
    fun setUp() {
        clipboardManager.clearPrimaryClip()
    }

    @After
    fun tearDown() {
        ClipboardAdapter(targetContext).setText("")
        scenario.close()
    }

    @Test
    fun testCopiedTextIsReportedUntilReplaced() {
        val clipboard = ClipboardAdapter(targetContext)
        assertEquals("", clipboard.getText())
        assertTrue(clipboard.setText("s3cret"))
        assertEquals("s3cret", clipboardText)
        assertEquals("s3cret", clipboard.getText())
        clipboardManager.setPrimaryClip(ClipData.newPlainText("Other", "something else"))
        assertEquals("Hisar never reads text another app copied", "", clipboard.getText())
    }

    @Test
    fun testEmptyTextClearsThePasteboard() {
        val clipboard = ClipboardAdapter(targetContext)
        clipboard.setText("s3cret")
        clipboard.setText("")
        assertEquals(null, clipboardText)
    }

    @Test
    fun testSetTextFromABackgroundThread() {
        val clipboard = ClipboardAdapter(targetContext)
        var isSet = false
        thread { isSet = clipboard.setText("from Go") }.join()
        assertTrue(isSet)
        assertEquals("from Go", clipboardText)
    }

    @Test
    fun testCopyIsMarkedSensitive() {
        ClipboardAdapter(targetContext).setText("s3cret")
        val extras = clipboardManager.primaryClipDescription?.extras
        assertTrue("keyboards and the copy overlay hide a sensitive clip", extras?.getBoolean("android.content.extra.IS_SENSITIVE") == true)
    }

    @Test
    fun testPlainCopyIsNotSensitiveAndNeverCleared() {
        val clipboard = ClipboardAdapter(targetContext, clearAfter = 10.seconds)
        clipboard.setText("s3cret")
        assertTrue(clipboard.setPlainText("support@tryhisar.com"))
        assertEquals("support@tryhisar.com", clipboardText)
        assertTrue(clipboardManager.primaryClipDescription?.extras?.getBoolean("android.content.extra.IS_SENSITIVE") != true)
        assertEquals("Go never clears plain text", "", clipboard.getText())
        Thread.sleep(45_000)
        assertEquals("the secret's alarm no longer clears the clipboard", "support@tryhisar.com", clipboardText)
    }

    @Test
    fun testAlarmClearsHisarsCopy() {
        ClipboardAdapter(targetContext, clearAfter = 10.seconds).setText("s3cret")
        assertEquals("s3cret", clipboardText)
        waitUntil(60_000, "the alarm clears the copy") { clipboardText == null }
    }

    @Test
    fun testAlarmKeepsANewerCopy() {
        ClipboardAdapter(targetContext, clearAfter = 10.seconds).setText("s3cret")
        clipboardManager.setPrimaryClip(ClipData.newPlainText("Other", "copied later"))
        Thread.sleep(40_000)
        assertEquals("the alarm, run while Hisar could see the newer copy, kept it", "copied later", clipboardText)
    }

    @Test
    fun testExpiredCopyIsClearedWhenHisarReturns() {
        val clipboard = ClipboardAdapter(targetContext, clearAfter = 5.seconds)
        ForegroundActivity(application, clipboard::clearExpiredCopy)
        clipboard.setText("s3cret")
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation()).pressHome()
        Thread.sleep(6_000)
        launchFocusedActivity().use {
            waitUntil(5_000, "Hisar clears its expired copy when it is back in front") { clipboardText == null }
        }
    }
}
