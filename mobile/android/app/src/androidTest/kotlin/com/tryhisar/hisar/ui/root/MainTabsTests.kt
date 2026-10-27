package com.tryhisar.hisar.ui.root

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.app.Tab
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test

class MainTabsTests {
    @get:Rule val composeRule = createComposeRule()

    @Test
    fun tabLabelsFitAtDoubleTextSizeInNarrowWindows() {
        val width = mutableStateOf(360)
        composeRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, 2f)) {
                HisarTheme {
                    Box(Modifier.width(width.value.dp)) { TabBar(Tab.VAULTS, 0, {}, Modifier) }
                }
            }
        }
        for (windowWidth in listOf(360, 320, 280)) {
            composeRule.runOnIdle { width.value = windowWidth }
            for ((tag, title) in listOf("vaults" to "Vaults", "approvals" to "Approvals", "settings" to "Settings")) {
                composeRule.onNodeWithTag("tab-$tag").assertTextContains(title)
                val layouts = mutableListOf<TextLayoutResult>()
                composeRule.onNodeWithText(title, useUnmergedTree = true)
                    .performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(layouts) }
                assertEquals(1, layouts.single().lineCount)
                assertFalse(layouts.single().hasVisualOverflow)
            }
        }
    }
}
