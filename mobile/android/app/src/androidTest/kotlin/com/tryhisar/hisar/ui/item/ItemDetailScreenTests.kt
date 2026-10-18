package com.tryhisar.hisar.ui.item

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemServer
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

    private fun openHost(): ItemDetailHost = ItemDetailHost(backend, FakeTextClipboard()).also { host = it }

    @After
    fun closeHost() {
        host?.close()
    }

    private fun exists(tag: String) = composeRule.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()

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
    fun testDeletingAPasskeyAsksFirstAndRemovesIt() {
        val host = openHost()
        host.show("personal", "github")
        composeRule.waitForTag("passkey-github.com")
        composeRule.tap("delete-passkey-github.com")
        composeRule.waitForTag("confirm-delete-passkey")
        assertTrue(composeRule.onAllNodes(hasText("You can no longer sign in to github.com as alice with it.")).fetchSemanticsNodes().isNotEmpty())
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
}
