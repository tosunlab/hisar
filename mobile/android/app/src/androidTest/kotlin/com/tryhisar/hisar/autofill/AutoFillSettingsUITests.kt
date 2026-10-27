package com.tryhisar.hisar.autofill

import android.content.Intent
import android.provider.Settings
import android.os.ParcelFileDescriptor
import android.view.autofill.AutofillManager
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.platform.waitUntil
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Settings → AutoFill says whether Hisar is the device's autofill service, notices when the user returns, and leads to the
 * system's settings on every API, as the iPhone's "Open AutoFill & Passwords".
 */
@RunWith(AndroidJUnit4::class)
class AutoFillSettingsUITests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val instrumentation = InstrumentationRegistry.getInstrumentation()

    @After
    fun turnOffHisarAutoFill() {
        shell("settings delete secure autofill_service")
    }

    @Test
    fun testSettingsShowsTheServiceState() {
        shell("settings delete secure autofill_service")
        composeRule.setContent { HisarTheme { Column { AutoFillSettingsSections() } } }
        waitForTag("turn-on-autofill")
        assertTrue(composeRule.onAllNodes(hasText("You can also turn it on", substring = true)).fetchSemanticsNodes().isNotEmpty())
        val device = UiDevice.getInstance(instrumentation)
        composeRule.activityRule.scenario.onActivity { it.startActivity(Intent(Settings.ACTION_SETTINGS)) }
        assertTrue("the user leaves for Settings", device.wait(Until.hasObject(By.pkg("com.android.settings")), 10_000))

        shell("settings put secure autofill_service ${instrumentation.targetContext.packageName}/${HisarAutofillService::class.java.name}")
        // The system applies the setting on its own thread, as it would while the user is still in its settings screen.
        val autofillManager = composeRule.activity.getSystemService(AutofillManager::class.java)
        waitUntil(10_000, "the system makes Hisar the autofill service") { autofillManager.hasEnabledAutofillServices() }
        device.pressBack()
        waitForTag("autofill-is-on")
        assertFalse(composeRule.onAllNodes(hasText("You can also turn it on", substring = true)).fetchSemanticsNodes().isNotEmpty())
        assertFalse(composeRule.onAllNodes(hasText("Change the service", substring = true)).fetchSemanticsNodes().isNotEmpty())
    }

    @Test
    fun testOnAutoFillOffersTheSystemSettings() {
        shell("settings put secure autofill_service ${instrumentation.targetContext.packageName}/${HisarAutofillService::class.java.name}")
        val autofillManager = composeRule.activity.getSystemService(AutofillManager::class.java)
        waitUntil(10_000, "the system makes Hisar the autofill service") { autofillManager.hasEnabledAutofillServices() }
        composeRule.setContent { HisarTheme { Column { AutoFillSettingsSections() } } }
        waitForTag("autofill-is-on")
        composeRule.onNodeWithTag("open-autofill-settings").performClick()
        val device = UiDevice.getInstance(instrumentation)
        assertTrue("the Settings app opens", device.wait(Until.hasObject(By.pkg("com.android.settings")), 10_000))
        device.pressBack()
        waitForTag("autofill-is-on")
    }

    private fun waitForTag(tag: String) {
        composeRule.waitUntil("$tag appears", 10_000) { composeRule.onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty() }
    }

    private fun shell(command: String) {
        ParcelFileDescriptor.AutoCloseInputStream(instrumentation.uiAutomation.executeShellCommand(command)).use { it.readBytes() }
    }
}
