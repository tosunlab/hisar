package com.tryhisar.hisar.ui

import androidx.compose.ui.test.hasText
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.model.FakeBackend
import org.junit.Test

class ExportCopyUITests : HisarUITestCase() {
    @Test
    fun testExportResultExplainsTemporaryCopiesAndPlaintextCleanup() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            isBiometricAvailable = false
        }
        launchApp(backend).use {
            waitFor("vaults-list")
            openSettings("open-account")
            tap("open-export")
            tap("export-format-plain")
            type(FakeBackend.PASSWORD, "export-password")
            tapWhenEnabled("export-start")
            waitFor("save-export")
            waitFor(hasText("Hisar deletes its temporary copy when you close this screen. Copies you save or share are not deleted."))
            waitFor(hasText("Delete saved or received Plain JSON files when you are finished."))
            waitFor("export-plain-warning")
        }
    }
}
