package com.tryhisar.hisar.ui

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.app.CredentialExchangeReceiver
import com.tryhisar.hisar.app.CredentialExchangeResult
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Settings → Account → Import from another app on a fake backend, with the credential transfer answered by the test:
 * the iPhone's import sheet (CredentialImportView), which iOS opens when another app exports to Hisar.
 */
@RunWith(AndroidJUnit4::class)
class CredentialImportUITests : HisarUITestCase() {
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    private val backend = FakeBackend().apply { seedSignedIn() }

    /** FakeBackend reads what the other app exported as the titles of the logins to import. */
    private val exportedTitles = """["GitHub","Docker","Slack","Figma"]"""

    /** Opens Settings → Account → Import from another app, whose transfer [receiver] answers. */
    private fun startImport(receiver: CredentialExchangeReceiver, test: () -> Unit) {
        launchApp(backend).use {
            application.credentialExchangeReceiver = receiver
            waitFor("vaults-list")
            openSettings("open-account")
            tap("open-credential-import")
            test()
        }
    }

    @Test
    fun testImportFromAnotherAppPreviewsChoosesAVaultAndImports() = startImport({ CredentialExchangeResult.Received(exportedTitles.encodeToByteArray()) }) {
        waitFor("credential-import-start")
        assertFalse("the import covers the tab bar", exists(hasTestTag("tab-vaults")))
        assertFalse("the import covers Search", exists(hasTestTag("tab-search")))
        assertTrue("the counts are listed", exists(hasTestTag("credential-import-counts")))
        assertTrue("GitHub is already in Personal", exists(hasText("1 item is already there")))
        saveScreenshot("credential-import-preview")

        tap("credential-import-vault-family")
        waitFor(hasText("Import into Family"))
        assertFalse("Family has no GitHub", exists(hasTestTag("credential-import-duplicates")))
        tap("credential-import-vault-personal")
        waitFor(hasText("Import into Personal"))

        tap("credential-import-start")
        waitFor("credential-import-finished")
        assertTrue(exists(hasText("Imported 3 items into Personal.")))
        saveScreenshot("credential-import-finished")
        val allTitle = backend.allItemByVaultID["personal"].orEmpty().map { it.title }
        assertTrue("the new logins are saved: $allTitle", listOf("Docker", "Slack", "Figma").all { it in allTitle })
        assertEquals("the duplicate is skipped", 1, allTitle.count { it == "GitHub" })

        tap("credential-import-done")
        composeRule.waitUntil("the import closes", 10_000) { !exists(hasTestTag("credential-import")) }
        assertEquals(1, backend.countCall("cancelImport"))
    }

    @Test
    fun testFailedTransferExplainsAndCloses() = startImport({ CredentialExchangeResult.Failed("No app on this device offered its passwords and passkeys to Hisar.") }) {
        waitFor("credential-import-failed-transfer")
        assertTrue(exists(hasText("No app on this device offered its passwords and passkeys to Hisar.")))
        saveScreenshot("credential-import-failed")
        tap("credential-import-cancel")
        composeRule.waitUntil("the import closes", 10_000) { !exists(hasTestTag("credential-import")) }
        assertEquals(0, backend.countCall("prepareCredentialExchangeImport"))
    }

    @Test
    fun testTransferFinishingWhileLockedImportsAfterTheUnlock() = startImport({
        // Hisar locked while the user was in the other app.
        backend.lock()
        application.sendEvent(BackendEvent.Locked)
        CredentialExchangeResult.Received(exportedTitles.encodeToByteArray())
    }) {
        waitFor("unlock-password")
        assertFalse("nothing shows over the lock screen", exists(hasTestTag("credential-import")))
        assertNull("nothing is prepared while locked", backend.allPendingImportTitle)

        type(FakeBackend.PASSWORD, "unlock-password")
        tap("unlock-submit")
        waitFor("credential-import-start")
        assertEquals(listOf("GitHub", "Docker", "Slack", "Figma"), backend.allPendingImportTitle)
        saveScreenshot("credential-import-after-unlock")
    }
}
