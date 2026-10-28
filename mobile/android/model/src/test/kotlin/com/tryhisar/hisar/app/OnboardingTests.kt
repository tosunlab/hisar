package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.EmergencyKitStatus
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.InviteLink
import com.tryhisar.hisar.model.SignUpPolicy
import com.tryhisar.hisar.model.VaultStatus
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext

class OnboardingTests {
    private val server = "https://hisar.example"

    @Test
    fun testPostponedKitAuthorizedByPasswordUnlock() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isBiometricAvailable = false
        val first = harness.startModel()
        assertTrue(first.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD))
        first.signIn.postponeNewSecretKey()
        runCurrent()
        first.lock()
        val restarted = harness.startModel()
        assertEquals(Screen.Locked, restarted.screen.value)
        assertFalse(restarted.pendingRecovery.isShown.value)
        assertNull(restarted.signIn.newAccountStep.value)
        assertTrue(restarted.lock.unlock(FakeBackend.PASSWORD))
        assertNull(restarted.signIn.newAccountStep.value)
        restarted.signIn.openEmergencyKit()
        assertEquals(NewAccountStep.SaveSecretKey, restarted.signIn.newAccountStep.value)
        val export = assertNotNull(restarted.signIn.exportNewAccountEmergencyKit())
        assertTrue(restarted.signIn.recordEmergencyKitDelivery(export, "file"))
        assertTrue(restarted.signIn.finishNewSecretKey())
        assertEquals(Screen.Unlocked, restarted.screen.value)
        assertNull(restarted.status.value.emergencyKit)
        assertEquals(0, harness.backend.countCall("authorizeNewAccountEmergencyKit"))
        File(export.emergencyKitPath).parentFile?.deleteRecursively()
    }

    @Test
    fun testBiometricUnlockAsksOnceForKit() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD)
        model.signIn.postponeNewSecretKey()
        assertTrue(model.signIn.turnOnBiometricUnlockFromOffer())
        model.lock()
        assertTrue(model.lock.unlockWithBiometric())
        model.signIn.openEmergencyKit()
        assertFalse(model.status.value.emergencyKit!!.isAuthorized)
        assertTrue(model.signIn.authorizeEmergencyKit())
        assertTrue(model.status.value.emergencyKit!!.isAuthorized)
        model.signIn.closeEmergencyKitGate()
        model.signIn.openEmergencyKit()
        val export = assertNotNull(model.signIn.exportNewAccountEmergencyKit())
        assertEquals(1, harness.backend.countCall("authorizeNewAccountEmergencyKitWithBiometric"))
        File(export.emergencyKitPath).parentFile?.deleteRecursively()
    }

    @Test
    fun testLockDuringKitReopensItAfterUnlock() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD)
        val export = assertNotNull(model.signIn.exportNewAccountEmergencyKit())
        model.lock()
        assertNull(model.signIn.newAccountStep.value)
        assertEquals(FakeBackend.NEW_ACCOUNT_OPERATION_ID, model.signIn.interruptedKitOperationID.value)
        assertTrue(model.lock.unlock(FakeBackend.PASSWORD))
        assertEquals(NewAccountStep.SaveSecretKey, model.signIn.newAccountStep.value)
        assertTrue(model.status.value.emergencyKit!!.isAuthorized)
        assertFalse(model.signIn.recordEmergencyKitDelivery(export, "share"), "an old chooser cannot use the new unlock's grant")
        assertEquals("", model.status.value.emergencyKit!!.savedBy)
        File(export.emergencyKitPath).parentFile?.deleteRecursively()
    }

    @Test
    fun testCopyOrCancelledShareDoesNotSaveKit() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD)
        val export = assertNotNull(model.signIn.exportNewAccountEmergencyKit())
        model.copyText(export.secretKey, "Secret Key")
        assertEquals("", model.status.value.emergencyKit!!.savedBy)
        assertFalse(model.signIn.finishNewSecretKey())
        assertTrue(model.signIn.recordEmergencyKitDelivery(export, "share"))
        assertTrue(model.signIn.finishNewSecretKey())
        File(export.emergencyKitPath).parentFile?.deleteRecursively()
    }

    @Test
    fun testSaveLaterBiometricAnswersPreserveKitAuthorization() = runTest {
        for (accept in listOf(false, true)) {
            val harness = AppModelHarness(this)
            val model = harness.startModel()
            model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD)
            model.signIn.postponeNewSecretKey()
            assertEquals(NewAccountStep.BiometricOffer, model.signIn.newAccountStep.value)
            if (accept) assertTrue(model.signIn.turnOnBiometricUnlockFromOffer()) else model.signIn.dismissNewSecretKey()
            runCurrent()
            assertTrue(model.status.value.emergencyKit!!.isAuthorized)
            model.signIn.openEmergencyKit()
            val export = assertNotNull(model.signIn.exportNewAccountEmergencyKit())
            assertTrue(model.signIn.recordEmergencyKitDelivery(export, "file"))
            assertTrue(model.signIn.finishNewSecretKey())
            assertNull(model.signIn.newAccountStep.value)
            File(export.emergencyKitPath).parentFile?.deleteRecursively()
        }
    }

    @Test
    fun testSignUpPolicyHidesInviteFieldOnOpenServer() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.signUpPolicyMode = "open"
        val model = harness.startModel()
        model.signIn.checkSignUpPolicy(server)
        val policy = model.signIn.signUpPolicy.value
        assertFalse(policy.showsInviteField)
        assertFalse(policy.requiresInvitation)
        assertTrue(policy.showsAccountFields(false))
    }

    @Test
    fun testSignUpPolicyRequiresInviteFirst() = runTest {
        for (mode in listOf("invite", "setup", "unknown")) {
            val harness = AppModelHarness(this)
            harness.backend.signUpPolicyMode = mode
            val model = harness.startModel()
            model.signIn.checkSignUpPolicy(server)
            val policy = model.signIn.signUpPolicy.value
            assertTrue(policy.showsInviteField)
            assertEquals(mode != "unknown", policy.requiresInvitation)
            assertEquals(mode == "unknown", policy.showsAccountFields(false))
            assertTrue(policy.showsAccountFields(true))
        }
    }

    @Test
    fun testSignUpPolicyIgnoresStaleAnswer() = runTest {
        val started = CountDownLatch(1)
        val release = CountDownLatch(1)
        val backend = FakeBackend().apply {
            signUpPolicyAnswer = { address ->
                if (address == server) { started.countDown(); check(release.await(10, TimeUnit.SECONDS)) }
                SignUpPolicy(address, address.removePrefix("https://"), if (address == server) "invite" else "open")
            }
        }
        val model = AppModelHarness(this, backend).createModel(Dispatchers.IO)
        model.start { backend }
        val old = async { model.signIn.checkSignUpPolicy(server) }
        runCurrent()
        try {
            assertTrue(withContext(Dispatchers.IO) { started.await(10, TimeUnit.SECONDS) })
            model.signIn.checkSignUpPolicy("https://second.example")
            assertFalse(model.signIn.signUpPolicy.value.showsInviteField)
        } finally { release.countDown() }
        old.await()
        assertEquals("https://second.example", (model.signIn.signUpPolicy.value as SignUpPolicyState.Ready).policy.server)
        assertFalse(model.signIn.signUpPolicy.value.showsInviteField)
    }

    @Test
    fun testRegistrationClosedAfterCheckKeepsForm() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.signUpPolicyMode = "open"
        val model = harness.startModel()
        model.signIn.checkSignUpPolicy(server)
        harness.backend.signUpWithoutInviteErrorMessage = "Sign-ups are not open on this server. Ask an admin for an invitation."
        harness.backend.signUpPolicyMode = "invite"
        assertFalse(model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD))
        assertTrue(model.signIn.isInvitationRequiredAfterSubmit.value)
        assertTrue(model.signIn.signUpPolicy.value.requiresInvitation)
        assertEquals(1, harness.backend.countCall("signUpWithoutInvite"))
        assertEquals(Screen.SignedOut, model.screen.value)
    }

    @Test
    fun testWelcomeOnlyOnFreshInstall() {
        assertTrue(SignInModel.shouldShowWelcome(VaultStatus.signedOut, false, false))
        assertFalse(SignInModel.shouldShowWelcome(VaultStatus.signedOut.copy(server = server), false, false))
        assertFalse(SignInModel.shouldShowWelcome(VaultStatus.signedOut, true, false))
        assertFalse(SignInModel.shouldShowWelcome(VaultStatus.signedOut, false, true))
    }

    @Test
    fun testInviteLinkEmailDoesNotDependOnInvitationSource() {
        for (source in listOf("", "&invitation=unknown", "&invitation=organization", "&invitation=share")) {
            assertEquals("ada+hisar@example.com", InviteLink.parse("hisar://invite/abc?email=Ada%2BHisar%40Example.com$source")?.email)
        }
        assertEquals("https://other.example/base/invite/code", InviteLink.parseInvitation("code", "https://other.example/base")?.invite)
    }

    @Test
    fun testEmergencyKitLateCompletionAfterUnlockIsIgnored() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD)
        val first = assertNotNull(model.signIn.exportNewAccountEmergencyKit())
        model.lock()
        assertTrue(model.lock.unlock(FakeBackend.PASSWORD))
        val current = assertNotNull(model.signIn.exportNewAccountEmergencyKit())
        assertFalse(model.signIn.recordEmergencyKitDelivery(first, "share"))
        assertEquals("", model.status.value.emergencyKit?.savedBy)
        assertTrue(model.signIn.recordEmergencyKitDelivery(current, "file"))
        assertEquals("file", model.status.value.emergencyKit?.savedBy)
        listOf(first, current).forEach { File(it.emergencyKitPath).parentFile?.deleteRecursively() }
    }

    @Test
    fun testLateBiometricEnrollmentCannotReopenALockedAccount() = runTest {
        lateinit var model: AppModel
        val backend = object : FakeBackend() {
            override fun enableNewAccountBiometricUnlock(): VaultStatus {
                val answer = super.enableNewAccountBiometricUnlock()
                model.applyStatus(lock())
                return answer
            }
        }
        model = AppModelHarness(this, backend).startModel()
        model.signIn.signUpWithoutInvite(server, "ada@example.com", "Ada", FakeBackend.PASSWORD)
        model.signIn.postponeNewSecretKey()
        assertFalse(model.signIn.turnOnBiometricUnlockFromOffer())
        assertEquals(Screen.Locked, model.screen.value)
        assertNull(model.signIn.newAccountStep.value)
        assertFalse(model.status.value.emergencyKit!!.isAuthorized)
    }

    @Test
    fun testSettingsKitReturningAfterLockCannotRevealTheQR() = runTest {
        lateinit var model: AppModel
        var path = ""
        val backend = object : FakeBackend() {
            override fun exportEmergencyKit(password: String, directory: File): com.tryhisar.hisar.model.EmergencyKitExport {
                val export = super.exportEmergencyKit(password, directory)
                path = export.emergencyKitPath
                model.applyStatus(lock())
                return export.copy(qrCode = "a QR that must never be published")
            }
        }
        model = AppModelHarness(this, backend).startUnlockedModel()
        assertNull(model.settings.exportEmergencyKit(FakeBackend.PASSWORD))
        assertFalse(File(path).exists())
        assertEquals(Screen.Locked, model.screen.value)
    }

    @Test
    fun testEmergencyKitReminderDistinguishesDeliveredKit() {
        val kit = EmergencyKitStatus("operation", savedBy = "file", reminder = "14-days")
        assertEquals("Confirm your Emergency Kit", kit.title)
        assertTrue(kit.reminderBody.startsWith("Emergency Kit saved."))
        assertFalse(kit.reminderBody.contains("isn't saved"))
        assertTrue(AccountLifecycleLogic.signOutMessage(kit, false).startsWith("You haven't confirmed"))
        assertTrue(AccountLifecycleLogic.signOutMessage(kit.copy(savedBy = ""), true).startsWith("You haven't saved"))
    }
}
