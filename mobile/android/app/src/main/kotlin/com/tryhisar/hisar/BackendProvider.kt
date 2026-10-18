package com.tryhisar.hisar

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.VaultBackend
import kotlinx.coroutines.flow.SharedFlow

/** Implemented by the Application: HisarKit in the app, a fake backend in UI tests. */
interface BackendProvider {
    val backend: VaultBackend
    val events: SharedFlow<BackendEvent>

    /** Copies a secret the UI shows, such as a new Secret Key, as Go copies fields: marked sensitive and cleared after about 90 seconds. */
    fun copySecret(text: String): Boolean
}
