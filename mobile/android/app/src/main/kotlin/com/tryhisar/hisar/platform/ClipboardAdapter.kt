package com.tryhisar.hisar.platform

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper
import android.os.PersistableBundle
import android.os.SystemClock
import com.tryhisar.hisarkit.Clipboard
import java.util.concurrent.FutureTask
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

/**
 * The clipboard for Go and the UI. A copy is marked sensitive, so the system and keyboards hide it, and is cleared about
 * 90 seconds later by an alarm, or when Hisar comes back to the front after that time. Android lets Hisar read the
 * clipboard only while it has focus, so a clear in the background cannot tell Hisar's copy from a newer one and clears
 * either; in the front it keeps a newer one.
 */
class ClipboardAdapter(private val context: Context, private val clearAfter: Duration = 90.seconds) : Clipboard {
    private val mainHandler = Handler(Looper.getMainLooper())

    @Volatile
    private var copiedText = ""

    override fun setText(text: String): Boolean = try {
        runOnMain { if (text.isEmpty()) clear(context) else copy(text) }
        true
    } catch (_: RuntimeException) {
        false
    }

    /** Hisar's own copy while it is still on the clipboard, never what another app copied. */
    override fun getText(): String = runOnMain { if (HisarClip.isOnClipboard(context)) copiedText else "" }

    /** Clears Hisar's copy once it is due; run while Hisar has focus. */
    fun clearExpiredCopy() {
        val clip = HisarClip.read(context) ?: return
        if (System.currentTimeMillis() >= clip.expiresAt) clearHisarsCopy(context)
    }

    private fun copy(text: String) {
        val clipData = ClipData.newPlainText("Hisar", text).apply {
            description.extras = PersistableBundle().apply { putBoolean(EXTRA_IS_SENSITIVE, true) }
        }
        val clipboard = context.getSystemService(ClipboardManager::class.java)
        clipboard.setPrimaryClip(clipData)
        // The timestamp exists only on the clipboard's copy of the clip, so it is read back.
        val timestamp = clipboard.primaryClipDescription?.timestamp ?: 0
        copiedText = text
        HisarClip.record(context, HisarClip(timestamp, System.currentTimeMillis() + clearAfter.inWholeMilliseconds))
        context.getSystemService(AlarmManager::class.java)
            .setAndAllowWhileIdle(AlarmManager.ELAPSED_REALTIME_WAKEUP, SystemClock.elapsedRealtime() + clearAfter.inWholeMilliseconds, clearIntent(context))
    }

    private fun <Value> runOnMain(operation: () -> Value): Value {
        if (Looper.myLooper() == Looper.getMainLooper()) return operation()
        val task = FutureTask(operation)
        mainHandler.post(task)
        return task.get()
    }

    companion object {
        /** ClipDescription.EXTRA_IS_SENSITIVE from API 33; older versions keep the extra on the clip. */
        private const val EXTRA_IS_SENSITIVE = "android.content.extra.IS_SENSITIVE"

        internal fun clearHisarsCopy(context: Context) {
            val clip = HisarClip.read(context) ?: return
            val description = context.getSystemService(ClipboardManager::class.java).primaryClipDescription
            if (description == null || clip.timestamp == 0L || description.timestamp == clip.timestamp) clear(context) else HisarClip.forget(context)
        }

        private fun clear(context: Context) {
            context.getSystemService(ClipboardManager::class.java).clearPrimaryClip()
            context.getSystemService(AlarmManager::class.java).cancel(clearIntent(context))
            HisarClip.forget(context)
        }

        private fun clearIntent(context: Context): PendingIntent = PendingIntent.getBroadcast(
            context,
            0,
            Intent(context, ClipboardClearReceiver::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
    }
}

/** The alarm ClipboardAdapter sets for each copy; it may run in a new process after Hisar was closed. */
class ClipboardClearReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) = ClipboardAdapter.clearHisarsCopy(context)
}

/**
 * Hisar's last copy, kept across processes: the clipboard's timestamp of the clip, or 0 when it could not be read, and
 * when it is due to be cleared, in wall-clock milliseconds. The text itself is never written down.
 */
internal data class HisarClip(val timestamp: Long, val expiresAt: Long) {
    companion object {
        private const val PREFERENCES = "clipboard"

        fun read(context: Context): HisarClip? {
            val preferences = context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE)
            if (!preferences.contains("expiresAt")) return null
            return HisarClip(preferences.getLong("timestamp", 0), preferences.getLong("expiresAt", 0))
        }

        fun record(context: Context, clip: HisarClip) {
            context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE).edit()
                .putLong("timestamp", clip.timestamp).putLong("expiresAt", clip.expiresAt).commit()
        }

        fun forget(context: Context) {
            context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE).edit().clear().commit()
        }

        /** Whether the clipboard still holds Hisar's copy; false whenever Hisar may not read it. */
        fun isOnClipboard(context: Context): Boolean {
            val clip = read(context) ?: return false
            val description = context.getSystemService(ClipboardManager::class.java).primaryClipDescription ?: return false
            return clip.timestamp != 0L && description.timestamp == clip.timestamp
        }
    }
}
