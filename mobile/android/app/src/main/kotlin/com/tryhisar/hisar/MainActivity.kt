package com.tryhisar.hisar

import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.tryhisar.hisar.app.Screen
import com.tryhisar.hisar.app.SettingsDestination
import com.tryhisar.hisar.app.Tab
import com.tryhisar.hisar.autofill.SavedLoginInbox
import com.tryhisar.hisar.notifications.HisarNotifications
import com.tryhisar.hisar.ui.root.RootScreen
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * The app's one activity. It is a FragmentActivity because biometric prompts need one, and it reads the backend only
 * through the Application's [BackendProvider].
 */
class MainActivity : FragmentActivity() {
    private val viewModel: AppViewModel by viewModels {
        viewModelFactory { initializer { AppViewModel(application as BackendProvider, HisarNotifications.Platform(application)) } }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        installSplashScreen()
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        followScreenshotSetting()
        if (savedInstanceState == null) {
            openLink(intent)
            openAutoFillSettings(intent)
            openNotification(intent)
        }
        // Once the user allowed notifications, each launch registers with the UnifiedPush distributor again, which then
        // gives HisarPushService its endpoint.
        if (viewModel.model.notifications.isPermissionGranted.value) startPush()
        setContent { RootScreen(viewModel.model, viewModel::copySecret, viewModel::retryStartup, viewModel::copyStartupDetails) }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        openLink(intent)
        openAutoFillSettings(intent)
        openNotification(intent)
    }

    // A tapped notification names its kind; the tabs open its screen once Hisar is unlocked.
    private fun openNotification(intent: Intent) {
        viewModel.model.notifications.open(intent.getStringExtra(HisarNotifications.EXTRA_KIND))
    }

    fun startPush() {
        HisarNotifications.startPush(this) { lifecycleScope.launch { viewModel.model.notifications.refresh() } }
    }

    override fun onResume() {
        super.onResume()
        SavedLoginInbox.open(viewModel.model)
    }

    // A link answers differently signed in and signed out, so it waits until the backend has told which.
    private fun openLink(intent: Intent) {
        val link = intent.takeIf { it.action == Intent.ACTION_VIEW }?.dataString ?: return
        val model = viewModel.model
        lifecycleScope.launch {
            model.screen.first { it != Screen.Starting }
            model.signIn.openLink(link)
        }
    }

    private fun openAutoFillSettings(intent: Intent) {
        if (intent.component?.className != AUTOFILL_SETTINGS_ACTIVITY) return
        viewModel.model.selectedTab.value = Tab.SETTINGS
        viewModel.model.requestedSettings.value = SettingsDestination.AUTOFILL
    }

    private companion object {
        /** The alias of this activity that the system's Autofill service screen opens for Hisar's settings. */
        const val AUTOFILL_SETTINGS_ACTIVITY = "com.tryhisar.hisar.autofill.AutoFillSettingsActivity"
    }
}
