package com.tryhisar.hisar

import android.content.Context
import android.content.SharedPreferences
import android.content.pm.ApplicationInfo
import kotlinx.coroutines.channels.awaitClose
import androidx.core.content.edit
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow

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
