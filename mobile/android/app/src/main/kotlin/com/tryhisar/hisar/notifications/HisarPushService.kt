package com.tryhisar.hisar.notifications

import com.tryhisar.hisar.BackendProvider
import com.tryhisar.hisar.HisarApplication
import com.tryhisar.hisar.model.PushKind
import com.tryhisar.hisar.model.VaultBackend
import kotlin.concurrent.thread
import org.unifiedpush.android.connector.FailedReason
import org.unifiedpush.android.connector.PushService
import org.unifiedpush.android.connector.data.PushEndpoint
import org.unifiedpush.android.connector.data.PushMessage

/**
 * Receives Hisar's pushes from the UnifiedPush distributor, even while the app is closed, and the endpoints it gives.
 * A push is a Web Push message encrypted to this phone and naming its kind only; the notification shows the app's own
 * sentence for it.
 */
class HisarPushService : PushService() {
    override fun onMessage(message: PushMessage, instance: String) {
        // A message the connector could not decrypt was not sent by a Hisar server, which always encrypts.
        if (!message.decrypted) return
        val notification = PushKind.parseNotification(message.content.decodeToString()) ?: return
        if (notification.kind == PushKind.APPROVAL && (application as HisarApplication).foregroundActivity.isMainActivityStarted) return
        HisarNotifications.postPush(this, notification)
    }

    override fun onNewEndpoint(endpoint: PushEndpoint, instance: String) {
        val keys = endpoint.pubKeySet ?: return
        runOnBackend { it.setWebPushEndpoint(endpoint.url, keys.pubKey, keys.auth) }
    }

    override fun onUnregistered(instance: String) {
        runOnBackend { it.removeWebPushEndpoint() }
    }

    // Settings → Notifications shows that no push has arrived; the next launch registers again.
    override fun onRegistrationFailed(reason: FailedReason, instance: String) = Unit

    // HisarKit blocks on the network, so it is called off the main thread; a failure is retried at the next launch.
    private fun runOnBackend(operation: (VaultBackend) -> Unit) {
        val backend = (application as BackendProvider).backend
        thread(name = "push-endpoint") {
            try {
                operation(backend)
            } catch (_: Exception) {
                // The next launch registers again.
            }
        }
    }
}
