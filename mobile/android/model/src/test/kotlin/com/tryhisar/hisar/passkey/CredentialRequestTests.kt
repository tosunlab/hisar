package com.tryhisar.hisar.passkey

import com.tryhisar.hisar.autofill.AndroidApp
import com.tryhisar.hisar.autofill.FillTarget
import com.tryhisar.hisar.autofill.LoginSource
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.decodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** What Hisar's credential provider offers each Credential Manager caller, and what it refuses. */
class CredentialRequestTests {
    private val backend = FakeBackend()
    private val allVouchingRelyingParty = mutableMapOf<String, AndroidApp>()
    private val allAskedRelyingParty = mutableListOf<String>()
    private val access = CredentialAccess(
        backend,
        relyingPartyVerifier = { relyingPartyID, app ->
            allAskedRelyingParty += relyingPartyID
            allVouchingRelyingParty[relyingPartyID] == app
        },
        appWebsiteVerifier = { _, _ -> emptyList() },
    )
    private val chrome = CredentialCaller.Browser("com.android.chrome", "https://github.com/")
    private val gitHubApp = AndroidApp("com.github.android", "AB:".repeat(31) + "AB")
    private val unrelatedApp = AndroidApp("com.example.unrelated", "CD:".repeat(31) + "CD")

    private fun passkeyOption(relyingPartyID: String, vararg allAllowedCredentialID: ByteString) =
        CredentialOption.Passkey(WebAuthnGetRequest(relyingPartyID, "Y2hhbGxlbmdl", allAllowedCredentialID.toList()))

    @Test
    fun testBrowserGetsThePasskeysAndLoginsOfItsPage() {
        backend.seedSignedIn()
        val offer = assertIs<CredentialOffer.Entries>(access.offer(chrome, listOf(passkeyOption("github.com"), CredentialOption.Password)))
        assertEquals(listOf(0 to "GitHub"), offer.allPasskey.map { it.optionIndex to it.value.title })
        assertEquals(listOf(1 to "GitHub"), offer.allLogin.map { it.optionIndex to it.value.login.title })
        assertEquals(LoginSource.Website("https://github.com/"), offer.allLogin.single().value.source)
        assertTrue(allAskedRelyingParty.isEmpty(), "a privileged browser's origin needs no Digital Asset Links")
    }

    @Test
    fun testBrowserPageOfAnotherSiteIsRefused() {
        backend.seedSignedIn()
        val evilPage = CredentialCaller.Browser("com.android.chrome", "https://evil.example/")
        assertEquals(CredentialOffer.Nothing, access.offer(evilPage, listOf(passkeyOption("github.com"))))
        assertFalse("listAllPasskeyForRelyingParty" in backend.allCallName)
        assertNull(access.resolveRelyingParty(evilPage, "github.com"))
        assertEquals("github.com", access.resolveRelyingParty(CredentialCaller.Browser("com.android.chrome", "https://gist.github.com/"), "github.com"))
        assertEquals("github.com", access.resolveRelyingParty(chrome, ""), "a request without rpId uses the page's host")
    }

    @Test
    fun testUnrelatedAppIsRefused() {
        backend.seedSignedIn()
        allVouchingRelyingParty["github.com"] = gitHubApp
        val unrelated = CredentialCaller.App(unrelatedApp, "Unrelated")
        assertEquals(CredentialOffer.Nothing, access.offer(unrelated, listOf(passkeyOption("github.com"))))
        assertEquals(listOf("github.com"), allAskedRelyingParty, "the relying party's own statement decides")
        assertFalse("listAllPasskeyForRelyingParty" in backend.allCallName, "nothing about the vault is read for it")
        assertNull(access.resolveRelyingParty(CredentialCaller.App(gitHubApp.copy(certificateSHA256 = unrelatedApp.certificateSHA256), "GitHub"), "github.com"), "the package name alone is not enough")
    }

