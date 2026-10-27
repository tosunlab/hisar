package com.tryhisar.hisar

import android.graphics.Bitmap
import android.os.Process
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.ComposeTimeoutException
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasScrollToNodeAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onLast
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import androidx.compose.ui.test.printToString
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import java.util.regex.Pattern
import org.junit.After
import org.junit.Rule

/**
 * Runs against a real hisar-server; scripts/test-android-ui.sh starts one and passes the invite links as instrumentation
 * arguments. Every test starts with no app data: the orchestrator clears it before each one.
 */
abstract class HisarUITestCase {
    @get:Rule
    val composeRule = createEmptyComposeRule()

    private var scenario: ActivityScenario<MainActivity>? = null

    @After
    fun closeApp() {
        scenario?.close()
        scenario = null
    }

    fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    fun readArgument(name: String): String {
        val value = InstrumentationRegistry.getArguments().getString(name)
        return requireNotNull(value?.takeIf { it.isNotEmpty() }) {
            "$name is not set; run scripts/test-android-ui.sh, which starts a server and passes it as an instrumentation argument"
        }
    }

    /** Saves the screen as android-[name].png; scripts/test-android-ui.sh copies it to HISAR_UI_TEST_SCREENSHOT_DIRECTORY. */
    fun saveScreenshot(name: String) {
        composeRule.waitForIdle()
        val screenshot = checkNotNull(InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()) { "no screenshot of $name" }
        TestStorage().openOutputFile("android-$name.png").use { screenshot.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

    /** On a timeout the failure carries the screen's semantics tree: an error banner, a disabled button or a spinner says why. */
    fun waitFor(matcher: SemanticsMatcher, timeoutMillis: Long = 30_000) {
        try {
            composeRule.waitUntil("${matcher.description} appears", timeoutMillis) { exists(matcher) }
        } catch (error: ComposeTimeoutException) {
            val screen = composeRule.onAllNodes(isRoot(), useUnmergedTree = true).printToString(maxDepth = Int.MAX_VALUE)
            throw AssertionError("${error.message}; the screen was:\n$screen", error)
        }
    }

    fun waitFor(tag: String, timeoutMillis: Long = 30_000) = waitFor(hasTestTag(tag), timeoutMillis)

    fun waitForGone(tag: String, timeoutMillis: Long = 30_000) {
        composeRule.waitUntil("$tag goes", timeoutMillis) { !exists(hasTestTag(tag)) }
    }

    fun dismissNotice() {
        if (!exists(hasTestTag("notice"))) return
        composeRule.onNode(hasTestTag("notice")).performTouchInput { swipeLeft() }
        waitForGone("notice")
    }

    fun exists(matcher: SemanticsMatcher): Boolean = composeRule.onAllNodes(matcher).fetchSemanticsNodes().isNotEmpty() ||
        composeRule.onAllNodes(matcher, useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty()

    fun tap(tag: String) {
        scrollTo(tag)
        composeRule.onNode(hasTestTag(tag)).performClick()
    }

    /** Waits until [tag] is enabled, then taps it: a disabled button ignores the tap, while a user would wait for it. */
    fun tapWhenEnabled(tag: String) {
        scrollTo(tag)
        waitFor(hasTestTag(tag) and isEnabled(), ACCOUNT_KEY_TIMEOUT)
        composeRule.onNode(hasTestTag(tag)).performClick()
    }

    fun readText(tag: String): String {
        waitFor(tag)
        val node = composeRule.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().singleOrNull()
            ?: composeRule.onNode(hasTestTag(tag), useUnmergedTree = true).fetchSemanticsNode()
        return node.config[SemanticsProperties.Text].joinToString("")
    }

    fun goBack() {
        Espresso.pressBack()
    }

    fun openTab(name: String) {
        val tab = (hasText(name) or hasContentDescription(name)) and SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.Tab)
        waitFor(tab)
        composeRule.onNode(tab).performClick()
    }

    fun openSettings(tag: String) {
        openTab("Settings")
        tap(tag)
    }

    /** Taps [label] in the confirmation dialog, not the button with [excludingTag] that opened it. */
    fun confirm(label: String, excludingTag: String = "") {
        val button = hasText(label) and hasClickAction() and !hasTestTag(excludingTag)
        waitFor(button)
        composeRule.onAllNodes(button).onLast().performClick()
    }

    fun choose(item: String, menu: String) {
        tap(menu)
        scrollTo(item)
        composeRule.onNode(hasTestTag(item)).performClick()
        composeRule.waitUntil("the $menu menu closes after choosing $item", 10_000) { !exists(hasTestTag(item)) }
    }

    /**
     * A lazy list composes only the rows on screen, so a row further down does not exist until the list scrolls to it; a
     * list still loading does not hold the row yet, so the list is scrolled again until it does.
     */
    fun scrollTo(tag: String) {
        val node = hasTestTag(tag)
        composeRule.waitUntil("$tag appears", 30_000) { exists(node) || isScrolledInto(node) }
        if (exists(node and hasAnyAncestor(hasScrollAction()))) {
            composeRule.onNode(node).performScrollTo()
            scrollClearOfFloatingControls(node)
        }
    }

    private fun isScrolledInto(node: SemanticsMatcher): Boolean {
        if (!exists(hasScrollToNodeAction())) return false
        return try {
            composeRule.onAllNodes(hasScrollToNodeAction()).onLast().performScrollToNode(node)
            exists(node)
        } catch (_: AssertionError) {
            false
        }
    }

    /**
     * performScrollTo stops at the scroll viewport's edge, beneath the floating tab bar and New button. Scroll farther
     * so taps and swipes reach the row instead of those controls.
     */
    private fun scrollClearOfFloatingControls(node: SemanticsMatcher) {
        val tabBar = hasTestTag("tab-vaults")
        if (!exists(tabBar)) return
        var controlTop = composeRule.onNode(tabBar).fetchSemanticsNode().boundsInRoot.top
        val newButton = hasTestTag("new-menu")
        if (exists(newButton)) {
            controlTop = minOf(controlTop, composeRule.onNode(newButton).fetchSemanticsNode().boundsInRoot.top)
        }
        val nodeBottom = composeRule.onNode(node).fetchSemanticsNode().boundsInRoot.bottom
        val overlap = nodeBottom - controlTop
        if (overlap > 0) {
            composeRule.onAllNodes(hasScrollAction() and hasAnyDescendant(node)).onLast()
                .performSemanticsAction(SemanticsActions.ScrollBy) { scrollBy -> scrollBy(0f, overlap + FLOATING_CONTROL_CLEARANCE) }
        }
    }

    /**
     * Typing needs no keyboard, but focusing the field asks for one, which a slow emulator shows a second or more later.
     * Arriving after the next step scrolled, it would cover the button scrolled to, and that step's tap would land on
     * nothing. Closing it at once keeps the screen as the next step finds it.
     */
    fun type(text: String, tag: String) {
        scrollTo(tag)
        composeRule.onNode(hasTestTag(tag)).performTextInput(text)
        Espresso.closeSoftKeyboard()
    }

    /** With [isBiometricOfferExpected], it stops at the biometric offer after the Secret Key; otherwise it declines any offer. */
    fun signUp(inviteLink: String, email: String, name: String, isBiometricOfferExpected: Boolean = false): String {
        waitFor("sign-in-mode", APP_START_TIMEOUT)
        saveScreenshot("sign-in")
        composeRule.onNode(hasText("Create account") and hasAnyAncestor(hasTestTag("sign-in-mode"))).performClick()
        type(inviteLink, "sign-up-invite-link")
        type(email, "sign-up-email")
        type(name, "sign-up-name")
        type(MASTER_PASSWORD, "sign-up-password")
        type(MASTER_PASSWORD, "sign-up-repeated-password")
        scrollTo("sign-in-submit")
        saveScreenshot("create-account")
        tap("sign-in-submit")
        waitFor("new-secret-key", ACCOUNT_KEY_TIMEOUT)
        val secretKey = readText("new-secret-key")
        saveScreenshot("new-secret-key")
        handOffEmergencyKit("new-secret-key-share-kit")
        tapWhenEnabled("new-secret-key-done")
        if (isBiometricOfferExpected) {
            waitFor("biometric-offer-not-now", 10_000)
            return secretKey
        }
        // The vaults are composed under the Secret Key screen and the biometric offer, so they exist before either closes.
        waitForGone("new-secret-key")
        if (exists(hasTestTag("biometric-offer-not-now"))) {
            tap("biometric-offer-not-now")
            waitForGone("biometric-offer-not-now")
        }
        waitFor("vault-Personal")
        return secretKey
    }

    /** Hands the Emergency Kit off as its owner would: opens the share sheet from [shareTag], then closes it. */
    fun handOffEmergencyKit(shareTag: String) {
        waitFor(shareTag, ACCOUNT_KEY_TIMEOUT)
        tap(shareTag)
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        val shareSheet = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))
        composeRule.waitUntil("the share sheet opens", 10_000) { device.hasObject(shareSheet) }
        device.pressBack()
        composeRule.waitUntil("the share sheet closes", 10_000) { !device.hasObject(shareSheet) }
    }

