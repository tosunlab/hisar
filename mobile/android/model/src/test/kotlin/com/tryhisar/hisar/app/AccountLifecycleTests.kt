package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.CredentialChangeResult
import com.tryhisar.hisar.model.DeletionOrganization
import com.tryhisar.hisar.model.DeletionVault
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.PendingRecovery
import com.tryhisar.hisar.model.PendingRecoveryExport
import com.tryhisar.hisar.model.SignUpResult
import com.tryhisar.hisar.model.backendJson
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Changing the master password, replacing the Secret Key and deleting the account. */
class AccountLifecycleTests {
    @Test
    fun testNewPasswordRules() {
        assertNull(AccountLifecycleLogic.describeNewPasswordProblem("old password", "", ""))
        assertEquals("Use at least 10 characters.", AccountLifecycleLogic.describeNewPasswordProblem("old password", "short", ""))
        assertEquals(
            "The new master password is the same as the current one.",
            AccountLifecycleLogic.describeNewPasswordProblem("old password", "old password", ""),
        )
        assertEquals("The passwords do not match.", AccountLifecycleLogic.describeNewPasswordProblem("old password", "a new master password", "a new master"))
        assertFalse(AccountLifecycleLogic.canChangePassword("", "a new master password", "a new master password"))
        assertFalse(AccountLifecycleLogic.canChangePassword("old password", "a new master password", ""))
        assertTrue(AccountLifecycleLogic.canChangePassword("old password", "a new master password", "a new master password"))
    }

    @Test
    fun testUnknownSignedOutDeviceCount() {
        assertEquals("Any other devices were signed out; their count could not be recovered.", AccountLifecycleLogic.describeSignedOutDevices(-1))
        assertEquals("No other devices were signed in.", AccountLifecycleLogic.describeSignedOutDevices(0))
    }

    @Test
    fun testSignOutConfirmationExplainsLocalRemoval() {
        assertEquals("Sign out of Hisar on this device?", AccountLifecycleLogic.SIGN_OUT_TITLE)
        assertEquals(
            "Hisar removes this device's saved Secret Key and offline copy of your vaults. To sign in again, you need your master password and the Secret Key from your Emergency Kit or another signed-in device.",
            AccountLifecycleLogic.SIGN_OUT_MESSAGE,
        )
        assertEquals(
            "${AccountLifecycleLogic.SIGN_OUT_MESSAGE} If you have not saved an Emergency Kit, cancel, unlock Hisar and save one in Settings, Account.",
            AccountLifecycleLogic.LOCKED_SIGN_OUT_MESSAGE,
        )
    }

    @Test
    fun testEmailConfirmationIgnoresCaseAndSpaces() {
        assertTrue(AccountLifecycleLogic.isEmailMatching(" Alice@Example.com ", "alice@example.com"))
        assertFalse(AccountLifecycleLogic.isEmailMatching("bob@example.com", "alice@example.com"))
        assertFalse(AccountLifecycleLogic.isEmailMatching("", ""))
    }

    @Test
    fun testDeletionConsequences() {
        val preview = AccountDeletionPreview(
            email = "alice@example.com",
            allOwnedVault = listOf(
                DeletionVault(id = "personal", name = "Personal", type = "personal", role = "owner", otherMemberCount = 0),
                DeletionVault(id = "family", name = "Family", type = "shared", role = "owner", otherMemberCount = 2),
            ),
            allMemberVault = listOf(DeletionVault(id = "work", name = "Work", type = "shared", role = "editor", otherMemberCount = 0)),
            shareLinkCount = 1, sentInvitationCount = 1, receivedInvitationCount = 1, deviceCount = 3,
        )
        assertEquals(2, preview.peopleLosingAccessCount)
        assertEquals("Deleted with its items. 2 people lose access.", AccountLifecycleLogic.describeOwnedVault(preview.allOwnedVault[1]))
        assertEquals("Stays with its owners. Your access is removed.", AccountLifecycleLogic.describeMemberVault(preview.allMemberVault[0]))
        assertEquals(
            listOf("1 share link stops working.", "2 invitations are withdrawn.", "This device and 2 other devices are signed out."),
            AccountLifecycleLogic.describeAllOtherConsequence(preview),
        )
    }

