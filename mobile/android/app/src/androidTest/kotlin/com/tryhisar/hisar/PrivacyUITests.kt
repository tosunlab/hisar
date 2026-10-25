package com.tryhisar.hisar

import android.graphics.Bitmap
import android.graphics.Rect
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.isOn
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * What the app switcher shows of Hisar on a real account, with screenshots allowed. A sheet is a window of its own,
 * above the activity's privacy cover, so the card Android captures for Recents must hide it as it hides the screen.
 */
@RunWith(AndroidJUnit4::class)
class PrivacyUITests : HisarUITestCase() {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)

    @Test
    fun testAppSwitcherHidesAnOpenSheet() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_PRIVACY_INVITE_LINK"), readArgument("HISAR_UI_TEST_PRIVACY_EMAIL"), "Android Privacy")
        openSettings("open-security")
        val isAllowed = hasTestTag("allow-screenshots-switch") and isOn()
        if (!exists(isAllowed)) tap("allow-screenshots-switch")
        waitFor(isAllowed)
        goBack()
        openTab("Vaults")
        tap("vault-Personal")
        choose("add-login", "add-item")
        tap("editor-generate-password")
        waitFor(hasTestTag("generator-use") and isEnabled())
        val password = readText("generated-password")
        val passwordBounds = findOnScreen(password)
        val inFront = countInk(captureScreen("privacy-sheet"), passwordBounds)
        assertTrue("a screenshot shows the sheet's password while Hisar is in front ($inFront marked pixels)", inFront > 0)

        device.pressHome()
        device.pressRecentApps()
        val card = device.findRecentsCard { it == "Hisar" }
        val inRecents = countInk(captureScreen("privacy-recents"), card)
        assertTrue("Recents shows Hisar's card with the sheet's password on it ($inRecents marked pixels of ${card.width() * card.height()})", inRecents * 1000 < card.width() * card.height())

        device.click(card.centerX(), card.centerY())
        waitFor(hasTestTag("generated-password") and hasText(password))
        composeRule.waitUntil("back in front, a screenshot shows the sheet's password again", 10_000) {
            countInk(takeScreenshot(), findOnScreen(password)) > 0
        }
        captureScreen("privacy-sheet-again")
    }

    private fun findOnScreen(text: String): Rect =
        checkNotNull(device.wait(Until.findObject(By.text(text)), 10_000)) { "\"$text\" is not on the screen" }.visibleBounds

    private fun captureScreen(name: String): Bitmap {
        val screenshot = takeScreenshot()
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
        return screenshot
    }
}
