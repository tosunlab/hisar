package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Importing from another app on this device (Settings → Account → Import from another app), the iPhone's CredentialImportTests. */
class CredentialImportTests {
    /** What the other app exported; FakeBackend reads it as the titles of the logins to import. */
    private val allExportedTitle = listOf("GitHub", "Docker", "Slack", "Figma")

    /** Stands in for the system's credential transfer: it answers with [result], after [answer] completes when set. */
    private inner class FakeReceiver(var result: CredentialExchangeResult? = null, val beforeAnswer: () -> Unit = {}) : CredentialExchangeReceiver {
        var receiveCount = 0
        var answer: CompletableDeferred<Unit>? = null
        val allReturnedData = mutableListOf<ByteArray>()

        override suspend fun receive(): CredentialExchangeResult {
            receiveCount += 1
            answer?.await()
            beforeAnswer()
            return result ?: CredentialExchangeResult.Received(
                allExportedTitle.joinToString(",", "[", "]") { "\"$it\"" }.encodeToByteArray().also { allReturnedData += it },
            )
        }
    }

    private suspend fun TestScope.receivePreview(model: AppModel, receiver: FakeReceiver = FakeReceiver()) {
        model.credentialImport.start(receiver)
        runCurrent()
        assertTrue(model.credentialImport.preview != null, "the preview arrives")
    }

    @Test
    fun testTransferWaitsForUnlockAndPreparesOnce() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        val receiver = FakeReceiver().apply { answer = CompletableDeferred() }
        model.credentialImport.start(receiver)
        runCurrent()
        assertEquals(CredentialImportSheet.Receiving, model.credentialImport.sheet.value)

        model.lock()
        receiver.answer?.complete(Unit)
        runCurrent()
        assertEquals(Screen.Locked, model.screen.value)
        assertTrue(model.credentialImport.isWaitingForUnlock, "what arrived while locked waits for the unlock")
        assertEquals(0, backend.countCall("prepareCredentialExchangeImport"), "nothing is read while locked")

        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertFalse(model.credentialImport.isWaitingForUnlock)
        assertTrue(model.credentialImport.isShown)
        assertEquals(1, receiver.receiveCount)
        assertTrue(receiver.allReturnedData.single().all { it == 0.toByte() }, "what the other app handed over is cleared once read")
        assertEquals("personal", model.credentialImport.preview?.vaultID, "the personal vault is chosen first")
        assertEquals(listOf("GitHub"), model.credentialImport.preview?.allDuplicate?.map { it.title })
        assertEquals(1, backend.countCall("prepareCredentialExchangeImport"))

