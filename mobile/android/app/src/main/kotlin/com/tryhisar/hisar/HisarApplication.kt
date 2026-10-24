package com.tryhisar.hisar

import android.app.Activity
import android.app.Application
import android.util.Log
import com.tryhisar.hisar.app.CredentialExchangeReceiver
import com.tryhisar.hisar.app.SettingsModel
import com.tryhisar.hisar.autofill.HttpAssetLinksVerifier
import com.tryhisar.hisar.backend.HisarKitBackend
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.passkey.RelyingPartyVerifier
import com.tryhisar.hisar.platform.BiometricKeystore
import com.tryhisar.hisar.platform.ClipboardAdapter
import com.tryhisar.hisar.platform.CredentialTransfer
import com.tryhisar.hisar.platform.CredentialTransfer.Companion.CREDENTIAL_TRANSFER_FOLDER_NAME
import com.tryhisar.hisar.platform.EventRelay
import com.tryhisar.hisar.platform.ForegroundActivity
import com.tryhisar.hisar.platform.KeystoreSecretKeyStore
import com.tryhisar.hisar.platform.StateDirectory
import com.tryhisar.hisar.platform.TrustedCertificates
import com.tryhisar.hisar.ui.settings.SHARED_FOLDER_NAME
import java.io.File
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

    override fun copyPlain(text: String): Boolean = clipboard.setPlainText(text)

    /** Asks a relying party whether an app may use its passkeys; instrumented tests answer for their own test domain. */
    open val relyingPartyVerifier: RelyingPartyVerifier get() = HttpAssetLinksVerifier

    /** Receives another app's credentials through Android's credential transfer; instrumented tests answer for it. */
    open fun createCredentialExchangeReceiver(activity: Activity): CredentialExchangeReceiver = CredentialTransfer(activity)

    override fun onCreate() {
        super.onCreate()
        deleteLeftoverHandoffFiles(cacheDir)
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

/**
 * Emergency Kits, exports and their share copies live only while their screen is open, and no screen outlives the
 * process, so whatever a killed process left behind is deleted before anything else runs. So is the file through which
 * another app handed over its credentials, which the credential transfer library deletes once it has read it.
 */
internal fun deleteLeftoverHandoffFiles(cacheDir: File) {
    File(cacheDir, SettingsModel.HANDOFF_FOLDER_NAME).deleteRecursively()
    File(cacheDir, SHARED_FOLDER_NAME).deleteRecursively()
    File(cacheDir, CREDENTIAL_TRANSFER_FOLDER_NAME).deleteRecursively()
}
