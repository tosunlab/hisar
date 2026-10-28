package com.tryhisar.hisar.backend

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.ExportFormat
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.describeError
import com.tryhisar.hisar.platform.TrustedCertificates
import com.tryhisar.hisar.platform.createTemporaryDirectory
import com.tryhisar.hisar.platform.requireArgument
import java.io.File
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.ZonedDateTime
import java.util.TimeZone
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class HisarKitBackendTests {
    private val password = "android platform master password"

    @Test
    fun testGoBackendStartsSignedOutAndGeneratesPasswords() {
        val backend = createTestBackend()
        assertEquals(VaultStatus.STATE_SIGNED_OUT, backend.getStatus().state)
        assertEquals(30, backend.generatePassword(length = 30, isDigitIncluded = true, isSymbolIncluded = false).length)
        val error = assertThrows(BackendError::class.java) { backend.listAllVault() }
        assertFalse(describeError(error).isEmpty())
        assertTrue(describeError(error), describeError(error).first().isUpperCase())
        assertThrows(BackendError::class.java) { backend.signIn(server = "http://127.0.0.1:1", email = "a@example.com", secretKey = "H1-NOT-VALID", password = "x") }
    }

    @Test
    fun testSignUpSaveLockUnlockAndSignInAgain() {
        val server = requireArgument("HISAR_UI_TEST_SERVER")
        val email = requireArgument("HISAR_UI_TEST_PLATFORM_BACKEND_EMAIL")
        val backend = createTestBackend()

        val signUp = backend.signUp(requireArgument("HISAR_UI_TEST_PLATFORM_BACKEND_INVITE_LINK"), email, "Android Platform", password)
        assertEquals(VaultStatus.STATE_UNLOCKED, signUp.status.state)
        assertTrue(signUp.secretKey, signUp.secretKey.startsWith("H1-"))
        val vault = backend.createVault("Platform vault")
        val login = ItemDetail(title = "Example", username = "ada", password = "correct horse battery", allURL = listOf("https://example.com"))
        val saved = backend.saveItem(vault.id, login)
        assertEquals(listOf("Example"), backend.listAllItem(vault.id).map { it.title })
        val thisDevice = backend.listAllDevice().single { it.isCurrent }
        assertEquals("Hisar on Android", thisDevice.name)
        assertEquals("android/mobile", thisDevice.platform)

        assertEquals(VaultStatus.STATE_LOCKED, backend.lock().state)
        assertThrows(BackendError::class.java) { backend.listAllItem(vault.id) }
        assertThrows(BackendError::class.java) { backend.unlock("not the master password") }
        assertEquals(VaultStatus.STATE_LOCKED, backend.getStatus().state)
        assertEquals(VaultStatus.STATE_UNLOCKED, backend.unlock(password).state)
        assertEquals(listOf(saved.id), backend.listAllItem(vault.id).map { it.id })
        assertEquals("correct horse battery", backend.getItem(vault.id, saved.id).password)

        assertEquals(VaultStatus.STATE_SIGNED_OUT, backend.signOut().state)
        val otherDevice = createTestBackend()
        assertEquals(VaultStatus.STATE_UNLOCKED, otherDevice.signIn(server, email, signUp.secretKey, password).state)
        assertTrue(otherDevice.listAllVault().any { it.id == vault.id })
        assertEquals(listOf("ada"), otherDevice.listAllItem(vault.id).map { it.username })
    }

    @Test
    fun testTimeZoneFollowsTheDevice() {
        // A zone whose date differs from UTC's right now, so a file named in UTC would carry another date.
        val zone = ZoneId.of(if (ZonedDateTime.now(ZoneOffset.UTC).hour >= 11) "Pacific/Kiritimati" else "Pacific/Pago_Pago")
        val deviceZone = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone(zone))
        val backend = try {
            createTestBackend()
        } finally {
            TimeZone.setDefault(deviceZone)
        }
        backend.signUp(
            requireArgument("HISAR_UI_TEST_PLATFORM_TIME_ZONE_INVITE_LINK"),
            requireArgument("HISAR_UI_TEST_PLATFORM_TIME_ZONE_EMAIL"),
            "Android Time Zone",
            password,
        )
        val export = backend.exportAllData(ExportFormat.ENCRYPTED, "a long export passphrase", password, createTemporaryDirectory("export"))
        val localDate = LocalDate.now(zone).toString()
        assertTrue("${File(export.path).name} is named for $localDate in $zone", File(export.path).name.contains(localDate))
        assertFalse(File(export.path).name.contains(LocalDate.now(ZoneOffset.UTC).toString()))
    }

    @Test
    fun testEveryCertificateTheDeviceTrustsIsPassedToGo() {
        val store = KeyStore.getInstance("AndroidCAStore").apply { load(null) }
        val allTrusted = store.aliases().asSequence().map { store.getCertificate(it) }.toSet()
        val allPassed = CertificateFactory.getInstance("X.509").generateCertificates(TrustedCertificates.readPem().byteInputStream()).toSet()
        assertTrue("the device trusts ${allTrusted.size} certificates", allTrusted.size > 100)
        assertEquals(allTrusted, allPassed)
    }
}
