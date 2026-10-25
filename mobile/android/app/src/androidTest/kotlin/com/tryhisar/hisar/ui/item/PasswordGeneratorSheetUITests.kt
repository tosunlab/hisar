package com.tryhisar.hisar.ui.item

import android.content.res.Configuration
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.performScrollTo
import androidx.test.core.app.ApplicationProvider
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.HisarUITestCase
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PasswordGeneratorSheetUITests : HisarUITestCase() {
    @Test
    fun testLandscapeGeneratorOptionsAreReachableAndSaved() {
        val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        launch()
        signUp(readArgument("HISAR_UI_TEST_GENERATOR_SHEET_INVITE_LINK"), readArgument("HISAR_UI_TEST_GENERATOR_SHEET_EMAIL"), "Generator sheet")
        tap("vault-Personal")
        choose("add-login", "add-item")
        type("Landscape password", "editor-title")
        try {
            device.setOrientationLeft()
            composeRule.waitUntil("the editor is in landscape", 10_000) {
                application.resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE
            }
            tap("editor-generate-password")
            waitFor("generated-password")
            composeRule.onNode(hasTestTag("generator-close")).assertIsDisplayed()
            composeRule.onNode(hasTestTag("generator-use")).assertIsDisplayed()
            composeRule.onNode(hasTestTag("generator-digits")).performScrollTo().assertIsDisplayed()
            tap("generator-digits")
            composeRule.onNode(hasTestTag("generator-symbols")).performScrollTo().assertIsDisplayed()
            tap("generator-symbols")
            composeRule.waitUntil("the generated password has neither digits nor symbols", 10_000) {
                val password = readText("generated-password")
                password.length == 24 && password.all { it in 'a'..'z' || it in 'A'..'Z' }
            }
            val generatedPassword = readText("generated-password")
            composeRule.onNode(hasTestTag("generator-close")).assertIsDisplayed()
            composeRule.onNode(hasTestTag("generator-use")).assertIsDisplayed().assertIsEnabled()
            tap("generator-use")
            waitForGone("generator-use")
            tap("editor-save")
            waitForGone("editor-save")
            val backend = application.backend
            val vault = backend.listAllVault().single { it.name == "Personal" }
            val item = backend.listAllItem(vault.id).single { it.title == "Landscape password" }
            val password = backend.getItem(vault.id, item.id).password
            assertEquals(generatedPassword, password)
            assertTrue(password.all { it in 'a'..'z' || it in 'A'..'Z' })
        } finally {
            device.setOrientationNatural()
            device.unfreezeRotation()
        }
    }
}
