package com.tryhisar.hisar.ui.settings

import androidx.activity.ComponentActivity
import androidx.compose.runtime.Composable
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.CredentialImportSheet
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportSourceGroup
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ChoiceGroupScreenTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val backend = FakeBackend().apply { seedSignedIn() }
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After
    fun stopModel() { scope.cancel() }

    private fun show(content: @Composable () -> Unit) {
        runBlocking(Dispatchers.Main) {
            model.start { backend }
            model.refreshAll()
        }
        composeRule.setContent { HisarTheme { content() } }
    }

    private fun assertChoices(groups: Int, options: Int, selected: Int) {
        val radio = SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.RadioButton)
        composeRule.onAllNodes(SemanticsMatcher.keyIsDefined(SemanticsProperties.SelectableGroup)).assertCountEquals(groups)
        composeRule.onAllNodes(radio and hasClickAction()).assertCountEquals(options)
        composeRule.onAllNodes(radio and SemanticsMatcher.expectValue(SemanticsProperties.Selected, true)).assertCountEquals(selected)
    }

    @Test
    fun testAppearanceGroupsExposeOneControlPerOptionAndChosenAppearanceAndColor() {
        show { AppearanceScreen(model) {} }
        assertChoices(groups = 2, options = 7, selected = 2)
        composeRule.onNodeWithTag("appearance-system").assertIsSelected()
        composeRule.onNodeWithTag("appearance-light").assertIsNotSelected()
        composeRule.onNodeWithTag("theme-color-neutral").assertIsSelected()
    }

    @Test
    fun testAutoLockChoicesExposeOneSelectedRadio() {
        show { SecurityScreen(model, {}, {}) }
        assertChoices(groups = 1, options = VaultStatus.allAutoLockSeconds.size, selected = 1)
        composeRule.onNodeWithTag("auto-lock-${model.status.value.autoLockSeconds}").assertIsSelected()
    }

    @Test
    fun testExportFormatChoicesExposeSelectedAndUnselectedRadios() {
        show { ExportDataScreen(model) {} }
        assertChoices(groups = 1, options = 2, selected = 1)
        composeRule.onNodeWithTag("export-format-encrypted").assertIsSelected()
        composeRule.onNodeWithTag("export-format-plain").assertIsNotSelected()
    }

    @Test
    fun testSingleImportDestinationGroupExposesOneSelectedVault() {
        val preview = ImportPreview(vaultID = "personal", vaultName = "Personal", itemCount = 1)
        show { CredentialImportScreen(model, CredentialImportSheet.Previewing(preview)) }
        assertChoices(groups = 1, options = model.credentialImport.allVault.size, selected = 1)
        composeRule.onNodeWithTag("credential-import-vault-personal").assertIsSelected()
        composeRule.onNodeWithTag("credential-import-vault-family").assertIsNotSelected()
    }

    @Test
    fun testEachGroupedImportDestinationHasItsOwnSelectedVault() {
        val preview = ImportPreview(groups = listOf(
            ImportSourceGroup(id = "first", name = "First", vaultID = "personal", vaultName = "Personal"),
            ImportSourceGroup(id = "second", name = "Second", vaultID = "family", vaultName = "Family"),
        ))
        show { CredentialImportScreen(model, CredentialImportSheet.Previewing(preview)) }
        assertChoices(groups = 2, options = model.credentialImport.allVault.size * 2, selected = 2)
        composeRule.onNodeWithTag("credential-import-group-0-vault-personal").assertIsSelected()
        composeRule.onNodeWithTag("credential-import-group-0-vault-family").assertIsNotSelected()
        composeRule.onNodeWithTag("credential-import-group-1-vault-family").assertIsSelected()
        composeRule.onNodeWithTag("credential-import-group-1-vault-personal").assertIsNotSelected()
    }
}
