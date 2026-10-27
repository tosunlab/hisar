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
    fun testSignUpWithoutInviteAsksTheServerAndShowsNewSecretKey() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        val isSignedUp = model.signIn.signUpWithoutInvite(" https://hisar.example.com ", "bob@example.com", "Bob", FakeBackend.PASSWORD)
        assertTrue(isSignedUp)
        assertEquals("https://hisar.example.com", harness.backend.signUpWithoutInviteServer)
        assertFalse("signUp" in harness.backend.allCallName, "no invite was used")
        assertEquals(NewAccountStep.SaveSecretKey(FakeBackend.SECRET_KEY), model.signIn.newAccountStep.value)
        assertEquals(Screen.Unlocked, model.screen.value)
        assertEquals("https://hisar.example.com", model.status.value.server)
        assertEquals("bob@example.com", model.status.value.email)
        assertNull(model.errorMessage.value)
    }

    @Test
    fun testSignUpWithoutInviteOnAClosedServerShowsTheServersSentence() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        harness.backend.signUpWithoutInviteErrorMessage = "Sign-ups are not open on this server. Ask an admin for an invitation."
        assertFalse(model.signIn.signUpWithoutInvite("https://hisar.example.com", "bob@example.com", "Bob", FakeBackend.PASSWORD))
        assertEquals("Sign-ups are not open on this server. Ask an admin for an invitation.", model.errorMessage.value)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertNull(model.signIn.newAccountStep.value)
    }

    @Test
    fun testLockHidesTheNewSecretKey() = runTest {
        val model = AppModelHarness(this).startModel()
        model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD)
        model.lock()
        assertEquals(Screen.Locked, model.screen.value)
        assertNull(model.signIn.newAccountStep.value, "the new Secret Key stayed over the lock screen")
    }

    @Test
    fun testAutoLockHidesTheBiometricOffer() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startModel()
        model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD)
        model.signIn.finishNewSecretKey()
        assertEquals(NewAccountStep.BiometricOffer, model.signIn.newAccountStep.value)
        harness.backend.lock()
        harness.events.emit(BackendEvent.Locked)
        runCurrent()
        assertEquals(Screen.Locked, model.screen.value)
        assertNull(model.signIn.newAccountStep.value, "the offer stayed over the lock screen")
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
        assertTrue(SignInModel.isDefaultServer("https://eu.tryhisar.com/"))
        assertFalse(SignInModel.isDefaultServer("http://127.0.0.1:18080"))
        assertEquals("eu.tryhisar.com", SignInModel.describeServer(SignInModel.DEFAULT_SERVER))
    }

    @Test
    fun testSecretKeyFormatting() {
        assertEquals("H1-ABC123-AAAAA", VaultLogic.formatSecretKey(" h1-abc123 -aaaaa\n"))
    }

    @Test
    fun testDescribeServerShowsTheHost() {
        assertEquals("eu.tryhisar.com", VaultLogic.describeServer("https://eu.tryhisar.com"))
        assertEquals("127.0.0.1:18080", VaultLogic.describeServer("http://127.0.0.1:18080"))
        assertEquals("https://faß.de:8443", VaultLogic.describeServer("https://faß.de:8443"))
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
    fun testPlainTextEmergencyKitPreservesBytesAndRejectsInvalidPayloads() = runTest {
        val payload = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL2hpc2FyLmV4YW1wbGUvYT94PW9uZSt0d29cdTAwMjZ5PSUyMCIsImVtYWlsIjoiYWxwZXIra2l0QHRyeWhpc2FyLmNvbSIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
        assertEquals(SignInLink("https://hisar.example/a?x=one+two&y=%20", "alper+kit@tryhisar.com", "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"), SignInLink.parse(payload))
        assertNull(InviteLink.parse(payload))
        for (invalid in listOf(payload + "=", payload + "\n", payload.replace("HISARKIT1 ", "HISARKIT2 "), "HISARKIT1 e30", "HISARKIT1:", "HISARKIT1 _w")) {
            assertNull(SignInLink.parse(invalid), invalid)
        }
        for (json in listOf("""{"server":"https://h","email":"a@b","secretKey":1}""", """{"server":"https://h","email":"a@b","secretKey":"H1-INVALID"}""", """{"server":"javascript:bad","email":"a@b","secretKey":"H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"}""")) {
            val encoded = java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(json.toByteArray())
            assertNull(SignInLink.parse("HISARKIT1 $encoded"))
        }
        val unicodeJson = """{"server":"https://hisar.example/İstanbul/東京","email":"élève+🔐@example.test","secretKey":"H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"}"""
        fun encode(bytes: ByteArray) = "HISARKIT1 " + java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
        assertEquals(SignInLink("https://hisar.example/İstanbul/東京", "élève+🔐@example.test", "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"), SignInLink.parse(encode(unicodeJson.toByteArray(Charsets.UTF_8))))
        for (encoding in listOf("UTF-16", "UTF-16LE", "UTF-16BE", "UTF-32", "UTF-32LE", "UTF-32BE")) {
            assertNull(SignInLink.parse(encode(unicodeJson.toByteArray(charset(encoding)))), encoding)
        }
        assertNull(SignInLink.parse(encode(unicodeJson.toByteArray(Charsets.UTF_8) + byteArrayOf(0xC3.toByte(), 0x28))))
        assertEquals(" key+bytes ", SignInLink.parse("hisar://sign-in?secretKey=%20key%2Bbytes%20")?.secretKey)
        val model = AppModelHarness(this).startModel()
        model.signIn.isScannerShown.value = true
        model.signIn.openScannedEmergencyKit(payload)
        assertFalse(model.signIn.isScannerShown.value)
        assertEquals(SignInLink.parse(payload), model.signIn.pendingSignInLink.value)
    }

    @Test
    fun testPlainTextEmergencyKitWithInternationalServerName() {
        val payload = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL-S-i-OBiC50ZXN0Ojg0NDMiLCJlbWFpbCI6Iuadjumbt0DkvovjgYgudGVzdCIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
        assertEquals(SignInLink("https://例え.test:8443", "李雷@例え.test", "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"), SignInLink.parse(payload))
        val withUser = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL3VzZXJA5L6L44GILnRlc3QiLCJlbWFpbCI6Iuadjumbt0DkvovjgYgudGVzdCIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
        assertNull(SignInLink.parse(withUser))
        val key = "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"
        fun kit(server: String): String {
            val json = """{"server":"$server","email":"alice@example.test","secretKey":"$key"}"""
            return "HISARKIT1 " + java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(json.toByteArray(Charsets.UTF_8))
        }
        for (server in listOf("https://Example.COM:8443/a%2Fb?q=%2F+one", "http://[2001:db8::1]:8080/path", "https://例え.test:8443/東京?q=%2F")) {
            assertEquals(SignInLink(server, "alice@example.test", key), SignInLink.parse(kit(server)), server)
        }
        for (invalid in listOf("https://例え..test", "https://-例え.test", "https://\u200D.test", "https://user@例え.test", "https://例え.test/#fragment")) {
            assertNull(SignInLink.parse(kit(invalid)), invalid)
        }
    }

    @Test
    fun testInviteLinkFromInvitePage() {
        val expectedByLink = mapOf(
            "hisar://invite/abc_DEF-123" to InviteLink("abc_DEF-123"),
            " hisar://invite/abc_DEF-123?server=https%3A%2F%2Feu.tryhisar.com " to InviteLink("abc_DEF-123"),
            "HISAR://INVITE/abc?server=https%3A%2F%2Fhisar.example.com" to InviteLink("https://hisar.example.com/invite/abc", "https://hisar.example.com"),
            "hisar://invite/abc?server=http%3A%2F%2F127.0.0.1%3A19400" to InviteLink("http://127.0.0.1:19400/invite/abc", "http://127.0.0.1:19400"),
            "hisar://invite/abc?server=hisar.example.com" to InviteLink("https://hisar.example.com/invite/abc", "https://hisar.example.com"),
            "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com%2F&other=yes" to InviteLink("https://hisar.example.com/invite/abc", "https://hisar.example.com"),
            // An organization or vault share invitation carries the kind and the address it is for; the email is lower-cased.
            "hisar://invite/abc?invitation=organization&email=Alice%40Example.com" to InviteLink("abc", invitation = "organization", email = "alice@example.com"),
            "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com&invitation=share&email=bob%40example.com" to
                InviteLink("https://hisar.example.com/invite/abc", "https://hisar.example.com", "share", "bob@example.com"),
            // An unknown invitation is a plain invite, as in an older app, so its email is dropped too.
            "hisar://invite/abc?invitation=team&email=carol%40example.com" to InviteLink("abc"),
        )
        for ((link, expected) in expectedByLink) {
            assertEquals(expected, InviteLink.parse(link), link)
        }
        val allInvalid = listOf(
            "", "hisar://invite/", "hisar://invite/a/b", "hisar://sign-in?server=x", "https://eu.tryhisar.com/invite/abc",
            "hisar://invite/a%20b", "hisar://invite/abc?server=ftp%3A%2F%2Fx",
        )
        for (link in allInvalid) {
            assertNull(InviteLink.parse(link), link)
        }
        assertNull(SignInLink.parse("hisar://invite/abc"))
    }

    @Test
    fun testInviteLinkPreservesInternationalServerOrigin() {
        val link = InviteLink.parse("hisar://invite/abc?server=https%3A%2F%2F%E4%BE%8B%E3%81%88.test%3A8443")
        assertEquals(InviteLink("https://例え.test:8443/invite/abc", "https://例え.test:8443"), link)
        assertEquals("https://例え.test:8443/invite/abc", VaultLogic.convertToOpenableURL(link!!.invite)?.toString())
        val expectedByServer = mapOf(
            "例え.test" to "https://例え.test/invite/abc",
            " https://例え.test:8443/// " to "https://例え.test:8443/invite/abc",
            "HTTP://bücher.test:8080" to "HTTP://bücher.test:8080/invite/abc",
            "http://user:pass@例え.test:8443" to "http://user:pass@例え.test:8443/invite/abc",
            "https://Example.COM:8443/base%2Fpath" to "https://Example.COM:8443/base%2Fpath/invite/abc",
            "http://user:pass@[2001:db8::1]:8080" to "http://user:pass@[2001:db8::1]:8080/invite/abc",
            "https://xn--r8jz45g.test:8443" to "https://xn--r8jz45g.test:8443/invite/abc",
        )
        fun appLink(server: String) = "hisar://invite/abc?server=" + java.net.URLEncoder.encode(server, Charsets.UTF_8).replace("+", "%20")
        for ((server, expected) in expectedByServer) {
            assertEquals(InviteLink(expected, expected.removeSuffix("/invite/abc")), InviteLink.parse(appLink(server)), server)
        }
        for (invalid in listOf("https://例え..test", "https://-例え.test", "https://例え-.test", "https://\u200D.test", "https://例え.test:port", "https://例え.test:８０", "https://例え.test/%zz", "ftp://例え.test")) {
            assertNull(InviteLink.parse(appLink(invalid)), invalid)
        }
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
        model.signIn.isScannerShown.value = true
        model.signIn.openScannedEmergencyKit("hisar://invite/abc")
        assertFalse(model.signIn.isScannerShown.value, "the first code closes the scanner")
        assertNull(model.signIn.pendingInviteLink.value, "an invite is no Emergency Kit")
        assertEquals("This QR code is not from a Hisar Emergency Kit.", model.signIn.scannerMessage.value)
        model.signIn.openScannedEmergencyKit("hisar://sign-in?server=https%3A%2F%2Fhisar.example.com&email=alice%40example.com&secretKey=H1-ABC123")
        assertEquals(SignInLink(server = "https://hisar.example.com", email = "alice@example.com", secretKey = "H1-ABC123"), model.signIn.pendingSignInLink.value)
    }

    @Test
    fun testReleaseBuildRefusesPlainHttpServersAndInvites() = runTest {
        val harness = AppModelHarness(this, isPlainHttpAllowed = false)
        val model = harness.startModel()
        for (server in listOf("http://127.0.0.1:18080", " HTTP://hisar.example.com ")) {
            assertFalse(model.signIn.signIn(server, "alice@example.com", FakeBackend.SECRET_KEY, FakeBackend.PASSWORD))
            assertEquals(SignInModel.PLAIN_HTTP_SERVER_REFUSAL, model.errorMessage.value)
        }
        assertFalse(model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD))
        assertEquals(SignInModel.PLAIN_HTTP_INVITE_REFUSAL, model.errorMessage.value)
        assertFalse(model.signIn.signUpWithoutInvite("http://127.0.0.1:18080", "bob@example.com", "Bob", FakeBackend.PASSWORD))
        assertEquals(SignInModel.PLAIN_HTTP_SERVER_REFUSAL, model.errorMessage.value)
        assertEquals(Screen.SignedOut, model.screen.value)
        assertFalse(
            harness.backend.allCallName.any { it in setOf("signIn", "signUp", "signUpWithoutInvite") },
            "the backend never saw an http:// address",
        )

        assertTrue(model.signIn.signUp("https://hisar.example.com/invite/abc", "bob@example.com", "Bob", FakeBackend.PASSWORD))
        assertEquals(Screen.Unlocked, model.screen.value)
    }

    @Test
    fun testReleaseBuildSignsInOverHttps() = runTest {
        for (server in listOf("https://hisar.example.com", "hisar.example.com")) {
            val model = AppModelHarness(this, isPlainHttpAllowed = false).startModel()
            assertTrue(model.signIn.signIn(server, "alice@example.com", FakeBackend.SECRET_KEY, FakeBackend.PASSWORD), "$server is not refused")
            assertEquals(Screen.Unlocked, model.screen.value)
        }
    }

    @Test
    fun testDebugBuildSignsInAndSignsUpOverPlainHttp() = runTest {
        val model = AppModelHarness(this, isPlainHttpAllowed = true).startModel()
        assertTrue(model.signIn.signUp(inviteLink, "bob@example.com", "Bob", FakeBackend.PASSWORD))
        val openServerModel = AppModelHarness(this, isPlainHttpAllowed = true).startModel()
        assertTrue(openServerModel.signIn.signUpWithoutInvite("http://127.0.0.1:18080", "bob@example.com", "Bob", FakeBackend.PASSWORD))
        val otherModel = AppModelHarness(this, isPlainHttpAllowed = true).startModel()
        assertTrue(otherModel.signIn.signIn("http://127.0.0.1:18080", "alice@example.com", FakeBackend.SECRET_KEY, FakeBackend.PASSWORD))
        assertNull(otherModel.errorMessage.value)
        assertEquals(Screen.Unlocked, otherModel.screen.value)
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
    fun testAnOrganizationInviteLinkOpensItsInvitationInSettings() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.signIn.openInviteLink(InviteLink("abc", "http://127.0.0.1:18080", "organization", "alice@example.com"))
        assertNull(model.signIn.inviteLinkMessage.value)
        assertNull(model.signIn.pendingInviteLink.value)
        assertEquals(Tab.SETTINGS, model.selectedTab.value)
        assertEquals(SettingsDestination.ORGANIZATIONS, model.requestedSettings.value)

        model.requestedSettings.value = null
        model.selectedTab.value = Tab.VAULTS
        model.signIn.openInviteLink(InviteLink("abc", "http://127.0.0.1:18080", "share", "alice@example.com"))
        assertEquals(Tab.SETTINGS, model.selectedTab.value)
        assertEquals(SettingsDestination.PEOPLE, model.requestedSettings.value)
    }

    @Test
    fun testAnInviteLinkForAnotherAccountSaysWhoItIsFor() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.signIn.openInviteLink(InviteLink("abc", "http://127.0.0.1:18080", "organization", "carol@example.com"))
        assertEquals(
            "This invitation is for carol@example.com on http://127.0.0.1:18080. You're signed in as alice@example.com.",
            model.signIn.inviteLinkMessage.value,
        )
        assertEquals(Tab.VAULTS, model.selectedTab.value)
        assertNull(model.requestedSettings.value)

        model.signIn.inviteLinkMessage.value = null
        model.signIn.openInviteLink(InviteLink("abc", "https://other.example.com", "share", "alice@example.com"))
        assertEquals(
            "This invitation is for alice@example.com on other.example.com. You're signed in as alice@example.com.",
            model.signIn.inviteLinkMessage.value,
        )
        assertNull(model.requestedSettings.value)
    }

    @Test
    fun testAnInvitationLinkOpensAfterTheUnlock() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.lock()
        assertEquals(Screen.Locked, model.screen.value)
        model.signIn.openInviteLink(InviteLink("abc", "http://127.0.0.1:18080", "organization", "alice@example.com"))
        assertEquals("Unlock Hisar to see the invitation.", model.notice.value)
        assertNull(model.requestedSettings.value)
        assertEquals(Tab.VAULTS, model.selectedTab.value)

        assertTrue(model.lock.unlock(FakeBackend.PASSWORD))
        assertEquals(Tab.SETTINGS, model.selectedTab.value)
        assertEquals(SettingsDestination.ORGANIZATIONS, model.requestedSettings.value)
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
