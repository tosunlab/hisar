package com.tryhisar.hisar

import com.tryhisar.hisar.autofill.AssetLinks
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.passkey.RelyingPartyVerifier
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

    override val backend: VaultBackend
        get() = fakeBackend ?: super.backend

    override val events: SharedFlow<BackendEvent>
        get() = if (fakeBackend != null) fakeEvents else super.events

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
        allCopiedSecret.clear()
        allCopiedPlain.clear()
    }
}
