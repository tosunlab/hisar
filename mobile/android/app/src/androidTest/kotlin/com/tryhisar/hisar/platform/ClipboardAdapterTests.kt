package com.tryhisar.hisar.platform

import android.content.ClipData
import android.content.ClipboardManager
import androidx.test.ext.junit.runners.AndroidJUnit4
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
    fun testCopyIsMarkedSensitive() {
        val clipboard = ClipboardAdapter(targetContext)
        clipboard.setText("s3cret")
        val extras = clipboardManager.primaryClipDescription?.extras
        assertTrue("keyboards and the copy overlay hide a sensitive clip", extras?.getBoolean("android.content.extra.IS_SENSITIVE") == true)
        clipboardManager.setPrimaryClip(ClipData.newPlainText("Other", "something else"))
        assertEquals("Hisar never reads text another app copied", "", clipboard.getText())
    }

    @Test
    fun testAlarmClearsHisarsCopy() {
        ClipboardAdapter(targetContext, clearAfter = 5.seconds).setText("s3cret")
        assertEquals("s3cret", clipboardText)
        waitUntil(60_000, "the alarm clears the copy") { clipboardText == null }
    }
}
