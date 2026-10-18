package com.tryhisar.hisar

import android.graphics.Bitmap
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasScrollToNodeAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onLast
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.services.storage.TestStorage
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

    fun waitFor(matcher: SemanticsMatcher, timeoutMillis: Long = 30_000) {
        composeRule.waitUntil("${matcher.description} appears", timeoutMillis) { exists(matcher) }
    }

    fun waitFor(tag: String, timeoutMillis: Long = 30_000) = waitFor(hasTestTag(tag), timeoutMillis)

    fun exists(matcher: SemanticsMatcher): Boolean = composeRule.onAllNodes(matcher).fetchSemanticsNodes().isNotEmpty()

    fun tap(tag: String) {
        scrollTo(tag)
        composeRule.onNode(hasTestTag(tag)).performClick()
    }

    fun readText(tag: String): String {
        waitFor(tag)
        return composeRule.onNode(hasTestTag(tag)).fetchSemanticsNode().config[SemanticsProperties.Text].joinToString("")
    }

    fun goBack() {
        Espresso.pressBack()
    }

    fun openTab(name: String) {
        val tab = hasText(name) and SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.Tab)
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
        waitFor(item)
        composeRule.onNode(hasTestTag(item)).performClick()
        composeRule.waitUntil("the $menu menu closes after choosing $item", 10_000) { !exists(hasTestTag(item)) }
    }

    /** A lazy list composes only the rows on screen, so a row further down does not exist until the list scrolls to it. */
    fun scrollTo(tag: String) {
        val node = hasTestTag(tag)
        if (!exists(node) && exists(hasScrollToNodeAction())) {
            composeRule.onAllNodes(hasScrollToNodeAction()).onLast().performScrollToNode(node)
        }
        waitFor(node)
        if (exists(node and hasAnyAncestor(hasScrollAction()))) {
            composeRule.onNode(node).performScrollTo()
            scrollClearOfTabBar(node)
        }
    }

    /**
     * performScrollTo stops with the node at the list's bottom edge, which lies under the floating tab bar, where a tap
     * lands on the bar. As XCUITest makes an element hittable, the list scrolls on until the node clears the bar.
     */
    private fun scrollClearOfTabBar(node: SemanticsMatcher) {
        val tabBar = hasTestTag("tab-vaults")
        if (!exists(tabBar)) return
        val barTop = composeRule.onNode(tabBar).fetchSemanticsNode().boundsInRoot.top
        val nodeBottom = composeRule.onNode(node).fetchSemanticsNode().boundsInRoot.bottom
        val overlap = nodeBottom - barTop
        if (overlap > 0) {
            composeRule.onAllNodes(hasScrollAction() and hasAnyDescendant(node)).onLast()
                .performSemanticsAction(SemanticsActions.ScrollBy) { scrollBy -> scrollBy(0f, overlap + TAB_BAR_CLEARANCE) }
        }
    }

    fun type(text: String, tag: String) {
        scrollTo(tag)
        composeRule.onNode(hasTestTag(tag)).performTextInput(text)
    }

    /** With [isBiometricOfferExpected], it stops at the biometric offer after the Secret Key; otherwise it declines any offer. */
    fun signUp(inviteLink: String, email: String, name: String, isBiometricOfferExpected: Boolean = false): String {
        waitFor("sign-in-mode")
        saveScreenshot("sign-in")
        composeRule.onNode(hasText("Create account") and hasAnyAncestor(hasTestTag("sign-in-mode"))).performClick()
        type(inviteLink, "sign-up-invite-link")
        type(email, "sign-up-email")
        type(name, "sign-up-name")
        type(MASTER_PASSWORD, "sign-up-password")
        type(MASTER_PASSWORD, "sign-up-repeated-password")
        scrollTo("create-account-note")
        saveScreenshot("create-account")
        tap("sign-in-submit")
        waitFor("new-secret-key", 60_000)
        val secretKey = readText("new-secret-key")
        saveScreenshot("new-secret-key")
        tap("new-secret-key-done")
        if (isBiometricOfferExpected) {
            waitFor("biometric-offer-not-now", 10_000)
            return secretKey
        }
        composeRule.waitUntil("the biometric offer or the vaults appear", 30_000) {
            exists(hasTestTag("biometric-offer-not-now")) || exists(hasTestTag("vault-Personal"))
        }
        if (exists(hasTestTag("biometric-offer-not-now"))) {
            tap("biometric-offer-not-now")
        }
        waitFor("vault-Personal")
        return secretKey
    }

    fun addLogin(title: String, username: String, password: String, website: String, oneTimePassword: String = "") {
        tap("vault-Personal")
        choose("add-login", "add-item")
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

        /** How far above the tab bar, in pixels, a tapped node ends up. */
        private const val TAB_BAR_CLEARANCE = 48f
    }
}
