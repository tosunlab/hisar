package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportSkippedEntry
import com.tryhisar.hisar.model.ImportProgress
import com.tryhisar.hisar.model.backendJson
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
    fun testPreviewCountsOmissionsRatherThanRejectedItems() {
        val preview = ImportPreview(fileName = "Passwords", vaultID = "personal", vaultName = "Personal", itemCount = 1,
            loginCount = 1, passkeyCount = 1, allSkipped = listOf(
                ImportSkippedEntry("GitHub", "Passkey extension data is not supported."),
                ImportSkippedEntry("GitHub", "Provider associations are not imported."),
            ))
        assertEquals(1, preview.itemCount)
        assertEquals(1, preview.loginCount)
        assertEquals(1, preview.passkeyCount)
        assertEquals("Not imported · 2", ImportSkippedEntry.heading(preview.allSkipped.size))
        assertEquals("Some entries below are fields or credentials from items that will still be imported.", ImportSkippedEntry.EXPLANATION)
        assertEquals(listOf("GitHub", "GitHub"), preview.allSkipped.map { it.title })
        assertEquals("Provider associations are not imported.", preview.allSkipped.last().reason)
    }

    @Test
    fun testCompletionKeepsReviewedPreviewAndFinalOutcomesUntilDone() = runTest {
        val harness = AppModelHarness(this)
        val reviewed = ImportPreview(fileName = "Passwords", vaultID = "personal", vaultName = "Personal", itemCount = 4,
            allSkipped = listOf(ImportSkippedEntry("GitHub", "Unsupported extension data.")),
            notice = "This is a cached partial export.", exportedFrom = "owner@example.com")
        val report = backendJson.decodeFromString<ImportProgress>("""
            {"fileName":"Passwords","vaultId":"personal","vaultName":"Personal","savedCount":2,"totalCount":3,"isDone":true,
             "leftBehindCount":3,"isSourceKept":true,
             "allSkipped":[{"title":"GitHub","reason":"Unsupported extension data."},{"title":"GitHub","reason":"Different recovery code remains only in the source."}],
             "allFailed":[{"title":"Figma","reason":"Save refused."}],
             "allDuplicateOutcomes":[{"title":"GitHub","reason":"Added 1 new passkey to the existing login in Hisar."},
               {"title":"Slack","reason":"Identical duplicate already present in Hisar."},{"title":"Docker","reason":"Included the duplicate as another item."}]}
        """.trimIndent())
        harness.backend.importPreviewOverride = reviewed
        harness.backend.importProgressOverride = report
        val model = harness.startUnlockedModel()
        receivePreview(model)
        model.credentialImport.startCredentialImport(true)
        val completed = model.credentialImport.sheet.value as CredentialImportSheet.Importing
        assertEquals(reviewed, completed.preview)
        assertEquals(report, completed.progress)
        assertEquals(3, completed.progress.leftBehindCount)
        assertEquals(2, completed.progress.allSkipped.size)
        assertEquals(3, completed.progress.allDuplicateOutcomes.size)
        assertEquals("Save refused.", completed.progress.allFailed.single().reason)
        model.credentialImport.closeCredentialImport()
        assertNull(model.credentialImport.sheet.value)
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
        assertEquals("family", model.credentialImport.preview?.groups?.first()?.vaultID)
        assertEquals(listOf("bob@example.com", "alice@example.com"), model.credentialImport.preview?.groups?.first()?.members?.map { it.email })
    }

    @Test
    fun testEachCollectionGoesToTheVaultChosenForIt() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        val model = harness.startUnlockedModel()
        receivePreview(model, FakeReceiver(CredentialExchangeResult.Received("""[["Docker","Slack"],["Figma"]]""".encodeToByteArray())))
        assertEquals(listOf("personal", "personal"), model.credentialImport.preview?.groups?.map { it.vaultID }, "every collection starts in the personal vault")

        model.credentialImport.previewCredentialImport("family")
        assertEquals(listOf("personal", "personal"), model.credentialImport.preview?.groups?.map { it.vaultID }, "one vault is never chosen for every collection at once")

        model.credentialImport.mapCredentialImportGroup("group-1", "family")
        assertEquals(listOf("personal", "family"), model.credentialImport.preview?.groups?.map { it.vaultID })
        assertEquals(listOf("bob@example.com", "alice@example.com"), model.credentialImport.preview?.groups?.last()?.members?.map { it.email }, "the shared vault's members are shown")
        assertEquals("", model.credentialImport.preview?.vaultID)

        model.credentialImport.startCredentialImport(isSkippingDuplicate = true)
        assertEquals(true, model.credentialImport.progress?.isDone)
        assertEquals(3, model.credentialImport.progress?.savedCount)
        val imported = setOf("Docker", "Slack", "Figma")
        assertEquals(listOf("Figma"), backend.allItemByVaultID["family"].orEmpty().map { it.title }.filter { it in imported })
        assertEquals(setOf("Docker", "Slack"), backend.allItemByVaultID["personal"].orEmpty().map { it.title }.filter { it in imported }.toSet())
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

    @Test
    fun testImportingAnEncryptedHisarExportAsksForItsPassphrase() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.credentialImport.startHisarExport("Alice's export.hisar", "cipher".encodeToByteArray())
        runCurrent()
        val sheet = model.credentialImport.sheet.value
        assertTrue(sheet is CredentialImportSheet.EnteringPassphrase, "an encrypted export asks for its passphrase")
        assertEquals("Alice's export.hisar", sheet.fileName)
        assertNull(model.credentialImport.preview, "there is no preview until the export is opened")
        assertEquals(1, harness.backend.countCall("prepareHisarExportImport"))
        assertEquals(0, harness.backend.countCall("openImportFile"), "nothing is opened before a passphrase is entered")
    }

    @Test
    fun testAWrongExportPassphraseKeepsTheExportWaiting() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.credentialImport.startHisarExport("Alice's export.hisar", "cipher".encodeToByteArray())
        runCurrent()
        assertTrue(model.credentialImport.sheet.value is CredentialImportSheet.EnteringPassphrase)

        model.credentialImport.openImportFile("the wrong one")
        assertTrue(model.credentialImport.sheet.value is CredentialImportSheet.EnteringPassphrase, "a wrong passphrase keeps the passphrase step")
        assertEquals("that passphrase doesn't open this export", model.errorMessage.value)
        assertNull(model.credentialImport.preview)

        model.credentialImport.openImportFile(FakeBackend.HISAR_EXPORT_PASSPHRASE)
        assertEquals("personal", model.credentialImport.preview?.vaultID, "the right passphrase opens the export")
        assertNull(model.errorMessage.value)
    }

    @Test
    fun testAPlainHisarExportPreviewsAtOnce() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isHisarExportEncrypted = false
        val model = harness.startUnlockedModel()
        model.credentialImport.startHisarExport("Alice's export.json", """["GitHub"]""".encodeToByteArray())
        runCurrent()
        assertTrue(model.credentialImport.sheet.value is CredentialImportSheet.Previewing, "a plain export previews at once")
        assertEquals("personal", model.credentialImport.preview?.vaultID)
        assertEquals(0, harness.backend.countCall("openImportFile"), "a plain export needs no passphrase")
    }
}
