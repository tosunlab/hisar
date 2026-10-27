package com.tryhisar.hisar.ui.settings

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class NavigationRowLayoutTests {
    @get:Rule val compose = createComposeRule()

    @Test
    fun testSettingsValuesStackWithoutTakingWidthFromTheirNames() {
        val setting = mutableStateOf("Appearance" to "System · Neutral")
        val scale = mutableStateOf(1.3f)
        compose.setContent {
            CompositionLocalProvider(LocalDensity provides Density(LocalDensity.current.density, scale.value)) {
                Box(Modifier.width(320.dp)) {
                    NavigationRow(setting.value.first, "setting", {}, value = setting.value.second)
                }
            }
        }
        for (fontScale in listOf(1.3f, 2f)) {
            for (row in listOf(
                "Appearance" to "System · Neutral",
                "Security" to "Biometrics · 15 minutes",
                "Mac approvals" to "Waiting for your Mac",
                "People" to "12 invitations waiting for you",
            )) {
                compose.runOnIdle { scale.value = fontScale; setting.value = row }
                val label = compose.onNodeWithText(row.first, useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
                val value = compose.onNodeWithText(row.second, useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
                assertTrue("$row at $fontScale does not overlap", value.top >= label.bottom || value.left >= label.right)
                val layouts = mutableListOf<TextLayoutResult>()
                compose.onNodeWithText(row.first, useUnmergedTree = true).performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(layouts) }
                assertTrue("the name fits without fragmentation", layouts.single().lineCount == 1)
                compose.onNodeWithTag("setting").assertHeightIsAtLeast(48.dp)
            }
        }
    }

    @Test
    fun testShortValuesStayBesideTheirNameWhenTheyFit() {
        compose.setContent {
            Box(Modifier.width(360.dp)) { NavigationRow("People", "setting", {}, value = "2") }
        }
        val label = compose.onNodeWithText("People", useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
        val value = compose.onNodeWithText("2", useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
        assertTrue(value.left >= label.right)
        assertTrue(value.top < label.bottom)
    }
}
