package com.tryhisar.hisar.ui.settings

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertExists
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.CredentialImportSheet
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportSourceGroup
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import org.junit.After
import org.junit.Rule
import org.junit.Test

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
}
