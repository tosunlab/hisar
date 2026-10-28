package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.describeError
import java.time.Instant
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Phone approvals: the open requests of the user's Mac, listed while Hisar is unlocked, in front and allowed to answer
 * them. The event stream announces new and finished requests; the requests are also polled, and expired ones leave the
 * list every second.
 */
class ApprovalsModel(private val context: ModelContext, private val now: () -> Instant) {
    enum class ErrorKind { GONE, NEEDS_PASSWORD, CANCELLED, OTHER }

    private val mutableAllPendingApproval = MutableStateFlow<List<PhoneApproval>>(emptyList())
    private val mutablePresentedApproval = MutableStateFlow<PhoneApproval?>(null)
    private val mutableIsAnsweringApproval = MutableStateFlow(false)
    private val mutableIsApprovalPasswordRequired = MutableStateFlow(false)
    private val mutableApprovalErrorMessage = MutableStateFlow<String?>(null)
    private val mutableRememberChoices = MutableStateFlow<Map<String, Boolean>>(emptyMap())
    private val allFinishedApprovalID = mutableSetOf<String>()
    private var startedApprovalListCount = 0L
    private var shownApprovalList = 0L

    /**
     * A request the Mac announced while Hisar was in front, waiting for a listing that shows it to open its sheet.
     * [listedAfter] is the last listing started before the announcement: a later listing without the request means it is gone.
     */
    private data class AnnouncedApproval(val requestID: String, val listedAfter: Long)
    private val allAnnouncedApproval = mutableListOf<AnnouncedApproval>()
    private var pollJob: Job? = null
    private var expiryJob: Job? = null

    val allPendingApproval: StateFlow<List<PhoneApproval>> = mutableAllPendingApproval.asStateFlow()

    /** The request whose sheet is open. */
    val presentedApproval: StateFlow<PhoneApproval?> = mutablePresentedApproval.asStateFlow()
    val isAnsweringApproval: StateFlow<Boolean> = mutableIsAnsweringApproval.asStateFlow()
    val isApprovalPasswordRequired: StateFlow<Boolean> = mutableIsApprovalPasswordRequired.asStateFlow()
    val approvalErrorMessage: StateFlow<String?> = mutableApprovalErrorMessage.asStateFlow()
    val rememberChoices: StateFlow<Map<String, Boolean>> = mutableRememberChoices.asStateFlow()

    private val canList: Boolean
        get() = context.status.value.let { it.isUnlocked && it.updateRequired == null }

    init {
        context.addStatusListener(::applyStatus)
        context.scope.launch(start = CoroutineStart.UNDISPATCHED) {
            context.isInForeground.collect { isInForeground ->
                if (!isInForeground) stopPolling() else if (canList) startPolling()
            }
        }
        context.scope.launch(start = CoroutineStart.UNDISPATCHED) {
            context.isMacApprovalAllowed.collect { isAllowed -> if (isAllowed) refreshPendingApproval() else finishAll() }
        }
        context.scope.launch(start = CoroutineStart.UNDISPATCHED) {
            context.backendEvents.filterIsInstance<BackendEvent.Approval>().collect { handleApprovalEvent(it.eventName, it.requestID) }
        }
    }

    private fun applyStatus(previous: VaultStatus, current: VaultStatus) {
        if (current.updateRequired != null || (previous.isUnlocked && !current.isUnlocked)) clearAll()
        if (!previous.isUnlocked && current.isUnlocked && current.updateRequired == null && context.isInForeground.value) startPolling()
    }

    fun handleApprovalEvent(eventName: String, requestID: String) {
        when (eventName) {
            // Recorded now, not in the listing's coroutine: a sheet the user closes before it runs must still cancel it.
            PhoneApproval.EVENT_REQUESTED -> if (context.isInForeground.value) {
                recordAnnouncedApproval(requestID)
                context.scope.launch { refreshPendingApproval() }
            }
            PhoneApproval.EVENT_ANSWERED, PhoneApproval.EVENT_RESOLVED -> removeApproval(requestID)
        }
    }

