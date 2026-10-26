package com.tryhisar.hisar.ui.item

import android.content.ClipboardManager
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTouchInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.platform.targetContext
import com.tryhisar.hisar.platform.waitUntil
import org.junit.After
import org.junit.Test
import org.junit.runner.RunWith

/** Item detail on an in-memory backend, with the application's real Android clipboard. */
@RunWith(AndroidJUnit4::class)
class ItemDetailClipboardTests : HisarUITestCase() {
    private val application get() = targetContext.applicationContext as HisarTestApplication
    private val clipboardManager get() = targetContext.getSystemService(ClipboardManager::class.java)
    private val clipboardText: String?
        get() = clipboardManager.primaryClip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.text?.toString()

    private val backend = FakeBackend().apply { seedSignedIn() }
    private var host: ItemDetailHost? = null

    private fun openHost() = ItemDetailHost(backend, application).also { host = it }

    @After
    fun closeHost() {
        host?.close()
    }

    @Test
    fun testCertificateMenuCopiesToTheAndroidClipboard() {
        val certificate = "14:6D:E9:83:C5:73:06:50:D8:EE:B9:95:2F:34:FC:64:16:A0:83:42:E6:1D:BE:A8:8A:04:96:B2:3F:CF:44:E5"
        backend.addAndroidApp("personal", "bank", "com.bank.app", certificate)
        openHost().show("personal", "bank")
        waitFor("android-app-com.bank.app")
        composeRule.onNodeWithTag("android-app-com.bank.app").performTouchInput { longClick() }
        tap("field-action-Copy certificate")
        waitForGone("field-action-Copy certificate")
        waitFor(hasText("Certificate copied"))
        waitUntil(10_000, "the clipboard holds the full colon-separated certificate") { clipboardText == certificate }
    }
}
