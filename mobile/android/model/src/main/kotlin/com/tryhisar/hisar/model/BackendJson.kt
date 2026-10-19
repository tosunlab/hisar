package com.tryhisar.hisar.model

import kotlin.io.encoding.Base64
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.decodeToByteString
import kotlinx.io.bytestring.encode
import kotlinx.serialization.KSerializer
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.Json

/**
 * The JSON of HisarKit's answers, which are core/app's Go types: unknown keys are skipped, and a `null` or missing value
 * reads as the property's default, as the Swift models' `decodeIfPresent(…) ?? default` do.
 */
val backendJson = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    explicitNulls = false
    encodeDefaults = true
}

/** Go encodes a `[]byte` as standard, padded base64. */
object Base64ByteStringSerializer : KSerializer<ByteString> {
    override val descriptor = PrimitiveSerialDescriptor("com.tryhisar.hisar.model.Base64ByteString", PrimitiveKind.STRING)

    override fun serialize(encoder: Encoder, value: ByteString) = encoder.encodeString(Base64.encode(value))

    override fun deserialize(decoder: Decoder): ByteString = Base64.decodeToByteString(decoder.decodeString())
}
