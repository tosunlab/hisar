package com.tryhisar.hisar.platform

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.SdkSuppress
import com.tryhisar.hisar.backend.createTestBackend
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.VaultStatus
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class BiometricKeystoreTests {
    private val directory = createTemporaryDirectory("biometric-tests")
    private val biometric = BiometricKeystore(targetContext, directory, ForegroundActivity(application) {}, "com.tryhisar.hisar.tests.biometric.")

    @After
    fun tearDown() {
        directory.deleteRecursively()
    }

    @Test
    fun testDeleteMissingKeyIsFine() {
        biometric.deleteKey("NOBODY")
        assertThrows("Go turns biometric unlock off when the key is gone", Exception::class.java) { biometric.prepareLoadKey("NOBODY") }
        assertThrows(Exception::class.java) { biometric.loadKey("NOBODY") }
    }

    @Test
    fun testPromptNeedsHisarInFront() {
        val error = assertThrows(Exception::class.java) { biometric.authenticate("unlock Hisar") }
        assertTrue(error.message.orEmpty(), error.message.orEmpty().startsWith("Open Hisar"))
    }

    /**
     * On a real Keystore with a fingerprint enrolled in Settings and played on the emulator's console. API 37 only: the API 29
     * emulator's fingerprint HAL never changes its authenticator ID, so a new enrollment cannot invalidate a key there.
     */
    @Test
    @SdkSuppress(minSdkVersion = 37)
    fun testEnrollmentChangeTurnsBiometricsOff() {
        val fingerprints = EmulatorFingerprints()
        val password = "android biometric master password"
        fingerprints.setPin()
        try {
            fingerprints.enroll(1)
            val biometric = BiometricKeystore(targetContext, createTemporaryDirectory("biometric"), ForegroundActivity(application) {}, "com.tryhisar.hisar.tests.enrollment.")
            val backend = createTestBackend(biometric)
            launchFocusedActivity().use {
                backend.signUp(
                    requireArgument("HISAR_UI_TEST_PLATFORM_BIOMETRIC_INVITE_LINK"),
                    requireArgument("HISAR_UI_TEST_PLATFORM_BIOMETRIC_EMAIL"),
                    "Android Biometric",
                    password,
                )
                assertEquals("turning biometric unlock on shows one prompt", 1, fingerprints.countPrompts { assertTrue(backend.enableBiometricUnlock(password).biometricEnabled) })
                backend.lock()
                assertEquals("unlocking shows one prompt", 1, fingerprints.countPrompts { assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlockWithBiometric().state) })
                backend.lock()
            }

            fingerprints.enroll(2)
            launchFocusedActivity().use {
                var error: BackendError? = null
                val promptCount = fingerprints.countPrompts { error = assertThrows(BackendError::class.java) { backend.unlockWithBiometric() } }
                assertEquals("a key a new fingerprint invalidated is refused before any prompt", 0, promptCount)
                assertTrue(error?.message, error?.message.orEmpty().contains("unlock was turned off"))
                assertFalse(backend.getStatus().biometricEnabled)
                assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlock(password).state)
            }
        } finally {
            fingerprints.clearPin()
        }
    }
}