    /**
     * Lists the open requests. [announcedRequestID] is a request the Mac announced: the first listing that shows it, this one
     * or a newer one that overtakes it, opens its sheet when none is open.
     */
    suspend fun refreshPendingApproval(announcedRequestID: String? = null) {
        if (announcedRequestID != null) recordAnnouncedApproval(announcedRequestID)
        if (!context.status.value.isUnlocked || !context.isInForeground.value || !context.isMacApprovalAllowed.value) return
        val approvalList = ++startedApprovalListCount
        val allApproval = context.attempt { it.listAllPendingApproval() } ?: return
        // Listings run at once and can finish out of order: a poll that started before a request's own listing must not
        // take that request back off the list and close its sheet.
        if (!context.status.value.isUnlocked || approvalList <= shownApprovalList) return
        shownApprovalList = approvalList
        val date = now()
        removeAllExpiredApproval()
        mutableAllPendingApproval.value = allApproval.filter { it.requestID !in allFinishedApprovalID && !it.isExpired(date) }
        mutableRememberChoices.update { choices -> choices.filterKeys { id -> mutableAllPendingApproval.value.any { it.requestID == id } } }
        openAnnouncedApproval(approvalList)
        closeSheetOfFinishedApproval()
        startExpiryCheck()
    }

    private fun recordAnnouncedApproval(requestID: String) {
        if (!context.status.value.isUnlocked || !context.isInForeground.value || !context.isMacApprovalAllowed.value) return
        allAnnouncedApproval += AnnouncedApproval(requestID, startedApprovalListCount)
    }

    // Opens the first announced request this listing shows, when no sheet is open. A request this listing reflects leaves
    // the queue: it opened, it is listed while another sheet is open (as one that arrived during that sheet always was),
    // or the listing started after its announcement and lacks it, so it was answered, withdrawn or expired.
    private fun openAnnouncedApproval(approvalList: Long) {
        val allListed = mutableAllPendingApproval.value
        for (announced in allAnnouncedApproval.toList()) {
            val approval = allListed.firstOrNull { it.requestID == announced.requestID }
            if (approvalList <= announced.listedAfter && approval == null) continue
            allAnnouncedApproval.removeAll { it.requestID == announced.requestID }
            if (mutablePresentedApproval.value == null && approval != null) present(approval)
        }
    }

    fun reviewApproval(approval: PhoneApproval) {
        if (mutableAllPendingApproval.value.none { it.requestID == approval.requestID }) return
        present(approval)
    }

    /** The user closed the sheet: requests announced before that do not open one now; a request announced later still does. */
    fun closeApprovalSheet() {
        if (mutableIsAnsweringApproval.value) return
        mutablePresentedApproval.value = null
        allAnnouncedApproval.clear()
    }

    fun removeAllExpiredApproval() {
        val date = now()
        mutableAllPendingApproval.value.filter { it.isExpired(date) }.forEach { approval ->
            if (mutablePresentedApproval.value?.requestID == approval.requestID) {
                context.showNotice("This request expired. Start it again on your Mac if you still need it.")
            }
            removeApproval(approval.requestID)
        }
    }

    fun rememberChoice(approval: PhoneApproval): Boolean = approval.resolveRememberChoice(mutableRememberChoices.value[approval.requestID])

    fun setRememberChoice(approval: PhoneApproval, isRemembered: Boolean) {
        mutableRememberChoices.update { it + (approval.requestID to (approval.isRememberable && isRemembered)) }
    }

    /** Approves the open sheet's request; [isRemembered] is its switch, which counts only for a request that offers it. */
    suspend fun approvePendingApproval(isRemembered: Boolean) {
        val approval = mutablePresentedApproval.value ?: return
        approve(approval, isRemembered)
    }

    suspend fun approvePendingApproval(password: String, isRemembered: Boolean? = null) {
        val approval = mutablePresentedApproval.value ?: return
        if (isRemembered != null) setRememberChoice(approval, isRemembered)
        val isApprovalRemembered = rememberChoice(approval)
        answer(approval, "Approved on your Mac") { it.approveApprovalWithPassword(approval.requestID, password, isApprovalRemembered) }
    }

    suspend fun denyPendingApproval() {
        val approval = mutablePresentedApproval.value ?: return
        deny(approval)
    }

    suspend fun approve(approval: PhoneApproval, isRemembered: Boolean? = null) {
        if (isRemembered != null) setRememberChoice(approval, isRemembered)
        val isApprovalRemembered = rememberChoice(approval)
        if (!context.status.value.biometricAvailable) {
            present(approval)
            mutableIsApprovalPasswordRequired.value = true
            return
        }
        answer(approval, "Approved on your Mac") { it.approveApproval(approval.requestID, isApprovalRemembered) }
    }

    suspend fun deny(approval: PhoneApproval) {
        answer(approval, "Denied on your Mac") { it.denyApproval(approval.requestID) }
    }

