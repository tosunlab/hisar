package com.tryhisar.hisar.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging
import com.tryhisar.hisar.BuildConfig
import com.tryhisar.hisar.MainActivity
import com.tryhisar.hisar.R
import com.tryhisar.hisar.app.NotificationPlatform
import com.tryhisar.hisar.model.PushKind

/**
 * Hisar's notifications on Android: one channel per kind, the notification a push or the reminder posts, and Firebase,
 * which delivers pushes. A notification shows Hisar's own sentence for its kind and nothing else, on the lock screen too.
 */
object HisarNotifications {
    /** The extra on MainActivity's intent naming the kind of the notification the user tapped. */
    const val EXTRA_KIND = "com.tryhisar.hisar.notification-kind"
    private const val REMINDER_CHANNEL = "reminders"
    private const val REMINDER_ID = 100
    private const val REMINDER_DELAY_MILLISECONDS = 5_000L

    private val reminderHandler = Handler(Looper.getMainLooper())
    private var pendingReminder: Runnable? = null

    fun getChannel(kind: PushKind): String = kind.value

    fun createChannels(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannels(
            listOf(
                NotificationChannel(getChannel(PushKind.APPROVAL), context.getString(R.string.notification_channel_approvals), NotificationManager.IMPORTANCE_HIGH),
                NotificationChannel(getChannel(PushKind.SECURITY), context.getString(R.string.notification_channel_security), NotificationManager.IMPORTANCE_HIGH),
                NotificationChannel(getChannel(PushKind.SHARING), context.getString(R.string.notification_channel_sharing), NotificationManager.IMPORTANCE_DEFAULT),
                NotificationChannel(REMINDER_CHANNEL, context.getString(R.string.notification_channel_reminders), NotificationManager.IMPORTANCE_LOW),
            ),
        )
    }

    private fun getText(context: Context, kind: PushKind): String = context.getString(
        when (kind) {
            PushKind.APPROVAL -> R.string.push_approval
            PushKind.SECURITY -> R.string.push_security
            PushKind.SHARING -> R.string.push_sharing
        },
    )

    private fun createOpenIntent(context: Context, kind: PushKind): PendingIntent {
        val intent = Intent(context, MainActivity::class.java).putExtra(EXTRA_KIND, kind.value).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP)
        return PendingIntent.getActivity(context, kind.ordinal, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
    }

    private fun post(context: Context, channel: String, id: Int, kind: PushKind, text: String) {
        val manager = NotificationManagerCompat.from(context)
        if (!manager.areNotificationsEnabled()) return
        val notification = NotificationCompat.Builder(context, channel)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(context.getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(createOpenIntent(context, kind))
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setCategory(if (kind == PushKind.APPROVAL) NotificationCompat.CATEGORY_REMINDER else NotificationCompat.CATEGORY_STATUS)
            .build()
        try {
            manager.notify(id, notification)
        } catch (_: SecurityException) {
            // The user took the permission back between the check and the post.
        }
    }

    /** Posts the notification for a push of [kind]; one per kind, so a newer one replaces the last. */
    fun postPush(context: Context, kind: PushKind) {
        post(context, getChannel(kind), kind.ordinal + 1, kind, getText(context, kind))
    }

    /** Sets Firebase up from the build's properties; false for a build without them, which has no push notifications. */
    fun startFirebase(context: Context): Boolean {
        if (BuildConfig.FIREBASE_APPLICATION_ID.isEmpty()) return false
        if (FirebaseApp.getApps(context).isEmpty()) {
            val options = FirebaseOptions.Builder()
                .setApplicationId(BuildConfig.FIREBASE_APPLICATION_ID)
                .setProjectId(BuildConfig.FIREBASE_PROJECT_ID)
                .setApiKey(BuildConfig.FIREBASE_API_KEY)
                .setGcmSenderId(BuildConfig.FIREBASE_SENDER_ID)
                .build()
            FirebaseApp.initializeApp(context, options)
        }
        return true
    }

    /** Asks Firebase for this launch's token; [receive] gets it on the main thread. */
    fun fetchToken(context: Context, receive: (String) -> Unit) {
        if (!startFirebase(context)) return
        FirebaseMessaging.getInstance().token.addOnSuccessListener { token -> if (!token.isNullOrEmpty()) receive(token) }
    }

    /** Android's side of the app model's notifications. */
    class Platform(private val context: Context) : NotificationPlatform {
        override fun isPermissionGranted(): Boolean = NotificationManagerCompat.from(context).areNotificationsEnabled()

        override fun postApprovalReminder() {
            cancelApprovalReminder()
            val reminder = Runnable { post(context, REMINDER_CHANNEL, REMINDER_ID, PushKind.APPROVAL, context.getString(R.string.push_approval_waiting)) }
            pendingReminder = reminder
            reminderHandler.postDelayed(reminder, REMINDER_DELAY_MILLISECONDS)
        }

        override fun cancelApprovalReminder() {
            pendingReminder?.let(reminderHandler::removeCallbacks)
            pendingReminder = null
            NotificationManagerCompat.from(context).cancel(REMINDER_ID)
        }
    }
}
