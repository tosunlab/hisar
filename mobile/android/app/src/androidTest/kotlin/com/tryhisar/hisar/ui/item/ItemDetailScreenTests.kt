package com.tryhisar.hisar.ui.item

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertContentDescriptionEquals
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.tap
import com.tryhisar.hisar.ui.waitForTag
import com.tryhisar.hisar.ui.waitForTagToGo
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** What the iPhone app's FakeBackend-only detail states show: an SSH key, a server's key link, passkeys and readers. */
@RunWith(AndroidJUnit4::class)
class ItemDetailScreenTests {
    @get:Rule
    val composeRule = createEmptyComposeRule()

    private val backend = FakeBackend().apply {
        seedSignedIn()
        allItemByVaultID.getValue("personal") += listOf(
            ItemDetail(
                id = "deploy-key", revision = "r1", category = ItemDetail.CATEGORY_SSH_KEY, title = "Deploy key", keyType = "ED25519",
                fingerprint = "SHA256:Zm9vYmFyYmF6", publicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOv deploy@example.com",
            ),
            ItemDetail(
                id = "build", revision = "r1", category = ItemDetail.CATEGORY_SERVER, title = "Build server", username = "deploy",
                server = ItemServer(host = "build.example.com", port = "70000", sshKeyID = "deploy-key", sshKeyTitle = "Deploy key", sshKeyFingerprint = "SHA256:Zm9vYmFyYmF6", portWarning = ItemServer.PORT_WARNING_TEXT),
            ),
        )
    }
    private var host: ItemDetailHost? = null

    private val clipboard = FakeTextClipboard()

    private fun openHost(): ItemDetailHost = ItemDetailHost(backend, clipboard).also { host = it }

    @After
    fun closeHost() {
        host?.close()
    }

    private fun exists(tag: String) = composeRule.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()

