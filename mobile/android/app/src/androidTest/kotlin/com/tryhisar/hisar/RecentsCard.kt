package com.tryhisar.hisar

import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.Rect
import android.os.SystemClock
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import java.util.regex.Pattern
import kotlin.math.abs
import kotlin.math.max

/**
 * The card Recents shows in its middle, the most recent task's, checked by the name the launcher gives its task. It
 * leaves out the card's rounded corners and the icon over its top, and is returned once its picture has not changed for
 * [STILL_MILLIS], so a placeholder shown while the capture loads is not read.
 */
fun UiDevice.findRecentsCard(isTaskName: (String) -> Boolean): Rect {
    val snapshot = By.res(Pattern.compile(".*:id/snapshot"))
    check(wait(Until.hasObject(snapshot), 10_000)) { "Recents shows no task card" }
    val middle = displayWidth / 2
    val inMiddle = findObjects(snapshot).minBy { abs(it.visibleBounds.centerX() - middle) }
    val card = Rect(inMiddle.visibleBounds)
    val icon = findObjects(By.res(Pattern.compile(".*:id/icon"))).map { it.visibleBounds }
        .filter { it.centerX() in card.left..card.right }
        .minByOrNull { abs(it.top - card.top) }
    val name = inMiddle.contentDescription?.takeIf { it.isNotEmpty() } ?: readTaskName(checkNotNull(icon) { "the card in the middle of Recents has no icon" })
    check(isTaskName(name)) { "the card in the middle of Recents is the task \"$name\"" }
    if (icon != null && Rect.intersects(icon, card)) card.top = max(card.top, icon.bottom)
    val corner = card.width() / 20
    card.inset(corner, corner)
    waitUntilStill(card)
    return card
}

// Android 13's launcher describes a card with its task's name; the launchers before it name the task only in the menu
// the card's icon opens.
private fun UiDevice.readTaskName(icon: Rect): String {
    val taskName = By.res(Pattern.compile(".*:id/task_name"))
    click(icon.centerX(), icon.centerY())
    val name = checkNotNull(wait(Until.findObject(taskName), 5_000)) { "the card's icon opens no menu with its name" }.text
    pressBack()
    check(wait(Until.gone(taskName), 5_000)) { "the card's menu stays open" }
    return name
}

private fun waitUntilStill(area: Rect) {
    var still = crop(takeScreenshot(), area)
    var stillSince = SystemClock.uptimeMillis()
    val deadline = stillSince + 20_000
    while (SystemClock.uptimeMillis() < deadline) {
        SystemClock.sleep(500)
        val next = crop(takeScreenshot(), area)
        if (!next.sameAs(still)) {
            still = next
            stillSince = SystemClock.uptimeMillis()
        } else if (SystemClock.uptimeMillis() - stillSince >= STILL_MILLIS) {
            return
        }
    }
    throw AssertionError("the card in Recents kept changing")
}

private const val STILL_MILLIS = 3_000L

fun takeScreenshot(): Bitmap {
    val screenshot = checkNotNull(InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()) { "no screenshot" }
    return if (screenshot.config == Bitmap.Config.HARDWARE) screenshot.copy(Bitmap.Config.ARGB_8888, false) else screenshot
}

private fun crop(screenshot: Bitmap, area: Rect): Bitmap = Bitmap.createBitmap(screenshot, area.left, area.top, area.width(), area.height())

fun countInk(screenshot: Bitmap, area: Rect): Int {
    val row = IntArray(area.width())
    return (area.top until area.bottom).sumOf { y ->
        screenshot.getPixels(row, 0, area.width(), area.left, y, area.width(), 1)
        val background = row.asIterable().groupingBy { it }.eachCount().maxBy { it.value }.key
        row.count { pixel ->
            max(abs(Color.red(pixel) - Color.red(background)), max(abs(Color.green(pixel) - Color.green(background)), abs(Color.blue(pixel) - Color.blue(background)))) > INK_DISTANCE
        }
    }
}

private const val INK_DISTANCE = 64
