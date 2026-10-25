package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.PushKind
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Notifications: the UnifiedPush distributor, the kinds turned on, what a tap opens, and the waiting-approval reminder. */
class PushNotificationTests {
    private val currentDate: Instant = Instant.ofEpochSecond(1_800_000_000)

    private suspend fun TestScope.createUnlockedModel(harness: AppModelHarness, allApproval: List<PhoneApproval> = emptyList()): AppModel {
        harness.currentDate = currentDate
        harness.backend.seedSignedIn()
        harness.backend.allPendingApproval = allApproval.toMutableList()
        val model = harness.startModel()
        model.approvals.refreshPendingApproval()
        runCurrent()
        return model
    }

    @Test
    fun testTurningNotificationsOnNeedsAUnifiedPushDistributor() = runTest {
        val harness = AppModelHarness(this)
        harness.notificationPlatform.isGranted = true
        harness.notificationPlatform.isDistributorInstalledValue = false
        val model = createUnlockedModel(harness)
        assertFalse(model.notifications.isDistributorInstalled.value, "without a distributor, Settings asks for one")
        harness.notificationPlatform.isDistributorInstalledValue = true
        model.notifications.refresh()
        assertTrue(model.notifications.isDistributorInstalled.value, "installing ntfy is seen once the user comes back")
        assertFalse(model.notifications.settings.value.isAvailable, "pushes are available only once the distributor gave an endpoint")
        harness.backend.setWebPushEndpoint("https://ntfy.example/upAbC", "public-key", "secret")
        model.notifications.refresh()
        assertTrue(model.notifications.settings.value.isAvailable)
        assertTrue(model.notifications.isPermissionGranted.value)
    }

    @Test
    fun testOnlyAHisarWebPushMessageNamesAKind() {
        assertEquals(PushKind.APPROVAL, PushKind.parseMessage("""{"kind":"approval"}"""))
        assertNull(PushKind.parseMessage("""{"kind":"marketing"}"""))
        assertNull(PushKind.parseMessage("not json"))
        assertNull(PushKind.parseMessage("""{"title":"Hello"}"""))
    }

    @Test
    fun testTurningAKindOffReachesTheBackend() = runTest {
        val harness = AppModelHarness(this)
        val model = createUnlockedModel(harness)
        model.notifications.setEnabled(PushKind.SHARING, isEnabled = false)
        assertFalse(model.notifications.settings.value.isEnabled(PushKind.SHARING))
        assertTrue(model.notifications.settings.value.isEnabled(PushKind.APPROVAL))
        assertEquals(listOf("approval", "security"), harness.backend.pushSettingsValue.enabledKinds.sorted())
    }

    @Test
    fun testATappedApprovalNotificationOpensTheApprovalsTab() = runTest {
        val harness = AppModelHarness(this)
        val model = createUnlockedModel(harness)
        model.selectedTab.value = Tab.VAULTS
        model.notifications.open("approval")
        assertEquals(PushKind.APPROVAL, model.notifications.openedKind.value)
        model.showNotification(PushKind.APPROVAL)
        assertEquals(Tab.APPROVALS, model.selectedTab.value)
        assertNull(model.notifications.openedKind.value)
        model.notifications.open("security")
        model.showNotification(PushKind.SECURITY)
        assertEquals(Tab.SETTINGS, model.selectedTab.value)
        model.notifications.openedKind.value = null
        model.notifications.open("marketing")
        assertNull(model.notifications.openedKind.value, "an unknown kind opens nothing")
    }

    @Test
    fun testLeavingWithAnApprovalWaitingLeavesAReminderThatSaysNoMore() = runTest {
        val harness = AppModelHarness(this)
        harness.notificationPlatform.isGranted = true
        val approval = PhoneApproval(
            requestID = "0123456789abcdef0123456789abcdef", requesterDeviceName = "MacBook Pro", expiresAt = currentDate.plusSeconds(60).toString(),
            kind = PhoneApproval.KIND_SSH, appName = "Terminal", purpose = "push to git.tryhisar.com", host = "git.tryhisar.com:22", commandLine = "git push",
            repositoryPath = "~/src/core", keyName = "work", isForwarded = false, forwardedFromHost = "",
        )
        val model = createUnlockedModel(harness, listOf(approval))
        assertEquals(1, model.approvals.allPendingApproval.value.size)
        model.handleDidEnterBackground()
        assertTrue(harness.notificationPlatform.isReminderPosted)
        model.handleWillEnterForeground()
        assertFalse(harness.notificationPlatform.isReminderPosted)
    }

    @Test
    fun testTheReminderLastsUntilTheLastWaitingApprovalExpires() = runTest {
        val harness = AppModelHarness(this)
        harness.notificationPlatform.isGranted = true
        val allApproval = listOf(150L, 60L).mapIndexed { index, secondCount ->
            PhoneApproval(requestID = "waiting-$index", requesterDeviceName = "MacBook Pro", expiresAt = currentDate.plusSeconds(secondCount).toString(), kind = PhoneApproval.KIND_SSH)
        }
        val model = createUnlockedModel(harness, allApproval)
        model.handleDidEnterBackground()
        assertEquals(currentDate.plusSeconds(150), harness.notificationPlatform.reminderWaitingUntil, "a reminder delivered after every request expired would point at nothing")
    }

    @Test
    fun testALaunchRemovesTheReminderAnEarlierProcessLeft() = runTest {
        val harness = AppModelHarness(this)
        harness.notificationPlatform.isGranted = true
        harness.notificationPlatform.isReminderPosted = true
        createUnlockedModel(harness)
        assertFalse(harness.notificationPlatform.isReminderPosted, "the reopened app shows the request itself")
    }

    @Test
    fun testNoReminderWithoutAWaitingApprovalOrPermission() = runTest {
        val harness = AppModelHarness(this)
        harness.notificationPlatform.isGranted = true
        val model = createUnlockedModel(harness)
        model.handleDidEnterBackground()
        assertEquals(0, harness.notificationPlatform.postCount)
        harness.notificationPlatform.isGranted = false
        model.notifications.remindOfWaitingApproval(waitingUntil = currentDate.plusSeconds(60))
        assertEquals(0, harness.notificationPlatform.postCount)
    }
}
