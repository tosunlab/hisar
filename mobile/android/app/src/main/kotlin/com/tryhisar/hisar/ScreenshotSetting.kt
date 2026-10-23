package com.tryhisar.hisar

import android.content.Context
import android.content.SharedPreferences
import android.content.pm.ApplicationInfo
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.core.content.edit
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch

/**
 * Settings → "Allow screenshots" (D6): while off, Hisar's windows are secure, so screenshots, screen recordings and the
 * app switcher show nothing. It starts off in release builds and on in debug builds, which UI tests screenshot.
 */
class ScreenshotSetting(context: Context) {
    private val preferences: SharedPreferences = context.getSharedPreferences(PREFERENCES_NAME, Context.MODE_PRIVATE)
    private val defaultValue = context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0

    var isAllowed: Boolean
        get() = preferences.getBoolean(KEY, defaultValue)
        set(value) = preferences.edit { putBoolean(KEY, value) }

    /** The setting now and after each change. */
    val allowedChanges: Flow<Boolean> = callbackFlow {
        val listener = SharedPreferences.OnSharedPreferenceChangeListener { _, key -> if (key == KEY) trySend(isAllowed) }
        preferences.registerOnSharedPreferenceChangeListener(listener)
        send(isAllowed)
        awaitClose { preferences.unregisterOnSharedPreferenceChangeListener(listener) }
    }

    private companion object {
        const val PREFERENCES_NAME = "app"
        const val KEY = "isScreenshotAllowed"
    }
}

/** Keeps the activity's window secure while screenshots are not allowed, now and after every change of the setting. */
fun ComponentActivity.followScreenshotSetting() {
    val setting = ScreenshotSetting(this)
    applyScreenshotSetting(setting.isAllowed)
    lifecycleScope.launch {
        repeatOnLifecycle(Lifecycle.State.CREATED) { setting.allowedChanges.collect(::applyScreenshotSetting) }
    }
}

private fun ComponentActivity.applyScreenshotSetting(isAllowed: Boolean) {
    if (isAllowed) window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE) else window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
}
