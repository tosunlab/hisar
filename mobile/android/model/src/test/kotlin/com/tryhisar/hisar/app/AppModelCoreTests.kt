package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.ReceivedShareInvitation
import com.tryhisar.hisar.model.UpdateRequirement
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.backendJson
import java.util.concurrent.Executors
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** The app model's core: starting, the screen, refreshing, the lifecycle, events and notices. */
class AppModelCoreTests {
    @Test
    fun testStartsSignedOut() = runTest {
        val model = AppModelHarness(this).startModel()
        assertEquals(Screen.SignedOut, model.screen.value)
    }

    @Test
    fun testStartFailureShowsMessage() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.createModel()
        model.start { throw BackendError("Disk full") }
        assertEquals(Screen.Failed("Disk full"), model.screen.value)
        model.start { harness.backend }
        assertEquals(Screen.SignedOut, model.screen.value)
        var createdAgain = false
        model.start { createdAgain = true; harness.backend }
        assertFalse(createdAgain)
    }

    @Test
    fun testSignInLoadsVaultsAndItems() = runTest {
        val model = AppModelHarness(this).startModel()
        val isSignedIn = model.signIn.signIn(" http://127.0.0.1:18080 ", "alice@example.com", "h1-abc123-aaaaa-bbbbb-ccccc-ddddd-eeeeee", FakeBackend.PASSWORD)
        assertTrue(isSignedIn)
        assertEquals(Screen.Unlocked, model.screen.value)
        assertEquals("http://127.0.0.1:18080", model.status.value.server)
        val data = model.unlockedData.value
        assertEquals(listOf("Personal", "Family"), data.allVault.map { it.name })
        assertEquals(listOf("Bank", "GitHub", "Netflix"), data.listAllItemReference().map { it.item.title })
        assertEquals(2, data.countItem("personal"))
        assertNull(model.errorMessage.value)
        assertFalse(model.isBusy.value)
    }

    @Test
    fun testLockedEventFromBackendShowsLockScreen() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val lockCountBefore = model.lockCount.value
        harness.backend.lock()
        harness.events.emit(BackendEvent.Locked)
        runCurrent()
        assertEquals(Screen.Locked, model.screen.value)
        assertEquals(lockCountBefore + 1, model.lockCount.value)
    }

    @Test
    fun testBackgroundLongerThanAutoLockLocks() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.handleDidEnterBackground()
        harness.currentDate += model.autoLockInterval.plusSeconds(1)
        model.handleWillEnterForeground()
        assertEquals(Screen.Locked, model.screen.value)
    }

    @Test
    fun testShortBackgroundStaysUnlocked() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.handleDidEnterBackground()
        harness.currentDate = harness.currentDate.plusSeconds(30)
        model.handleWillEnterForeground()
        assertEquals(Screen.Unlocked, model.screen.value)
    }

    @Test
    fun testReturningToTheForegroundReconnectsAndLists() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.handleDidEnterBackground()
        harness.backend.allVault += VaultSummary(id = "new", name = "Added on the Mac", type = VaultSummary.TYPE_SHARED, role = Member.VAULT_ROLE_OWNER)
        harness.backend.allItemByVaultID["new"] = mutableListOf()
        harness.backend.allPendingApproval = mutableListOf(
            PhoneApproval(requestID = "posted-while-away", requesterDeviceName = "MacBook Pro", expiresAt = harness.currentDate.plusSeconds(60).toString(), kind = PhoneApproval.KIND_SSH),
        )
        model.approvals.refreshPendingApproval()
        assertTrue(model.approvals.allPendingApproval.value.isEmpty(), "nothing is listed in the background")
        harness.events.emit(BackendEvent.Approval(PhoneApproval.EVENT_REQUESTED, "posted-while-away"))
        model.handleWillEnterForeground()
        runCurrent()
        assertTrue("reconnectEvents" in harness.backend.allCallName, "the event stream is reconnected")
        assertNotNull(model.unlockedData.value.findVault("new"), "the vaults are listed again")
        assertEquals(listOf("posted-while-away"), model.approvals.allPendingApproval.value.map { it.requestID })
        assertNull(model.approvals.presentedApproval.value, "a request found on returning is listed, not pushed into a sheet")
    }

    @Test
    fun testRemoteChangeReloadsVaultsAndInvitations() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        runCurrent()
        backend.allVault += VaultSummary(id = "shared-with-me", name = "Deploy keys", type = "shared", role = Member.VAULT_ROLE_READER, itemCount = 0)
        backend.shareInvitations = backend.shareInvitations.copy(
            allReceived = listOf(
                ReceivedShareInvitation(
                    id = "from-carol", vaultID = "carol-vault", role = Member.VAULT_ROLE_EDITOR, invitedByName = "Carol",
                    invitedByEmail = "carol@example.com", expiresAt = "2027-01-22T08:00:00Z",
                ),
            ),
        )
        val remoteChangeCount = model.remoteChangeCount.value
        harness.events.emit(BackendEvent.RemoteChange)
        runCurrent()
        assertEquals(remoteChangeCount + 1, model.remoteChangeCount.value, "an open sharing sheet reloads on this count")
        assertNotNull(model.unlockedData.value.findVault("shared-with-me"))
        assertEquals(listOf("from-carol"), model.unlockedData.value.allReceivedInvitation.map { it.id })

        model.lock()
        harness.events.emit(BackendEvent.RemoteChange)
        runCurrent()
        assertEquals(remoteChangeCount + 1, model.remoteChangeCount.value, "a locked app ignores remote changes")
    }

    // A change made while a refresh is still listing: the refresh has listed everything and finishes after the change.
    private suspend fun TestScope.finishRefresh(harness: AppModelHarness, model: AppModel, change: suspend () -> Unit) {
        harness.backend.onListAllPasskey = {
            harness.backend.onListAllPasskey = null
            backgroundScope.launch { change() }
        }
        model.refreshAll()
        runCurrent()
    }

    @Test
    fun testARemoteChangeDuringAnotherRefreshListsAgain() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        runCurrent()
        finishRefresh(harness, model) {
            harness.backend.allVault += VaultSummary(id = "shared-with-me", name = "Deploy keys", type = "shared", role = Member.VAULT_ROLE_READER, itemCount = 0)
            model.handleRemoteChange()
        }
        assertNotNull(model.unlockedData.value.findVault("shared-with-me"), "the refresh listed the vaults before the change and nothing listed them again")
    }

    @Test
    fun testRefreshFinishingAfterSignOutBringsNothingBack() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        runCurrent()
        finishRefresh(harness, model) {
            harness.backend.status = VaultStatus.signedOut
            harness.events.emit(BackendEvent.Locked)
        }
        assertEquals(Screen.SignedOut, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty(), "the refresh restored the signed-out account's vaults")
        assertTrue(model.unlockedData.value.allItemByVaultID.isEmpty(), "the refresh restored the signed-out account's items")
    }

    @Test
    fun testRefreshFinishingAfterLockingBringsNothingBack() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        runCurrent()
        finishRefresh(harness, model) { model.lock() }
        assertEquals(Screen.Locked, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty(), "the refresh restored the vaults behind the lock screen")
        assertTrue(model.unlockedData.value.allItemByVaultID.isEmpty(), "the refresh restored the items behind the lock screen")
    }

    // The backend runs inline, so a refresh started from its last listing finishes before the first one does.
    @Test
    fun testRefreshFinishingAfterANewerRefreshKeepsTheNewerLists() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn()
        val model = harness.createModel(ioDispatcher = Dispatchers.Unconfined).also { it.start { harness.backend } }
        runCurrent()
        harness.backend.onListAllPasskey = {
            harness.backend.onListAllPasskey = null
            harness.backend.allFavoriteRecordID = mutableSetOf("family/netflix")
            runBlocking { model.refreshAll() }
        }
        model.refreshAll()
        runCurrent()
        assertEquals(listOf("Netflix"), model.unlockedData.value.listAllFavoriteReference().map { it.item.title }, "the older refresh brought back the older lists")
    }

    @Test
    fun testOfflineStatusIsPublished() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isOffline = true
        val model = harness.startUnlockedModel()
        assertTrue(model.status.value.isOffline)
    }

    @Test
    fun testCopyFieldShowsNotice() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.copyField("personal", "github", "password", "Password")
        assertEquals(listOf("github.password"), harness.backend.allCopiedField)
        assertEquals("Password copied. It clears in about 2 minutes.", model.notice.value?.message)
    }

    @Test
    fun testCopyingCountsForHapticsAndCopiesReferences() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.copyField("personal", "github", "password", "Password")
        assertEquals(1, model.copyCount.value)
        model.copySecretReference("personal", "github", "password")
        assertEquals(2, model.copyCount.value)
        assertEquals(listOf("hisar://Personal/GitHub/password"), harness.backend.allCopiedReference)
        assertEquals("Secret reference copied.", model.notice.value?.message)
    }

    @Test
    fun testCopyTextCopiesASecretAndPlainTextStaysPlain() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.copyText("https://github.com", "Website")
        assertEquals(listOf("https://github.com"), harness.clipboard.allCopiedSecret)
        assertEquals("Website copied. It clears in about 2 minutes.", model.notice.value?.message)
        model.copyPlainText("support@tryhisar.com", "Support address")
        assertEquals(listOf("support@tryhisar.com"), harness.clipboard.allCopiedPlain)
        assertEquals(listOf("https://github.com"), harness.clipboard.allCopiedSecret)
        assertEquals("Support address copied", model.notice.value?.message)
        assertEquals(2, model.copyCount.value)
    }

    @Test
    fun testOperationsRunOffTheMainThread() = runTest {
        val listingThreads = mutableListOf<Thread>()
        val recordingBackend = object : FakeBackend() {
            override fun listAllVault() = super.listAllVault().also { listingThreads += Thread.currentThread() }
        }
        recordingBackend.seedSignedIn()
        Executors.newSingleThreadExecutor().asCoroutineDispatcher().use { backgroundDispatcher ->
            val model = AppModelHarness(this, recordingBackend).createModel(backgroundDispatcher)
            model.start { recordingBackend }
            model.refreshAll()
        }
        assertTrue(listingThreads.isNotEmpty())
        listingThreads.forEach { assertNotEquals(Thread.currentThread(), it, "the backend was called on the calling thread") }
    }

    @Test
    fun testUpdateRequiredBlocksTheAppUntilTheServerAcceptsIt() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allPendingApproval = mutableListOf(
            PhoneApproval(requestID = "waiting", requesterDeviceName = "MacBook Pro", expiresAt = harness.currentDate.plusSeconds(60).toString(), kind = PhoneApproval.KIND_SSH),
        )
        val model = harness.startUnlockedModel()
        runCurrent()
        assertEquals(1, model.approvals.allPendingApproval.value.size)
        val requirement = UpdateRequirement(currentVersion = "0.1.0", minimumVersion = "0.2.0", updateURL = "https://tryhisar.com/download")
        harness.backend.serverUpdateRequirement = requirement
        model.checkMinimumVersion()
        assertEquals(Screen.UpdateRequired(requirement), model.screen.value)
        assertTrue(model.approvals.allPendingApproval.value.isEmpty())

        harness.backend.serverUpdateRequirement = null
        model.checkMinimumVersion()
        assertEquals(Screen.Unlocked, model.screen.value)
        assertFalse(model.isCheckingMinimumVersion.value)
    }

    @Test
    fun testUpdateRequiredEventShowsTheBlockingScreen() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = harness.startModel()
        runCurrent()
        assertEquals(Screen.Locked, model.screen.value)
        val requirement = UpdateRequirement(currentVersion = "", minimumVersion = "0.2.0", updateURL = "https://tryhisar.com/download")
        harness.backend.status = harness.backend.status.copy(updateRequired = requirement)
        harness.events.emit(BackendEvent.UpdateRequired)
        runCurrent()
        assertEquals(Screen.UpdateRequired(requirement), model.screen.value)
    }

    @Test
    fun testStatusDecodesUpdateRequirement() {
        val json = """{"state":"locked","updateRequired":{"currentVersion":"","minimumVersion":"0.2.0","updateURL":"https://tryhisar.com/download"}}"""
        val status = backendJson.decodeFromString<VaultStatus>(json)
        assertEquals(UpdateRequirement(currentVersion = "", minimumVersion = "0.2.0", updateURL = "https://tryhisar.com/download"), status.updateRequired)
        assertEquals("a development build", status.updateRequired?.currentVersionDescription)
        assertNull(backendJson.decodeFromString<VaultStatus>("""{"state":"locked","updateRequired":null}""").updateRequired)
    }

    @Test
    fun testLockReturnsToTheVaultsTab() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.selectedTab.value = Tab.SETTINGS
        model.lock()
        assertEquals(Tab.VAULTS, model.selectedTab.value)
    }
}
