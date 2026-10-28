package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ShareAwaitingUnlock
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withTimeout

/** Locking and unlocking, signing out from the lock screen, and the lock's settings. */
class LockModelTests {
    @Test
    fun testLockAndUnlockWithPassword() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.lock()
        assertEquals(Screen.Locked, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty(), "vault data is dropped on lock")
        val isWrongUnlocked = model.lock.unlock("nope")
        assertFalse(isWrongUnlocked)
        assertEquals(Screen.Locked, model.screen.value)
        assertNotNull(model.errorMessage.value)
        val isUnlocked = model.lock.unlock(FakeBackend.PASSWORD)
        assertTrue(isUnlocked)
        assertEquals(Screen.Unlocked, model.screen.value)
        assertNull(model.errorMessage.value)
        assertEquals(2, model.unlockedData.value.allVault.size)
    }

    @Test
    fun testUnlockFinishesWhenTheLockScreenLeaves() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.lock()
        val lockScreenScope = CoroutineScope(coroutineContext + Job())
        backgroundScope.launch {
            model.screen.first { it == Screen.Unlocked }
            lockScreenScope.cancel()
        }
        lockScreenScope.launch { model.lock.unlock(FakeBackend.PASSWORD) }
        val unlockedData = withTimeout(5_000) { model.unlockedData.first { it.allVault.isNotEmpty() } }
        assertEquals(2, unlockedData.allVault.size, "the refresh after unlocking still lands")
        assertEquals(Screen.Unlocked, model.screen.value)
        assertFalse(lockScreenScope.isActive, "the lock screen left while unlocking")
        assertNull(model.errorMessage.value, "leaving the lock screen is not an error")
    }

    @Test
    fun testBiometricUnlockNeedsReadyStatus() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.lock()
        val isUnlockedWithoutEnabling = model.lock.unlockWithBiometric()
        assertFalse(isUnlockedWithoutEnabling)
        assertFalse("unlockWithBiometric" in harness.backend.allCallName)
    }

    @Test
    fun testEnableBiometricRequiresMasterPasswordThenUnlocks() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        val isEnabledWithWrongPassword = model.lock.setBiometricUnlock(isEnabled = true, password = "wrong")
        assertFalse(isEnabledWithWrongPassword)
        assertFalse(model.status.value.biometricEnabled)
        val isEnabled = model.lock.setBiometricUnlock(isEnabled = true, password = FakeBackend.PASSWORD)
        assertTrue(isEnabled)
        assertTrue(model.status.value.biometricEnabled)
        model.lock()
        val isUnlocked = model.lock.unlockWithBiometric()
        assertTrue(isUnlocked)
        assertEquals(Screen.Unlocked, model.screen.value)
        model.lock.setBiometricUnlock(isEnabled = false, password = "")
        assertFalse(model.status.value.biometricEnabled)
    }

    @Test
    fun testFailedBiometricStaysLocked() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.lock.setBiometricUnlock(isEnabled = true, password = FakeBackend.PASSWORD)
        model.lock()
        harness.backend.isBiometricSucceeding = false
        val isUnlocked = model.lock.unlockWithBiometric()
        assertFalse(isUnlocked)
        assertEquals(Screen.Locked, model.screen.value)
        assertNull(model.errorMessage.value)
        harness.backend.isBiometricSucceeding = true
        harness.backend.biometricUnlockError = com.tryhisar.hisar.model.BackendError("Enter your master password.")
        assertFalse(model.lock.unlockWithBiometric())
        assertNull(model.errorMessage.value)
        harness.backend.biometricUnlockError = com.tryhisar.hisar.model.BackendError("Biometrics are locked. Unlock with your master password.")
        assertFalse(model.lock.unlockWithBiometric())
        assertEquals(harness.backend.biometricUnlockError?.message, model.errorMessage.value)
    }

    @Test
    fun testLockScreenAsksTheOwnerToUnlockToFinishAcceptedShares() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        backend.allShareAwaitingUnlock = listOf(
            ShareAwaitingUnlock(invitationID = "to-ada", recipientName = "Ada", recipientEmail = "ada@example.com", acceptedAt = "2027-01-15T06:00:00Z"),
            ShareAwaitingUnlock(invitationID = "to-bo", recipientName = "", recipientEmail = "bo@example.com", acceptedAt = "2027-01-15T07:00:00Z"),
        )
        val model = harness.startModel()
        assertEquals(Screen.Locked, model.screen.value)
        model.lock.refreshShareAwaitingUnlock()
        assertEquals(
            listOf(
                "Ada accepted your invitation to a vault. Unlock to continue sharing.",
                "bo@example.com accepted your invitation to a vault. Unlock to continue sharing.",
            ),
            model.lock.allShareAwaitingUnlock.value.map { it.message },
        )
        assertNull(model.errorMessage.value)

        backend.isShareAwaitingUnlockFailing = true
        model.lock.refreshShareAwaitingUnlock()
        assertEquals(2, model.lock.allShareAwaitingUnlock.value.size, "a failed refresh keeps the last banner")
        assertNull(model.errorMessage.value, "the lock screen stays quiet offline")

        val isUnlocked = model.lock.unlock(FakeBackend.PASSWORD)
        assertTrue(isUnlocked)
        assertTrue(model.lock.allShareAwaitingUnlock.value.isEmpty(), "the unlocked reminder replaces the locked notice")
        val callCount = backend.allCallName.count { it == "listAllShareAwaitingUnlock" }
        model.lock.refreshShareAwaitingUnlock()
        assertEquals(callCount, backend.allCallName.count { it == "listAllShareAwaitingUnlock" }, "an unlocked app does not ask")
    }

    @Test
    fun testShareAwaitingUnlockDecodesFromHisarKit() {
        val json = """[{"invitationId":"i","recipientName":"Ada","recipientEmail":"ada@example.com","acceptedAt":"2027-01-15T06:00:00Z"}]"""
        val decoded = backendJson.decodeFromString<List<ShareAwaitingUnlock>>(json)
        assertEquals("i", decoded.first().id)
        assertEquals("Ada", decoded.first().recipientDisplayName)
        val invitations = backendJson.decodeFromString<ShareInvitations>(
            """{"received":null,"sent":null,"waiting":[{"id":"w","vaultId":"v","role":"reader","invitedByName":"","invitedByEmail":"carol@example.com","acceptedAt":"2027-01-15T06:00:00Z"}],"completionError":""}""",
        )
        assertEquals("carol@example.com", invitations.allWaiting.first().inviterName)
    }

    @Test
    fun testSigningOutKeepsTheServerForSigningBackIn() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        val server = model.status.value.server
        assertTrue(server.isNotEmpty())
        val isSignedOut = model.lock.signOut()
        assertTrue(isSignedOut)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertEquals(server, model.status.value.server, "the sign-in form opens on the server this phone used")
        assertEquals("", model.status.value.email)
    }

    @Test
    fun testSignOutErrorStillShowsTheSignedOutScreen() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        harness.backend.isOffline = true
        model.lock.signOut()
        assertEquals(Screen.SignedOut, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty())
        assertEquals(FakeBackend.SIGN_OUT_OFFLINE_MESSAGE, model.errorMessage.value)
    }

    @Test
    fun testAutoLockSettingChangesTheBackgroundLock() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertEquals(300, model.autoLockInterval.seconds, "5 minutes until the user picks another delay")
        val isThirtySecondsSet = model.lock.setAutoLock(30)
        assertTrue(isThirtySecondsSet)
        assertEquals(30, model.status.value.autoLockSeconds)
        model.handleDidEnterBackground()
        harness.currentDate = harness.currentDate.plusSeconds(31)
        model.handleWillEnterForeground()
        assertEquals(Screen.Locked, model.screen.value)
    }

    @Test
    fun testAutoLockRefusesOtherDelays() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        val isOddDelaySet = model.lock.setAutoLock(45)
        assertFalse(isOddDelaySet)
        assertNotNull(model.errorMessage.value)
        assertEquals(300, model.status.value.autoLockSeconds)
    }

    @Test
    fun testAutoLockStatusDecodesAndDescribes() {
        val status = backendJson.decodeFromString<VaultStatus>("""{"state":"unlocked","autoLockSeconds":600}""")
        assertEquals(600, status.autoLockSeconds)
        val oldStatus = backendJson.decodeFromString<VaultStatus>("""{"state":"locked"}""")
        assertEquals(VaultStatus.DEFAULT_AUTO_LOCK_SECONDS, oldStatus.autoLockSeconds)
        assertEquals(listOf("30 seconds", "1 minute", "5 minutes", "10 minutes", "1 hour"), VaultStatus.allAutoLockSeconds.map { VaultStatus.describeAutoLock(it) })
        assertEquals("5 min", VaultStatus.describeShortAutoLock(300))
    }
}
