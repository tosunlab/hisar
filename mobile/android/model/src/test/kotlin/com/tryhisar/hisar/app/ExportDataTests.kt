package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ExportFormat
import com.tryhisar.hisar.model.ExportLogic
import com.tryhisar.hisar.model.FakeBackend
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

/** Settings → Account → Export data. The iPhone keeps these two in VaultManagementTests. */
class ExportDataTests {
    @Test
    fun testExportInputChecks() {
        assertNull(ExportLogic.checkInput(ExportFormat.PLAIN, "", "", minPassphraseLength = 10))
        assertEquals(
            "Use at least 10 characters for the export passphrase.",
            ExportLogic.checkInput(ExportFormat.ENCRYPTED, "short", "short", minPassphraseLength = 10),
        )
        assertEquals("The passphrases don't match.", ExportLogic.checkInput(ExportFormat.ENCRYPTED, "a long passphrase", "a long passphrasf", minPassphraseLength = 10))
        assertNull(ExportLogic.checkInput(ExportFormat.ENCRYPTED, "a long passphrase", "a long passphrase", minPassphraseLength = 10))
    }

    @Test
    fun testExportWritesIntoItsOwnFolderAndIsRemoved() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertEquals(10, model.settings.loadExportOptions()?.minPassphraseLength)

        val result = assertNotNull(model.settings.exportAllData(ExportFormat.ENCRYPTED, "a long passphrase", ""))
        val file = File(result.path)
        val folder = file.parentFile
        assertTrue(file.exists())
        assertTrue(folder.name.startsWith("Export-"))
        assertEquals(harness.backend.allVault.size, result.vaultCount)
        model.settings.removeExport(result)
        assertFalse(folder.exists())

        harness.backend.isBiometricSucceeding = false
        assertNull(model.settings.exportAllData(ExportFormat.PLAIN, "", ""))
        assertEquals("Face ID was cancelled.", model.errorMessage.value)
        val withPassword = assertNotNull(model.settings.exportAllData(ExportFormat.PLAIN, "ignored", FakeBackend.PASSWORD))
        assertTrue(withPassword.path.endsWith(".json"))
        model.settings.removeExport(withPassword)
    }

    @Test
    fun testClosingExportKeepsSavedAndReceivedCopies() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        val copies = java.nio.file.Files.createTempDirectory("UserCopies-").toFile()
        try {
            for (format in listOf(ExportFormat.ENCRYPTED, ExportFormat.PLAIN)) {
                val result = assertNotNull(model.settings.exportAllData(format, "a long passphrase", FakeBackend.PASSWORD))
                val source = File(result.path)
                val expected = source.readText()
                val saved = source.copyTo(File(copies, "saved-${source.name}"))
                val received = source.copyTo(File(copies, "received-${source.name}"))
                model.settings.removeExport(result)
                assertFalse(source.parentFile.exists())
                assertEquals(expected, saved.readText())
                assertEquals(expected, received.readText())
            }
        } finally {
            copies.deleteRecursively()
        }
    }
}
