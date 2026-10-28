package com.tryhisar.hisar.ui.settings

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.IntentSender
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.CancellationSignal
import android.os.ParcelFileDescriptor
import android.print.PageRange
import android.print.PrintAttributes
import android.print.PrintDocumentAdapter
import android.print.PrintDocumentInfo
import android.print.PrintJob
import android.print.PrintManager
import android.provider.OpenableColumns
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
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

sealed interface FileSaveResult {
    data class Saved(val uri: Uri) : FileSaveResult
    data object Cancelled : FileSaveResult
    data object Failed : FileSaveResult
}

/**
 * Saves a file Hisar wrote where the user picks. [onResult] distinguishes cancellation from a failed write.
 * [onFinish] gets the saved document's URI, or null on failure; picking nothing calls only [onResult].
 */
@Composable
fun rememberFileSaver(
    mimeType: String,
    onResult: (FileSaveResult) -> Unit = {},
    onFinish: (savedURI: Uri?) -> Unit,
): (File) -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var pendingFile by remember { mutableStateOf<File?>(null) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument(mimeType)) { uri ->
        val file = pendingFile
        pendingFile = null
        if (file == null) return@rememberLauncherForActivityResult
        if (uri == null) {
            onResult(FileSaveResult.Cancelled)
            return@rememberLauncherForActivityResult
        }
        scope.launch {
            val isSaved = withContext(Dispatchers.IO) {
                try {
                    val output = context.contentResolver.openOutputStream(uri) ?: return@withContext false
                    output.use { file.inputStream().use { input -> input.copyTo(it) } }
                    true
                } catch (_: IOException) {
                    false
                } catch (_: SecurityException) {
                    false
                }
            }
            onResult(if (isSaved) FileSaveResult.Saved(uri) else FileSaveResult.Failed)
            onFinish(uri.takeIf { isSaved })
        }
    }
    return { file ->
        pendingFile = file
        launcher.launch(file.name)
    }
}

/** Reports the activity the chooser opened, not whether it kept the file. System actions can report null. */
class FileShareReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val callback = callbacks.remove(intent.data) ?: return
        @Suppress("DEPRECATION")
        val component = intent.getParcelableExtra<ComponentName>(Intent.EXTRA_CHOSEN_COMPONENT)
        callback(component)
    }

    class Callback internal constructor(private val identity: Uri, private val pending: PendingIntent) : AutoCloseable {
        val intentSender: IntentSender get() = pending.intentSender

        override fun close() {
            callbacks.remove(identity)
            pending.cancel()
        }
    }

    companion object {
        private val callbacks = mutableMapOf<Uri, (ComponentName?) -> Unit>()

        fun register(context: Context, onChosen: (ComponentName?) -> Unit): Callback {
            val identity = Uri.parse("hisar-share:${UUID.randomUUID()}")
            val intent = Intent(context, FileShareReceiver::class.java).setData(identity)
            // The chooser fills EXTRA_CHOSEN_COMPONENT into this explicitly targeted intent.
            val mutable = if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0
            val pending = PendingIntent.getBroadcast(context, 0, intent, PendingIntent.FLAG_ONE_SHOT or mutable)
            callbacks[identity] = onChosen
            return Callback(identity, pending)
        }
    }
}

const val SHARE_FAILED_MESSAGE = "Hisar could not prepare the file to share. Try again."

/** The cache folder Hisar's FileProvider serves (`res/xml/shared_files.xml`). */
const val SHARED_FOLDER_NAME = "shared"

/**
 * Sends a file Hisar wrote to another app through the system's share sheet, as the iPhone's ShareLink. Only a copy in
 * the folder Hisar's FileProvider serves goes out, made for the share; it is deleted when the screen leaves, as the
 * iPhone deletes its file. [onFailure] runs when the copy cannot be made, [onShared] once the share sheet opened.
 * [onTargetChosen] observes the chooser's component; a callback with null is not a target selection.
 */