    private suspend fun answer(approval: PhoneApproval, successMessage: String, operation: (VaultBackend) -> Unit) = context.runInModel {
        if (mutableIsAnsweringApproval.value) return@runInModel
        mutableIsAnsweringApproval.value = true
        if (mutablePresentedApproval.value?.requestID == approval.requestID) mutableApprovalErrorMessage.value = null
        val result = try {
            context.attempt { backend ->
                try {
                    operation(backend)
                    Result.success(Unit)
                } catch (error: Exception) {
                    Result.failure(error)
                }
            }
        } finally {
            mutableIsAnsweringApproval.value = false
        } ?: return@runInModel
        result.onSuccess {
            removeApproval(approval.requestID)
            context.showNotice(successMessage)
        }.onFailure { error ->
            val message = describeError(error)
            when (classifyApprovalError(message)) {
                ErrorKind.GONE -> {
                    removeApproval(approval.requestID)
                    context.showNotice("The request is no longer waiting")
                }
                ErrorKind.NEEDS_PASSWORD -> {
                    present(approval)
                    mutableIsApprovalPasswordRequired.value = true
                }
                ErrorKind.CANCELLED -> Unit
                ErrorKind.OTHER -> {
                    present(approval)
                    mutableApprovalErrorMessage.value = message
                }
            }
        }
    }

    private fun present(approval: PhoneApproval) {
        if (mutablePresentedApproval.value?.requestID != approval.requestID) {
            mutableIsApprovalPasswordRequired.value = !context.status.value.biometricAvailable
            mutableApprovalErrorMessage.value = null
        }
        mutablePresentedApproval.value = approval
    }

    private fun closeSheetOfFinishedApproval() {
        val presented = mutablePresentedApproval.value ?: return
        if (mutableAllPendingApproval.value.any { it.requestID == presented.requestID }) return
        mutablePresentedApproval.value = null
        mutableIsAnsweringApproval.value = false
        mutableIsApprovalPasswordRequired.value = false
        mutableApprovalErrorMessage.value = null
    }

    private fun removeApproval(requestID: String) {
        allFinishedApprovalID += requestID
        mutableRememberChoices.update { it - requestID }
        mutableAllPendingApproval.update { all -> all.filter { it.requestID != requestID } }
        allAnnouncedApproval.removeAll { it.requestID == requestID }
        closeSheetOfFinishedApproval()
    }

    // Turned off, the requests on show count as finished, so turning it on again does not bring them back.
    private fun finishAll() {
        allFinishedApprovalID += mutableAllPendingApproval.value.map { it.requestID }
        mutableAllPendingApproval.value = emptyList()
        mutableRememberChoices.value = emptyMap()
        allAnnouncedApproval.clear()
        shownApprovalList = startedApprovalListCount
        mutablePresentedApproval.value = null
    }

    private fun startExpiryCheck() {
        if (mutableAllPendingApproval.value.isEmpty() || expiryJob != null) return
        expiryJob = context.scope.launch {
            while (true) {
                delay(EXPIRY_CHECK_INTERVAL_MILLISECONDS)
                removeAllExpiredApproval()
                if (mutableAllPendingApproval.value.isEmpty()) break
            }
            expiryJob = null
        }
    }

    // The event stream can miss requests on a phone (a frozen app, a dead connection), so the open requests are also polled.
    private fun startPolling() {
        if (pollJob != null) return
        pollJob = context.scope.launch {
            while (true) {
                refreshPendingApproval()
                delay(POLL_INTERVAL_MILLISECONDS)
            }
        }
    }

    private fun stopPolling() {
        pollJob?.cancel()
        pollJob = null
    }

    private fun clearAll() {
        stopPolling()
        expiryJob?.cancel()
        expiryJob = null
        mutableAllPendingApproval.value = emptyList()
        mutableRememberChoices.value = emptyMap()
        allAnnouncedApproval.clear()
        // A listing still running started before the clear and must not bring the requests back.
        shownApprovalList = startedApprovalListCount
        mutablePresentedApproval.value = null
        mutableIsAnsweringApproval.value = false
        mutableIsApprovalPasswordRequired.value = false
        mutableApprovalErrorMessage.value = null
    }

    companion object {
        const val POLL_INTERVAL_MILLISECONDS = 15_000L
        private const val EXPIRY_CHECK_INTERVAL_MILLISECONDS = 1_000L

        fun classifyApprovalError(message: String): ErrorKind {
            val lowercased = message.lowercase()
            return when {
                "no longer waiting" in lowercased -> ErrorKind.GONE
                "was cancelled" in lowercased -> ErrorKind.CANCELLED
                "master password" in lowercased && "incorrect" !in lowercased -> ErrorKind.NEEDS_PASSWORD
                else -> ErrorKind.OTHER
            }
        }
    }
}
