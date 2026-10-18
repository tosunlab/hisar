package com.tryhisar.hisar.ui.settings

import android.content.Context
import android.os.Bundle
import android.os.CancellationSignal
import android.os.ParcelFileDescriptor
import android.print.PageRange
import android.print.PrintAttributes
import android.print.PrintDocumentAdapter
import android.print.PrintDocumentInfo
import android.print.PrintManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Saves a file Hisar wrote (an Emergency Kit, an export) where the user picks, through the system's file creator.
 * [onFinish] gets whether the copy was written; picking nothing calls nothing.
 */
@Composable
fun rememberFileSaver(mimeType: String, onFinish: (isSaved: Boolean) -> Unit): (File) -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var pendingFile by remember { mutableStateOf<File?>(null) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument(mimeType)) { uri ->
        val file = pendingFile
        pendingFile = null
        if (uri == null || file == null) return@rememberLauncherForActivityResult
        scope.launch {
            val isSaved = withContext(Dispatchers.IO) {
                try {
                    val output = context.contentResolver.openOutputStream(uri) ?: return@withContext false
                    output.use { file.inputStream().use { input -> input.copyTo(it) } }
                    true
                } catch (_: IOException) {
                    false
                }
            }
            onFinish(isSaved)
        }
    }
    return { file ->
        pendingFile = file
        launcher.launch(file.name)
    }
}

/** Opens the system's print dialog for a PDF Hisar wrote. */
fun printPdf(context: Context, file: File, jobName: String) {
    context.getSystemService(PrintManager::class.java).print(jobName, PdfFilePrintAdapter(file), null)
}

private class PdfFilePrintAdapter(private val file: File) : PrintDocumentAdapter() {
    override fun onLayout(
        oldAttributes: PrintAttributes?,
        newAttributes: PrintAttributes,
        cancellationSignal: CancellationSignal,
        callback: LayoutResultCallback,
        extras: Bundle?,
    ) {
        if (cancellationSignal.isCanceled) {
            callback.onLayoutCancelled()
            return
        }
        val info = PrintDocumentInfo.Builder(file.name).setContentType(PrintDocumentInfo.CONTENT_TYPE_DOCUMENT).build()
        callback.onLayoutFinished(info, oldAttributes != newAttributes)
    }

    override fun onWrite(pages: Array<out PageRange>, destination: ParcelFileDescriptor, cancellationSignal: CancellationSignal, callback: WriteResultCallback) {
        try {
            file.inputStream().use { input -> FileOutputStream(destination.fileDescriptor).use { input.copyTo(it) } }
            callback.onWriteFinished(arrayOf(PageRange.ALL_PAGES))
        } catch (error: IOException) {
            callback.onWriteFailed(error.message)
        }
    }
}
