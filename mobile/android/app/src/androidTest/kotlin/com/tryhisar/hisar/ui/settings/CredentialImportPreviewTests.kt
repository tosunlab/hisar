package com.tryhisar.hisar.ui.settings

import androidx.compose.runtime.mutableStateOf
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.dp
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.CredentialImportSheet
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportSourceGroup
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue

class CredentialImportPreviewTests {
    @get:Rule val compose = createComposeRule()
    private val scope = MainScope()
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After fun close() { scope.cancel() }

    @Test
    fun testPreviewUsesTheRestoreExplanationOnlyForHisarExports() {
        val format = mutableStateOf("hisar-export")
        compose.setContent {
            CredentialImportScreen(model, CredentialImportSheet.Previewing(ImportPreview(
                fileName = "export", format = format.value,
                groups = List(2) { ImportSourceGroup(id = "$it", name = "Source $it", vaultName = "Work") },
            )))
        }
        for (hisarFormat in listOf("hisar-export", "hisar-encrypted-export")) {
            compose.runOnIdle { format.value = hisarFormat }
            compose.onNodeWithText("This export has 2 vaults. Each goes to the vault it came from, or your Personal vault, unless you choose another.").assertExists()
        }
        compose.runOnIdle { format.value = "credential-exchange" }
        compose.onNodeWithText("The other app kept these items in 2 groups. Each goes to your Personal vault unless you choose another vault for it.").assertExists()
    }

    @Test
    fun testAccessParagraphsKeepTheirCardInsetsInBothThemes() {
        val appearance = mutableStateOf(ThemeAppearanceName.LIGHT)
        val member = "An unusually long destination member <long.member.address@example.com> · reader"
        val preview = ImportPreview(
            fileName = "export", exportedFrom = "alice@example.com",
            groups = listOf(ImportSourceGroup(
                name = "Personal", vaultName = "Work", members = listOf(Member(
                    accountID = "member", name = "An unusually long destination member",
                    email = "long.member.address@example.com", role = "reader",
                )),
            )),
        )
        compose.setContent {
            HisarTheme(VaultTheme(appearance = appearance.value)) {
                Box(Modifier.width(320.dp)) { CredentialImportScreen(model, CredentialImportSheet.Previewing(preview)) }
            }
        }
        for (theme in listOf(ThemeAppearanceName.LIGHT, ThemeAppearanceName.DARK)) {
            compose.runOnIdle { appearance.value = theme }
            compose.onNodeWithText(member).performScrollTo()
            val row = compose.onNodeWithTag("credential-import-group-0-access-details", useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
            val text = compose.onNodeWithText(member, useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
            assertEquals(with(compose.density) { 16.dp.toPx() }, text.left - row.left, 1f)
            assertTrue("wrapped text stays above the bottom inset", text.bottom < row.bottom)
            val warning = compose.onNodeWithText("Imported items are readable by the destination's current members. Access can change later.", useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
            assertEquals(with(compose.density) { 8.dp.toPx() }, warning.top - text.bottom, 1f)
        }
    }
}
