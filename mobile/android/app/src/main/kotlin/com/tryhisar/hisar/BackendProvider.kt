package com.tryhisar.hisar

import com.tryhisar.hisar.app.TextClipboard
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.VaultBackend
import kotlinx.coroutines.flow.SharedFlow

/** Implemented by the Application: HisarKit in the app, a fake backend in UI tests. */
interface BackendProvider : TextClipboard {
    val backend: VaultBackend
    val events: SharedFlow<BackendEvent>
}
