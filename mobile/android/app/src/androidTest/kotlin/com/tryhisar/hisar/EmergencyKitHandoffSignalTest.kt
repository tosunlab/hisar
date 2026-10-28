package com.tryhisar.hisar

import android.content.ComponentName
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.pdf.PdfDocument
import android.graphics.pdf.PdfRenderer
import android.os.Build
import android.os.Bundle
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import android.print.PrintJob
import android.print.PrintJobInfo
import androidx.activity.ComponentActivity
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.UiObject2
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.ui.settings.FileSaveResult
import com.tryhisar.hisar.ui.settings.FileShareReceiver
import com.tryhisar.hisar.ui.settings.printPdf
import com.tryhisar.hisar.ui.settings.rememberFileSaver
import com.tryhisar.hisar.ui.settings.rememberFileSharer
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicReference
import java.util.regex.Pattern
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class EmergencyKitHandoffSignalTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val chosen = CopyOnWriteArrayList<String>()
    private val saved = AtomicReference<FileSaveResult>()
    private val finishedState = AtomicReference<Int>()
    private lateinit var pdf: File
    private lateinit var evidenceFolder: File
    private lateinit var job: PrintJob
    private val chooser = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))
    private val documents = By.pkg(Pattern.compile("com\\.android\\.documentsui|com\\.google\\.android\\.documentsui"))

    @Before fun prepare() {
        evidenceFolder = File(compose.activity.getExternalFilesDir(null), "handoff-evidence/api${Build.VERSION.SDK_INT}").apply { mkdirs() }
        pdf = File(compose.activity.cacheDir, "handoff-${SystemClock.uptimeMillis()}.pdf")
        val document = PdfDocument()
        try {
            val page = document.startPage(PdfDocument.PageInfo.Builder(300, 400, 1).create())
            page.canvas.drawText("Emergency Kit handoff experiment", 20f, 50f, android.graphics.Paint())
            document.finishPage(page)
            pdf.outputStream().use(document::writeTo)
        } finally {
            document.close()
        }
    }

    @After fun closeSheets() {
        repeat(3) {
            if (device.currentPackageName != instrumentation.targetContext.packageName) device.pressBack()
        }
        pdf.delete()
    }

    @Test fun testShareTargetChosen() {
        openShare()
        required(By.text("Handoff receiver")).click()
        required(By.text("Received ${pdf.length()} PDF bytes"))
        compose.waitUntil(10_000) { chosen.isNotEmpty() }
        record("share-target", "callbacks=$chosen; receiver read ${pdf.length()} bytes")
        assertEquals(listOf(ComponentName(instrumentation.context.packageName, HandoffReceiverActivity::class.java.name).flattenToString()), chosen.toList())
        device.pressBack()
    }

    @Test fun testShareBack() {
        openShare()
        device.pressBack()
        assertNoShare("share-back")
    }

    @Test fun testShareOutsideTap() {
        openShare()
        device.click(device.displayWidth / 2, device.displayHeight / 10)
        assertTrue("outside tap dismisses chooser", device.wait(Until.gone(chooser), 5_000))
        assertNoShare("share-outside")
    }

    @Test fun testShareSystemActions() {
        openShare()
        val actions = listOf("Copy", "Edit", "Quick Share", "Nearby Share")
        val offered = actions.filter { device.hasObject(By.textContains(it)) || device.hasObject(By.descContains(it)) }
        record("share-system-actions", "PDF chooser offers=$offered")
        if (offered.contains("Copy")) {
            (device.findObject(By.textContains("Copy")) ?: required(By.descContains("Copy"))).click()
            assertNoShare("share-copy")
        } else if (offered.contains("Quick Share")) {
            (device.findObject(By.textContains("Quick Share")) ?: required(By.descContains("Quick Share"))).click()
            SystemClock.sleep(2_000)
            record("share-quick-share", "callbacks=$chosen; foreground=${device.currentPackageName}")
        } else {
            device.pressBack()
            assertNoShare("share-system-actions-closed")
        }
    }

    @Test fun testChooserCopyTextControl() {
        lateinit var callback: FileShareReceiver.Callback
        compose.runOnUiThread {
            callback = FileShareReceiver.register(compose.activity) { chosen.add(it?.flattenToString() ?: "null") }
            val send = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, "Emergency Kit experiment, no secrets")
            compose.activity.startActivity(Intent.createChooser(send, null, callback.intentSender))
        }
        try {
            required(chooser)
            capture("share-text-control")
            val copy = device.findObject(By.textContains("Copy")) ?: device.findObject(By.descContains("Copy"))
            if (copy == null) {
                record("share-copy-text-control", "Copy not offered")
                assertTrue("API34+ text chooser offers Copy", Build.VERSION.SDK_INT < 34)
                device.pressBack()
            } else {
                copy.click()
                assertTrue("Copy dismisses chooser", device.wait(Until.gone(chooser), 5_000))
                SystemClock.sleep(2_000)
                record("share-copy-text-control", "callbacks=$chosen")
                assertTrue("Copy is not a chosen component", chosen.all { it == "null" })
            }
        } finally {
            compose.runOnUiThread { callback.close() }
        }
    }

    @Test fun testChooserEditImageControl() {
        val image = File(compose.activity.cacheDir, "handoff-image.png")
        val bitmap = Bitmap.createBitmap(300, 400, Bitmap.Config.ARGB_8888)
        bitmap.eraseColor(android.graphics.Color.BLUE)
        image.outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        bitmap.recycle()
        try {
            openShare(image, "image/png")
            val edit = device.findObject(By.textContains("Edit")) ?: device.findObject(By.descContains("Edit"))
            if (edit == null) {
                record("share-edit-image-control", "Edit not offered")
            } else {
                edit.click()
                SystemClock.sleep(2_000)
                record("share-edit-image-control", "callbacks=$chosen; foreground=${device.currentPackageName}")
            }
        } finally {
            image.delete()
        }
    }

    @Test fun testSaveDocumentWritten() {
        openSave()
        chooseRoot("Downloads")
        saveDocument()
        compose.waitUntil(10_000) { saved.get() != null }
        val result = saved.get()
        record("saf-written", "result=$result")
        assertTrue(result is FileSaveResult.Saved)
        val uri = (result as FileSaveResult.Saved).uri
        assertArrayEquals(pdf.readBytes(), compose.activity.contentResolver.openInputStream(uri)!!.use { it.readBytes() })
    }

    @Test fun testSaveDocumentCancelled() {
        openSave()
        device.pressBack()
        compose.waitUntil(10_000) { saved.get() != null }
        record("saf-cancelled", "result=${saved.get()}")
        assertEquals(FileSaveResult.Cancelled, saved.get())
    }

    @Test fun testSaveDocumentWriteFailed() {
        openSave()
        chooseRoot("Reject writes")
        saveDocument()
        compose.waitUntil(10_000) { saved.get() != null }
        record("saf-failed", "result=${saved.get()}; provider rejects openDocument(w)")
        assertEquals(FileSaveResult.Failed, saved.get())
    }

    @Test fun testPrintSaveAsPdf() {
        openPrint()
        openPrintDestination()
        chooseRoot("Downloads")
        saveDocument()
        observePrintFinish("print-saved", PrintJobInfo.STATE_COMPLETED)
        val printed = File(evidenceFolder, pdf.name)
        device.executeShellCommand("cp /sdcard/Download/${pdf.name} ${printed.absolutePath}")
        val descriptor = ParcelFileDescriptor.open(printed, ParcelFileDescriptor.MODE_READ_ONLY)
        val renderer = PdfRenderer(descriptor)
        try {
            assertEquals("Save as PDF creates a readable page", 1, renderer.pageCount)
            record("print-file", "Downloads/${pdf.name} size=${printed.length()}; pages=${renderer.pageCount}")
        } finally {
            renderer.close()
        }
    }

    @Test fun testPrintCancelDestination() {
        openPrint()
        openPrintDestination()
        device.pressBack()
        required(By.pkg("com.android.printspooler"))
        record("print-destination-back", "onFinish=${finishedState.get()}; current=${job.info.state}")
        assertEquals(null, finishedState.get())
        device.pressBack()
        observePrintFinish("print-destination-cancelled", PrintJobInfo.STATE_CANCELED)
    }

    @Test fun testPrintCancelPreview() {
        openPrint()
        device.pressBack()
        observePrintFinish("print-preview-cancelled", PrintJobInfo.STATE_CANCELED)
    }

    @Test fun testPrintWriteFailure() {
        pdf.delete()
        openPrint()
        required(By.textContains("ENOENT"))
        record("print-write-error", "onFinish=${finishedState.get()}; current=${job.info.state}")
        device.pressBack()
        observePrintFinish("print-write-failed-cancelled", PrintJobInfo.STATE_CANCELED)
    }

    private fun openShare(file: File = pdf, mimeType: String = "application/pdf") {
        compose.setContent {
            val share = rememberFileSharer(mimeType, onTargetChosen = { chosen.add(it?.flattenToString() ?: "null") }, onFailure = { error("File preparation failed") })
            Button(onClick = { share(file) }) { Text("Share file") }
        }
        compose.onNodeWithText("Share file").performClick()
        compose.waitUntil(30_000) { device.hasObject(chooser) }
        if (mimeType == "application/pdf") required(By.text("Handoff receiver")) else required(chooser)
        capture("share-open")
    }

    private fun assertNoShare(name: String) {
        assertTrue("chooser dismissed", device.wait(Until.gone(chooser), 5_000))
        SystemClock.sleep(2_000)
        record(name, "callbacks=$chosen after 2 seconds")
        assertTrue("cancellation or Copy is not target selection", chosen.isEmpty())
    }

    private fun openSave() {
        compose.setContent {
            val save = rememberFileSaver("application/pdf", onResult = saved::set) {}
            Button(onClick = { save(pdf) }) { Text("Save PDF") }
        }
        compose.onNodeWithText("Save PDF").performClick()
        compose.waitUntil(30_000) { device.hasObject(documents) }
        capture("saf-open")
    }

    private fun chooseRoot(name: String) {
        required(By.desc("Show roots")).click()
        val root = By.res("android", "title").text(name)
        required(root).click()
        assertTrue("the roots drawer closes", device.wait(Until.gone(root), 10_000))
    }

    private fun saveDocument() {
        required(By.res(Pattern.compile(".*:id/button1")).enabled(true)).click()
        val closed = device.wait(Until.gone(documents), 10_000)
        if (!closed) capture("picker-not-closed")
        assertTrue("file picker closes", closed)
    }

    private fun openPrint() {
        compose.runOnUiThread { job = printPdf(compose.activity, pdf, pdf.name) { finishedState.set(it.info.state) } }
        required(By.pkg("com.android.printspooler"))
        if (!device.hasObject(By.text("Save as PDF"))) {
            required(By.res("com.android.printspooler", "destination_spinner")).click()
            required(By.text("Save as PDF")).click()
        }
        capture("print-preview")
    }

    private fun openPrintDestination() {
        required(By.res("com.android.printspooler", "print_button")).click()
        required(documents)
        capture("print-destination")
    }

    private fun observePrintFinish(name: String, expectedState: Int) {
        compose.waitUntil(10_000) { finishedState.get() != null }
        val immediate = finishedState.get()
        SystemClock.sleep(2_000)
        record(name, "onFinish=$immediate; after2s=${job.info.state}; job=${job.id}")
        assertEquals(expectedState, job.info.state)
    }

    private fun required(selector: BySelector): UiObject2 {
        assertTrue("Missing $selector; evidence folder $evidenceFolder", device.wait(Until.hasObject(selector), 30_000))
        device.waitForIdle()
        return checkNotNull(device.findObject(selector)) { "Missing $selector; hierarchy at ${capture("missing")}" }
    }

    private fun capture(name: String): File {
        val base = File(evidenceFolder, "$name-${SystemClock.uptimeMillis()}")
        device.dumpWindowHierarchy(File("$base.xml"))
        device.takeScreenshot(File("$base.png"))
        return base
    }

    private fun record(name: String, result: String) {
        val evidence = capture(name)
        val line = "API ${Build.VERSION.SDK_INT} | $name | $result | ${evidence.name}\n"
        File(evidenceFolder, "results.txt").appendText(line)
        instrumentation.sendStatus(0, Bundle().apply { putString("stream", line) })
    }
}

