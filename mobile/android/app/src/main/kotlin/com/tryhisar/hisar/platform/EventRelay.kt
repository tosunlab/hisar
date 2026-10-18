package com.tryhisar.hisar.platform

import android.os.Handler
import android.os.Looper
import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisarkit.EventListener
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow

/** Turns HisarKit's event callbacks, which come on Go's threads, into [events] on the main thread, in order. */
class EventRelay : EventListener {
    private val mainHandler = Handler(Looper.getMainLooper())
    private val mutableEvents = MutableSharedFlow<BackendEvent>(extraBufferCapacity = 64)

    val events: SharedFlow<BackendEvent> = mutableEvents.asSharedFlow()

    override fun onLocked() = post(BackendEvent.Locked)

    override fun onApprovalEvent(eventName: String, requestID: String) = post(BackendEvent.Approval(eventName, requestID))

    override fun onRemoteChange() = post(BackendEvent.RemoteChange)

    override fun onUpdateRequired() = post(BackendEvent.UpdateRequired)

    private fun post(event: BackendEvent) {
        mainHandler.post { mutableEvents.tryEmit(event) }
    }
}
