package com.tryhisar.hisar.platform

import java.security.KeyStore
import java.util.Base64

/**
 * The device's trusted CA certificates as PEM, system and user-installed alike. Go reads only the system folders on
 * Android, so without these a server behind a private CA the user installed would be refused. Reading the store takes
 * up to half a second, so call it off the main thread.
 */
object TrustedCertificates {
    fun readPem(): String {
        val store = KeyStore.getInstance("AndroidCAStore").apply { load(null) }
        val encoder = Base64.getMimeEncoder(64, "\n".toByteArray())
        return store.aliases().asSequence().mapNotNull { store.getCertificate(it) }.joinToString("") { certificate ->
            "-----BEGIN CERTIFICATE-----\n${encoder.encodeToString(certificate.encoded)}\n-----END CERTIFICATE-----\n"
        }
    }
}