    data class PendingRecoveryKit(val recordTag: String, val secretKey: String, val phase: String, val processID: Int)

    fun preparePendingRecoveryKit(email: String, password: String): PendingRecoveryKit {
        val record = SemanticsMatcher("a pending recovery record") { node ->
            node.config.getOrNull(SemanticsProperties.TestTag)?.startsWith("pending-recovery-") == true
        }
        val account = record and hasText(email, substring = true) and !hasText("manual-unlock", substring = true)
        waitFor(account, ACCOUNT_KEY_TIMEOUT)
        val row = composeRule.onAllNodes(account).onLast()
        val recordTag = row.fetchSemanticsNode().config[SemanticsProperties.TestTag]
        row.performClick()
        val phase = readText("pending-recovery-phase")
        type(password, "pending-recovery-password")
        tapWhenEnabled("pending-recovery-export")
        waitFor("pending-recovery-secret-key", ACCOUNT_KEY_TIMEOUT)
        val secretKey = readText("pending-recovery-secret-key")
        saveScreenshot("pending-recovery-kit")
        return PendingRecoveryKit(recordTag, secretKey, phase, Process.myPid())
    }

    fun finishPendingRecovery(email: String, password: String, preparedKit: PendingRecoveryKit? = null): String {
        val kit = preparedKit ?: preparePendingRecoveryKit(email, password)
        if (preparedKit != null) {
            check(Process.myPid() != kit.processID) { "recovery must resume in a different app process" }
            waitFor(hasTestTag(kit.recordTag) and hasText(email, substring = true), ACCOUNT_KEY_TIMEOUT)
            tap(kit.recordTag)
            check(readText("pending-recovery-phase") == kit.phase) { "the prepared recovery changed across process restart" }
        }
        type(password, "pending-recovery-password")
        tapWhenEnabled("pending-recovery-acknowledge")
        if (kit.phase == "awaiting-kit") {
            waitFor(hasTestTag("pending-recovery-phase") and hasText("prepared"), ACCOUNT_KEY_TIMEOUT)
            type(kit.secretKey, "pending-recovery-input-secret-key")
            type(password, "pending-recovery-password")
            tapWhenEnabled("pending-recovery-resume")
            waitFor(hasTestTag("pending-recovery-phase") and hasText("delivery-pending"), ACCOUNT_KEY_TIMEOUT)
            type(password, "pending-recovery-password")
            tapWhenEnabled("pending-recovery-acknowledge")
        }
        composeRule.waitUntil("recovery delivery is acknowledged", ACCOUNT_KEY_TIMEOUT) {
            !exists(hasTestTag("pending-recovery-phase")) || exists(hasTestTag("pending-recovery-phase") and hasText("manual-unlock"))
        }
        if (exists(hasTestTag("pending-recovery-phase"))) tap("pending-recovery-later")
        return kit.secretKey
    }

