package com.tryhisar.hisar.passkey

import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.PasskeyAssertionCredential
import com.tryhisar.hisar.model.PasskeyRegistrationCredential
import java.security.KeyPairGenerator
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.encodeToByteString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** The request and response JSON of Credential Manager, with the shapes Chrome sent and accepted on API 37. */
class WebAuthnJsonTests {
    private val clientDataHash = ByteString(ByteArray(32) { it.toByte() })

    @Test
    fun testReadsChromesGetRequest() {
        val request = WebAuthnJson.parseGetRequest(
            """{"allowCredentials":[{"id":"Pe6VusRxr_xkSV11bqnJSw","transports":["hybrid","internal"],"type":"public-key"}],"challenge":"sbcPpuj7ryfBV343OcinxKNIf-RCV1U85PPjF-MsscRYB88gYC5L-vGs4HUWbzF13HyUOgqAPY7mD8Y9LPWpiA","rpId":"webauthn.io","timeout":180000,"userVerification":"preferred"}""",
        )
        assertEquals("webauthn.io", request?.relyingPartyID)
        assertEquals("sbcPpuj7ryfBV343OcinxKNIf-RCV1U85PPjF-MsscRYB88gYC5L-vGs4HUWbzF13HyUOgqAPY7mD8Y9LPWpiA", request?.challenge)
        assertEquals<List<ByteString?>?>(listOf(Base64URL.decode("Pe6VusRxr_xkSV11bqnJSw")), request?.allAllowedCredentialID)
        assertEquals(WebAuthnGetRequest("", "Y2hhbGxlbmdl", emptyList()), WebAuthnJson.parseGetRequest("""{"challenge":"Y2hhbGxlbmdl"}"""), "a missing rpId means the caller's own")
    }

    @Test
    fun testReadsChromesCreateRequest() {
        val request = WebAuthnJson.parseCreateRequest(
            """{"attestation":"none","authenticatorSelection":{"residentKey":"preferred","userVerification":"preferred"},"challenge":"na2IK-EFOv2vFLOfNP1RPIakFQpNk-KeaG2A3_betRGZlSTVJiW8qRVnPybY9hg2aFq8zhc9RLr0EIK2BUTeFQ","excludeCredentials":[],"extensions":{"credProps":true},"pubKeyCredParams":[{"alg":-8,"type":"public-key"},{"alg":-7,"type":"public-key"},{"alg":-257,"type":"public-key"}],"rp":{"id":"webauthn.io","name":"webauthn.io"},"timeout":180000,"user":{"displayName":"hisarspike38936","id":"d2ViYXV0aG5pby1oaXNhcnNwaWtlMzg5MzY","name":"hisarspike38936"}}""",
        )
        assertEquals("webauthn.io", request?.relyingPartyID)
        assertEquals("hisarspike38936", request?.userName)
        assertEquals("webauthnio-hisarspike38936".encodeToByteString(), request?.userHandle)
        assertEquals(listOf(-8, -7, -257), request?.allAlgorithm)
        assertEquals(emptyList(), request?.allExcludedCredentialID)
    }

    @Test
    fun testReadsDiscouragedUserVerification() {
        assertEquals(false, WebAuthnJson.parseGetRequest("""{"challenge":"AQ","userVerification":"preferred"}""")?.isUserVerificationDiscouraged)
        assertEquals(false, WebAuthnJson.parseGetRequest("""{"challenge":"AQ"}""")?.isUserVerificationDiscouraged, "WebAuthn's default is preferred")
        assertEquals(true, WebAuthnJson.parseGetRequest("""{"challenge":"AQ","userVerification":"discouraged"}""")?.isUserVerificationDiscouraged)
        val create = """{"rp":{"id":"example.com"},"challenge":"AQ","user":{"id":"AQ","name":"a"},"pubKeyCredParams":[]%s}"""
        assertEquals(false, WebAuthnJson.parseCreateRequest(create.format(""))?.isUserVerificationDiscouraged)
        assertEquals(false, WebAuthnJson.parseCreateRequest(create.format(""","authenticatorSelection":{"userVerification":"required"}"""))?.isUserVerificationDiscouraged)
        assertEquals(true, WebAuthnJson.parseCreateRequest(create.format(""","authenticatorSelection":{"userVerification":"discouraged"}"""))?.isUserVerificationDiscouraged)
    }

