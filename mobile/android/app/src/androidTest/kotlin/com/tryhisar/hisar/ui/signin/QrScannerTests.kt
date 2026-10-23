package com.tryhisar.hisar.ui.signin

import android.Manifest
import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import com.tryhisar.hisar.HisarUITestCase.Companion.MASTER_PASSWORD
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.app.SignInModel
import com.tryhisar.hisar.backend.createTestBackend
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.SignInLink
import com.tryhisar.hisar.platform.createTemporaryDirectory
import com.tryhisar.hisar.platform.requireArgument
import com.tryhisar.hisar.platform.targetContext
import com.tryhisar.hisar.ui.assertFieldText
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.theme.HisarTheme
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The sign-in screen's Emergency Kit scanner: the row opens the camera sheet, and the text of a QR code read from camera
 * frames fills the form in or explains why it did not. Frames come from pictures, since an emulator's camera shows no QR
 * code; the sheet passes the analyzer's text to [SignInModel.openScannedEmergencyKit] as these tests do.
 */
@RunWith(AndroidJUnit4::class)
class QrScannerTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @Before
    fun showSignIn() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.grantRuntimePermission(targetContext.packageName, Manifest.permission.CAMERA)
        runBlocking(Dispatchers.Main) { model.start { FakeBackend() } }
        composeRule.setContent {
            HisarTheme {
                Box(Modifier.fillMaxSize().exposeTestTags()) { SignInScreen(model) }
            }
        }
    }

    @After
    fun stopModel() {
        scope.cancel()
    }

    @Test
    fun testNonHisarCodeShowsTheAlert() {
        val code = renderQrCode("https://example.com/not-an-emergency-kit")
        val allScannedText = mutableListOf<String>()
        val analyzer = QrCodeAnalyzer { allScannedText += it }
        repeat(2) {
            val frame = BitmapImageProxy(code)
            analyzer.analyze(frame)
            assertTrue("the analyzer closes every frame", frame.isClosed)
        }
        assertEquals("only the first code is handed on", listOf("https://example.com/not-an-emergency-kit"), allScannedText)

        openScanner()
        composeRule.runOnUiThread { model.signIn.openScannedEmergencyKit(allScannedText.single()) }
        composeRule.waitUntil(10_000) { !exists("scanner") }
        composeRule.waitUntil(10_000) { composeRule.onAllNodesWithText("This QR code is not from a Hisar Emergency Kit.").fetchSemanticsNodes().isNotEmpty() }
        saveScreenshot("scanner-not-an-emergency-kit")
        composeRule.onNodeWithTag("scanner-message-ok").performClick()
        composeRule.waitUntil(10_000) { !exists("scanner-message-ok") }
        composeRule.assertFieldText("sign-in-email", "")
    }

    @Test
    fun testPlainTextAndLegacyKitFramesDecodeForSignIn() {
        val payloads = listOf(
            "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL2hpc2FyLmV4YW1wbGUvYT94PW9uZSt0d29cdTAwMjZ5PSUyMCIsImVtYWlsIjoiYWxwZXIra2l0QHRyeWhpc2FyLmNvbSIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ",
            "hisar://sign-in?server=https%3A%2F%2Fhisar.example%2Fa%3Fx%3Done%2Btwo%26y%3D%2520&email=alper%2Bkit%40tryhisar.com&secretKey=H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M",
        )
        for (payload in payloads) {
            var scanned: String? = null
            QrCodeAnalyzer { scanned = it }.analyze(BitmapImageProxy(renderQrCode(payload)))
            assertEquals(payload, scanned)
            openScanner()
            composeRule.runOnUiThread { model.signIn.openScannedEmergencyKit(checkNotNull(scanned)) }
            composeRule.waitUntil(10_000) { !exists("scanner") }
            composeRule.assertFieldText("sign-in-email", "alper+kit@tryhisar.com")
            composeRule.assertFieldText("sign-in-secret-key", "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M")
        }
        saveScreenshot("plain-text-and-legacy-kit")
    }

    @Test
    fun testEmergencyKitQrCodeFillsInTheSignIn() {
        val email = requireArgument("HISAR_UI_TEST_QR_SCANNER_EMAIL")
        val backend = createTestBackend()
        backend.signUp(requireArgument("HISAR_UI_TEST_QR_SCANNER_INVITE_LINK"), email, "Android QR Scanner", MASTER_PASSWORD)
        val server = backend.getStatus().server
        val kit = backend.exportEmergencyKit(MASTER_PASSWORD, createTemporaryDirectory("emergency-kit"))
        val page = renderFirstPage(File(kit.emergencyKitPath))
        TestStorage().openOutputFile("android-emergency-kit-page.png").use { page.compress(Bitmap.CompressFormat.PNG, 100, it) }

        var scannedText: String? = null
        QrCodeAnalyzer { scannedText = it }.analyze(BitmapImageProxy(page))
        val text = checkNotNull(scannedText) { "no QR code was read from the Emergency Kit" }
        assertEquals(SignInLink(server = server, email = email, secretKey = kit.secretKey), SignInLink.parse(text))

        openScanner()
        composeRule.runOnUiThread { model.signIn.openScannedEmergencyKit(text) }
        composeRule.waitUntil(10_000) { !exists("scanner") }
        composeRule.assertFieldText("sign-in-email", email)
        composeRule.assertFieldText("sign-in-secret-key", kit.secretKey)
        assertTrue(composeRule.onAllNodesWithText("Signs in to ${SignInModel.describeServer(server)}.").fetchSemanticsNodes().isNotEmpty())
        saveScreenshot("scanner-filled-sign-in")
    }

    private fun openScanner() {
        composeRule.onNodeWithTag("scan-emergency-kit").performClick()
        composeRule.waitUntil(10_000) { exists("scanner") }
        composeRule.waitUntil(10_000) {
            composeRule.onAllNodesWithText("Point the camera at the QR code on your Emergency Kit.").fetchSemanticsNodes().isNotEmpty()
        }
        saveScreenshot("scanner")
    }

    private fun exists(tag: String) = composeRule.onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty()

    private fun saveScreenshot(name: String) {
        composeRule.waitForIdle()
        val screenshot = checkNotNull(InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()) { "no screenshot of $name" }
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

    private fun renderQrCode(text: String): Bitmap {
        val matrix = QRCodeWriter().encode(text, BarcodeFormat.QR_CODE, 480, 480)
        return Bitmap.createBitmap(matrix.width, matrix.height, Bitmap.Config.ARGB_8888).apply {
            for (y in 0 until height) for (x in 0 until width) setPixel(x, y, if (matrix[x, y]) Color.BLACK else Color.WHITE)
        }
    }

    /** The Emergency Kit's page at twice its size in points, on white as it prints. */
    private fun renderFirstPage(pdf: File): Bitmap = ParcelFileDescriptor.open(pdf, ParcelFileDescriptor.MODE_READ_ONLY).use { descriptor ->
        PdfRenderer(descriptor).use { renderer ->
            renderer.openPage(0).use { page ->
                Bitmap.createBitmap(page.width * 2, page.height * 2, Bitmap.Config.ARGB_8888).apply {
                    eraseColor(Color.WHITE)
                    page.render(this, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                }
            }
        }
    }
}
