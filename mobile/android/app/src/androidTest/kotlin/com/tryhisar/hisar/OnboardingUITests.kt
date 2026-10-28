package com.tryhisar.hisar

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeUp
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
import com.tryhisar.hisar.ui.assertFieldText
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
    private val chooser = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))
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

    @Test fun testCancelledShareOrPrintDoesNotSaveKit() {
        createAccount()
        tap("new-secret-key-share-kit")
        compose.waitUntil(10_000) { device.hasObject(chooser) }
        capture("kit-share-chooser")
        device.pressBack()
        assertTrue(device.wait(Until.gone(chooser), 10_000))
        compose.waitForIdle()
        assertEquals("", model.status.value.emergencyKit?.savedBy)
        compose.onNodeWithTag("new-secret-key-done").assertDoesNotExist()
        tap("new-secret-key-print-kit")
        assertTrue(device.wait(Until.hasObject(By.pkg("com.android.printspooler")), 10_000))
        device.pressBack()
        assertTrue(device.wait(Until.gone(By.pkg("com.android.printspooler")), 10_000))
        compose.waitForIdle()
        assertEquals("", model.status.value.emergencyKit?.savedBy)
        assertEquals(0, backend.countCall("recordNewAccountEmergencyKitDelivery"))
        compose.onNodeWithTag("new-secret-key-done").assertDoesNotExist()
        compose.onNodeWithTag("new-secret-key-later").assertIsDisplayed()
        capture("kit-after-cancelled-print")
        device.pressBack()
        compose.waitUntil(10_000) { model.signIn.newAccountStep.value == null }
        assertTrue(model.status.value.emergencyKit != null)
        compose.onNodeWithTag("emergency-kit-banner").assertExists()
    }

    @Test fun testAutoLockDuringKitSaveResumesAfterUnlock() {
        createAccount()
        runBlocking(Dispatchers.Main) { model.lock() }
        compose.waitUntil(10_000) { model.status.value.state == VaultStatus.STATE_LOCKED }
        compose.onNodeWithTag("lock-emergency-kit-reminder").performScrollTo().assertExists()
        capture("locked-during-kit")
        type("unlock-password", FakeBackend.PASSWORD)
        tap("unlock-submit")
        compose.waitUntil(10_000) { model.signIn.newAccountStep.value == NewAccountStep.SaveSecretKey }
        compose.onNodeWithTag("kit-master-password").assertDoesNotExist()
        compose.onNodeWithTag("new-secret-key-later").assertIsDisplayed()
        capture("resumed-kit-after-unlock")
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

    @Test fun testLostKeyHelpPreservesCredentials() {
        start()
        tap("welcome-sign-in")
        type("sign-in-email", "ada@example.com")
        type("sign-in-secret-key", FakeBackend.SECRET_KEY)
        type("sign-in-password", FakeBackend.PASSWORD)
        tap("secret-key-note")
        capture("secret-key-help")
        tap("secret-key-help-done")
        compose.onNodeWithTag("sign-in-email").performScrollTo().assertExists()
        compose.assertFieldText("sign-in-email", "ada@example.com")
        compose.assertFieldText("sign-in-password", FakeBackend.PASSWORD)
    }

    @Test fun testInvitationEmailAndServerStayBound() {
        start()
        compose.runOnUiThread { model.signIn.openLink("hisar://invite/code?server=https%3A%2F%2Finvite.example%2Fbase&email=Ada%40Example.com") }
        compose.onNodeWithTag("sign-up-bound-email").performScrollTo().assertExists()
        compose.onNodeWithTag("sign-up-email").assertDoesNotExist()
        capture("invitation-bound-email")
        tap("remove-invitation")
        compose.waitUntil(10_000) { model.signIn.signUpPolicy.value is com.tryhisar.hisar.app.SignUpPolicyState.Ready }
        compose.onNodeWithTag("sign-up-email").assertExists()
    }

    @Test fun testRegistrationClosedAfterCheckKeepsForm() {
        backend.signUpPolicyMode = "open"
        start()
        tap("welcome-create-account")
        compose.waitUntil(10_000) { model.signIn.signUpPolicy.value is com.tryhisar.hisar.app.SignUpPolicyState.Ready }
        type("sign-up-name", "Ada")
        type("sign-up-email", "ada@example.com")
        type("sign-up-password", FakeBackend.PASSWORD)
        type("sign-up-repeated-password", FakeBackend.PASSWORD)
        backend.signUpWithoutInviteErrorMessage = "Sign-ups are not open on this server. Ask an admin for an invitation."
        backend.signUpPolicyMode = "invite"
        tap("sign-in-submit")
        compose.waitUntil(10_000) { model.signIn.isInvitationRequiredAfterSubmit.value }
        compose.onNodeWithTag("sign-up-invite-link").performScrollTo().assertExists()
        for ((tag, expected) in mapOf("sign-up-name" to "Ada", "sign-up-email" to "ada@example.com", "sign-up-password" to FakeBackend.PASSWORD, "sign-up-repeated-password" to FakeBackend.PASSWORD)) {
            compose.assertFieldText(tag, expected)
        }
        assertEquals(1, backend.countCall("signUpWithoutInvite"))
        capture("registration-closed-form-kept")
    }

    @Test fun testMissingCredentialFocusesItsFieldWithoutSendingRequest() {
        start()
        tap("welcome-sign-in")
        tap("sign-in-submit")
        compose.onNodeWithText("Enter your email.").assertExists()
        val email = compose.onNodeWithTag("sign-in-email").fetchSemanticsNode().config
        assertTrue(email[androidx.compose.ui.semantics.SemanticsProperties.Focused])
        assertEquals("Email, required", email[androidx.compose.ui.semantics.SemanticsProperties.StateDescription])
        assertEquals(0, backend.countCall("signIn"))
    }

    @Test fun testSecretKeySettingsQRNeedsAuthorizationAndClearsOnLock() {
        val server = "https://qr.example/base"
        val email = "ada@example.com"
        val payload = "hisar://sign-in?server=https%3A%2F%2Fqr.example%2Fbase&email=ada%40example.com&secretKey=${FakeBackend.SECRET_KEY}"
        val matrix = com.google.zxing.qrcode.QRCodeWriter().encode(payload, com.google.zxing.BarcodeFormat.QR_CODE, 480, 480)
        val bitmap = android.graphics.Bitmap.createBitmap(matrix.width, matrix.height, android.graphics.Bitmap.Config.ARGB_8888)
        for (y in 0 until bitmap.height) for (x in 0 until bitmap.width) bitmap.setPixel(x, y, if (matrix[x, y]) android.graphics.Color.BLACK else android.graphics.Color.WHITE)
        val bytes = java.io.ByteArrayOutputStream().also { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }.toByteArray()
        val encoded = android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP)
        backend = object : FakeBackend() {
            override fun exportEmergencyKit(password: String, directory: File): com.tryhisar.hisar.model.EmergencyKitExport =
                super.exportEmergencyKit(password, directory).copy(qrCode = encoded)
        }.apply {
            isBiometricAvailable = false
            seedSignedIn()
            status = status.copy(server = server, email = email)
        }
        start()
        tap("tab-settings")
        tap("open-account")
        tap("open-account-secret-key")
        compose.onNodeWithTag("secret-key-qr-code").assertDoesNotExist()
        type("secret-key-password", FakeBackend.PASSWORD)
        tap("secret-key-reveal")
        compose.waitUntil(10_000) { backend.countCall("exportEmergencyKit") == 1 }
        compose.onNodeWithTag("secret-key-qr-code").performScrollTo().assertIsDisplayed()
        val qrBottom = compose.onNodeWithTag("secret-key-qr-code").fetchSemanticsNode().boundsInRoot.bottom
        val tabTop = compose.onNodeWithTag("tab-settings").fetchSemanticsNode().boundsInRoot.top
        if (qrBottom > tabTop) {
            compose.onNode(hasScrollAction() and hasAnyDescendant(hasTestTag("secret-key-qr-code")))
                .performSemanticsAction(SemanticsActions.ScrollBy) { it(0f, qrBottom - tabTop + 32f) }
        }
        val rendered = compose.onNodeWithTag("secret-key-qr-code").captureToImage().asAndroidBitmap()
        capture("settings-sign-in-qr")
        val imageFile = File(application.getExternalFilesDir(null), "onboarding-s6/settings-qr-widget.png")
        imageFile.outputStream().use { rendered.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
        var decoded: String? = null
        com.tryhisar.hisar.ui.signin.QrCodeAnalyzer { decoded = it }.analyze(com.tryhisar.hisar.ui.signin.BitmapImageProxy(rendered))
        assertEquals(com.tryhisar.hisar.model.SignInLink(server, email, FakeBackend.SECRET_KEY), com.tryhisar.hisar.model.SignInLink.parse(checkNotNull(decoded)))
        runBlocking(Dispatchers.Main) { model.lock() }
        compose.onNodeWithTag("secret-key-qr-code").assertDoesNotExist()
    }

    @Test fun testOwedKitSignOutCancelKeepsSession() {
        createAccount()
        tap("new-secret-key-later")
        tap("tab-settings")
        tap("sign-out")
        compose.onNodeWithText("You haven't saved this account's Emergency Kit yet.", substring = true).assertExists()
        tap("cancel-sign-out")
        assertTrue(model.status.value.isUnlocked)
        tap("sign-out")
        tap("sign-out-save-emergency-kit")
        compose.onNodeWithTag("kit-master-password").assertDoesNotExist()
        compose.onNodeWithTag("new-secret-key-later").assertIsDisplayed()
        capture("sign-out-save-instead")
    }

    @Test fun testEmergencyKitBackClosesAuthorizationGate() {
        backend.seedSignedIn()
        backend.allPendingRecovery += PendingRecovery("owed", "signup", backend.status.server, "account", backend.status.email, "Ada", "delivery-pending", keychainProtected = true)
        start()
        tap("emergency-kit-save")
        compose.onNodeWithTag("kit-master-password").performScrollTo().performClick().performTextReplacement(FakeBackend.PASSWORD)
        val keyboardPackage = device.executeShellCommand("settings get secure default_input_method").substringBefore('/').trim()
        compose.waitUntil(10_000) { device.hasObject(By.pkg(keyboardPackage)) }
        compose.onNodeWithTag("kit-authorize-cancel").assertIsDisplayed()
        capture("kit-authorization-keyboard")
        device.pressBack()
        compose.onNodeWithTag("kit-master-password").assertExists()
        device.pressBack()
        compose.waitUntil(10_000) { model.signIn.newAccountStep.value == null }
        assertEquals("", model.status.value.emergencyKit?.savedBy)
    }

    @Test fun testCreateAccountWaitsForCurrentPasswordFeedback() {
        val started = java.util.concurrent.CountDownLatch(1)
        val release = java.util.concurrent.CountDownLatch(1)
        val checks = java.util.concurrent.atomic.AtomicInteger()
        backend = object : FakeBackend() {
            override fun checkMasterPassword(password: String): com.tryhisar.hisar.model.MasterPasswordFeedback {
                if (password == FakeBackend.PASSWORD) {
                    checks.incrementAndGet()
                    started.countDown()
                    check(release.await(30, java.util.concurrent.TimeUnit.SECONDS))
                }
                return super.checkMasterPassword(password)
            }
        }.apply { isBiometricAvailable = false; signUpPolicyMode = "open" }
        try {
            start()
            tap("welcome-create-account")
            compose.waitUntil(10_000) { model.signIn.signUpPolicy.value is com.tryhisar.hisar.app.SignUpPolicyState.Ready }
            type("sign-up-name", "Ada")
            type("sign-up-email", "ada@example.com")
            type("sign-up-password", FakeBackend.PASSWORD)
            type("sign-up-repeated-password", FakeBackend.PASSWORD)
            compose.waitUntil(10_000) { started.count == 0L }
            tap("sign-in-submit")
            assertEquals(0, backend.countCall("signUpWithoutInvite"))
            release.countDown()
            compose.waitUntil(30_000) { model.signIn.newAccountStep.value == NewAccountStep.SaveSecretKey }
            assertEquals(1, checks.get())
            assertEquals(1, backend.countCall("signUpWithoutInvite"))
        } finally { release.countDown() }
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
