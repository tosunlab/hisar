package com.tryhisar.hisar.ui.settings

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.PendingRecovery
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.theme.HisarTheme
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** What the account recovery screen keeps when Android recreates it, as it does when the phone turns. */
@RunWith(AndroidJUnit4::class)
class PendingRecoveryStateTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After
    fun stopModel() {
        scope.cancel()
    }

    @Test
    fun testPreparedKitStaysWhenTheScreenIsRecreated() {
        val backend = FakeBackend().apply {
            allPendingRecovery += PendingRecovery(
                operationId = "pending-1", kind = "signup", origin = "https://recovery.example", accountId = "pending-account",
                email = "pending@example.com", name = "Pending", phase = "delivery-pending", keychainProtected = true,
            )
        }
        runBlocking(Dispatchers.Main) { model.start { backend } }
        val restoration = StateRestorationTester(composeRule)
        restoration.setContent {
            HisarTheme {
                Box(Modifier.fillMaxSize().exposeTestTags()) { PendingRecoveryScreen(model) }
            }
        }
        composeRule.onNodeWithTag("pending-recovery-pending-1").performClick()
        assertTrue(runBlocking(Dispatchers.Main) { model.pendingRecovery.exportKit("pending-1", FakeBackend.PASSWORD) })
        composeRule.onNodeWithTag("pending-recovery-secret-key").assertExists()
        val kit = File(model.pendingRecovery.export.value!!.path)

        restoration.emulateSavedInstanceStateRestore()

        composeRule.onNodeWithTag("pending-recovery-phase").assertTextEquals("Your account is ready. Save and confirm your Emergency Kit to finish.")
        composeRule.onNodeWithTag("pending-recovery-secret-key").assertExists()
        assertTrue("recreating the screen deleted the prepared kit", kit.exists())
    }
}
