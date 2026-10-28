package com.tryhisar.hisar.ui

import android.content.ActivityNotFoundException
import android.content.ClipboardManager
import android.content.ContextWrapper
import android.content.Intent
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.platform.app.InstrumentationRegistry
import com.tryhisar.hisar.model.UpdateRequirement
import com.tryhisar.hisar.ui.root.UpdateRequiredScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class UpdateRequiredScreenTests {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun testUnavailableUpdatePageCanBeCopiedAndCheckedAgain() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val noBrowserContext = object : ContextWrapper(context) {
            override fun startActivity(intent: Intent) {
                throw ActivityNotFoundException()
            }
        }
        val updateURL = "https://github.com/tosunlab/hisar/releases"
        var checkCount = 0
        composeRule.setContent {
            CompositionLocalProvider(LocalContext provides noBrowserContext) {
                HisarTheme {
                    UpdateRequiredScreen(UpdateRequirement("0.1.0", "0.2.0", updateURL), false) { checkCount++ }
                }
            }
        }
        composeRule.onNodeWithTag("update-required-copy-link").assertDoesNotExist()
        composeRule.onNodeWithTag("update-required-install").performClick()
        composeRule.onNodeWithTag("update-required-open-failed").assertIsDisplayed().assertTextContains(updateURL, substring = true)
        composeRule.onNodeWithTag("update-required-copy-link").performClick()
        composeRule.runOnIdle {
            assertEquals(updateURL, context.getSystemService(ClipboardManager::class.java).primaryClip?.getItemAt(0)?.text?.toString())
        }
        composeRule.onNodeWithTag("update-required-check-again").performClick()
        composeRule.runOnIdle { assertEquals(1, checkCount) }
    }
}
