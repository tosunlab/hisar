package com.tryhisar.hisar.platform

import android.app.Application
import android.content.Context
import androidx.fragment.app.testing.EmptyFragmentActivity
import androidx.test.core.app.ActivityScenario
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.util.UUID
import org.junit.Assert.fail

val targetContext: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

val application: Application get() = targetContext.applicationContext as Application

/** A value scripts/test-android-ui.sh passes to the tests, such as HISAR_UI_TEST_SERVER. */
fun requireArgument(name: String): String =
    InstrumentationRegistry.getArguments().getString(name) ?: throw AssertionError("$name is not set; run the tests with scripts/test-android-ui.sh")

fun createTemporaryDirectory(name: String): File = File(targetContext.cacheDir, "$name-${UUID.randomUUID()}").apply { mkdirs() }

fun waitUntil(timeoutMillis: Long, description: String, condition: () -> Boolean) {
    val deadline = System.currentTimeMillis() + timeoutMillis
    while (!condition()) {
        if (System.currentTimeMillis() > deadline) fail("timed out after $timeoutMillis ms waiting until $description")
        Thread.sleep(200)
    }
}

/** One of Hisar's activities in front with window focus, which Android requires to read the clipboard. */
fun launchFocusedActivity(): ActivityScenario<EmptyFragmentActivity> {
    val scenario = ActivityScenario.launch(EmptyFragmentActivity::class.java)
    waitUntil(10_000, "the activity has window focus") {
        var isFocused = false
        scenario.onActivity { isFocused = it.hasWindowFocus() }
        isFocused
    }
    return scenario
}
