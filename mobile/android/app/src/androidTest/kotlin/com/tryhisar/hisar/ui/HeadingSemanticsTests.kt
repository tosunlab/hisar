package com.tryhisar.hisar.ui

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.Text
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.GroupedForm
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.item.InlineTitleForm
import com.tryhisar.hisar.ui.settings.SettingsScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class HeadingSemanticsTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()
    private val heading = SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading)

    @Test
    fun testScreenAndSectionTitlesAreHeadingsButLabelsAndFootnotesAreNot() {
        composeRule.setContent {
            HisarTheme {
                GroupedForm("Login") {
                    for (title in listOf("Websites", "Other fields", "Notes")) {
                        FormSection(header = title, footer = { Text("Explanation") }) { Text("Field label") }
                    }
                    FormSection(header = "") { Text("No heading") }
                    LargeTitle("")
                }
            }
        }
        for (title in listOf("Login", "Websites", "Other fields", "Notes")) {
            composeRule.onAllNodes(heading and hasText(title)).assertCountEquals(1)
        }
        composeRule.onAllNodes(heading).assertCountEquals(4)
    }

    @Test
    fun testEmptyToolbarDoesNotDuplicateLargeTitle() {
        composeRule.setContent {
            HisarTheme {
                Column { PushedTopBar("", {}); LargeTitle("Vaults") }
            }
        }
        composeRule.onAllNodes(heading).assertCountEquals(1)
        composeRule.onAllNodes(heading and hasText("Vaults")).assertCountEquals(1)
    }

    @Test
    fun testPushedAndEditorInlineTitlesAreHeadings() {
        composeRule.setContent {
            HisarTheme {
                Column {
                    PushedTopBar("Account", {})
                    InlineTitleForm("Edit Login", leading = {}, trailing = {}) {}
                }
            }
        }
        composeRule.onAllNodes(heading and hasText("Account")).assertCountEquals(1)
        composeRule.onAllNodes(heading and hasText("Edit Login")).assertCountEquals(1)
    }

    @Test
    fun testSettingsCustomInlineTitleHasNoDuplicate() {
        composeRule.setContent {
            HisarTheme { SettingsScreen("Appearance", null, isTitleInline = true, leading = {}) {} }
        }
        composeRule.onAllNodes(heading).assertCountEquals(1)
        composeRule.onAllNodes(heading and hasText("Appearance")).assertCountEquals(1)
    }
}
