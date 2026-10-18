package com.tryhisar.hisar.platform

import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import java.util.regex.Pattern
import kotlin.concurrent.thread

/**
 * The emulator's fingerprint sensor: a screen-lock PIN (fingerprints need one), fingers enrolled in Settings, and
 * fingers played on Hisar's biometric prompts through the emulator console.
 */
class EmulatorFingerprints {
    val device: UiDevice = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
    private val console = EmulatorConsole.fromArguments()

    fun setPin() {
        clearPin()
        device.executeShellCommand("locksettings set-pin $PIN")
    }

    fun clearPin() {
        device.executeShellCommand("locksettings clear --old $PIN")
    }

    /** Adds a fingerprint in Settings, playing finger [fingerID] on the sensor. */
    fun enroll(fingerID: Int) {
        device.executeShellCommand("am start -a android.settings.FINGERPRINT_ENROLL")
        val passwordEntry = By.res("com.android.settings", "password_entry")
        val sensorPrompt = By.text("Touch the sensor")
        waitUntil(60_000, "Settings asks to touch the sensor") {
            device.findObject(passwordEntry)?.let {
                it.text = PIN
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

    /** Whether Hisar's biometric prompt shows within [timeoutMillis]. */
    fun isPromptShown(timeoutMillis: Long): Boolean = device.wait(Until.hasObject(promptButton), timeoutMillis)

    /** Plays finger [fingerID] on the prompt that is showing, and waits for it to close. */
    fun touch(fingerID: Int = 1) {
        console.touchFinger(fingerID)
        check(device.wait(Until.gone(promptButton), 10_000)) { "the biometric prompt stayed open after the finger" }
    }

    /** Runs a backend call on another thread, as the app does, and answers each biometric prompt it shows with finger 1. */
    fun countPrompts(operation: () -> Unit): Int {
        var failure: Throwable? = null
        val worker = thread { runCatching(operation).onFailure { failure = it } }
        var count = 0
        while (worker.isAlive) {
            if (isPromptShown(1_000)) {
                count += 1
                touch(1)
            }
        }
        failure?.let { throw it }
        return count
    }

    private companion object {
        const val PIN = "1111"

        /** The prompt's negative button, which only Hisar's biometric prompt has. */
        val promptButton: BySelector = By.text(Pattern.compile("Use master password", Pattern.CASE_INSENSITIVE))
    }
}
