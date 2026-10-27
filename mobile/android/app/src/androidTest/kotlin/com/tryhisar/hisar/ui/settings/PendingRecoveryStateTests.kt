package com.tryhisar.hisar.ui.settings

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.PendingRecovery
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.assertFieldText
import com.tryhisar.hisar.ui.tap
import com.tryhisar.hisar.ui.type
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
        composeRule.onNodeWithTag("pending-recovery-pending-1").assertDoesNotExist()
        composeRule.onNodeWithTag("pending-recovery-phase").assertExists()
        composeRule.onNodeWithTag("pending-recovery-acknowledge").assertDoesNotExist()
        composeRule.type("pending-recovery-password", FakeBackend.PASSWORD)
        composeRule.tap("pending-recovery-export")
        composeRule.waitUntil(30_000) { model.pendingRecovery.export.value != null }
        composeRule.assertFieldText("pending-recovery-password", FakeBackend.PASSWORD)
        composeRule.onNodeWithTag("pending-recovery-secret-key").assertExists()
        composeRule.tap("pending-recovery-copy-secret-key")
        composeRule.onNodeWithTag("pending-recovery-copy-secret-key").assertTextEquals("Copied · clears in about 2 minutes")
        assertTrue("acknowledgePendingRecovery" !in backend.allCallName)
        val kit = File(model.pendingRecovery.export.value!!.path)

        restoration.emulateSavedInstanceStateRestore()

        composeRule.onNodeWithTag("pending-recovery-phase").assertTextEquals("Your account is ready. Save and confirm your Emergency Kit to finish.")
        composeRule.onNodeWithTag("pending-recovery-secret-key").assertExists()
        composeRule.assertFieldText("pending-recovery-password", "")
        assertTrue("recreating the screen deleted the prepared kit", kit.exists())
    }
}
