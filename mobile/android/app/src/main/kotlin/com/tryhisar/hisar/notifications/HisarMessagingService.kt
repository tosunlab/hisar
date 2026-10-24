package com.tryhisar.hisar.notifications

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import com.tryhisar.hisar.BackendProvider
import com.tryhisar.hisar.app.NotificationsModel
import com.tryhisar.hisar.model.PushKind
import kotlin.concurrent.thread

/**
 * Receives Hisar's pushes from Firebase, even while the app is closed, and the tokens Firebase replaces. A push is a data
 * message naming its kind only; the notification shows the app's own sentence for it.
 */
class HisarMessagingService : FirebaseMessagingService() {
    override fun onMessageReceived(message: RemoteMessage) {
        val kind = PushKind.parse(message.data["kind"]) ?: return
        HisarNotifications.postPush(this, kind)
    }

    // HisarKit blocks on the network, so the new token is registered off the main thread.
    override fun onNewToken(token: String) {
        val backend = (application as BackendProvider).backend
        thread(name = "push-token") {
            try {
                backend.setPushToken(NotificationsModel.PLATFORM, token)
            } catch (_: Exception) {
                // The next launch registers the token again.
            }
        }
    }
}
