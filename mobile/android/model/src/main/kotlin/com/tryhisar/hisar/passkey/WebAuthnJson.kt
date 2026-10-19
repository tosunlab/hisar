package com.tryhisar.hisar.passkey

import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.PasskeyAssertionCredential
import com.tryhisar.hisar.model.PasskeyRegistrationCredential
import kotlinx.io.bytestring.ByteString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject

/**
 * A `navigator.credentials.get` request as Credential Manager passes it; an empty relying party ID means the caller's.
 * [isUserVerificationDiscouraged] when its `userVerification` is `discouraged`; WebAuthn's default is `preferred`.
 */
data class WebAuthnGetRequest(
    val relyingPartyID: String,
    val challenge: String,
    val allAllowedCredentialID: List<ByteString>,
    val isUserVerificationDiscouraged: Boolean = false,
)

/** A `navigator.credentials.create` request as Credential Manager passes it, with `authenticatorSelection.userVerification`. */
data class WebAuthnCreateRequest(
    val relyingPartyID: String,
    val userName: String,
    val userHandle: ByteString,
    val challenge: String,
    val allAlgorithm: List<Int>,
    val allExcludedCredentialID: List<ByteString>,
    val isUserVerificationDiscouraged: Boolean = false,
)

/**
 * The WebAuthn JSON of Credential Manager: the caller's `requestJson`, and the registration and authentication responses
 * Hisar returns. A response carries `clientDataJSON` only when Hisar wrote the client data; a browser makes its own.
 */
object WebAuthnJson {
    private val transports = buildJsonArray {
        add(JsonPrimitive("internal"))
        add(JsonPrimitive("hybrid"))
    }
    private const val USER_VERIFICATION_DISCOURAGED = "discouraged"

    /** The request, or null when it is not a WebAuthn get request. */
    fun parseGetRequest(requestJSON: String): WebAuthnGetRequest? {
        val request = parseObject(requestJSON) ?: return null
        return WebAuthnGetRequest(
            relyingPartyID = readString(request["rpId"]).orEmpty(),
            challenge = readString(request["challenge"])?.takeIf { it.isNotEmpty() } ?: return null,
            allAllowedCredentialID = readAllCredentialID(request["allowCredentials"]) ?: return null,
            isUserVerificationDiscouraged = readString(request["userVerification"]) == USER_VERIFICATION_DISCOURAGED,
        )
    }

    /** The request, or null when it is not a WebAuthn create request. */
    fun parseCreateRequest(requestJSON: String): WebAuthnCreateRequest? {
        val request = parseObject(requestJSON) ?: return null
        val user = request["user"] as? JsonObject ?: return null
        val allParameter = request["pubKeyCredParams"] as? JsonArray ?: return null
        return WebAuthnCreateRequest(
            relyingPartyID = readString((request["rp"] as? JsonObject)?.get("id")).orEmpty(),
            userName = readString(user["name"]).orEmpty(),
            userHandle = readString(user["id"])?.let(Base64URL::decode)?.takeIf { it.size > 0 } ?: return null,
            challenge = readString(request["challenge"])?.takeIf { it.isNotEmpty() } ?: return null,
            allAlgorithm = allParameter.mapNotNull { ((it as? JsonObject)?.get("alg") as? JsonPrimitive)?.intOrNull },
            allExcludedCredentialID = readAllCredentialID(request["excludeCredentials"]) ?: return null,
            isUserVerificationDiscouraged = readString((request["authenticatorSelection"] as? JsonObject)?.get("userVerification")) == USER_VERIFICATION_DISCOURAGED,
        )
    }

    fun buildAuthenticationResponse(credential: PasskeyAssertionCredential, clientDataJSON: ByteString?): String {
        val credentialID = Base64URL.encode(credential.credentialID)
        return buildJsonObject {
            put("id", credentialID)
            put("rawId", credentialID)
            put("type", "public-key")
            put("authenticatorAttachment", "platform")
            putJsonObject("response") {
                if (clientDataJSON != null) put("clientDataJSON", Base64URL.encode(clientDataJSON))
                put("authenticatorData", Base64URL.encode(credential.authenticatorData))
                put("signature", Base64URL.encode(credential.signature))
                put("userHandle", Base64URL.encode(credential.userHandle))
            }
            putJsonObject("clientExtensionResults") {}
        }.toString()
    }

    /** The response, or null when the authenticator data holds no ES256 key to publish. */
    fun buildRegistrationResponse(credential: PasskeyRegistrationCredential, clientDataJSON: ByteString?): String? {
        val publicKey = readPublicKey(credential.authenticatorData) ?: return null
        val credentialID = Base64URL.encode(credential.credentialID)
        return buildJsonObject {
            put("id", credentialID)
            put("rawId", credentialID)
            put("type", "public-key")
            put("authenticatorAttachment", "platform")
            putJsonObject("response") {
                if (clientDataJSON != null) put("clientDataJSON", Base64URL.encode(clientDataJSON))
                put("attestationObject", Base64URL.encode(credential.attestationObject))
                put("transports", transports)
                put("authenticatorData", Base64URL.encode(credential.authenticatorData))
                put("publicKeyAlgorithm", ALGORITHM_ES256)
                put("publicKey", Base64URL.encode(publicKey))
            }
            putJsonObject("clientExtensionResults") {
                putJsonObject("credProps") { put("rk", true) }
            }
        }.toString()
    }