    @Test
    fun testAppRelyingPartyThatIsNotADomainIsRefusedBeforeAnyRequest() {
        backend.seedSignedIn()
        val caller = CredentialCaller.App(gitHubApp, "GitHub")
        for (relyingPartyID in listOf("evil.example/x?", "user@evil.example", "evil.example:8443", "evil.example#", "127.0.0.1", "github.com.", " ")) {
            assertNull(access.resolveRelyingParty(caller, relyingPartyID), relyingPartyID)
        }
        assertTrue(allAskedRelyingParty.isEmpty(), "asked $allAskedRelyingParty")
    }

    @Test
    fun testAppTheRelyingPartyVouchesForGetsItsPasskeysAndHisarsClientData() {
        backend.seedSignedIn()
        allVouchingRelyingParty["github.com"] = gitHubApp
        val caller = CredentialCaller.App(gitHubApp, "GitHub")
        val offer = assertIs<CredentialOffer.Entries>(access.offer(caller, listOf(passkeyOption("GitHub.com"))))
        assertEquals(listOf("GitHub"), offer.allPasskey.map { it.value.title })

        val clientData = assertNotNull(access.buildClientData(caller, "webauthn.get", "Y2hhbGxlbmdl", browserHash = null))
        val fields = Json.parseToJsonElement(assertNotNull(clientData.json).decodeToString()).jsonObject
        assertEquals("android:apk-key-hash:q6urq6urq6urq6urq6urq6urq6urq6urq6urq6urq6s", fields.getValue("origin").jsonPrimitive.content)
        assertEquals("com.github.android", fields.getValue("androidPackageName").jsonPrimitive.content)
        assertEquals("Y2hhbGxlbmdl", fields.getValue("challenge").jsonPrimitive.content)
        assertEquals(32, clientData.hash.size)
    }

    @Test
    fun testBrowserClientDataIsItsOwnHash() {
        val hash = ByteString(ByteArray(32) { 3 })
        assertEquals(AnswerClientData(hash, null), access.buildClientData(chrome, "webauthn.get", "Y2hhbGxlbmdl", hash))
        assertNull(access.buildClientData(chrome, "webauthn.get", "Y2hhbGxlbmdl", browserHash = null), "a browser that sends no hash is refused")
        assertFalse("buildPasskeyClientData" in backend.allCallName)
    }

    @Test
    fun testAllowedCredentialsFilterThePasskeys() {
        backend.seedSignedIn()
        val offer = assertIs<CredentialOffer.Entries>(access.offer(chrome, listOf(passkeyOption("github.com", ByteString(9)))))
        assertTrue(offer.allPasskey.isEmpty())
        val allowed = assertIs<CredentialOffer.Entries>(access.offer(chrome, listOf(passkeyOption("github.com", ByteString(9), ByteString(1, 1, 1)))))
        assertEquals(1, allowed.allPasskey.size)
    }

    @Test
    fun testLockedHisarOffersTheUnlockActionAndSignedOutOffersNothing() {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        assertEquals(CredentialOffer.Unlock, access.offer(chrome, listOf(passkeyOption("github.com"))))
        backend.status = VaultStatus.signedOut
        assertEquals(CredentialOffer.Nothing, access.offer(chrome, listOf(passkeyOption("github.com"), CredentialOption.Password)))
    }

    @Test
    fun testPasswordsForAnAppComeFromItsLinkedLogins() {
        backend.seedSignedIn()
        backend.addAndroidApp("personal", "bank", gitHubApp.packageName, gitHubApp.certificateSHA256)
        val caller = CredentialCaller.App(gitHubApp, "GitHub")
        assertEquals(FillTarget.App(gitHubApp, "GitHub"), access.buildFillTarget(caller))
        val offer = assertIs<CredentialOffer.Entries>(access.offer(caller, listOf(CredentialOption.Password)))
        assertEquals(listOf("Bank"), offer.allLogin.map { it.value.login.title })
        assertEquals(LoginSource.App(gitHubApp), offer.allLogin.single().value.source)
        assertTrue(allAskedRelyingParty.isEmpty(), "a password request names no relying party")
    }
}
