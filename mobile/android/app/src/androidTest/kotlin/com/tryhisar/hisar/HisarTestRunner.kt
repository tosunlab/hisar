package com.tryhisar.hisar

import android.app.Application
import android.content.Context
import androidx.test.runner.AndroidJUnitRunner

/** Runs instrumented tests in [HisarTestApplication], so a test can give the app a fake backend. */
class HisarTestRunner : AndroidJUnitRunner() {
    override fun newApplication(classLoader: ClassLoader, className: String, context: Context): Application =
        super.newApplication(classLoader, HisarTestApplication::class.java.name, context)
}
