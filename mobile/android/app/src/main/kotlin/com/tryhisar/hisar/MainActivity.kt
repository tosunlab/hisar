package com.tryhisar.hisar

import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.tryhisar.hisar.app.Screen
import com.tryhisar.hisar.autofill.SavedLoginInbox
import com.tryhisar.hisar.ui.root.RootScreen
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * The app's one activity. It is a FragmentActivity because biometric prompts need one, and it reads the backend only
 * through the Application's [BackendProvider].
 */
class MainActivity : FragmentActivity() {
    private val viewModel: AppViewModel by viewModels {
        viewModelFactory { initializer { AppViewModel(application as BackendProvider) } }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        installSplashScreen()
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) setRecentsScreenshotEnabled(false)
        val screenshotSetting = ScreenshotSetting(this)
        applyScreenshotSetting(screenshotSetting.isAllowed)
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.CREATED) { screenshotSetting.allowedChanges.collect(::applyScreenshotSetting) }
        }
        if (savedInstanceState == null) {
            openLink(intent)
            SavedLoginInbox.open(intent, viewModel.model, lifecycleScope)
        }
        setContent { RootScreen(viewModel.model, viewModel::copySecret) }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        openLink(intent)
        SavedLoginInbox.open(intent, viewModel.model, lifecycleScope)
    }

    private fun applyScreenshotSetting(isAllowed: Boolean) {
        if (isAllowed) window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE) else window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
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
}