@Composable
fun rememberFileSharer(
    mimeType: String,
    onShared: () -> Unit = {},
    onTargetChosen: (ComponentName?) -> Unit = {},
    onFailure: () -> Unit,
): (File) -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val allPublishedFolder = remember { mutableListOf<File>() }
    val pendingChoice = remember { mutableStateOf<FileShareReceiver.Callback?>(null) }
    DisposableEffect(Unit) {
        onDispose {
            pendingChoice.value?.close()
            allPublishedFolder.forEach(File::deleteRecursively)
        }
    }
    return { file ->
        scope.launch {
            // Each share has a folder of its own, so a share that fails deletes only its own copy. The copy blocks and
            // runs to its end even when the screen leaves meanwhile. Cancelled, the coroutine resumes only once the copy
            // has ended, so a copy that never reached the share sheet is deleted here, after it is complete.
            val folder = File(context.cacheDir, "$SHARED_FOLDER_NAME/${UUID.randomUUID()}")
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
                pendingChoice.value?.close()
                val choice = FileShareReceiver.register(context, onTargetChosen)
                pendingChoice.value = choice
                context.startActivity(Intent.createChooser(intent, null, choice.intentSender))
                allPublishedFolder += folder
                isPublished = true
                onShared()
            } finally {
                if (!isPublished) folder.deleteRecursively()
            }
        }
    }
}

/** The largest import file HisarKit reads, 100 MB; a larger file is refused before it is read. */
const val IMPORT_FILE_SIZE_LIMIT = 100L * 1024 * 1024

const val IMPORT_FILE_TOO_LARGE_MESSAGE = "That file is too large to import. A Hisar export is under 100 MB."
const val IMPORT_FILE_UNREADABLE_MESSAGE = "Hisar couldn't read that file. Choose the Hisar export again."

/**
 * Lets the user pick a Hisar export (a `.hisar` or a plain `.json`) through the system's file picker, reads its bytes off
 * the main thread under the 100 MB limit, and hands them to [onFile] with the file's display name. [onProblem] gets a
 * sentence when the file is too large or cannot be read; picking nothing calls nothing.
 */
@Composable
fun rememberHisarExportPicker(onProblem: (String) -> Unit, onFile: (fileName: String, content: ByteArray) -> Unit): () -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri ?: return@rememberLauncherForActivityResult
        scope.launch {
            when (val result = withContext(Dispatchers.IO) { readImportFile(context, uri) }) {
                is ImportFileRead.Content -> onFile(result.fileName, result.content)
                is ImportFileRead.Problem -> onProblem(result.message)
            }
        }
    }
    // ".hisar" has no registered type, so the octet-stream and */* entries keep it selectable beside the plain JSON.
    return { launcher.launch(arrayOf("application/json", "application/octet-stream", "*/*")) }
}

private sealed interface ImportFileRead {
    data class Content(val fileName: String, val content: ByteArray) : ImportFileRead

    data class Problem(val message: String) : ImportFileRead
}

private fun readImportFile(context: Context, uri: Uri): ImportFileRead {
    val fileName = queryDisplayName(context, uri) ?: "export"
    return try {
        val content = context.contentResolver.openInputStream(uri)?.use { input ->
            val buffer = ByteArrayOutputStream()
            val chunk = ByteArray(1 shl 16)
            while (true) {
                val read = input.read(chunk)
                if (read < 0) break
                if (buffer.size().toLong() + read > IMPORT_FILE_SIZE_LIMIT) return ImportFileRead.Problem(IMPORT_FILE_TOO_LARGE_MESSAGE)
                buffer.write(chunk, 0, read)
            }
            buffer.toByteArray()
        } ?: return ImportFileRead.Problem(IMPORT_FILE_UNREADABLE_MESSAGE)
        ImportFileRead.Content(fileName, content)
    } catch (_: IOException) {
        ImportFileRead.Problem(IMPORT_FILE_UNREADABLE_MESSAGE)
    } catch (_: SecurityException) {
        ImportFileRead.Problem(IMPORT_FILE_UNREADABLE_MESSAGE)
    }
}

private fun queryDisplayName(context: Context, uri: Uri): String? =
    context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor ->
        val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
        if (cursor.moveToFirst() && index >= 0) cursor.getString(index) else null
    }?.takeIf { it.isNotEmpty() }

/** Opens the print dialog. [onFinish] can run while the returned job is still CREATED, even after Save as PDF. */
fun printPdf(context: Context, file: File, jobName: String, onFinish: (PrintJob) -> Unit = {}): PrintJob {
    lateinit var job: PrintJob
    val adapter = PdfFilePrintAdapter(file) { onFinish(job) }
    job = context.getSystemService(PrintManager::class.java).print(jobName, adapter, null)
    return job
}

private class PdfFilePrintAdapter(private val file: File, private val onFinished: () -> Unit) : PrintDocumentAdapter() {
    override fun onFinish() {
        onFinished()
    }

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
