package com.tryhisar.hisar.ui.sharing

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.BarCapsuleButton
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ModalSheetBarTests {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun testSheetHeadingDoesNotOverlapActionsAsTextGrows() {
        var fontScale by mutableFloatStateOf(1f)
        composeRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                HisarTheme {
                    Column(Modifier.width(320.dp)) {
                        ModalSheetBar("Create organization", onCancel = {}) {
                            BarCapsuleButton("Create", {}, testTag = "create")
                        }
                    }
                }
            }
        }
        var headingHeight = 0f
        for (scale in listOf(1f, 1.3f, 2f)) {
            composeRule.runOnIdle { fontScale = scale }
            composeRule.waitForIdle()
            val cancel = composeRule.onNodeWithTag("sheet-cancel").fetchSemanticsNode().boundsInRoot
            val create = composeRule.onNodeWithTag("create").fetchSemanticsNode().boundsInRoot
            val heading = composeRule.onNodeWithText("Create organization").fetchSemanticsNode().boundsInRoot
            assertTrue("Cancel stays separate from Create at text scale $scale", cancel.right <= create.left)
            assertTrue("The full heading is below Cancel at text scale $scale", cancel.bottom <= heading.top)
            assertTrue("The full heading is below Create at text scale $scale", create.bottom <= heading.top)
            assertTrue("The heading grows at text scale $scale", heading.height > headingHeight)
            headingHeight = heading.height
        }
    }
}
