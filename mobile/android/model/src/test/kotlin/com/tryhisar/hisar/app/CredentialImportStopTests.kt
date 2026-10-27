package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ImportProgress
import com.tryhisar.hisar.model.VaultBackend
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

class CredentialImportStopTests {
    @Test
    fun testStopDuringPreparationKeepsZeroSavedAndSchedulesNoBatch() = runTest {
        val fake = FakeBackend().apply { seedSignedIn() }
        val model = AppModelHarness(this, fake).createModel()
        model.start { object : VaultBackend by fake {
            override fun startImport(vaultID: String, isSkippingDuplicate: Boolean): ImportProgress {
                model.credentialImport.closeCredentialImport()
                return fake.startImport(vaultID, isSkippingDuplicate)
            }
        } }
        model.refreshAll()
        model.credentialImport.start { CredentialExchangeResult.Received("[\"Docker\",\"Slack\"]".encodeToByteArray()) }
        runCurrent()
        model.credentialImport.startCredentialImport(false)
        val report = assertNotNull(model.credentialImport.stoppedReport.value)
        assertEquals(0, report.progress?.savedCount)
        assertFalse(report.isSettling)
        assertEquals(0, fake.countCall("continueImport"))
        assertEquals(1, fake.countCall("cancelImport"))
    }

    @Test
    fun testStopBetweenBatchesRetainsKnownProgressAndReopensTheReport() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.credentialImport.start { CredentialExchangeResult.Received("[\"Docker\",\"Slack\",\"Mail\"]".encodeToByteArray()) }
        runCurrent()
        harness.backend.continueImportErrorMessage = "Offline"
        model.credentialImport.startCredentialImport(false)
        model.credentialImport.closeCredentialImport()
        runCurrent()
        val report = assertNotNull(model.credentialImport.stoppedReport.value)
        assertEquals(0, report.progress?.savedCount)
        assertFalse(report.isSettling)
        assertTrue(report.isOutcomeUncertain, "a failed response may hide saves in the paused batch")
        model.credentialImport.closeCredentialImport()
        assertFalse(model.credentialImport.isShown)
        assertEquals("View report", model.notice.value?.action?.label)
        model.credentialImport.reopenStoppedReport()
        assertEquals(report, model.credentialImport.sheet.value)
        assertEquals(1, harness.backend.countCall("continueImport"))
        model.lock()
        assertNull(model.credentialImport.stoppedReport.value)
    }

    @Test
    fun testStopDuringABatchKeepsItsReturnedProgressAfterLeaving() = runTest {
        val fake = FakeBackend().apply { seedSignedIn() }
        val model = AppModelHarness(this, fake).createModel()
        model.start { object : VaultBackend by fake {
            override fun continueImport(): ImportProgress {
                model.credentialImport.closeCredentialImport()
                assertTrue(model.credentialImport.stoppedReport.value?.isSettling == true)
                model.credentialImport.closeCredentialImport()
                return fake.continueImport()
            }
        } }
        model.refreshAll()
        model.credentialImport.start { CredentialExchangeResult.Received("[\"Docker\",\"Slack\",\"Mail\"]".encodeToByteArray()) }
        runCurrent()
        model.credentialImport.startCredentialImport(false)
        val report = assertNotNull(model.credentialImport.stoppedReport.value)
        assertEquals(2, report.progress?.savedCount)
        assertFalse(report.isSettling)
        assertFalse(report.isOutcomeUncertain)
        assertEquals(1, fake.countCall("continueImport"))
        assertEquals("Import stopped. 2 items saved.", model.notice.value?.message)
        model.credentialImport.reopenStoppedReport()
        assertEquals(report, model.credentialImport.sheet.value)
        assertTrue(model.unlockedData.value.listAllItemReference("personal").any { it.item.title == "Docker" })
    }

    @Test
    fun testFailedBatchAfterStopReportsOnlyAKnownLowerBound() = runTest {
        val fake = FakeBackend().apply { seedSignedIn() }
        val model = AppModelHarness(this, fake).createModel()
        model.start { object : VaultBackend by fake {
            override fun continueImport(): ImportProgress {
                model.credentialImport.closeCredentialImport()
                fake.continueImport()
                throw BackendError("Response lost")
            }
        } }
        model.refreshAll()
        model.credentialImport.start { CredentialExchangeResult.Received("[\"Docker\",\"Slack\",\"Mail\"]".encodeToByteArray()) }
        runCurrent()
        model.credentialImport.startCredentialImport(false)
        val report = assertNotNull(model.credentialImport.stoppedReport.value)
        assertEquals(0, report.progress?.savedCount)
        assertTrue(report.isOutcomeUncertain)
        assertFalse(report.isSettling)
        assertEquals(1, fake.countCall("continueImport"))
    }
}
