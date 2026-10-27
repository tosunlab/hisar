package com.tryhisar.hisar.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.unit.Density
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.ItemRow
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class BrowseRowTests {
    @get:Rule val composeRule = createComposeRule()

    @Test
    fun testCrossVaultRowsReserveSourceLine() {
        val scale = mutableFloatStateOf(1f)
        val summary = "long-host.".repeat(20)
        val item = ItemDetail(id = "database", title = "Production", username = summary)
        val model = AppModel(CoroutineScope(Dispatchers.Main), Dispatchers.Unconfined, FakeTextClipboard())
        composeRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, scale.floatValue)) {
                HisarTheme {
                    Column {
                        ItemRow(model, item, "personal", {}, "Personal")
                        ItemRow(model, item, "shared", {}, "Organization production credentials")
                    }
                }
            }
        }
        for (fontScale in listOf(1f, 2f)) {
            composeRule.runOnIdle { scale.floatValue = fontScale }
            val summaries = composeRule.onAllNodesWithText(summary, useUnmergedTree = true).fetchSemanticsNodes()
            for ((index, vault) in listOf("Personal", "Organization production credentials").withIndex()) {
                val name = composeRule.onNodeWithText(vault, useUnmergedTree = true).fetchSemanticsNode()
                assertTrue("the complete source vault has its own line", name.boundsInRoot.top >= summaries[index].boundsInRoot.bottom)
            }
        }
    }
}
