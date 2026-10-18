package com.tryhisar.hisar.backend

import com.tryhisar.hisar.BuildConfig
import com.tryhisar.hisar.platform.BiometricKeystore
import com.tryhisar.hisar.platform.KeystoreSecretKeyStore
import com.tryhisar.hisar.platform.TrustedCertificates
import com.tryhisar.hisar.platform.createTemporaryDirectory

/** A Go backend with its own state and Secret Key directory, as the app builds it. */
fun createTestBackend(
    biometric: BiometricKeystore? = null,
    trustedCertificates: String = TrustedCertificates.readPem(),
): HisarKitBackend = HisarKitBackend(
    stateDirectory = createTemporaryDirectory("state"),
    secretKeyStore = KeystoreSecretKeyStore(createTemporaryDirectory("secret-keys"), "com.tryhisar.hisar.tests.go"),
    biometric = biometric,
    clipboard = null,
    eventListener = null,
    appVersion = BuildConfig.VERSION_NAME,
    trustedCertificates = trustedCertificates,
)
