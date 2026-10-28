package com.tryhisar.hisar.ui

import android.content.Intent
import android.net.Uri
import android.os.SystemClock
import android.view.View
import android.view.ViewGroup
import androidx.compose.ui.platform.ViewRootForTest
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getAllSemanticsNodes
import androidx.compose.ui.semantics.getOrNull
import androidx.core.view.children
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.junit4.ComposeTestRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.text.AnnotatedString
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.MainActivity
import com.tryhisar.hisar.model.FakeBackend

/** Launches the app on [backend], optionally opening [link] as a link from another app would. */
fun launchApp(backend: FakeBackend, link: String? = null): ActivityScenario<MainActivity> {
    val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
    application.installFakeBackend(backend)
    val intent = if (link == null) {
        Intent(application, MainActivity::class.java)
    } else {
        Intent(Intent.ACTION_VIEW, Uri.parse(link)).setClass(application, MainActivity::class.java)
    }
    return ActivityScenario.launch(intent)
}

fun ComposeTestRule.waitForTag(tag: String, timeoutMillis: Long = 30_000) {
    waitUntil(timeoutMillis) { onAllNodesWithTag(tag, useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty() }
}

fun ComposeTestRule.waitForTagToGo(tag: String, timeoutMillis: Long = 30_000) {
    waitUntil(timeoutMillis) { onAllNodesWithTag(tag, useUnmergedTree = true).fetchSemanticsNodes().isEmpty() }
}

fun ComposeTestRule.isTagShown(tag: String): Boolean = onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty()

/** The text typed into the field with [tag], whether or not the field masks it. */
fun ComposeTestRule.assertFieldText(tag: String, text: String) {
    onNodeWithTag(tag).assert(SemanticsMatcher.expectValue(SemanticsProperties.InputText, AnnotatedString(text)))
}

/** Scrolls the control with [tag] into view and taps it, as a finger would. */
fun ComposeTestRule.tap(tag: String) {
    onNodeWithTag(tag).performScrollTo().performClick()
}

/** Scrolls the field with [tag] into view and types [text] into it. */
fun ComposeTestRule.type(tag: String, text: String) {
    onNodeWithTag(tag).performScrollTo().performTextInput(text)
}

/**
 * Whether the activity shows a node tagged [tag]. Compose's test rule sees only resumed activities, so this reads the
 * semantics of the activity's Compose root itself, in any lifecycle state.
 */
fun ActivityScenario<MainActivity>.showsTag(tag: String): Boolean {
    var isShown = false
    onActivity { activity ->
        fun findComposeRoot(view: View): ViewRootForTest? =
            view as? ViewRootForTest ?: (view as? ViewGroup)?.children?.firstNotNullOfOrNull(::findComposeRoot)
        val root = findComposeRoot(activity.window.decorView) ?: return@onActivity
        isShown = root.semanticsOwner.getAllSemanticsNodes(mergingEnabled = false).any { it.config.getOrNull(SemanticsProperties.TestTag) == tag }
    }
    return isShown
}

/** Waits for [scenario]'s activity, in any lifecycle state, to show a node tagged [tag]; the test clock draws its frames. */
fun ComposeTestRule.waitForTag(scenario: ActivityScenario<MainActivity>, tag: String, timeoutMillis: Long = 30_000) {
    val deadline = SystemClock.uptimeMillis() + timeoutMillis
    while (!scenario.showsTag(tag)) {
        check(SystemClock.uptimeMillis() < deadline) { "$tag was not shown after $timeoutMillis ms" }
        mainClock.advanceTimeByFrame()
        // A poll: the test rule's idle signal covers resumed activities only.
        SystemClock.sleep(50)
    }
}
