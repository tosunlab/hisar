package com.tryhisar.hisar

import android.app.Activity
import com.tryhisar.hisar.app.CredentialExchangeReceiver
import com.tryhisar.hisar.autofill.AssetLinks
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.passkey.RelyingPartyVerifier
import java.io.File
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow

/**
 * The Application of instrumented tests. A test that drives the screens without a server installs a [FakeBackend]
 * before it launches an activity; every other test, the end-to-end UI tests among them, gets the real HisarKit.
 */
class HisarTestApplication : HisarApplication() {
    private var fakeBackend: FakeBackend? = null
    private val fakeEvents = MutableSharedFlow<BackendEvent>()
    val allCopiedSecret = mutableListOf<String>()
    val allCopiedPlain = mutableListOf<String>()

    /** The assetlinks.json each test relying party serves; a relying party without one vouches for no app. */
    val statementByRelyingParty = ConcurrentHashMap<String, String>()

    override val relyingPartyVerifier = RelyingPartyVerifier { relyingPartyID, app ->
        AssetLinks.isVouchingFor(statementByRelyingParty[relyingPartyID].orEmpty(), app)
    }

    /** What the credential transfer answers in a test that drives the import from another app; null asks Android. */
    var credentialExchangeReceiver: CredentialExchangeReceiver? = null

    override fun createCredentialExchangeReceiver(activity: Activity): CredentialExchangeReceiver =
        credentialExchangeReceiver ?: super.createCredentialExchangeReceiver(activity)

    var shareCopy: ((source: File, target: File) -> File)? = null

    override fun copyForShare(source: File, target: File): File = shareCopy?.invoke(source, target) ?: super.copyForShare(source, target)

    override val backend: VaultBackend
        get() = fakeBackend ?: super.backend

    override val events: SharedFlow<BackendEvent>
        get() = if (fakeBackend != null) fakeEvents else super.events

    /** Sends an event as HisarKit would, to the app on the fake backend. */
    suspend fun sendEvent(event: BackendEvent) = fakeEvents.emit(event)

    override fun copySecret(text: String): Boolean {
        if (fakeBackend == null) return super.copySecret(text)
        allCopiedSecret += text
        return true
    }

    override fun copyPlain(text: String): Boolean {
        if (fakeBackend == null) return super.copyPlain(text)
        allCopiedPlain += text
        return true
    }

    override fun startBackend() {}

    fun installFakeBackend(backend: FakeBackend) {
        fakeBackend = backend
        credentialExchangeReceiver = null
        allCopiedSecret.clear()
        allCopiedPlain.clear()
    }
}
