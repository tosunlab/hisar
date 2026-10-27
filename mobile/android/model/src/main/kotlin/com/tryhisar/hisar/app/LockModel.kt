package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ShareAwaitingUnlock
import com.tryhisar.hisar.model.VaultStatus
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Unlocking, signing out from the lock screen, and the lock's settings: the auto-lock delay and biometric unlock. */
class LockModel(private val context: ModelContext) {
    private val mutableAllShareAwaitingUnlock = MutableStateFlow<List<ShareAwaitingUnlock>>(emptyList())

    /** Shares people accepted that only this app, unlocked, can finish; the lock screen lists them. */
    val allShareAwaitingUnlock: StateFlow<List<ShareAwaitingUnlock>> = mutableAllShareAwaitingUnlock.asStateFlow()

    init {
        context.addStatusListener { _, current ->
            if (current.state != VaultStatus.STATE_LOCKED) mutableAllShareAwaitingUnlock.value = emptyList()
        }
    }

    suspend fun unlock(password: String): Boolean = context.runInModel {
        context.errorMessage.value = null
        val newStatus = context.perform { it.unlock(password) } ?: return@runInModel false
        context.applyStatus(newStatus)
        context.refreshAll()
        true
    }

    suspend fun unlockWithBiometric(): Boolean = context.runInModel {
        val newStatus = if (context.status.value.biometricReady) context.perform { it.unlockWithBiometric() } else null
        if (newStatus == null) {
            context.refreshStatus()
            return@runInModel false
        }
        context.applyStatus(newStatus)
        context.refreshAll()
        true
    }

    // Signing out finishes on this phone even when telling the server fails; the error keeps its message, and the status
    // read again shows where it ended.
    suspend fun signOut(): Boolean = context.runInModel {
        val newStatus = context.perform { it.signOut() }
        if (newStatus == null) {
            context.refreshStatus()
            return@runInModel false
        }
        context.applyStatus(newStatus)
        true
    }

    /** Turning biometric unlock on needs the master password; turning it off does not. */
    suspend fun setBiometricUnlock(isEnabled: Boolean, password: String): Boolean {
        val newStatus = context.perform { if (isEnabled) it.enableBiometricUnlock(password) else it.disableBiometricUnlock() } ?: return false
        context.applyStatus(newStatus)
        return true
    }

    suspend fun setAutoLock(seconds: Int): Boolean {
        val newStatus = context.perform(isBusyShown = false) { it.setAutoLock(seconds) } ?: return false
        context.applyStatus(newStatus)
        return true
    }

    // Failures stay silent: the lock screen works offline.
    suspend fun refreshShareAwaitingUnlock() {
        if (context.screen.value != Screen.Locked) return
        val allAwaiting = context.attempt { it.listAllShareAwaitingUnlock() } ?: return
        if (context.screen.value == Screen.Locked) mutableAllShareAwaitingUnlock.value = allAwaiting
    }
}
