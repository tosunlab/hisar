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
import kotlin.math.ceil
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
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
                val label = composeRule.onNodeWithText(title, useUnmergedTree = true)
                val layouts = mutableListOf<TextLayoutResult>()
                label.performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(layouts) }
                val layout = layouts.single()
                assertEquals(1, layout.lineCount)
                // The rebuilt layout of a no-wrap label reports overflow whenever it is narrower than its constraints, so the
                // label's own size is compared with the space its whole line needs.
                val labelNode = label.fetchSemanticsNode()
                assertTrue("$title at $windowWidth dp is cut short", ceil(layout.multiParagraph.intrinsics.maxIntrinsicWidth) <= labelNode.size.width && ceil(layout.multiParagraph.height) <= labelNode.size.height)
                val tabBounds = composeRule.onNodeWithTag("tab-$tag").fetchSemanticsNode().boundsInRoot
                val labelBounds = labelNode.boundsInRoot
                assertTrue("$title at $windowWidth dp leaves its tab", labelBounds.left >= tabBounds.left && labelBounds.right <= tabBounds.right && labelBounds.top >= tabBounds.top && labelBounds.bottom <= tabBounds.bottom)
            }
        }
    }
}