        model.refreshStatus()
        model.refreshAll()
        runCurrent()
        assertEquals(1, backend.countCall("prepareCredentialExchangeImport"), "a later status change does not import again")
    }

    @Test
    fun testCredentialsArrivingAsHisarLocksWaitForTheUnlock() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        // The backend locks while the user is in the other app, and the credentials come before its Locked event.
        val receiver = FakeReceiver(beforeAnswer = { backend.lock() })
        model.credentialImport.start(receiver)
        runCurrent()
        assertEquals(Screen.Locked, model.screen.value)
        assertTrue(model.credentialImport.isWaitingForUnlock)
        assertNull(model.errorMessage.value, "the lock screen shows no sentence about the import")
        assertFalse(receiver.allReturnedData.single().all { it == 0.toByte() }, "the credentials are kept for the unlock")

        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertEquals("personal", model.credentialImport.preview?.vaultID)
        assertEquals(2, backend.countCall("prepareCredentialExchangeImport"), "read again once unlocked")
    }

    @Test
    fun testImportWhileSignedOutExplainsInstead() = runTest {
        val model = AppModelHarness(this).startModel()
        val receiver = FakeReceiver()
        model.credentialImport.start(receiver)
        runCurrent()
        assertEquals(CredentialImportModel.SIGNED_OUT_MESSAGE, model.credentialImport.message.value)
        assertFalse(model.credentialImport.isWaitingForUnlock)
        assertFalse(model.credentialImport.isShown)
        assertEquals(0, receiver.receiveCount)
    }

    @Test
    fun testSignOutDuringTheTransferExplainsAndDropsIt() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val receiver = FakeReceiver().apply { answer = CompletableDeferred() }
        model.credentialImport.start(receiver)
        runCurrent()

        model.lock()
        model.lock.signOut()
        receiver.answer?.complete(Unit)
        runCurrent()
        assertEquals(VaultStatus.STATE_SIGNED_OUT, model.status.value.state)
        assertEquals(CredentialImportModel.SIGNED_OUT_MESSAGE, model.credentialImport.message.value)
        assertFalse(model.credentialImport.isShown)
        assertFalse(model.credentialImport.isWaitingForUnlock)
        assertEquals(0, harness.backend.countCall("prepareCredentialExchangeImport"))
    }

    @Test
    fun testCancelledTransferClosesWithoutASentence() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.credentialImport.start(FakeReceiver(CredentialExchangeResult.Cancelled))
        runCurrent()
        assertFalse(model.credentialImport.isShown)
        assertNull(model.credentialImport.message.value)
        assertNull(model.errorMessage.value)
    }

    @Test
    fun testFailedTransferExplainsInTheSheet() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.credentialImport.start(FakeReceiver(CredentialExchangeResult.Failed("No app on this device can export to Hisar.")))
        runCurrent()
        assertEquals(CredentialImportSheet.Failed("No app on this device can export to Hisar."), model.credentialImport.sheet.value)

        model.credentialImport.closeCredentialImport()
        assertFalse(model.credentialImport.isShown)
        runCurrent()
        assertEquals(0, harness.backend.countCall("cancelImport"), "nothing was prepared to drop")
    }

    @Test
    fun testUnreadableCredentialsExplainInTheSheet() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.credentialImport.start(FakeReceiver(CredentialExchangeResult.Received("not json".encodeToByteArray())))
        runCurrent()
        assertTrue(model.credentialImport.sheet.value is CredentialImportSheet.Failed)
        assertNull(model.errorMessage.value, "the sheet shows the sentence once")
    }

    @Test
    fun testImportContinuesUntilDoneAndRefreshes() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        receivePreview(model)

        model.credentialImport.startCredentialImport(isSkippingDuplicate = true)
        assertEquals(1, backend.countCall("startImport"))
        assertEquals(2, backend.countCall("continueImport"), "two batches of two save the three new logins")
        assertEquals(true, model.credentialImport.progress?.isDone)
        assertEquals(3, model.credentialImport.progress?.savedCount)
        assertEquals(3, model.credentialImport.progress?.totalCount)
        assertNull(model.errorMessage.value)
        assertFalse(model.credentialImport.isImporting.value)
        val allTitle = model.unlockedData.value.listAllItemReference("personal").map { it.item.title }
        assertTrue(listOf("Docker", "Slack", "Figma").all { it in allTitle }, "the finished import lists the vault again")

        model.credentialImport.closeCredentialImport()
        assertFalse(model.credentialImport.isShown)
        runCurrent()
        assertEquals(1, backend.countCall("cancelImport"), "the finished import is dropped")
    }

    @Test
    fun testPausedImportRetries() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        receivePreview(model)
        harness.backend.continueImportErrorMessage = "Hisar can't reach the server"

        model.credentialImport.startCredentialImport(isSkippingDuplicate = false)
        assertEquals("Hisar can't reach the server", model.errorMessage.value)
        assertEquals(false, model.credentialImport.progress?.isDone)
        assertEquals(0, model.credentialImport.progress?.savedCount)

        model.credentialImport.continueCredentialImport()
        assertNull(model.errorMessage.value)
        assertEquals(true, model.credentialImport.progress?.isDone)
        assertEquals(4, model.credentialImport.progress?.savedCount)
    }

    @Test
    fun testChoosingAnotherVaultPreviewsAgain() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        receivePreview(model)
        assertEquals(listOf("personal", "family"), model.credentialImport.allVault.map { it.id })

        model.credentialImport.previewCredentialImport("family")
        assertEquals("family", model.credentialImport.preview?.vaultID)
        assertEquals(emptyList(), model.credentialImport.preview?.allDuplicate)
    }

    @Test
    fun testCancelDropsTheImport() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        receivePreview(model)

        model.credentialImport.closeCredentialImport()
        assertFalse(model.credentialImport.isShown)
        assertNull(model.credentialImport.preview)
        runCurrent()
        assertEquals(1, backend.countCall("cancelImport"), "the import is cancelled")
        assertNull(backend.allPendingImportTitle)
        assertEquals(0, backend.countCall("startImport"))
    }

    @Test
    fun testLockClosesTheImport() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        receivePreview(model)

        model.lock()
        assertFalse(model.credentialImport.isShown)
        assertNull(model.credentialImport.preview)
    }
}
