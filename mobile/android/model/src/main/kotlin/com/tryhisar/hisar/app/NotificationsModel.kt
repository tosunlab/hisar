package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.PushKind
import com.tryhisar.hisar.model.PushSettings
import com.tryhisar.hisar.model.VaultBackend
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * What only Android does for notifications: whether the user allowed them, whether a UnifiedPush distributor that
 * delivers them is installed, and the reminder of a waiting approval.
 */
interface NotificationPlatform {
    fun isPermissionGranted(): Boolean
    fun isDistributorInstalled(): Boolean

    /** Posts the reminder of an approval left waiting, after a moment; it says no more than a push would. */
    fun postApprovalReminder()

    /** Removes the reminder, still due or already shown, even one an earlier process left. */
    fun cancelApprovalReminder()

    /** A phone without notifications, as in tests that do not need them. */
    object None : NotificationPlatform {
        override fun isPermissionGranted(): Boolean = false
        override fun isDistributorInstalled(): Boolean = false
        override fun postApprovalReminder() = Unit
        override fun cancelApprovalReminder() = Unit
    }
}

/**
 * Settings → Notifications and what a notification does, the iPhone app's PushNotificationsModel ported: whether a
 * UnifiedPush distributor can deliver pushes, the kinds turned on, the screen a tapped notification opens, and the
 * reminder of an approval left waiting. The distributor's endpoint reaches HisarKit from HisarPushService, not from
 * here. A notification says only what kind of thing happened; Hisar shows the rest once unlocked.
 */
class NotificationsModel(private val context: ModelContext, private val platform: NotificationPlatform) {
    private val mutableSettings = MutableStateFlow(PushSettings.unavailable)
    private val mutableIsPermissionGranted = MutableStateFlow(platform.isPermissionGranted())
    private val mutableIsDistributorInstalled = MutableStateFlow(platform.isDistributorInstalled())
    private val mutableErrorMessage = MutableStateFlow<String?>(null)

    val settings: StateFlow<PushSettings> = mutableSettings.asStateFlow()
    val isPermissionGranted: StateFlow<Boolean> = mutableIsPermissionGranted.asStateFlow()

    /** False until the user installs a UnifiedPush distributor, such as ntfy: without one, no push reaches Hisar. */
    val isDistributorInstalled: StateFlow<Boolean> = mutableIsDistributorInstalled.asStateFlow()
    val errorMessage: StateFlow<String?> = mutableErrorMessage.asStateFlow()

    /** The kind of the notification the user tapped, until the app has shown its screen. */
    val openedKind = MutableStateFlow<PushKind?>(null)

    suspend fun refresh() {
        mutableIsPermissionGranted.value = platform.isPermissionGranted()
        mutableIsDistributorInstalled.value = platform.isDistributorInstalled()
        context.attempt { it.getPushSettings() }?.let { mutableSettings.value = it }
    }

    suspend fun setEnabled(kind: PushKind, isEnabled: Boolean) {
        mutableErrorMessage.value = runCatchingBackend { it.setPushKindEnabled(kind, isEnabled) }
        refresh()
    }

    /** Records the notification the user tapped; its kind is all a Hisar push carries. */
    fun open(kindText: String?) {
        PushKind.parse(kindText)?.let { openedKind.value = it }
    }

    /** Leaves the reminder when Hisar goes to the background with an approval waiting, and removes it otherwise. */
    fun remindOfWaitingApproval(isWaiting: Boolean) {
        platform.cancelApprovalReminder()
        if (isWaiting && settings.value.isEnabled(PushKind.APPROVAL) && platform.isPermissionGranted()) platform.postApprovalReminder()
    }

    fun clearApprovalReminder() {
        platform.cancelApprovalReminder()
    }

    /** Runs [operation] in the background and returns its error's sentence, or null. */
    private suspend fun runCatchingBackend(operation: (VaultBackend) -> Unit): String? {
        var failure: String? = null
        context.perform(isBusyShown = false) { backend ->
            try {
                operation(backend)
            } catch (error: Exception) {
                failure = error.message
            }
        }
        return failure
    }
}