    @Test
    fun testMoveSheetKeepsItsSelectionAfterFailureAndOpensTheMovedItem() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("move-item")
        composeRule.onNodeWithTag("move-item").performClick()
        composeRule.waitForTag("move-item-sheet")
        composeRule.tap("move-item-target-family")
        assertTrue(composeRule.onAllNodes(hasText("Shared vault / Family")).fetchSemanticsNodes().isNotEmpty())
        assertTrue(composeRule.onAllNodes(hasText("Copies they retained independently cannot be revoked.", substring = true)).fetchSemanticsNodes().isNotEmpty())
        backend.moveErrorMessage = "The server could not be reached."
        composeRule.onNodeWithTag("move-item-confirm").performClick()
        composeRule.waitUntil(5_000) { host.model.errorMessage.value == "The server could not be reached." }
        assertTrue(exists("move-item-sheet"))
        assertTrue(backend.getItem("personal", "github").allPasskey.isNotEmpty())
        backend.moveErrorMessage = null
        composeRule.onNodeWithTag("move-item-confirm").performClick()
        composeRule.waitForTagToGo("move-item-sheet")
        composeRule.waitForTag("edit-item")
        composeRule.onNodeWithTag("edit-item").performClick()
        composeRule.waitUntil(5_000) { host.allEditorRequest.isNotEmpty() }
        assertEquals("family", (host.allEditorRequest.last() as com.tryhisar.hisar.app.EditorRequest.Edit).vaultID)
        assertTrue(backend.getItem("family", "github").allPasskey.isNotEmpty())
        assertEquals(0, host.backCount)
    }

    private fun dismissNotice() {
        composeRule.waitForTag("notice")
        composeRule.onNodeWithTag("notice").performTouchInput { swipeLeft() }
        composeRule.waitForTagToGo("notice")
    }

    @Test
    fun testCustomRowsCopyTheirExactValueAndRevealIndependently() {
        val fields = listOf(
            ItemField("Recovery code", "first-secret", true),
            ItemField("Recovery code", "second-secret", true),
            ItemField("recovery CODE", "case-secret", true),
            ItemField("Password", "custom-password", true),
        )
        backend.allItemByVaultID.getValue("personal") += ItemDetail(id = "duplicates", title = "Duplicates", password = "main-password", allField = fields)
        openHost().show("personal", "duplicates")
        composeRule.waitForTag("password-value")
        for ((index, field) in fields.withIndex()) {
            val copies = composeRule.onAllNodes(hasTestTag("copy-${field.name.lowercase()}"))
            val row = if (index == 3) 1 else index
            copies[row].performScrollTo().performClick()
            composeRule.waitUntil(5_000) { clipboard.allCopiedSecret.lastOrNull() == field.value }
            dismissNotice()
        }
        composeRule.onAllNodes(hasTestTag("reveal-recovery code"))[1].performScrollTo().performClick()
        assertTrue(composeRule.onAllNodes(hasText("second-secret")).fetchSemanticsNodes().isNotEmpty())
        assertTrue(composeRule.onAllNodes(hasText("first-secret")).fetchSemanticsNodes().isEmpty())
        assertTrue(composeRule.onAllNodes(hasText("case-secret")).fetchSemanticsNodes().isEmpty())
    }

    @Test
    fun testServerOpensItsSSHKeyAndBackReturnsToTheServer() {
        val host = openHost()
        host.show("personal", "build")
        composeRule.waitForTag("server-ssh-key")
        composeRule.waitUntil(5_000) { composeRule.onAllNodes(hasText(ItemServer.PORT_WARNING_TEXT)).fetchSemanticsNodes().isNotEmpty() }
        composeRule.tap("server-ssh-key")
        composeRule.waitForTag("public key-value")
        assertTrue(composeRule.onAllNodes(hasText("ED25519")).fetchSemanticsNodes().isNotEmpty())
        assertTrue("an SSH key never shows its private key", composeRule.onAllNodes(hasText("PRIVATE KEY", substring = true)).fetchSemanticsNodes().isEmpty())
        composeRule.onNodeWithTag("back").performClick()
        composeRule.waitForTag("server-ssh-key")
        assertEquals("the server's own back is not used to leave the key", 0, host.backCount)
    }

    @Test
    fun testServerHostMatchesTheCopiedFieldWithDefaultAndCustomPorts() {
        val host = openHost()
        for ((port, id) in listOf("22" to "default-server", "2222" to "custom-server")) {
            backend.allItemByVaultID.getValue("personal") += ItemDetail(
                id = id, category = ItemDetail.CATEGORY_SERVER, title = id,
                server = ItemServer(host = "build.example.com", port = port),
            )
            host.show("personal", id)
            composeRule.waitForTag("host-value")
            assertTrue(composeRule.onAllNodes(hasText("build.example.com")).fetchSemanticsNodes().isNotEmpty())
            assertTrue(composeRule.onAllNodes(hasText(port)).fetchSemanticsNodes().isNotEmpty())
            composeRule.onNodeWithTag("copy-host").performClick()
            composeRule.waitUntil(5_000) { backend.allCopiedField.contains("$id.Host") }
            dismissNotice()
        }
    }

    @Test
    fun testDeletingAPasskeyAsksFirstAndRemovesIt() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("passkey-github.com")
        composeRule.tap("delete-passkey-github.com")
        composeRule.waitForTag("confirm-delete-passkey")
        assertTrue(composeRule.onAllNodes(hasText("You can no longer sign in to github.com as alice with it. It is removed from all your devices. Make sure you can sign in another way first.")).fetchSemanticsNodes().isNotEmpty())
        composeRule.onNodeWithTag("confirm-delete-passkey").performClick()
        composeRule.waitForTagToGo("passkey-github.com")
        assertEquals(emptyList<Any>(), backend.getItem("personal", "github").allPasskey)
    }

    @Test
    fun testSwipingAPasskeyLeftAsksFirstAndRemovesIt() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("passkey-github.com")
        composeRule.onNodeWithTag("passkey-github.com").performScrollTo().performTouchInput { swipeLeft() }
        composeRule.tap("swipe-delete-passkey-github.com")
        composeRule.waitForTag("confirm-delete-passkey")
        composeRule.onNodeWithTag("confirm-delete-passkey").performClick()
        composeRule.waitForTagToGo("passkey-github.com")
        assertEquals(emptyList<Any>(), backend.getItem("personal", "github").allPasskey)
    }

    @Test
    fun testAReaderSeesNoEditOrDeleteButtons() {
        backend.allVault = mutableListOf(VaultSummary(id = "personal", name = "Shared with me", type = VaultSummary.TYPE_SHARED, role = "reader", itemCount = 3))
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("passkey-github.com")
        assertFalse(exists("edit-item"))
        assertFalse(exists("delete-passkey-github.com"))
        assertFalse(exists("swipe-delete-passkey-github.com"))
        assertTrue("a reader still stars items, which belong to their account", exists("favorite-item"))
    }

    @Test
    fun testDeletedItemClosesItsScreen() {
        val host = openHost()
        host.show("personal", "bank")
        composeRule.waitForTag("password-value")
        backend.deleteItem("personal", "bank")
        host.refresh()
        composeRule.waitUntil(5_000) { host.backCount == 1 }
    }

    @Test
    fun testTouchAndHoldCopiesAReferenceAndAWebsite() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("username-value")
        composeRule.onNodeWithTag("username-value", useUnmergedTree = true).performTouchInput { longClick() }
        composeRule.onNodeWithTag("field-action-Copy secret reference").performClick()
        composeRule.waitUntil(5_000) { backend.allCopiedReference == listOf("hisar://Personal/GitHub/username") }

        dismissNotice()
        composeRule.onNodeWithTag("website-https://github.com").performScrollTo().performTouchInput { longClick() }
        composeRule.onNodeWithTag("field-action-Copy").performClick()
        composeRule.waitUntil(5_000) { clipboard.allCopiedSecret == listOf("https://github.com") }
        composeRule.waitUntil(5_000) { host.model.notice.value?.message == "Website copied. It clears in about 2 minutes." }
    }

    @Test
    fun testTouchAndHoldCopiesAnAndroidAppCertificate() {
        val certificate = List(32) { "14" }.joinToString(":")
        backend.addAndroidApp("personal", "bank", "com.bank.app", certificate)
        val host = openHost()
        host.show("personal", "bank")
        composeRule.waitForTag("android-app-com.bank.app")
        composeRule.onNodeWithTag("android-app-com.bank.app").performScrollTo().performTouchInput { longClick() }
        composeRule.waitForTag("field-action-Copy certificate")
        composeRule.onNodeWithTag("field-action-Copy certificate").performClick()
        composeRule.waitForTagToGo("field-action-Copy certificate")
        composeRule.waitUntil(5_000) { clipboard.allCopiedPlain == listOf(certificate) }
        composeRule.waitUntil(5_000) { host.model.notice.value?.message == "Certificate copied" }
    }

    @Test
    fun testFavoriteSnackbarUndoUpdatesTheDetailToolbar() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("favorite-item")
        composeRule.onNodeWithTag("favorite-item").assertContentDescriptionEquals("Add to Favorites")
        composeRule.onNodeWithTag("favorite-item").performClick()
        composeRule.waitForTag("notice")
        composeRule.onNodeWithTag("favorite-item").assertContentDescriptionEquals("Remove from Favorites")
        composeRule.onNode(hasText("Undo")).performClick()
        composeRule.waitUntil(5_000) { backend.allFavoriteRecordID.isEmpty() }
        composeRule.onNodeWithTag("favorite-item").assertContentDescriptionEquals("Add to Favorites")
    }

    @Test
    fun testTappingAFieldRowCopiesItAndItsButtonsKeepTheirOwnAction() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("password-value")
        val hiddenPassword = composeRule.onNodeWithTag("password-value", useUnmergedTree = true).fetchSemanticsNode().config[SemanticsProperties.Text].joinToString("")
        composeRule.onNode(hasClickLabel("Copy username")).performClick()
        composeRule.waitUntil(5_000) { backend.allCopiedField == listOf("github.username") }
        composeRule.onNode(hasClickLabel("Copy password")).performClick()
        composeRule.waitUntil(5_000) { backend.allCopiedField == listOf("github.username", "github.password") }
        composeRule.waitUntil(5_000) { host.model.notice.value?.message == "Password copied. It clears in about 2 minutes." }
        assertEquals("a tap copies without revealing", hiddenPassword, composeRule.onNodeWithTag("password-value", useUnmergedTree = true).fetchSemanticsNode().config[SemanticsProperties.Text].joinToString(""))

        composeRule.tap("reveal-password")
        composeRule.waitUntil(5_000) { composeRule.onAllNodes(hasTestTag("password-value") and hasText("github-password"), useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty() }
        assertEquals("the reveal button does not also copy", 2, backend.allCopiedField.size)

        dismissNotice()
        composeRule.onNode(hasClickLabel("Copy PIN")).performScrollTo().performClick()
        composeRule.waitUntil(5_000) { clipboard.allCopiedSecret == listOf("1234") }
        dismissNotice()
        composeRule.onNodeWithTag("website-https://github.com").performScrollTo().performClick()
        composeRule.waitUntil(5_000) { clipboard.allCopiedSecret == listOf("1234", "https://github.com") }
        val username = composeRule.onNode(hasClickLabel("Copy username")).fetchSemanticsNode()
        assertEquals(Role.Button, username.config[SemanticsProperties.Role])
        assertEquals(listOf("Copy username"), username.config[SemanticsProperties.ContentDescription])
        assertTrue("the row owns its label and value", username.config[SemanticsProperties.Text].size >= 2)
        assertEquals("inner copy stays a separate action", 1, composeRule.onAllNodes(hasTestTag("copy-username")).fetchSemanticsNodes().size)

        composeRule.waitForTag("one-time-code-value")
        val code = composeRule.onNodeWithTag("one-time-code-value", useUnmergedTree = true)
            .fetchSemanticsNode().config[SemanticsProperties.Text].single().text
        val codeRow = composeRule.onNode(hasClickLabel("Copy one-time code")).fetchSemanticsNode()
        assertEquals(Role.Button, codeRow.config[SemanticsProperties.Role])
        assertTrue("TalkBack can read the code on the copy row", codeRow.config[SemanticsProperties.Text].any { it.text == code })
        assertEquals(FakeBackend.oneTimeCode.formattedCode, code)
    }

    private fun hasClickLabel(label: String) = SemanticsMatcher("a click action labelled $label") { it.config.getOrNull(SemanticsActions.OnClick)?.label == label }
}
