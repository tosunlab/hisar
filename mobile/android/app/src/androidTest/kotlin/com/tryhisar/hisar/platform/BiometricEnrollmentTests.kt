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
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Biometric unlock on a real Keystore with an enrolled fingerprint, which the test enrolls in Settings and plays on the
 * emulator's console. API 37 only: the API 29 emulator's fingerprint HAL never changes its authenticator ID, so a new
 * enrollment cannot invalidate a key there.
 */
@SdkSuppress(minSdkVersion = 37)
@RunWith(AndroidJUnit4::class)
class BiometricEnrollmentTests {
    private val fingerprints = EmulatorFingerprints()
    private val password = "android biometric master password"

    @Before
    fun setUp() = fingerprints.setPin()

    @After
    fun tearDown() = fingerprints.clearPin()

    @Test
    fun testEnrollmentChangeTurnsBiometricsOff() {
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
    }

    @Test
    fun testChangingCredentialsKeepsBiometricUnlockWithOnePromptEach() {
        fingerprints.enroll(1)
        val biometric = BiometricKeystore(targetContext, createTemporaryDirectory("biometric"), ForegroundActivity(application) {}, "com.tryhisar.hisar.tests.credentials.")
        val backend = createTestBackend(biometric)
        val newPassword = "android biometric new master password"
        launchFocusedActivity().use {
            backend.signUp(
                requireArgument("HISAR_UI_TEST_CREDENTIAL_CHANGE_INVITE_LINK"),
                requireArgument("HISAR_UI_TEST_CREDENTIAL_CHANGE_EMAIL"),
                "Android Credential Change",
                password,
            )
            assertEquals(1, fingerprints.countPrompts { assertTrue(backend.enableBiometricUnlock(password).biometricEnabled) })

            val passwordPromptCount = fingerprints.countPrompts { backend.changeMasterPassword(password, newPassword) }
            assertEquals("changing the master password asks for one fingerprint to keep biometric unlock", 1, passwordPromptCount)
            assertTrue(backend.getStatus().biometricEnabled)
            backend.lock()
            assertEquals(1, fingerprints.countPrompts { assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlockWithBiometric().state) })

            val secretKeyPromptCount = fingerprints.countPrompts { backend.replaceSecretKey(newPassword, createTemporaryDirectory("emergency-kit")) }
            assertEquals("replacing the Secret Key asks for one fingerprint to keep biometric unlock", 1, secretKeyPromptCount)
            assertTrue(backend.getStatus().biometricEnabled)
            backend.lock()
            assertEquals(1, fingerprints.countPrompts { assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlockWithBiometric().state) })
        }
    }
}
