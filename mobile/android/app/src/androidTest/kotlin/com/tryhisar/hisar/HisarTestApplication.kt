package com.tryhisar.hisar

import android.app.Application
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultBackend
import kotlinx.coroutines.flow.MutableSharedFlow

/**
 * The Application of instrumented tests. A test that drives the screens without a server installs a [FakeBackend]
 * before it launches an activity; the app then reads that backend through [BackendProvider] as it reads HisarKit.
 */
class HisarTestApplication : Application(), BackendProvider {
    private var fakeBackend: FakeBackend? = null
    override val events = MutableSharedFlow<BackendEvent>()
    val allCopiedSecret = mutableListOf<String>()

    override val backend: VaultBackend
        get() = fakeBackend ?: throw BackendError("This test installed no backend.")

    override fun copySecret(text: String): Boolean {
        allCopiedSecret += text
        return true
    }

    fun installFakeBackend(backend: FakeBackend) {
        fakeBackend = backend
        allCopiedSecret.clear()
    }
}
