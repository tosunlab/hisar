package com.tryhisar.hisar.ui.item

import androidx.activity.ComponentActivity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performSemanticsAction
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PasswordGeneratorControlTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After
    fun stopModel() {
        scope.cancel()
    }

    private fun show(isSheet: Boolean) {
        val backend = FakeBackend().apply { seedSignedIn() }
        runBlocking(Dispatchers.Main) { model.start { backend } }
        composeRule.setContent {
            HisarTheme {
                if (isSheet) PasswordGeneratorSheet(model, onUse = {}, onDismiss = {})
                else StandalonePasswordGeneratorScreen(model, onBack = {})
            }
        }
    }

    private fun adjustLength() {
        composeRule.onAllNodesWithTag("generator-length").assertCountEquals(1)
        composeRule.onAllNodes(hasContentDescription("Increment") or hasContentDescription("Decrement")).assertCountEquals(0)
        val slider = composeRule.onNodeWithTag("generator-length").performScrollTo()
        val range = slider.fetchSemanticsNode().config[SemanticsProperties.ProgressBarRangeInfo]
        assertEquals(8f..64f, range.range)
        assertEquals(55, range.steps)
        for (length in listOf(8, 9, 64, 63)) {
            slider.performSemanticsAction(SemanticsActions.SetProgress) { setProgress -> setProgress(length.toFloat()) }
            composeRule.onNodeWithTag("generator-length-value").assertTextEquals("$length")
            composeRule.waitUntil(5_000) {
                composeRule.onNodeWithTag("generated-password").fetchSemanticsNode()
                    .config[SemanticsProperties.Text].single().text.length == length
            }
        }
    }

    @Test
    fun testStandaloneLengthHasOneAccessibleAdjustableControl() {
        show(isSheet = false)
        adjustLength()
    }

    @Test
    fun testEditorSheetLengthHasOneAccessibleAdjustableControl() {
        show(isSheet = true)
        adjustLength()
    }
}
