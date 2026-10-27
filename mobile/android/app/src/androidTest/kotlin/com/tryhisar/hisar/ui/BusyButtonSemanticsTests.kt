package com.tryhisar.hisar.ui

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertContentDescriptionEquals
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.click
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTouchInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.ui.components.GlassButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class BusyButtonSemanticsTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun testBusyButtonsKeepTheirNamesAndAnnounceOneDisabledBusyState() {
        var busy by mutableStateOf(false)
        var calls = 0
        composeRule.setContent {
            HisarTheme {
                Column {
                    ProminentButton("Sign in", { calls++ }, "primary", isBusy = busy, busyLabel = "Signing in…")
                    GlassButton("Unlock", { calls++ }, "secondary", isBusy = busy, busyLabel = "Unlocking…")
                }
            }
        }
        composeRule.onNodeWithTag("primary").performTouchInput { click() }
        composeRule.runOnIdle { assertEquals(1, calls); busy = true }
        for ((tag, name, state) in listOf(Triple("primary", "Sign in", "Signing in…"), Triple("secondary", "Unlock", "Unlocking…"))) {
            composeRule.onNodeWithTag(tag)
                .assertContentDescriptionEquals(name)
                .assertIsNotEnabled()
                .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, state))
                .performTouchInput { click() }
            composeRule.onAllNodes(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, state), useUnmergedTree = true).assertCountEquals(1)
        }
        composeRule.onAllNodes(SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo), useUnmergedTree = true).assertCountEquals(0)
        composeRule.runOnIdle { assertEquals(1, calls) }
    }
}
