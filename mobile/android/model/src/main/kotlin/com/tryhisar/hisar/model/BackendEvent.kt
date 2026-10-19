package com.tryhisar.hisar.model

/** What HisarKit's event listener reports, delivered to the app model on the main thread. */
sealed interface BackendEvent {
    data object Locked : BackendEvent

    data object RemoteChange : BackendEvent

    data object UpdateRequired : BackendEvent

    data class Approval(val eventName: String, val requestID: String) : BackendEvent
}
