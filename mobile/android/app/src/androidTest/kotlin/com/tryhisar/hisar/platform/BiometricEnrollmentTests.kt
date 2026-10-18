package com.tryhisar.hisar.platform

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.backend.createTestBackend
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.VaultStatus
import java.util.regex.Pattern
import kotlin.concurrent.thread
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
    private val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
    private val console = EmulatorConsole.fromArguments()
    private val pin = "1111"
    private val password = "android biometric master password"

    @Before
    fun setUp() {
        device.executeShellCommand("locksettings clear --old $pin")
        device.executeShellCommand("locksettings set-pin $pin")
    }

    @After
    fun tearDown() {
        device.executeShellCommand("locksettings clear --old $pin")
    }

    @Test
    fun testEnrollmentChangeTurnsBiometricsOff() {
        enrollFinger(1)
        val biometric = BiometricKeystore(targetContext, createTemporaryDirectory("biometric"), ForegroundActivity(application) {}, "com.tryhisar.hisar.tests.enrollment.")
        val backend = createTestBackend(biometric)
        launchFocusedActivity().use {
            backend.signUp(
                requireArgument("HISAR_UI_TEST_PLATFORM_BIOMETRIC_INVITE_LINK"),
                requireArgument("HISAR_UI_TEST_PLATFORM_BIOMETRIC_EMAIL"),
                "Android Biometric",
                password,
            )
            assertEquals("turning biometric unlock on shows one prompt", 1, countPrompts { assertTrue(backend.enableBiometricUnlock(password).biometricEnabled) })
            backend.lock()
            assertEquals("unlocking shows one prompt", 1, countPrompts { assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlockWithBiometric().state) })
            backend.lock()
        }

        enrollFinger(2)
        launchFocusedActivity().use {
            var error: BackendError? = null
            val promptCount = countPrompts { error = assertThrows(BackendError::class.java) { backend.unlockWithBiometric() } }
            assertEquals("a key a new fingerprint invalidated is refused before any prompt", 0, promptCount)
            assertTrue(error?.message, error?.message.orEmpty().contains("unlock was turned off"))
            assertFalse(backend.getStatus().biometricEnabled)
            assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlock(password).state)
        }
    }

    /** Runs a backend call on another thread, as the app does, and answers each biometric prompt it shows with finger 1. */
    private fun countPrompts(operation: () -> Unit): Int {
        var failure: Throwable? = null
        val worker = thread { runCatching(operation).onFailure { failure = it } }
        var count = 0
        while (worker.isAlive) {
            if (device.wait(Until.hasObject(negativeButton), 1_000)) {
                count += 1
                console.touchFinger(1)
                assertTrue("the prompt closes after the finger", device.wait(Until.gone(negativeButton), 10_000))
            }
        }
        failure?.let { throw it }
        return count
    }

    /** Adds a fingerprint in Settings, playing finger [fingerID] on the sensor. */
    private fun enrollFinger(fingerID: Int) {
        device.executeShellCommand("am start -a android.settings.FINGERPRINT_ENROLL")
        val passwordEntry = By.res("com.android.settings", "password_entry")
        val sensorPrompt = By.text("Touch the sensor")
        waitUntil(60_000, "Settings asks to touch the sensor") {
            device.findObject(passwordEntry)?.let {
                it.text = pin
                device.pressEnter()
            }
            device.findObject(By.text(Pattern.compile("MORE|I AGREE|NEXT|START", Pattern.CASE_INSENSITIVE)))?.click()
            device.wait(Until.hasObject(sensorPrompt), 1_000)
        }
        val added = By.text("Fingerprint added")
        waitUntil(60_000, "the fingerprint is added") {
            console.touchFinger(fingerID)
            device.wait(Until.hasObject(added), 1_500)
        }
        device.findObject(By.text(Pattern.compile("DONE", Pattern.CASE_INSENSITIVE))).click()
    }

    private companion object {
        val negativeButton = By.text(Pattern.compile("Use master password", Pattern.CASE_INSENSITIVE))
    }
}
