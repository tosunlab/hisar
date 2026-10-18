package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.InviteLink
import com.tryhisar.hisar.model.SignInLink
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Signing in, creating an account, the links that fill the forms in, and the screens right after sign-up. */
class SignInModelTests {
    private val inviteLink = "http://127.0.0.1:18080/invite/abc"

    @Test
    fun testSignInFailureKeepsSignedOutWithMessage() = runTest {
        val model = AppModelHarness(this).startModel()
        val isSignedIn = model.signIn.signIn("http://127.0.0.1:18080", "alice@example.com", FakeBackend.SECRET_KEY, "wrong")
        assertFalse(isSignedIn)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertEquals("Incorrect email, master password or Secret Key", model.errorMessage.value)
    }

    @Test
    fun testSignUpShowsNewSecretKey() = runTest {
        val model = AppModelHarness(this).startModel()
        val isSignedUp = model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD)
        assertTrue(isSignedUp)
        assertEquals(NewAccountStep.SaveSecretKey(FakeBackend.SECRET_KEY), model.signIn.newAccountStep.value)
        assertEquals(Screen.Unlocked, model.screen.value)
        model.signIn.dismissNewSecretKey()
        assertNull(model.signIn.newAccountStep.value)
    }

    @Test
    fun testSecretKeyScreenOffersBiometricsAndTurnsThemOnInOneTap() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD)
        model.signIn.finishNewSecretKey()
        assertEquals(NewAccountStep.BiometricOffer, model.signIn.newAccountStep.value, "the cover stays up for the offer")
        val isTurnedOn = model.signIn.turnOnBiometricUnlockFromOffer()
        assertTrue(isTurnedOn)
        assertTrue(model.status.value.biometricEnabled)
        assertNull(model.signIn.newAccountStep.value)
        assertEquals("Biometric unlock is on", model.notice.value)
        assertFalse("enableBiometricUnlock" in harness.backend.allCallName, "no master password asked again")
        runCurrent()
        assertTrue("forgetNewAccountSecretKey" in harness.backend.allCallName)
    }

    @Test
    fun testBiometricOfferFailureStaysOnTheOffer() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD)
        model.signIn.finishNewSecretKey()
        harness.backend.isBiometricSucceeding = false
        val isTurnedOn = model.signIn.turnOnBiometricUnlockFromOffer()
        assertFalse(isTurnedOn)
        assertEquals(NewAccountStep.BiometricOffer, model.signIn.newAccountStep.value)
        assertEquals("Face ID was cancelled.", model.errorMessage.value)
        model.signIn.dismissNewSecretKey()
        assertNull(model.signIn.newAccountStep.value)
        assertFalse(model.status.value.biometricEnabled)
        runCurrent()
        assertTrue("forgetNewAccountSecretKey" in harness.backend.allCallName)
    }

    @Test
    fun testSecretKeyScreenSkipsTheOfferWithoutBiometrics() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isBiometricAvailable = false
        val model = harness.startModel()
        model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD)
        model.signIn.finishNewSecretKey()
        assertNull(model.signIn.newAccountStep.value)
        runCurrent()
        assertTrue("forgetNewAccountSecretKey" in harness.backend.allCallName)
    }

    @Test
    fun testSignInServerDefaultsToTheHostedService() {
        assertTrue(SignInModel.isDefaultServer(""))
        assertTrue(SignInModel.isDefaultServer("https://api.tryhisar.com/"))
        assertFalse(SignInModel.isDefaultServer("http://127.0.0.1:18080"))
        assertEquals("api.tryhisar.com", SignInModel.describeServer(SignInModel.DEFAULT_SERVER))
    }

    @Test
    fun testSecretKeyFormatting() {
        assertEquals("H1-ABC123-AAAAA", VaultLogic.formatSecretKey(" h1-abc123 -aaaaa\n"))
    }

    @Test
    fun testDescribeServerShowsTheHost() {
        assertEquals("api.tryhisar.com", VaultLogic.describeServer("https://api.tryhisar.com"))
        assertEquals("127.0.0.1:18080", VaultLogic.describeServer("http://127.0.0.1:18080"))
    }

    @Test
    fun testSignInLinkFromEmergencyKit() {
        val link = SignInLink.parse("hisar://sign-in?email=alice%40example.com&secretKey=H1-ABC123-AAAAA&server=https%3A%2F%2Fhisar.example.com")
        assertEquals(SignInLink(server = "https://hisar.example.com", email = "alice@example.com", secretKey = "H1-ABC123-AAAAA"), link)
        assertNull(SignInLink.parse("https://hisar.example.com/invite/abc"))
        assertNull(SignInLink.parse("hisar://sign-in"))
        assertNull(SignInLink.parse("hisar://other?server=x"))
    }

    @Test
    fun testInviteLinkFromInvitePage() {
        val expectedByLink = mapOf(
            "hisar://invite/abc_DEF-123" to "abc_DEF-123",
            " hisar://invite/abc_DEF-123?server=https%3A%2F%2Fapi.tryhisar.com " to "abc_DEF-123",
            "HISAR://INVITE/abc?server=https%3A%2F%2Fhisar.example.com" to "https://hisar.example.com/invite/abc",
            "hisar://invite/abc?server=http%3A%2F%2F127.0.0.1%3A19400" to "http://127.0.0.1:19400/invite/abc",
            "hisar://invite/abc?server=hisar.example.com" to "https://hisar.example.com/invite/abc",
            "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com%2F&other=yes" to "https://hisar.example.com/invite/abc",
        )
        for ((link, expected) in expectedByLink) {
            assertEquals(InviteLink(expected), InviteLink.parse(link), link)
        }
        val allInvalid = listOf(
            "", "hisar://invite/", "hisar://invite/a/b", "hisar://sign-in?server=x", "https://api.tryhisar.com/invite/abc",
            "hisar://invite/a%20b", "hisar://invite/abc?server=ftp%3A%2F%2Fx",
        )
        for (link in allInvalid) {
            assertNull(InviteLink.parse(link), link)
        }
        assertNull(SignInLink.parse("hisar://invite/abc"))
    }

    @Test
    fun testInviteLinkPrefillsCreateAccountWhenSignedOut() = runTest {
        val model = AppModelHarness(this).startModel()
        val link = InviteLink("https://hisar.example.com/invite/abc")
        model.signIn.openInviteLink(link)
        assertEquals(link, model.signIn.pendingInviteLink.value)
        assertNull(model.signIn.inviteLinkMessage.value)
    }

    @Test
    fun testHisarLinksFillTheSignInOrCreateAccountForm() = runTest {
        val model = AppModelHarness(this).startModel()
        model.signIn.openLink("hisar://sign-in?email=alice%40example.com&secretKey=H1-ABC123")
        assertEquals(SignInLink(server = "", email = "alice@example.com", secretKey = "H1-ABC123"), model.signIn.pendingSignInLink.value)
        model.signIn.openLink("hisar://invite/abc")
        assertEquals(InviteLink("abc"), model.signIn.pendingInviteLink.value)
    }

    @Test
    fun testScannedQRCodeFillsInSignInOrExplains() = runTest {
        val model = AppModelHarness(this).startModel()
        model.signIn.openScannedEmergencyKit("hisar://invite/abc")
        assertNull(model.signIn.pendingInviteLink.value, "an invite is no Emergency Kit")
        assertEquals("This QR code is not from a Hisar Emergency Kit.", model.signIn.scannerMessage.value)
        model.signIn.openScannedEmergencyKit("hisar://sign-in?server=https%3A%2F%2Fhisar.example.com&email=alice%40example.com&secretKey=H1-ABC123")
        assertEquals(SignInLink(server = "https://hisar.example.com", email = "alice@example.com", secretKey = "H1-ABC123"), model.signIn.pendingSignInLink.value)
    }

    @Test
    fun testInviteLinkWhileSignedInExplainsInstead() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.signIn.openInviteLink(InviteLink("abc"))
        assertNull(model.signIn.pendingInviteLink.value)
        assertEquals("You're signed in as alice@example.com. Sign out to create an account with this invite.", model.signIn.inviteLinkMessage.value)
        assertEquals(Screen.Unlocked, model.screen.value)
    }

    @Test
    fun testSignedOutByServerShowsReason() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        harness.backend.status = VaultStatus.signedOut.copy(signedOutReason = "This device was signed out.")
        harness.events.emit(BackendEvent.Locked)
        runCurrent()
        assertEquals(Screen.SignedOut, model.screen.value)
        assertEquals("This device was signed out.", model.status.value.signedOutReason)
        model.signIn.dismissSignedOutReason()
        assertEquals("", model.status.value.signedOutReason)
    }
}
