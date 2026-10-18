package com.tryhisar.hisar

import android.app.Application
import android.util.Log
import com.tryhisar.hisar.backend.HisarKitBackend
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.platform.BiometricKeystore
import com.tryhisar.hisar.platform.ClipboardAdapter
import com.tryhisar.hisar.platform.EventRelay
import com.tryhisar.hisar.platform.ForegroundActivity
import com.tryhisar.hisar.platform.KeystoreSecretKeyStore
import com.tryhisar.hisar.platform.StateDirectory
import com.tryhisar.hisar.platform.TrustedCertificates
import kotlin.concurrent.thread
import kotlinx.coroutines.flow.SharedFlow

/**
 * The one Go backend of the process, shared by the app, AutoFill and the credential provider. It starts building on a
 * background thread when the process starts; [backend] waits for it, and throws its BackendError when it could not start.
 */
open class HisarApplication : Application(), BackendProvider {
    private val eventRelay = EventRelay()
    lateinit var clipboard: ClipboardAdapter
        private set
    lateinit var foregroundActivity: ForegroundActivity
        private set
    private val hisarKit = lazy { createBackend() }

    override val backend: VaultBackend get() = hisarKit.value
    override val events: SharedFlow<BackendEvent> get() = eventRelay.events

    override fun copySecret(text: String): Boolean = clipboard.setText(text)

    override fun onCreate() {
        super.onCreate()
        clipboard = ClipboardAdapter(this)
        foregroundActivity = ForegroundActivity(this, onWindowFocus = clipboard::clearExpiredCopy)
        startBackend()
    }

    protected open fun startBackend() {
        thread(name = "HisarKit start") {
            try {
                backend
            } catch (error: Exception) {
                Log.e("Hisar", "HisarKit did not start", error)
            }
        }
    }

    private fun createBackend() = HisarKitBackend(
        stateDirectory = StateDirectory.prepare(this),
        secretKeyStore = KeystoreSecretKeyStore(noBackupFilesDir),
        biometric = BiometricKeystore(this, noBackupFilesDir, foregroundActivity),
        clipboard = clipboard,
        eventListener = eventRelay,
        appVersion = BuildConfig.VERSION_NAME,
        trustedCertificates = TrustedCertificates.readPem(),
    )
}
