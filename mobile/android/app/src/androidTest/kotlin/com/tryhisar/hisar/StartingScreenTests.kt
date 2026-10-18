package com.tryhisar.hisar

import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class StartingScreenTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<MainActivity>()

    @Test
    fun testNewInstallShowsTheBackendSignedOut() {
        composeRule.waitUntil(timeoutMillis = 10_000) { composeRule.onAllNodes(hasText("Signed out")).fetchSemanticsNodes().isNotEmpty() }
    }
}