    @Test
    fun testUnreadableRequestsAreRefused() {
        assertNull(WebAuthnJson.parseGetRequest("not json"))
        assertNull(WebAuthnJson.parseGetRequest("""{"rpId":"example.com"}"""), "no challenge")
        assertNull(WebAuthnJson.parseGetRequest("""{"rpId":"example.com","challenge":"AQ","allowCredentials":[{"id":"not base64!"}]}"""))
        assertNull(WebAuthnJson.parseCreateRequest("""{"rp":{"id":"example.com"},"challenge":"AQ","pubKeyCredParams":[]}"""), "no user")
        assertNull(WebAuthnJson.parseCreateRequest("""{"rp":{"id":"example.com"},"challenge":"AQ","user":{"id":"","name":"a"},"pubKeyCredParams":[]}"""), "an empty user handle")
    }

    @Test
    fun testAuthenticationResponseCarriesClientDataOnlyWhenHisarWroteIt() {
        val credential = PasskeyAssertionCredential("example.com", clientDataHash, ByteString(1, 2, 3), ByteString(4), ByteString(5), "alice".encodeToByteString())
        val forBrowser = parse(WebAuthnJson.buildAuthenticationResponse(credential, clientDataJSON = null))
        assertEquals("AQID", forBrowser.getValue("id").jsonPrimitive.content)
        assertEquals("AQID", forBrowser.getValue("rawId").jsonPrimitive.content)
        assertEquals("platform", forBrowser.getValue("authenticatorAttachment").jsonPrimitive.content)
        val browserResponse = forBrowser.getValue("response").jsonObject
        assertFalse("clientDataJSON" in browserResponse, "Chrome writes its own client data")
        assertEquals("BA", browserResponse.getValue("authenticatorData").jsonPrimitive.content)
        assertEquals("BQ", browserResponse.getValue("signature").jsonPrimitive.content)
        assertEquals("YWxpY2U", browserResponse.getValue("userHandle").jsonPrimitive.content)

        val clientDataJSON = """{"type":"webauthn.get"}""".encodeToByteString()
        val forApp = parse(WebAuthnJson.buildAuthenticationResponse(credential, clientDataJSON)).getValue("response").jsonObject
        assertEquals(Base64URL.encode(clientDataJSON), forApp.getValue("clientDataJSON").jsonPrimitive.content)
    }

    @Test
    fun testRegistrationResponsePublishesTheKeyOfTheAuthenticatorData() {
        val keyPair = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()
        val publicKey = keyPair.public as ECPublicKey
        val credentialID = byteArrayOf(7, 7, 7)
        val authenticatorData = ByteString(
            ByteArray(32) + byteArrayOf(0x5d, 0, 0, 0, 0) + ByteArray(16) + byteArrayOf(0, credentialID.size.toByte()) + credentialID +
                encodeCoseKey(toFixed(publicKey.w.affineX.toByteArray()), toFixed(publicKey.w.affineY.toByteArray())),
        )
        val credential = PasskeyRegistrationCredential("example.com", clientDataHash, ByteString(credentialID), "attestation".encodeToByteString(), authenticatorData)
        val response = parse(assertNotNull(WebAuthnJson.buildRegistrationResponse(credential, clientDataJSON = null))).getValue("response").jsonObject
        assertEquals(ByteString(publicKey.encoded), Base64URL.decode(response.getValue("publicKey").jsonPrimitive.content), "the SubjectPublicKeyInfo Java writes")
        assertEquals(-7, response.getValue("publicKeyAlgorithm").jsonPrimitive.int)
        assertEquals(listOf("internal", "hybrid"), response.getValue("transports").jsonArray.map { it.jsonPrimitive.content })
        assertEquals(Base64URL.encode(authenticatorData), response.getValue("authenticatorData").jsonPrimitive.content)
        assertEquals(Base64URL.encode("attestation".encodeToByteString()), response.getValue("attestationObject").jsonPrimitive.content)
        assertFalse("clientDataJSON" in response)

        val withoutKey = credential.copy(authenticatorData = ByteString(ByteArray(32) + byteArrayOf(0x1d, 0, 0, 0, 0)))
        assertNull(WebAuthnJson.buildRegistrationResponse(withoutKey, clientDataJSON = null))
    }

    private fun parse(text: String): JsonObject = Json.parseToJsonElement(text).jsonObject

    private fun toFixed(bytes: ByteArray): ByteArray = bytes.takeLast(32).toByteArray().let { ByteArray(32 - it.size) + it }

    // The COSE key Go writes: {1: 2, 3: -7, -1: 1, -2: x, -3: y}.
    private fun encodeCoseKey(x: ByteArray, y: ByteArray): ByteArray =
        byteArrayOf(0xa5.toByte(), 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20) + x + byteArrayOf(0x22, 0x58, 0x20) + y
}
