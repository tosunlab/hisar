package com.tryhisar.hisar.platform

import android.app.Activity
import android.app.Application
import android.os.Bundle
import android.view.ViewTreeObserver
import androidx.fragment.app.FragmentActivity
import java.lang.ref.WeakReference

/**
 * Tracks Hisar's resumed activity, which hosts the biometric prompt, and reports each time one of Hisar's windows gains
 * focus: only then may Hisar read the clipboard.
 */
class ForegroundActivity(application: Application, private val onWindowFocus: () -> Unit) {
    @Volatile
    private var resumed = WeakReference<FragmentActivity>(null)

    /** The resumed activity that can show a biometric prompt; read it on the main thread. */
    val current: FragmentActivity? get() = resumed.get()

    init {
        application.registerActivityLifecycleCallbacks(object : Application.ActivityLifecycleCallbacks {
            override fun onActivityPostCreated(activity: Activity, savedInstanceState: Bundle?) {
                val listener = ViewTreeObserver.OnWindowFocusChangeListener { hasFocus -> if (hasFocus) onWindowFocus() }
                activity.window.decorView.viewTreeObserver.addOnWindowFocusChangeListener(listener)
            }

            override fun onActivityResumed(activity: Activity) {
                if (activity is FragmentActivity) resumed = WeakReference(activity)
            }

            override fun onActivityPaused(activity: Activity) {
                if (resumed.get() === activity) resumed = WeakReference(null)
            }

            override fun onActivityCreated(activity: Activity, savedInstanceState: Bundle?) = Unit

            override fun onActivityStarted(activity: Activity) = Unit

            override fun onActivityStopped(activity: Activity) = Unit

            override fun onActivitySaveInstanceState(activity: Activity, outState: Bundle) = Unit

            override fun onActivityDestroyed(activity: Activity) = Unit
        })
    }
}
