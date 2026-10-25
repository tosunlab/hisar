package com.tryhisar.hisar.ui.settings

import android.content.ClipData
import android.content.Context
import android.content.Intent
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
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.FileProvider
import com.tryhisar.hisar.HisarApplication
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.util.UUID
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

const val SHARE_FAILED_MESSAGE = "Hisar could not prepare the file to share. Try again."

/** The cache folder Hisar's FileProvider serves (`res/xml/shared_files.xml`). */
const val SHARED_FOLDER_NAME = "shared"

/**
 * Sends a file Hisar wrote to another app through the system's share sheet, as the iPhone's ShareLink. Only a copy in
 * the folder Hisar's FileProvider serves goes out, made for the share; it is deleted when the screen leaves, as the
 * iPhone deletes its file. [onFailure] runs when the copy cannot be made.
 */
@Composable
fun rememberFileSharer(mimeType: String, onFailure: () -> Unit): (File) -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val folder = remember { File(context.cacheDir, "$SHARED_FOLDER_NAME/${UUID.randomUUID()}") }
    DisposableEffect(folder) {
        onDispose { folder.deleteRecursively() }
    }
    return { file ->
        scope.launch {
            // The copy blocks and runs to its end even when the screen leaves meanwhile, and the screen's deletion can fail
            // on a copy that appears while it runs. Cancelled, the coroutine resumes only once the copy has ended, so a
            // copy that never reached the share sheet is deleted here, after it is complete.
            var isPublished = false
            try {
                val copy = withContext(Dispatchers.IO) {
                    try {
                        (context.applicationContext as HisarApplication).copyForShare(file, File(folder, file.name))
                    } catch (_: IOException) {
                        null
                    }
                }
                if (copy == null) {
                    onFailure()
                    return@launch
                }
                val uri = FileProvider.getUriForFile(context, "${context.packageName}.files", copy)
                val intent = Intent(Intent.ACTION_SEND)
                    .setType(mimeType)
                    .putExtra(Intent.EXTRA_STREAM, uri)
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                intent.clipData = ClipData.newRawUri(file.name, uri)
                context.startActivity(Intent.createChooser(intent, null))
                isPublished = true
            } finally {
                if (!isPublished) folder.deleteRecursively()
            }
        }
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
