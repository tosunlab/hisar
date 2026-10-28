package com.tryhisar.hisar.notifications

import android.app.AlarmManager
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.SystemClock
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import android.app.Activity
import com.tryhisar.hisar.MainActivity
import com.tryhisar.hisar.R
import com.tryhisar.hisar.app.NotificationPlatform
import com.tryhisar.hisar.model.PushKind
import com.tryhisar.hisar.model.PushNotification
import java.time.Instant
import org.unifiedpush.android.connector.UnifiedPush

/**
 * Hisar's notifications on Android: one channel per kind, the notification a push or the reminder posts, and the
 * UnifiedPush distributor the user installed, such as ntfy, which delivers pushes. A notification shows Hisar's own sentence for its kind and nothing else, on the lock screen too.
 */
object HisarNotifications {
    /** The extra on MainActivity's intent naming the kind of the notification the user tapped. */
    const val EXTRA_KIND = "com.tryhisar.hisar.notification-kind"
    private const val REMINDER_CHANNEL = "reminders"
    private const val REMINDER_ID = 100
    private const val REMINDER_DELAY_MILLISECONDS = 5_000L

    internal const val EXTRA_WAITING_UNTIL = "com.tryhisar.hisar.reminder-waiting-until"

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

    private fun post(context: Context, channel: String, id: Int, kind: PushKind, text: String, detailText: String? = null) {
        val manager = NotificationManagerCompat.from(context)
        if (!manager.areNotificationsEnabled()) return
        val builder = NotificationCompat.Builder(context, channel)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(context.getString(R.string.app_name))
            .setContentText(detailText ?: text)
            .setContentIntent(createOpenIntent(context, kind))
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setCategory(if (kind == PushKind.APPROVAL) NotificationCompat.CATEGORY_REMINDER else NotificationCompat.CATEGORY_STATUS)
        if (detailText != null) {
            // A lock screen set to hide sensitive content shows the public version, the kind's own sentence.
            builder.setPublicVersion(
                NotificationCompat.Builder(context, channel)
                    .setSmallIcon(R.drawable.ic_notification)
                    .setContentTitle(context.getString(R.string.app_name))
                    .setContentText(text)
                    .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
                    .build(),
            )
        }
        try {
            manager.notify(id, builder.build())
        } catch (_: SecurityException) {
            // The user took the permission back between the check and the post.
        }
    }

    /** Posts the notification for a [notification]; one per kind, so a newer one replaces the last. */
    fun postPush(context: Context, notification: PushNotification) {
        val kind = notification.kind
        post(context, getChannel(kind), kind.ordinal + 1, kind, getText(context, kind), notification.detailText())
    }

    /**
     * Posts the notification for an approval push whose signed preview opened: its [title] and [body] name the Mac, the
     * app and the request in full, with the kind's own sentence on a lock screen set to hide content.
     */
    fun postApprovalPush(context: Context, title: String, body: String) {
        val manager = NotificationManagerCompat.from(context)
        if (!manager.areNotificationsEnabled()) return
        val channel = getChannel(PushKind.APPROVAL)
        val builder = NotificationCompat.Builder(context, channel)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(title)
            .setContentText(body)
            .setStyle(NotificationCompat.BigTextStyle().setBigContentTitle(title).bigText(body))
            .setContentIntent(createOpenIntent(context, PushKind.APPROVAL))
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setCategory(NotificationCompat.CATEGORY_REMINDER)
            .setPublicVersion(
                NotificationCompat.Builder(context, channel)
                    .setSmallIcon(R.drawable.ic_notification)
                    .setContentTitle(context.getString(R.string.app_name))
                    .setContentText(getText(context, PushKind.APPROVAL))
                    .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
                    .build(),
            )
        try {
            manager.notify(PushKind.APPROVAL.ordinal + 1, builder.build())
        } catch (_: SecurityException) {
            // The user took the permission back between the check and the post.
        }
    }

    fun isDistributorInstalled(context: Context): Boolean = UnifiedPush.getDistributors(context).any { it != context.packageName }

    /**
     * Registers with the distributor the user chose before, else their default one; with several and no default, Android
     * asks which. The distributor then gives HisarPushService the endpoint. [done] runs once the choice is made.
     */
    fun startPush(activity: Activity, done: () -> Unit) {
        if (!isDistributorInstalled(activity)) return
        UnifiedPush.tryUseCurrentOrDefaultDistributor(activity) { isChosen ->
            if (isChosen) UnifiedPush.register(activity, messageForDistributor = activity.getString(R.string.app_name))
            done()
        }
    }

    internal fun postApprovalReminder(context: Context) {
        post(context, REMINDER_CHANNEL, REMINDER_ID, PushKind.APPROVAL, context.getString(R.string.push_approval_waiting))
    }

    private fun createReminderIntent(context: Context, waitingUntil: Instant? = null): PendingIntent {
        val intent = Intent(context, ApprovalReminderReceiver::class.java)
        waitingUntil?.let { intent.putExtra(EXTRA_WAITING_UNTIL, it.toEpochMilli()) }
        return PendingIntent.getBroadcast(context, 0, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
    }

    /** Android's side of the app model's notifications. */
    class Platform(private val context: Context) : NotificationPlatform {
        override fun isPermissionGranted(): Boolean = NotificationManagerCompat.from(context).areNotificationsEnabled()

        override fun isDistributorInstalled(): Boolean = isDistributorInstalled(context)

        override fun postApprovalReminder(waitingUntil: Instant) {
            context.getSystemService(AlarmManager::class.java).setAndAllowWhileIdle(
                AlarmManager.ELAPSED_REALTIME_WAKEUP,
                SystemClock.elapsedRealtime() + REMINDER_DELAY_MILLISECONDS,
                createReminderIntent(context, waitingUntil),
            )
        }

        override fun cancelApprovalReminder() {
            context.getSystemService(AlarmManager::class.java).cancel(createReminderIntent(context))
            NotificationManagerCompat.from(context).cancel(REMINDER_ID)
        }
    }
}

/** The alarm of the reminder of an approval left waiting; it may run in a new process after Hisar was closed. */
class ApprovalReminderReceiver : BroadcastReceiver() {
    // Android may deliver the alarm long after it was due, when every approval it reminds of has expired.
    override fun onReceive(context: Context, intent: Intent) {
        if (System.currentTimeMillis() < intent.getLongExtra(HisarNotifications.EXTRA_WAITING_UNTIL, 0)) HisarNotifications.postApprovalReminder(context)
    }
}
