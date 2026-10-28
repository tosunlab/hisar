package com.tryhisar.hisar.ui

import androidx.compose.ui.test.assertIsSelected
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
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportProgress
import com.tryhisar.hisar.model.ImportSkippedEntry
import com.tryhisar.hisar.model.ImportDuplicate
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
        waitFor(hasText("Import into Shared vaults › Family"))
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
    fun testImportCompletionReportShowsRemainingEntries() {
        backend.importPreviewOverride = ImportPreview(fileName = "Passwords", vaultID = "personal", vaultName = "Personal", itemCount = 4,
            notice = "This is a cached partial export.", exportedFrom = "owner@example.com", exportedAt = "2026-10-20T12:00:00Z",
            allDuplicate = listOf(ImportDuplicate(title = "GitHub", username = "alice", detail = "alice · github.com · different password")))
        backend.importProgressOverride = ImportProgress(fileName = "Passwords", vaultID = "personal", vaultName = "Personal",
            savedCount = 2, totalCount = 3, isDone = true, leftBehindCount = 3,
            allSkipped = listOf(
                ImportSkippedEntry("GitHub", "Unsupported extension data."),
                ImportSkippedEntry("GitHub", "Different recovery code remains only in the source."),
            ),
            allDuplicateOutcomes = listOf(
                ImportSkippedEntry("GitHub", "Added 1 new passkey to the existing login in Hisar."),
                ImportSkippedEntry("Slack", "Identical duplicate already present in Hisar."),
                ImportSkippedEntry("Docker", "Included the duplicate as another item."),
            ),
            allFailed = listOf(ImportSkippedEntry("Figma", "Save refused.")))
        startImport({ CredentialExchangeResult.Received(exportedTitles.encodeToByteArray()) }) {
            waitFor("credential-import-start")
            tap("credential-import-start")
            waitFor("credential-import-finished")
            for (text in listOf(
                "This is a cached partial export.", "Unsupported extension data.",
                "Different recovery code remains only in the source.",
                "alice · github.com · different password",
                "Added 1 new passkey to the existing login in Hisar.",
                "Identical duplicate already present in Hisar.", "Included the duplicate as another item.",
                "Save refused.", "Keep the other app or source file until you have checked these entries.",
            )) assertTrue(text, exists(hasText(text)))
            tap("credential-import-done")
            waitForGone("credential-import")
        }
    }


    @Test
    fun testGroupedPreviewKeepsOneSelectorAndRetainsEachDestinationAndAccess() = startImport({
        CredentialExchangeResult.Received("""[["Docker","Slack"],["Figma"],["Mail"]]""".encodeToByteArray())
    }) {
        waitFor("credential-import-start")
        assertFalse("choices stay collapsed", exists(hasTestTag("credential-import-group-vault-family")))
        tap("credential-import-group-0-destination")
        tap("credential-import-group-vault-family")
        waitFor(hasText("Collection 1 to Shared vaults › Family"))
        assertTrue("members remain in the group card", exists(hasText("bob@example.com", substring = true)))
        assertTrue(exists(hasText("Imported items are readable by the destination's current members. Access can change later.")))
        tap("credential-import-group-2-destination")
        tap("credential-import-group-vault-family")
        tap("credential-import-group-0-destination")
        composeRule.onNode(hasTestTag("credential-import-group-vault-family")).assertIsSelected()
        tap("credential-import-vault-cancel")
        assertTrue(exists(hasText("Collection 1 to Shared vaults › Family")))
        assertTrue(exists(hasText("Collection 3 to Shared vaults › Family")))
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

    @Test
    fun testImportPickerShowsOrganizationAndStableIdentity() {
        backend.allOrganization = mutableListOf(Organization("acme", "Acme"), Organization("beta", "Beta"))
        backend.allVault += listOf(
            VaultSummary("production-a", "Production", "organization", "editor", organizationID = "acme"),
            VaultSummary("production-b", "Production", "organization", "editor", organizationID = "beta"),
            VaultSummary("missing", "Production", "organization", "editor", organizationID = "unknown-organization"),
            VaultSummary("12345678-a", "Duplicate", "organization", "editor", organizationID = "acme"),
            VaultSummary("12345678-b", "Duplicate", "organization", "editor", organizationID = "acme"),
        )
        startImport({ CredentialExchangeResult.Received(exportedTitles.encodeToByteArray()) }) {
            waitFor("credential-import-start")
            for ((id, label) in listOf(
                "production-a" to "Acme › Production", "production-b" to "Beta › Production",
                "missing" to "unknown-organization › Production",
                "12345678-a" to "Acme › Duplicate (12345678-a)", "12345678-b" to "Acme › Duplicate (12345678-b)",
            )) {
                tap("credential-import-vault-$id")
                waitFor(hasText("Import into $label"))
                assertTrue(exists(hasTestTag("credential-import-access")))
            }
        }
    }
}