    /** Signs in to the harness server with an account another device created. */
    fun signIn(email: String, secretKey: String, password: String) {
        waitFor("sign-in-use-different-server", APP_START_TIMEOUT)
        tap("sign-in-use-different-server")
        composeRule.onNode(hasTestTag("sign-in-server")).performTextReplacement(readArgument("HISAR_UI_TEST_SERVER"))
        type(email, "sign-in-email")
        type(secretKey, "sign-in-secret-key")
        type(password, "sign-in-password")
        tap("sign-in-submit")
        waitFor("vault-Personal", ACCOUNT_KEY_TIMEOUT)
    }

    fun addLogin(title: String, username: String, password: String, website: String, oneTimePassword: String = "") {
        tap("vault-Personal")
        choose("new-login", "new-menu")
        type(title, "editor-title")
        type(username, "editor-username")
        type(password, "editor-password")
        type(website, "editor-website-0")
        if (oneTimePassword.isNotEmpty()) {
            type(oneTimePassword, "editor-one-time-password")
        }
        saveScreenshot("add-item")
        tap("editor-save")
        waitFor("item-$title")
    }

    companion object {
        const val MASTER_PASSWORD = "ui test master password"

        /** From launching the app to its first screen, while the process and the Go backend start; over 30 seconds when loaded. */
        const val APP_START_TIMEOUT = 60_000L

        /**
         * A step that derives the account key from the master password on the device (Argon2id) and talks to the server:
         * signing up or in, changing the password or Secret Key, approving with the password. Up to 26 seconds on a loaded
         * arm64 emulator, and CI's x86_64 emulator is up to five times slower.
         */
        const val ACCOUNT_KEY_TIMEOUT = 130_000L

        /** How far above floating controls, in pixels, a tapped node ends up. */
        private const val FLOATING_CONTROL_CLEARANCE = 48f
    }
}
