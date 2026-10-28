package com.tryhisar.hisar

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.app.NewAccountStep
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.PendingRecovery
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.ui.root.RootScreen
import java.io.File
import java.util.regex.Pattern
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class OnboardingUITests {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    private val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var backend = FakeBackend().apply { isBiometricAvailable = false }
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard(), isPlainHttpAllowed = true)
    private val fontScale = mutableFloatStateOf(1f)
    private val documents = By.pkg(Pattern.compile("com\\.android\\.documentsui|com\\.google\\.android\\.documentsui"))

    @After fun close() {
        repeat(3) { if (device.currentPackageName != application.packageName) device.pressBack() }
        scope.cancel()
    }

    private fun start() {
        application.installFakeBackend(backend)
        runBlocking(Dispatchers.Main) { model.start { application.backend } }
        compose.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale.floatValue)) {
                Box(Modifier.requiredSize(360.dp, 640.dp)) {
                    RootScreen(model, application::copySecret, {}, {})
                }
            }
        }
    }

    private fun createAccount() {
        backend.signUpPolicyMode = "open"
        start()
        tap("welcome-create-account")
        compose.waitUntil(10_000) { model.signIn.signUpPolicy.value is com.tryhisar.hisar.app.SignUpPolicyState.Ready }
        type("sign-up-name", "Ada")
        type("sign-up-email", "ada@example.com")
        type("sign-up-password", FakeBackend.PASSWORD)
        type("sign-up-repeated-password", FakeBackend.PASSWORD)
        tap("sign-in-submit")
        compose.waitUntil(30_000) { model.signIn.newAccountStep.value == NewAccountStep.SaveSecretKey }
        compose.waitUntil(30_000) { backend.countCall("exportNewAccountEmergencyKit") > 0 }
    }

    @Test fun testPostponedKitNeedsOnlyTheUnlockPassword() {
        createAccount()
        tap("new-secret-key-later")
        compose.waitUntil(10_000) { model.signIn.newAccountStep.value == null }
        tap("lock-now")
        compose.waitUntil(10_000) { model.status.value.state == VaultStatus.STATE_LOCKED }
        type("unlock-password", FakeBackend.PASSWORD)
        tap("unlock-submit")
        tap("emergency-kit-save")
        compose.onNodeWithTag("kit-master-password").assertDoesNotExist()
        compose.onNodeWithTag("pending-recovery-password").assertDoesNotExist()
        tap("new-secret-key-save-kit")
        assertTrue(device.wait(Until.hasObject(documents), 10_000))
        device.findObject(By.clazz("android.widget.EditText"))?.text = "S6-${System.currentTimeMillis()}.pdf"
        device.wait(Until.findObject(By.res(Pattern.compile(".*:id/button1")).enabled(true)), 10_000).click()
        assertTrue(device.wait(Until.gone(documents), 10_000))
        compose.waitUntil(10_000) { model.status.value.emergencyKit?.savedBy == "file" }
        capture("postponed-kit-saved")
        tap("new-secret-key-done")
        compose.waitUntil(10_000) { model.status.value.emergencyKit == null }
        assertEquals(1, backend.countCall("unlock"))
        assertEquals(0, backend.countCall("authorizeNewAccountEmergencyKit"))
        assertTrue(model.status.value.isUnlocked)
    }

    @Test fun testEmergencyKitBannerDoesNotCoverControls() {
        backend.seedSignedIn()
        backend.status = backend.status.copy(email = "a".repeat(52) + "@example.com")
        backend.allPendingRecovery += PendingRecovery("owed", "signup", backend.status.server, "account", backend.status.email, "Ada", "delivery-pending", keychainProtected = true)
        start()
        for (scale in listOf(1f, 2f)) {
            compose.runOnUiThread { fontScale.floatValue = scale }
            compose.waitForIdle()
            assertNoBannerOverlap()
            compose.onNodeWithTag("lock-now").assertIsDisplayed()
            compose.onNodeWithTag("new-menu").assertIsDisplayed()
            capture("banner-font-$scale")
            compose.onNodeWithTag("emergency-kit-not-now").performScrollTo().assertIsDisplayed()
            assertNoBannerOverlap()
            capture("banner-buttons-font-$scale")
        }
        tap("emergency-kit-not-now")
        compose.onNodeWithTag("emergency-kit-banner").assertDoesNotExist()
        tap("lock-now")
        type("unlock-password", FakeBackend.PASSWORD)
        tap("unlock-submit")
        compose.onNodeWithTag("emergency-kit-banner").assertExists()
    }

    @Test fun testInviteOnlyServerAsksForInvitationFirst() {
        backend.signUpPolicyMode = "invite"
        start()
        tap("welcome-create-account")
        compose.waitUntil(10_000) { model.signIn.signUpPolicy.value is com.tryhisar.hisar.app.SignUpPolicyState.Ready }
        compose.onNodeWithTag("sign-up-invite-link").assertExists()
        compose.onNodeWithTag("sign-up-name").assertDoesNotExist()
        compose.onNodeWithTag("sign-up-password").assertDoesNotExist()
        capture("invite-only-before-password")
    }

    @Test fun testOpenServerCreatesAccountWithoutInvitation() {
        backend.signUpPolicyMode = "open"
        start()
        tap("welcome-create-account")
        compose.waitUntil(10_000) { model.signIn.signUpPolicy.value is com.tryhisar.hisar.app.SignUpPolicyState.Ready }
        compose.onNodeWithTag("sign-up-invite-link").assertDoesNotExist()
        capture("open-server-create-account")
        type("sign-up-email", "ada@example.com")
        type("sign-up-name", "Ada")
        type("sign-up-password", FakeBackend.PASSWORD)
        type("sign-up-repeated-password", FakeBackend.PASSWORD)
        tap("sign-in-submit")
        compose.waitUntil(30_000) { model.signIn.newAccountStep.value == NewAccountStep.SaveSecretKey }
        assertEquals(1, backend.countCall("signUpWithoutInvite"))
        compose.waitUntil(10_000) { compose.onAllNodes(hasTestTag("new-secret-key")).fetchSemanticsNodes().isNotEmpty() }
    }

    private fun assertNoBannerOverlap() {
        val banner = compose.onNodeWithTag("emergency-kit-banner", useUnmergedTree = true).fetchSemanticsNode().boundsInRoot
        val controls = compose.onAllNodes(hasClickAction() and hasAnyAncestor(hasTestTag("emergency-kit-banner")).not(), useUnmergedTree = true).fetchSemanticsNodes()
        controls.forEach { node ->
            val bounds = node.boundsInRoot
            assertFalse("banner $banner covers control ${node.config} at $bounds", !bounds.isEmpty && banner.overlaps(bounds))
        }
    }

    private fun tap(tag: String) {
        val node = compose.onNodeWithTag(tag)
        if (runCatching { node.assertIsDisplayed() }.isFailure) node.performScrollTo()
        node.performClick()
    }
    private fun type(tag: String, value: String) {
        compose.onNodeWithTag(tag).performScrollTo().performTextReplacement(value)
        androidx.test.espresso.Espresso.closeSoftKeyboard()
    }
    private fun capture(name: String) {
        val directory = File(application.getExternalFilesDir(null), "onboarding-s6").apply { mkdirs() }
        device.takeScreenshot(File(directory, "$name.png"))
    }
}