    @Test
    fun testAccountDeletionSaysWhatHappensToEachOrganization() {
        assertEquals("Acme, its vaults, and their items are deleted with your account", AccountLifecycleLogic.describeOrganization(DeletionOrganization(id = "a", name = "Acme", isDeleted = true)))
        assertEquals("Ada becomes an admin of Acme", AccountLifecycleLogic.describeOrganization(DeletionOrganization(id = "a", name = "Acme", newAdminName = "Ada")))
        assertEquals("You leave Acme", AccountLifecycleLogic.describeOrganization(DeletionOrganization(id = "a", name = "Acme")))
    }

    @Test
    fun testPreviewDecodesTheBackendJSON() {
        val json = """{"email":"a@example.com","ownedVaults":[{"id":"v1","name":"Family","type":"shared","role":"owner","otherMemberCount":2}],"memberVaults":[],"shareLinkCount":3,"sentInvitationCount":0,"receivedInvitationCount":0,"deviceCount":1}"""
        val preview = backendJson.decodeFromString<AccountDeletionPreview>(json)
        assertEquals(2, preview.allOwnedVault.first().otherMemberCount)
        assertEquals(3, preview.shareLinkCount)
        val status = backendJson.decodeFromString<VaultStatus>("""{"state":"signed-out","signedOutReason":"This device was signed out."}""")
        assertEquals("This device was signed out.", status.signedOutReason)
    }

    @Test
    fun testPendingResultPreservesCommittedKeyAndProblems() {
        val json = """{"status":{"state":"signed-out"},"secretKey":"H1-NEW-SECRET","pendingOperationId":"pending-1","setupState":"needs-finishing","problems":["The keychain is unavailable."],"localError":"The offline copy could not finish."}"""
        val result = backendJson.decodeFromString<CredentialChangeResult>(json)
        assertEquals("H1-NEW-SECRET", result.secretKey)
        assertEquals("pending-1", result.pendingOperationId)
        assertEquals("needs-finishing", result.setupState)
        assertEquals(listOf("The keychain is unavailable."), result.problems)
        assertEquals("The offline copy could not finish.", result.localError)
        val signup = backendJson.decodeFromString<SignUpResult>(json)
        assertEquals("pending-1", signup.pendingOperationId)
        assertEquals(result.problems, signup.problems)
    }

    @Test
    fun testCommittedReplacementDecodesLocalWarningWithoutLosingTheKeyOrKit() {
        val json = """{"status":{"state":"unlocked"},"secretKey":"H1-NEW-SECRET","signedOutDeviceCount":-1,"keychainError":"","localError":"credentials were changed; offline copy could not be deleted","emergencyKitPath":"/tmp/EmergencyKit/new.pdf"}"""
        val result = backendJson.decodeFromString<CredentialChangeResult>(json)
        assertEquals("H1-NEW-SECRET", result.secretKey)
        assertEquals("/tmp/EmergencyKit/new.pdf", result.emergencyKitPath)
        assertTrue(result.localError.contains("offline copy could not be deleted"))
        assertEquals(-1, result.signedOutDeviceCount)
    }

