package com.tryhisar.hisar.model

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.encodeToByteString
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

class PasskeyTests {
    @Test
    fun testBase64URLRoundTrip() {
        val allValue = listOf(ByteString(), ByteString(0xfb.toByte()), ByteString(0xfb.toByte(), 0xff.toByte()), ByteString(0xfb.toByte(), 0xff.toByte(), 1), ByteString(ByteArray(64) { (it * 4).toByte() }))
        for (value in allValue) {
            val encoded = Base64URL.encode(value)
            assertFalse(encoded.contains('+') || encoded.contains('/') || encoded.contains('='), encoded)
            assertEquals(value, Base64URL.decode(encoded))
        }
        assertEquals("-_8", Base64URL.encode(ByteString(0xfb.toByte(), 0xff.toByte())))
        assertNull(Base64URL.decode("not base64!"))
    }

    @Test
    fun testDecodesItemPasskeysFromGo() {
        val json = """{"id":"i1","title":"GitHub","passkeys":[{"credentialId":"AQID","relyingPartyId":"github.com","userName":"alice","userDisplayName":"Alice","createdAt":"2026-10-09T12:00:00Z"}]}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertEquals(listOf(ItemPasskey("AQID", "github.com", "alice", "Alice", "2026-10-09T12:00:00Z")), item.allPasskey)
        assertEquals(Instant.ofEpochSecond(1_791_547_200), item.allPasskey.first().createdDate)
        val withoutPasskeys = backendJson.decodeFromString<ItemDetail>("""{"id":"i1","passkeys":null}""")
        assertEquals(emptyList(), withoutPasskeys.allPasskey)
        val encoded = backendJson.encodeToJsonElement(ItemDetail.serializer(), item).jsonObject
        assertEquals("github.com", encoded.getValue("passkeys").jsonArray.first().jsonObject.getValue("relyingPartyId").jsonPrimitive.content)
    }

    @Test
    fun testDecodesStoredPasskeysAndRegistrationAnswers() {
        val allPasskey = backendJson.decodeFromString<List<StoredPasskey>>(
            """[{"credentialId":"AQID","relyingPartyId":"github.com","userName":"alice","userDisplayName":"","userHandle":"YWxpY2U","vaultID":"v","itemID":"i","title":"GitHub"}]""",
        )
        assertEquals("v/i", allPasskey.first().recordIdentifier)
        assertEquals("alice".encodeToByteString(), Base64URL.decode(allPasskey.first().userHandle))
        val options = backendJson.decodeFromString<PasskeyRegistrationOptions>(
            """{"isExcluded":false,"logins":[{"vaultID":"v","itemID":"i","title":"GitHub","username":"alice","vaultName":"Personal","hasOneTimePassword":false}]}""",
        )
        assertEquals(listOf("v/i"), options.allLogin.map { it.id })
        val registration = backendJson.decodeFromString<PasskeyRegistrationResult>(
            """{"credentialId":"AQID","attestationObject":"o2Nm","authenticatorData":"AA","vaultID":"v","itemID":"i"}""",
        )
        val credential = PasskeyRegistrationCredential.create("github.com", ByteString(9), registration)
        assertEquals(ByteString(1, 2, 3), credential?.credentialID)
        assertEquals(ByteString(0xa3.toByte(), 0x63, 0x66), credential?.attestationObject)
        val assertion = PasskeyAssertionResult(credentialID = "AQID", authenticatorData = "AA", signature = "MEU", userHandle = "YWxpY2U")
        assertEquals("alice".encodeToByteString(), PasskeyAssertionCredential.create("github.com", ByteString(9), assertion)?.userHandle)
        assertNull(PasskeyAssertionCredential.create("github.com", ByteString(9), PasskeyAssertionResult("!", "AA", "MEU", "")))
    }

    @Test
    fun testRegistrationNeedsES256() {
        var request = PasskeyRegistrationRequest("github.com", "alice", ByteString(1), ByteString())
        assertTrue(request.isAlgorithmSupported, "an empty list leaves the choice to the authenticator")
        request = request.copy(allSupportedAlgorithm = listOf(-8, -257))
        assertFalse(request.isAlgorithmSupported)
        request = request.copy(allSupportedAlgorithm = listOf(-257, -7))
        assertTrue(request.isAlgorithmSupported)
    }
}
