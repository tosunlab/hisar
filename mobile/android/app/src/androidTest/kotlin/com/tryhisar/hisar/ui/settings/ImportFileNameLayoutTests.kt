package com.tryhisar.hisar.ui.settings

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ImportFileNameLayoutTests {
    @get:Rule val composeRule = createComposeRule()
    private val fileName = "Hisar export 2026-10-27 production credentials before Secret Key replacement.encrypted.json"

    private fun checkWrapping(fontScale: Float) {
        composeRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                HisarTheme { Box(Modifier.width(288.dp)) { ImportFileNameRow(fileName) } }
            }
        }
        val layouts = mutableListOf<TextLayoutResult>()
        composeRule.onNodeWithText(fileName, useUnmergedTree = true).performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(layouts) }
        assertTrue("the complete dated filename is laid out", layouts.isNotEmpty())
        assertTrue("the complete filename wraps", layouts.single().lineCount > 1)
        assertFalse("no part of the filename is clipped", layouts.single().hasVisualOverflow)
    }

    @Test fun testImportFileNameWrapsAtStandardTextSize() = checkWrapping(1f)
    @Test fun testImportFileNameWrapsAtLargeTextSize() = checkWrapping(2f)
}
