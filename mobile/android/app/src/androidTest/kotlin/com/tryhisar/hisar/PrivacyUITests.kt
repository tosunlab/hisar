package com.tryhisar.hisar

import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.Rect
import android.os.SystemClock
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
import java.util.regex.Pattern
import kotlin.math.abs
import kotlin.math.max
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
        val card = findRecentsCard()
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

    /**
     * The task card Recents shows for Hisar, the most recent task in the middle, without its rounded corners and the app
     * icon over its top edge. It is returned once its picture has stopped changing, so a card still loading is not read.
     */
    private fun findRecentsCard(): Rect {
        val snapshot = By.res(Pattern.compile(".*:id/snapshot"))
        check(device.wait(Until.hasObject(snapshot), 10_000)) { "Recents shows no task card" }
        val middle = device.displayWidth / 2
        val card = Rect(device.findObjects(snapshot).minBy { abs(it.visibleBounds.centerX() - middle) }.visibleBounds)
        for (icon in device.findObjects(By.res(Pattern.compile(".*:id/icon")))) {
            val bounds = icon.visibleBounds
            if (Rect.intersects(bounds, card) && bounds.top <= card.top) card.top = max(card.top, bounds.bottom)
        }
        val corner = card.width() / 20
        card.inset(corner, corner)
        var last = crop(takeScreenshot(), card)
        val deadline = SystemClock.uptimeMillis() + 10_000
        while (SystemClock.uptimeMillis() < deadline) {
            SystemClock.sleep(1_000)
            val next = crop(takeScreenshot(), card)
            if (next.sameAs(last)) return card
            last = next
        }
        throw AssertionError("Hisar's card in Recents kept changing")
    }

    private fun takeScreenshot(): Bitmap {
        val screenshot = checkNotNull(instrumentation.uiAutomation.takeScreenshot()) { "no screenshot" }
        return if (screenshot.config == Bitmap.Config.HARDWARE) screenshot.copy(Bitmap.Config.ARGB_8888, false) else screenshot
    }

    private fun captureScreen(name: String): Bitmap {
        val screenshot = takeScreenshot()
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
        return screenshot
    }

    private fun crop(screenshot: Bitmap, area: Rect): Bitmap = Bitmap.createBitmap(screenshot, area.left, area.top, area.width(), area.height())

    /** The pixels in [area] that stand out from its most common colour as text does, beyond anti-aliasing and shading. */
    private fun countInk(screenshot: Bitmap, area: Rect): Int {
        val pixels = IntArray(area.width() * area.height())
        screenshot.getPixels(pixels, 0, area.width(), area.left, area.top, area.width(), area.height())
        val background = pixels.asIterable().groupingBy { it }.eachCount().maxBy { it.value }.key
        return pixels.count { pixel ->
            max(abs(Color.red(pixel) - Color.red(background)), max(abs(Color.green(pixel) - Color.green(background)), abs(Color.blue(pixel) - Color.blue(background)))) > INK_DISTANCE
        }
    }

    private companion object {
        const val INK_DISTANCE = 64
    }
}