    @Test
    fun testChangeMasterPassword() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertNull(model.settings.changeMasterPassword("wrong", "a new master password"))
        assertEquals("The master password is not correct", model.errorMessage.value)
        val result = model.settings.changeMasterPassword(FakeBackend.PASSWORD, "a new master password")
        assertEquals(1, result?.signedOutDeviceCount)
        assertEquals(Screen.Unlocked, model.screen.value)
        assertEquals("Master password changed. 1 other device was signed out.", model.notice.value?.message)
        assertEquals("a new master password", harness.backend.currentPassword)
    }

    @Test
    fun testReplaceSecretKeyWritesAndRemovesTheNewEmergencyKit() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val result = assertNotNull(model.settings.replaceSecretKey(FakeBackend.PASSWORD))
        assertNotEquals(FakeBackend.SECRET_KEY, result.secretKey)
        assertTrue(File(result.emergencyKitPath).exists())
        model.settings.finishSecretKeyReplacement(result)
        assertFalse(File(result.emergencyKitPath).exists())
        runCurrent()
        assertTrue("forgetNewAccountSecretKey" in harness.backend.allCallName, "the new Secret Key was not forgotten")
    }

    @Test
    fun testDeleteAccountSignsOut() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertEquals("alice@example.com", model.settings.loadAccountDeletionPreview()?.email)
        assertFalse(model.settings.deleteAccount(FakeBackend.PASSWORD, "bob@example.com", harness.backend.deletionPreview.planVersion))
        assertEquals(Screen.Unlocked, model.screen.value)
        assertTrue(model.settings.deleteAccount(FakeBackend.PASSWORD, "alice@example.com", harness.backend.deletionPreview.planVersion))
        assertTrue(harness.backend.isAccountDeleted)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty())
        assertEquals("Your Hisar account was deleted.", model.notice.value?.message)
    }

    @Test
    fun testDeleteAccountCleanupErrorStillSignsOut() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        harness.backend.isCacheRemovalFailing = true
        assertFalse(model.settings.deleteAccount(FakeBackend.PASSWORD, "alice@example.com", harness.backend.deletionPreview.planVersion))
        assertTrue(harness.backend.isAccountDeleted)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty())
        assertEquals(FakeBackend.CACHE_REMOVAL_MESSAGE, model.errorMessage.value)
    }

    @Test
    fun testDeletionRejectsChangedPlanUntilReviewedAgain() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val reviewed = assertNotNull(model.settings.loadAccountDeletionPreview())
        harness.backend.deletionPreview = reviewed.copy(planVersion = "new-plan")
        assertFalse(model.settings.deleteAccount(FakeBackend.PASSWORD, reviewed.email, reviewed.planVersion))
        assertFalse(harness.backend.isAccountDeleted)
        assertEquals(Screen.Unlocked, model.screen.value)
        val refreshed = assertNotNull(model.settings.loadAccountDeletionPreview())
        assertTrue(model.settings.deleteAccount(FakeBackend.PASSWORD, refreshed.email, refreshed.planVersion))
    }

    private fun seedPendingRecovery(backend: FakeBackend, phase: String = "outcome-unknown") {
        backend.allPendingRecovery += PendingRecovery(
            operationId = "pending-1", kind = "signup", origin = "https://recovery.example", accountId = "pending-account",
            email = "pending@example.com", name = "Pending", phase = phase,
            problems = listOf("The keychain is unavailable. Use your master password to finish."),
        )
    }

    @Test
    fun testPendingRecoverySurvivesModelRestartWithoutSigningIn() = runTest {
        val harness = AppModelHarness(this)
        seedPendingRecovery(harness.backend)
        val first = harness.startModel()
        assertTrue(first.pendingRecovery.isShown.value)
        first.pendingRecovery.close()
        assertEquals(1, harness.backend.allPendingRecovery.size)
        val restarted = harness.startModel()
        assertTrue(restarted.pendingRecovery.isShown.value)
        assertEquals("pending@example.com", restarted.pendingRecovery.allRecovery.value.first().email)
        assertEquals(Screen.SignedOut, restarted.screen.value)
        restarted.pendingRecovery.resume("pending-1", FakeBackend.PASSWORD)
        assertEquals(Screen.SignedOut, restarted.screen.value)
        assertEquals("delivery-pending", restarted.pendingRecovery.allRecovery.value.first().phase)
        assertTrue(restarted.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD))
        val export = assertNotNull(restarted.pendingRecovery.export.value)
        assertEquals(FakeBackend.SECRET_KEY, export.secretKey)
        assertTrue(File(export.path).exists())
        assertEquals(Screen.SignedOut, restarted.screen.value)
        assertTrue(restarted.pendingRecovery.acknowledge("pending-1", FakeBackend.PASSWORD))
        assertTrue(restarted.pendingRecovery.allRecovery.value.isEmpty())
        assertFalse(File(export.path).exists())
    }

    @Test
    fun testPendingRecoveryRequiresPasswordAndExplicitKitAcknowledgement() = runTest {
        val harness = AppModelHarness(this)
        seedPendingRecovery(harness.backend, "delivery-pending")
        val model = harness.startModel()
        assertFalse(model.pendingRecovery.exportKit("pending-1", "wrong"))
        assertNull(model.pendingRecovery.export.value)
        assertFalse(model.pendingRecovery.acknowledge("pending-1", FakeBackend.PASSWORD))
        assertEquals(1, harness.backend.allPendingRecovery.size)
        assertTrue(model.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD))
        model.pendingRecovery.close()
        assertNull(model.pendingRecovery.export.value)
        assertEquals(1, harness.backend.allPendingRecovery.size)
        assertTrue(harness.backend.allPendingRecovery.first().kitSaved)
    }

    @Test
    fun testLockWhilePreparingTheKitHidesItAndSaysToPrepareItAgain() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        seedPendingRecovery(harness.backend, "delivery-pending")
        model.pendingRecovery.refresh()
        harness.backend.onExportPendingRecoveryEmergencyKit = { model.applyStatus(harness.backend.lock()) }
        assertFalse(model.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD))
        assertNull(model.pendingRecovery.export.value)
        assertEquals(
            "Hisar locked, signed out or switched accounts while it prepared the Emergency Kit. Prepare it again.",
            model.errorMessage.value,
        )
        harness.backend.onExportPendingRecoveryEmergencyKit = null
        assertTrue(model.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD))
        assertEquals(FakeBackend.SECRET_KEY, model.pendingRecovery.export.value?.secretKey)
    }

    // Hisar's activity finishing for good cancels the model's scope while the blocking backend call still writes the kit.
    @Test
    fun testFinishingWhilePreparingTheKitLeavesNoFile() = runTest {
        val isWriting = CountDownLatch(1)
        val mayWrite = CountDownLatch(1)
        val hasWritten = CountDownLatch(1)
        var kitDirectory: File? = null
        val backend = object : FakeBackend() {
            override fun exportPendingRecoveryEmergencyKit(operationID: String, password: String, directory: File): PendingRecoveryExport {
                kitDirectory = directory
                isWriting.countDown()
                check(mayWrite.await(5, TimeUnit.SECONDS))
                return super.exportPendingRecoveryEmergencyKit(operationID, password, directory).also { hasWritten.countDown() }
            }
        }
        seedPendingRecovery(backend, "delivery-pending")
        val scope = CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler))
        val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())
        try {
            model.start { backend }
            val export = async { runCatching { model.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD) } }
            runCurrent()
            assertTrue(isWriting.await(5, TimeUnit.SECONDS))
            model.pendingRecovery.clearExport()
            scope.cancel()
            mayWrite.countDown()
            assertTrue(hasWritten.await(5, TimeUnit.SECONDS))
            export.await()
            assertNull(model.pendingRecovery.export.value)
            assertFalse(kitDirectory!!.exists(), "the finished activity's kit stayed at $kitDirectory")
        } finally {
            mayWrite.countDown()
            scope.cancel()
            kitDirectory?.deleteRecursively()
        }
    }

    @Test
    fun testPendingSignupKeepsProblemsWithoutApplyingStaleStatus() = runTest {
        val harness = AppModelHarness(this)
        seedPendingRecovery(harness.backend)
        harness.backend.pendingSignUpResult = SignUpResult(
            VaultStatus(state = VaultStatus.STATE_UNLOCKED), pendingOperationId = "pending-1",
            setupState = "outcome-unknown", problems = listOf("The server response was lost."),
        )
        val model = harness.startModel()
        assertTrue(model.signIn.signUp("https://recovery.example/invite/test", "pending@example.com", "Pending", FakeBackend.PASSWORD))
        assertEquals(Screen.SignedOut, model.screen.value)
        assertNull(model.signIn.newAccountStep.value)
        assertTrue(model.pendingRecovery.isShown.value)
        assertEquals("The server response was lost.", model.errorMessage.value)
    }

    @Test
    fun testPendingSignupThatInstalledTheAccountShowsItsVaults() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        harness.backend.seedSignedIn()
        seedPendingRecovery(harness.backend, "delivery-pending")
        harness.backend.pendingSignUpResult = SignUpResult(harness.backend.status, pendingOperationId = "pending-1", setupState = "delivery-pending")
        assertTrue(model.signIn.signUp("https://recovery.example/invite/test", "pending@example.com", "Pending", FakeBackend.PASSWORD))
        assertEquals(Screen.Unlocked, model.screen.value)
        assertEquals(listOf("Family", "Personal"), model.unlockedData.value.allVault.map { it.name }.sorted())
        assertTrue(model.pendingRecovery.isShown.value)
    }

    @Test
    fun testPendingReplacementKeepsRecoveryAndLocalProblems() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        seedPendingRecovery(harness.backend, "needs-finishing")
        harness.backend.pendingCredentialChangeResult = CredentialChangeResult(
            harness.backend.status, secretKey = FakeBackend.SECRET_KEY, localError = "The keychain is unavailable.",
            pendingOperationId = "pending-1", setupState = "needs-finishing", problems = listOf("The keychain is unavailable."),
        )
        val result = model.settings.replaceSecretKey(FakeBackend.PASSWORD)
        assertEquals("pending-1", result?.pendingOperationId)
        assertTrue(model.pendingRecovery.isShown.value)
        assertEquals("The keychain is unavailable.", model.errorMessage.value)
        model.pendingRecovery.close()
        assertEquals(1, harness.backend.allPendingRecovery.size)
        model.pendingRecovery.refresh()
        model.pendingRecovery.resume("pending-1", FakeBackend.PASSWORD)
        model.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD)
        val exported = assertNotNull(model.pendingRecovery.export.value)
        model.lock()
        assertNull(model.pendingRecovery.export.value)
        assertFalse(File(exported.path).exists())
        assertEquals(1, harness.backend.allPendingRecovery.size)
    }
    @Test
    fun testManualRecoveryRequiresSavedKitAfterRestart() = runTest {
        val harness = AppModelHarness(this)
        seedPendingRecovery(harness.backend, "awaiting-kit")
        harness.backend.allPendingRecovery[0] = harness.backend.allPendingRecovery[0].copy(manualUnlock = true)
        val first = harness.startModel()
        assertTrue(first.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD))
        val kit = assertNotNull(first.pendingRecovery.export.value)
        assertTrue(first.pendingRecovery.acknowledge("pending-1", FakeBackend.PASSWORD))
        assertEquals("prepared", first.pendingRecovery.allRecovery.value.first().phase)
        harness.backend.isPendingRecoveryFactorAvailable = false
        val restarted = harness.startModel()
        assertFalse(restarted.pendingRecovery.resume("pending-1", FakeBackend.PASSWORD))
        assertEquals("prepared", restarted.pendingRecovery.allRecovery.value.first().phase)
        assertFalse(restarted.pendingRecovery.resume("pending-1", FakeBackend.PASSWORD, "wrong"))
        assertEquals("prepared", restarted.pendingRecovery.allRecovery.value.first().phase)
        assertTrue(restarted.pendingRecovery.resume("pending-1", FakeBackend.PASSWORD, kit.secretKey))
        assertEquals("delivery-pending", restarted.pendingRecovery.allRecovery.value.first().phase)
        assertTrue(restarted.pendingRecovery.acknowledge("pending-1", FakeBackend.PASSWORD))
        assertEquals("manual-unlock", restarted.pendingRecovery.allRecovery.value.first().phase)
        assertEquals(Screen.SignedOut, restarted.screen.value)
        assertTrue("resumePendingRecoveryWithSecretKey" in harness.backend.allCallName)
    }

    @Test
    fun testOnlyUncommittedRecoveryCanBeDiscarded() = runTest {
        val harness = AppModelHarness(this)
        seedPendingRecovery(harness.backend, "awaiting-kit")
        harness.backend.isPendingRecoveryFactorAvailable = false
        val model = harness.startModel()
        assertFalse(model.pendingRecovery.discard("pending-1", "wrong"))
        assertEquals(1, model.pendingRecovery.allRecovery.value.size)
        assertTrue(model.pendingRecovery.discard("pending-1", FakeBackend.PASSWORD))
        assertTrue(model.pendingRecovery.allRecovery.value.isEmpty())
        seedPendingRecovery(harness.backend, "outcome-unknown")
        model.pendingRecovery.refresh()
        assertFalse(model.pendingRecovery.discard("pending-1", FakeBackend.PASSWORD))
        assertEquals(1, model.pendingRecovery.allRecovery.value.size)
        assertEquals(Screen.SignedOut, model.screen.value)
    }

    @Test
    fun testVersionCheckFinishingAfterSignOutDoesNotRestoreAccount() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        runCurrent()
        harness.backend.onCheckMinimumVersion = {
            harness.backend.status = VaultStatus.signedOut
            model.applyStatus(VaultStatus.signedOut)
        }
        model.checkMinimumVersion()
        harness.backend.onCheckMinimumVersion = null
        assertEquals(Screen.SignedOut, model.screen.value)
        assertTrue(model.unlockedData.value.allVault.isEmpty())
        assertNull(model.signIn.newAccountStep.value)
    }

    @Test
    fun testPendingRecoveryRefreshDoesNotDiscardActionError() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        runCurrent()
        model.errorMessage.value = "The recovery password is not correct."
        model.pendingRecovery.refresh()
        assertEquals("The recovery password is not correct.", model.errorMessage.value)
    }
}
