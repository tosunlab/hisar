package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.PendingRecovery
import com.tryhisar.hisar.model.PendingRecoveryExport
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.describeError
import java.io.File
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Durable setup recovery is separate from the account currently open on this phone. */
class PendingRecoveryModel(private val context: ModelContext, private val onSignupDelivered: () -> Unit = {}) {
    private val mutableAllRecovery = MutableStateFlow<List<PendingRecovery>>(emptyList())
    private val mutableIsShown = MutableStateFlow(false)
    private val mutableExport = MutableStateFlow<PendingRecoveryExport?>(null)
    val allRecovery = mutableAllRecovery.asStateFlow()
    val isShown = mutableIsShown.asStateFlow()
    val export = mutableExport.asStateFlow()
    private var epoch = 0L
    private var exportGeneration = 0L

    init {
        context.addStatusListener { previous, current ->
            if (previous.state != current.state || previous.email != current.email || previous.server != current.server) {
                epoch++
                clearExport()
            }
        }
    }

    suspend fun refresh() {
        val listing = context.attempt {
            try {
                Result.success(it.listPendingRecovery())
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                Result.failure(error)
            }
        } ?: return
        val records = listing.getOrElse {
            if (context.errorMessage.value == null) context.errorMessage.value = describeError(it)
            return
        }
        val hadNone = mutableAllRecovery.value.isEmpty()
        mutableAllRecovery.value = records
        if (records.isEmpty()) close() else if (hadNone) mutableIsShown.value = true
    }

    fun open() { mutableIsShown.value = true }

    fun clearExport() {
        exportGeneration++
        mutableExport.value?.path?.takeIf { it.isNotEmpty() }?.let { File(it).parentFile?.deleteRecursively() }
        mutableExport.value = null
    }

    fun close() {
        clearExport()
        mutableIsShown.value = false
    }

    suspend fun resume(operationID: String, password: String, secretKey: String = "", priorSecretKey: String = ""): Boolean = context.runInModel {
        val startedEpoch = epoch
        val result = context.perform {
            if (secretKey.isEmpty()) it.resumePendingRecovery(operationID, password)
            else it.resumePendingRecoveryWithSecretKey(operationID, password, VaultLogic.formatSecretKey(secretKey), VaultLogic.formatSecretKey(priorSecretKey))
        }
        if (result == null || epoch != startedEpoch) {
            val recoveryError = context.errorMessage.value
            refresh()
            if (recoveryError != null) context.errorMessage.value = recoveryError
            return@runInModel false
        }
        context.refreshStatus()
        if (context.status.value.isUnlocked) context.refreshAll()
        refresh()
        if (result.problems.isNotEmpty()) context.errorMessage.value = result.problems.joinToString("\n")
        result.setupState == "delivery-pending"
    }

    suspend fun discard(operationID: String, password: String): Boolean = context.runInModel {
        context.perform { it.discardPreparedRecovery(operationID, password) } ?: return@runInModel false
        clearExport()
        refresh()
        true
    }

    suspend fun exportKit(operationID: String, password: String): Boolean = context.runInModel {
        clearExport()
        val startedEpoch = epoch
        val generation = exportGeneration
        val directory = File(System.getProperty("java.io.tmpdir"), "handoff/PendingRecovery-${UUID.randomUUID()}")
        val result = context.perform { it.exportPendingRecoveryEmergencyKit(operationID, password, directory) }
        if (result == null || startedEpoch != epoch || generation != exportGeneration) {
            directory.deleteRecursively()
            return@runInModel false
        }
        mutableExport.value = result
        refresh()
        true
    }

    suspend fun acknowledge(operationID: String, password: String): Boolean = context.runInModel {
        val startedEpoch = epoch
        val recovery = mutableAllRecovery.value.firstOrNull { it.operationId == operationID }
        context.perform { it.acknowledgePendingRecovery(operationID, password) } ?: return@runInModel false
        if (startedEpoch != epoch) return@runInModel false
        clearExport()
        refresh()
        val current = context.status.value
        if (startedEpoch == epoch && recovery?.kind == "signup" && recovery.phase == "delivery-pending" && !recovery.manualUnlock && current.isUnlocked &&
            current.email == recovery.email && current.server == recovery.origin
        ) onSignupDelivered()
        true
    }
}
