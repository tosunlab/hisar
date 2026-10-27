package com.tryhisar.hisar.ui.lock

import androidx.activity.ComponentActivity
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class UnlockPanelFocusTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun unsuccessfulBiometricsFocusThePasswordForAutomaticAndManualAttempts() {
        var attempts = 0
        val resumed = mutableStateOf(false)
        val lockCount = mutableStateOf(1)
        composeRule.setContent {
            HisarTheme {
                UnlockPanel(
                    email = "alice@example.com", isBiometricReady = true, isBusy = false, errorMessage = null,
                    lockCount = lockCount.value, isResumed = resumed.value,
                    isCurrentLock = { true },
                    submitPassword = { false }, submitBiometric = { attempts++; false },
                )
            }
        }
        composeRule.runOnIdle { assertEquals(0, attempts); resumed.value = true }
        composeRule.onNodeWithTag("unlock-password").assertIsFocused()
        composeRule.runOnIdle { assertEquals(1, attempts) }
        composeRule.onNodeWithTag("unlock-biometric").performClick()
        composeRule.onNodeWithTag("unlock-password").assertIsFocused()
        composeRule.runOnIdle { assertEquals(2, attempts); resumed.value = false }
        composeRule.runOnIdle { resumed.value = true }
        composeRule.runOnIdle { assertEquals(2, attempts); lockCount.value = 2 }
        composeRule.onNodeWithTag("unlock-password").assertIsFocused()
        composeRule.runOnIdle { assertEquals(3, attempts) }
    }
}
