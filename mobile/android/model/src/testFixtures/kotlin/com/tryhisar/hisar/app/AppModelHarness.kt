package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import java.time.Instant
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope

/**
 * An app model on a [FakeBackend] for a test in `runTest`: its main work and its backend calls run on the test's
 * scheduler, so `runCurrent()` finishes what the model started, and its clock reads [currentDate], which the test moves.
 * It takes http:// servers, as a debug build does, unless [isPlainHttpAllowed] says otherwise.
 */
class AppModelHarness(private val testScope: TestScope, val backend: FakeBackend = FakeBackend(), private val isPlainHttpAllowed: Boolean = true) {
    var currentDate: Instant = Instant.ofEpochSecond(1_800_000_000)
    val events = MutableSharedFlow<BackendEvent>()
    val clipboard = FakeTextClipboard()

    val notificationPlatform = FakeNotificationPlatform()

    fun createModel(ioDispatcher: CoroutineDispatcher = StandardTestDispatcher(testScope.testScheduler)): AppModel = AppModel(
        testScope.backgroundScope, ioDispatcher, clipboard, events, now = { currentDate }, isPlainHttpAllowed = isPlainHttpAllowed,
        notificationPlatform = notificationPlatform,
    )

    suspend fun startModel(): AppModel = createModel().also { model -> model.start { backend } }

    suspend fun startUnlockedModel(): AppModel {
        backend.seedSignedIn()
        return startModel().also { it.refreshAll() }
    }
}

/**
 * A phone's notifications for tests: the permission and the UnifiedPush distributor are switches, and the reminder is
 * a count of posts.
 */
class FakeNotificationPlatform : NotificationPlatform {
    var isGranted = false
    var isDistributorInstalledValue = true
    /** True while a reminder is due or shown; a test sets it for one an earlier process left. */
    var isReminderPosted = false
    var postCount = 0
        private set

    override fun isPermissionGranted(): Boolean = isGranted

    override fun isDistributorInstalled(): Boolean = isDistributorInstalledValue

    override fun postApprovalReminder() {
        postCount += 1
        isReminderPosted = true
    }

    override fun cancelApprovalReminder() {
        isReminderPosted = false
    }
}

/** A clipboard that keeps what the app copied, secret and plain apart. */
class FakeTextClipboard : TextClipboard {
    val allCopiedSecret = mutableListOf<String>()
    val allCopiedPlain = mutableListOf<String>()

    override fun copySecret(text: String): Boolean = allCopiedSecret.add(text)

    override fun copyPlain(text: String): Boolean = allCopiedPlain.add(text)
}
