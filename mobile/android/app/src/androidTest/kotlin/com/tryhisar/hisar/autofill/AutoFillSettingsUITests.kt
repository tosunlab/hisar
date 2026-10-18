package com.tryhisar.hisar.autofill

import android.os.ParcelFileDescriptor
import android.view.autofill.AutofillManager
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.lifecycle.Lifecycle
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.tryhisar.hisar.platform.waitUntil
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Settings → AutoFill says whether Hisar is the device's autofill service, and notices when the user returns. */
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

        shell("settings put secure autofill_service ${instrumentation.targetContext.packageName}/${HisarAutofillService::class.java.name}")
        // The system applies the setting on its own thread, as it would while the user is still in its settings screen.
        val autofillManager = composeRule.activity.getSystemService(AutofillManager::class.java)
        waitUntil(10_000, "the system makes Hisar the autofill service") { autofillManager.hasEnabledAutofillServices() }
        composeRule.activityRule.scenario.moveToState(Lifecycle.State.STARTED)
        composeRule.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        waitForTag("autofill-is-on")
    }

    private fun waitForTag(tag: String) {
        composeRule.waitUntil("$tag appears", 10_000) { composeRule.onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty() }
    }

    private fun shell(command: String) {
        ParcelFileDescriptor.AutoCloseInputStream(instrumentation.uiAutomation.executeShellCommand(command)).use { it.readBytes() }
    }
}
