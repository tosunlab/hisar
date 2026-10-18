package com.tryhisar.hisar.platform

import android.os.ParcelFileDescriptor
import androidx.test.platform.app.InstrumentationRegistry

/**
 * The emulator's console, reached from inside the emulator at the host's loopback address, 10.0.2.2. It plays a finger
 * on the fingerprint sensor as `adb emu finger touch` does on the host. The shell connects, not the app: the app's own
 * connection timed out on API 37 while Settings was in front.
 */
class EmulatorConsole(private val port: Int, private val token: String) {
    fun touchFinger(fingerID: Int) = send("finger touch $fingerID")

    private fun send(command: String) {
        val (output, input) = InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommandRw("nc -w 5 10.0.2.2 $port")
        ParcelFileDescriptor.AutoCloseOutputStream(input).use { it.write("auth $token\n$command\nquit\n".toByteArray()) }
        val answer = ParcelFileDescriptor.AutoCloseInputStream(output).use { it.readBytes().decodeToString() }
        val allLine = answer.lines().map { it.trim() }
        if (allLine.any { it.startsWith("KO") } || allLine.count { it == "OK" } < 3) {
            throw AssertionError("the emulator console did not run $command: $answer")
        }
    }

    companion object {
        fun fromArguments() = EmulatorConsole(
            requireArgument("HISAR_UI_TEST_EMULATOR_CONSOLE_PORT").toInt(),
            requireArgument("HISAR_UI_TEST_EMULATOR_CONSOLE_TOKEN"),
        )
    }
}