    /**
     * The credential's P-256 key as a SubjectPublicKeyInfo, read from the COSE key after the credential ID in the
     * authenticator data (WebAuthn §6.5.1), or null when the data holds none.
     */
    fun readPublicKey(authenticatorData: ByteString): ByteString? {
        val bytes = authenticatorData.toByteArray()
        if (bytes.size < CREDENTIAL_ID_OFFSET || bytes[FLAGS_OFFSET].toInt() and ATTESTED_CREDENTIAL_DATA == 0) return null
        val credentialIDLength = (bytes[CREDENTIAL_ID_OFFSET - 2].toInt() and 0xff shl 8) or (bytes[CREDENTIAL_ID_OFFSET - 1].toInt() and 0xff)
        val key = CoseKeyReader(bytes, CREDENTIAL_ID_OFFSET + credentialIDLength).readMap() ?: return null
        if (key[COSE_KEY_TYPE] != COSE_KEY_TYPE_EC2 || key[COSE_ALGORITHM] != ALGORITHM_ES256.toLong() || key[COSE_CURVE] != COSE_CURVE_P256) return null
        val x = key[COSE_X] as? ByteArray ?: return null
        val y = key[COSE_Y] as? ByteArray ?: return null
        if (x.size != 32 || y.size != 32) return null
        return ByteString(P256_SPKI_PREFIX + byteArrayOf(0x04) + x + y)
    }

    private fun parseObject(text: String): JsonObject? = try {
        Json.parseToJsonElement(text) as? JsonObject
    } catch (_: IllegalArgumentException) {
        null
    }

    private fun readString(element: JsonElement?): String? = (element as? JsonPrimitive)?.takeIf { it.isString }?.content

    // A missing list allows every credential; a list with an ID that is not base64url makes the request unreadable.
    private fun readAllCredentialID(element: JsonElement?): List<ByteString>? {
        val allDescriptor = element as? JsonArray ?: return emptyList()
        return allDescriptor.map { descriptor -> readString((descriptor as? JsonObject)?.get("id"))?.let(Base64URL::decode) ?: return null }
    }

    private const val ALGORITHM_ES256 = -7
    private const val FLAGS_OFFSET = 32
    private const val ATTESTED_CREDENTIAL_DATA = 0x40
    private const val CREDENTIAL_ID_OFFSET = 32 + 1 + 4 + 16 + 2
    private const val COSE_KEY_TYPE = 1L
    private const val COSE_ALGORITHM = 3L
    private const val COSE_CURVE = -1L
    private const val COSE_X = -2L
    private const val COSE_Y = -3L
    private const val COSE_KEY_TYPE_EC2 = 2L
    private const val COSE_CURVE_P256 = 1L

    /** The DER of SEQUENCE { SEQUENCE { id-ecPublicKey, prime256v1 }, BIT STRING } up to the uncompressed point. */
    private val P256_SPKI_PREFIX = byteArrayOf(
        0x30, 0x59, 0x30, 0x13, 0x06, 0x07, 0x2a, 0x86.toByte(), 0x48, 0xce.toByte(), 0x3d, 0x02, 0x01,
        0x06, 0x08, 0x2a, 0x86.toByte(), 0x48, 0xce.toByte(), 0x3d, 0x03, 0x01, 0x07, 0x03, 0x42, 0x00,
    )
}

/** Reads a COSE key: one CBOR map of small integer keys to integers or byte strings. */
private class CoseKeyReader(private val bytes: ByteArray, private var offset: Int) {
    fun readMap(): Map<Long, Any>? {
        val (major, count) = readHead() ?: return null
        if (major != MAJOR_MAP) return null
        return (0 until count).associate { _ -> (readValue() as? Long ?: return null) to (readValue() ?: return null) }
    }

    private fun readValue(): Any? {
        val (major, argument) = readHead() ?: return null
        return when (major) {
            MAJOR_UNSIGNED -> argument.toLong()
            MAJOR_NEGATIVE -> -1L - argument
            MAJOR_BYTES -> if (offset + argument > bytes.size) null else bytes.copyOfRange(offset, offset + argument).also { offset += argument }
            else -> null
        }
    }

    private fun readHead(): Pair<Int, Int>? {
        if (offset >= bytes.size) return null
        val initial = bytes[offset++].toInt() and 0xff
        val additional = initial and 0x1f
        val argument = when {
            additional < 24 -> additional
            additional == 24 && offset < bytes.size -> bytes[offset++].toInt() and 0xff
            additional == 25 && offset + 1 < bytes.size -> ((bytes[offset].toInt() and 0xff) shl 8 or (bytes[offset + 1].toInt() and 0xff)).also { offset += 2 }
            else -> return null
        }
        return (initial shr 5) to argument
    }

    private companion object {
        const val MAJOR_UNSIGNED = 0
        const val MAJOR_NEGATIVE = 1
        const val MAJOR_BYTES = 2
        const val MAJOR_MAP = 5
    }
}
