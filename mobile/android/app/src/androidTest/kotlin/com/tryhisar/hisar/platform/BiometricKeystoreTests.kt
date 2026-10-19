package com.tryhisar.hisar.platform

import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.After
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
}
