package com.tryhisar.hisar

import androidx.compose.ui.test.assertDoesNotExist
import androidx.compose.ui.test.assertExists
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.ui.sharing.VaultSharingScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SharingRefreshTests {
    @get:Rule val compose = createComposeRule()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val backend = FakeBackend()
    private lateinit var model: AppModel
    private lateinit var vaultID: String
    private val bob = Member("bob-id", "Bob", "bob@example.com", Member.VAULT_ROLE_READER, "3333 4444")

    @Before
    fun start() = runBlocking {
        backend.seedSignedIn()
        withContext(Dispatchers.Main) {
            model = AppModel(scope, Dispatchers.Default, FakeTextClipboard())
            model.start { backend }
            vaultID = checkNotNull(model.vaults.createVault("Streaming")).id
        }
        backend.allVaultMemberByVaultID.getValue(vaultID) += bob
    }

    @After fun stop() { scope.cancel() }

    @Test
    fun testMemberDetailUsesRefreshedRoleCodeAndRemoval() {
        compose.setContent { HisarTheme { VaultSharingScreen(model, vaultID, onDone = {}) } }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("vault-member-bob@example.com")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("vault-member-bob@example.com").performClick()
        compose.onNodeWithTag("member-security-code").assertTextContains("3333 4444")
        compose.runOnIdle {
            backend.allVaultMemberByVaultID[vaultID] = backend.allVaultMemberByVaultID.getValue(vaultID).map {
                if (it.accountID == bob.accountID) it.copy(role = Member.VAULT_ROLE_EDITOR, securityCode = "AAAA BBBB", isKeyChanged = true) else it
            }.toMutableList()
            model.handleRemoteChange()
        }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasText("AAAA BBBB")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("member-role").assertTextContains("Editor", substring = true)
        compose.onNodeWithTag("member-key-changed").assertExists()
        compose.runOnIdle {
            backend.allVaultMemberByVaultID.getValue(vaultID).removeAll { it.accountID == bob.accountID }
            model.handleRemoteChange()
        }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("member-security-code")).fetchSemanticsNodes().isEmpty() }
        compose.onNodeWithTag("vault-member-bob@example.com").assertDoesNotExist()
    }
}
