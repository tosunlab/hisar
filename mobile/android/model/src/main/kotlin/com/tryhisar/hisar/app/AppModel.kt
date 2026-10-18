package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FavoriteItem
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.Organizations
import com.tryhisar.hisar.model.RemovedVaultNotice
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.StoredPasskey
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.WaitingVaultKeys
import com.tryhisar.hisar.model.describeError
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The state of the app behind every screen, the port of the iPhone app's AppModel. This core holds the backend's status,
 * the data an unlock loads, refreshing, the app's lifecycle, events and notices; one area model per feature holds the
 * rest. Call it from the main thread: every backend call runs on [ioDispatcher], because HisarKit blocks on the network
 * and Argon2 and waits on the main thread for biometric prompts.
 */
class AppModel(
    override val scope: CoroutineScope,
    private val ioDispatcher: CoroutineDispatcher,
    private val clipboard: TextClipboard,
    override val backendEvents: Flow<BackendEvent> = emptyFlow(),
    private val now: () -> Instant = Instant::now,
) : ModelContext {
    private var backend: VaultBackend? = null
    private val mutableStatus = MutableStateFlow(VaultStatus.signedOut)
    private val mutableScreen = MutableStateFlow<Screen>(Screen.Starting)
    private val mutableUnlockedData = MutableStateFlow(UnlockedData())
    private val mutableIsInForeground = MutableStateFlow(true)
    private val mutableIsBusy = MutableStateFlow(false)
    private val mutableIsRefreshing = MutableStateFlow(false)
    private val mutableIsCheckingMinimumVersion = MutableStateFlow(false)
    private val mutableNotice = MutableStateFlow<String?>(null)
    private val mutableLockCount = MutableStateFlow(0)
    private val mutableCopyCount = MutableStateFlow(0)
    private val mutableRemoteChangeCount = MutableStateFlow(0)
    private val allStatusListener = mutableListOf<(VaultStatus, VaultStatus) -> Unit>()
    private var enteredBackgroundAt: Instant? = null
    private var noticeJob: Job? = null
    private var isRefreshRequestedAgain = false

    override val status: StateFlow<VaultStatus> = mutableStatus.asStateFlow()
    override val screen: StateFlow<Screen> = mutableScreen.asStateFlow()
    override val unlockedData: StateFlow<UnlockedData> = mutableUnlockedData.asStateFlow()
    override val isInForeground: StateFlow<Boolean> = mutableIsInForeground.asStateFlow()
    override val isBusy: StateFlow<Boolean> = mutableIsBusy.asStateFlow()
    override val errorMessage = MutableStateFlow<String?>(null)
    override val theme = MutableStateFlow(VaultTheme.standard)
    override val isMacApprovalAllowed = MutableStateFlow(true)
    override val isWebsiteIconsEnabled = MutableStateFlow(true)
    val isRefreshing: StateFlow<Boolean> = mutableIsRefreshing.asStateFlow()
    val isCheckingMinimumVersion: StateFlow<Boolean> = mutableIsCheckingMinimumVersion.asStateFlow()
    val notice: StateFlow<String?> = mutableNotice.asStateFlow()

    /** An editor some other surface asks the app to open, such as AutoFill's save prompt; the Vaults tab shows it. */
    val requestedEditor = MutableStateFlow<EditorRequest?>(null)

    /** Grows with every lock; the lock screen offers biometrics once per value. */
    val lockCount: StateFlow<Int> = mutableLockCount.asStateFlow()

    /** Grows with every copy; the root plays a haptic for each. */
    val copyCount: StateFlow<Int> = mutableCopyCount.asStateFlow()

    /** Grows with every remote change while unlocked; open screens reload what they show on it. */
    val remoteChangeCount: StateFlow<Int> = mutableRemoteChangeCount.asStateFlow()
    val selectedTab = MutableStateFlow(Tab.VAULTS)

    val signIn = SignInModel(this)
    val lock = LockModel(this)
    val itemEditor = ItemEditorModel(this)
    val favorites = FavoritesModel(this)

    val autoLockInterval: Duration get() = Duration.ofSeconds(status.value.autoLockSeconds.toLong())

    /** Builds the backend, then shows the screen its status asks for; a backend that cannot start shows why. */
    suspend fun start(createBackend: () -> VaultBackend) {
        // Events are not replayed, so they are collected before the backend is first asked for its status.
        scope.launch(start = CoroutineStart.UNDISPATCHED) { backendEvents.collect(::handleEvent) }
        val started = try {
            withContext(ioDispatcher) {
                val backend = createBackend()
                StartedBackend(backend, backend.isMacApprovalAllowed(), backend.isWebsiteIconsEnabled(), backend.getTheme(), backend.getStatus())
            }
        } catch (error: Exception) {
            mutableScreen.value = Screen.Failed(describeError(error))
            return
        }
        backend = started.backend
        isMacApprovalAllowed.value = started.isMacApprovalAllowed
        isWebsiteIconsEnabled.value = started.isWebsiteIconsEnabled
        theme.value = started.theme
        applyStatus(started.status)
        if (status.value.isUnlocked) scope.launch { refreshAll() }
        scope.launch { checkMinimumVersion() }
    }

    private class StartedBackend(
        val backend: VaultBackend,
        val isMacApprovalAllowed: Boolean,
        val isWebsiteIconsEnabled: Boolean,
        val theme: VaultTheme,
        val status: VaultStatus,
    )

    private suspend fun handleEvent(event: BackendEvent) {
        when (event) {
            BackendEvent.Locked, BackendEvent.UpdateRequired -> refreshStatus()
            BackendEvent.RemoteChange -> handleRemoteChange()
            is BackendEvent.Approval -> Unit
        }
    }

    // Offline the backend keeps what it knew, so a failed check never blocks the app.
    suspend fun checkMinimumVersion() {
        if (backend == null) return
        mutableIsCheckingMinimumVersion.value = true
        try {
            attempt { it.checkMinimumVersion() }?.let(::applyStatus)
        } finally {
            mutableIsCheckingMinimumVersion.value = false
        }
    }

    override fun addStatusListener(listener: (previous: VaultStatus, current: VaultStatus) -> Unit) {
        allStatusListener += listener
    }

    override fun applyStatus(newStatus: VaultStatus) {
        val previous = mutableStatus.value
        mutableStatus.value = newStatus
        mutableScreen.value = when {
            newStatus.updateRequired != null -> Screen.UpdateRequired(newStatus.updateRequired)
            newStatus.state == VaultStatus.STATE_UNLOCKED -> Screen.Unlocked
            newStatus.state == VaultStatus.STATE_LOCKED -> Screen.Locked
            else -> Screen.SignedOut
        }
        if (previous.isUnlocked && !newStatus.isUnlocked) {
            mutableUnlockedData.value = UnlockedData()
            mutableLockCount.update { it + 1 }
            selectedTab.value = Tab.VAULTS
        }
        allStatusListener.forEach { it(previous, newStatus) }
    }

    override fun updateUnlockedData(transform: (UnlockedData) -> UnlockedData) {
        mutableUnlockedData.update(transform)
    }

    override suspend fun <Value> perform(isBusyShown: Boolean, operation: (VaultBackend) -> Value): Value? {
        val backend = backend ?: return null
        if (isBusyShown) mutableIsBusy.value = true
        try {
            val value = withContext(ioDispatcher) { operation(backend) }
            errorMessage.value = null
            return value
        } catch (error: Exception) {
            errorMessage.value = describeError(error)
            return null
        } finally {
            if (isBusyShown) mutableIsBusy.value = false
        }
    }

    override suspend fun <Value> attempt(operation: (VaultBackend) -> Value): Value? {
        val backend = backend ?: return null
        return try {
            withContext(ioDispatcher) { operation(backend) }
        } catch (_: Exception) {
            null
        }
    }

    override fun showNotice(message: String) {
        mutableNotice.value = message
        noticeJob?.cancel()
        noticeJob = scope.launch {
            delay(NOTICE_DURATION_MILLISECONDS)
            mutableNotice.value = null
        }
    }

    override suspend fun refreshStatus() {
        attempt { it.getStatus() }?.let(::applyStatus)
    }

    suspend fun lock() {
        attempt { it.lock() }?.let(::applyStatus)
    }

    fun reportActivity() {
        if (!status.value.isUnlocked) return
        scope.launch { attempt { it.reportActivity() } }
    }

    fun handleDidEnterBackground() {
        enteredBackgroundAt = now()
        mutableIsInForeground.value = false
    }

    suspend fun handleWillEnterForeground() {
        scope.launch { checkMinimumVersion() }
        mutableIsInForeground.value = true
        val backgroundStart = enteredBackgroundAt
        enteredBackgroundAt = null
        if (backgroundStart != null && status.value.isUnlocked && Duration.between(backgroundStart, now()) >= autoLockInterval) {
            lock()
        } else {
            refreshStatus()
        }
        if (status.value.isUnlocked && status.value.updateRequired == null) {
            attempt { it.reconnectEvents() }
            handleRemoteChange()
        }
    }

    // Listing the share invitations also completes the shares people accepted, and listing the waiting vault keys gives
    // team members the keys of the organization vaults this user owns, as the Mac's sync does. Both run before the vaults
    // are listed, so a vault rotated meanwhile is listed with its new key.
    override suspend fun refreshAll() {
        if (!status.value.isUnlocked) return
        mutableIsRefreshing.value = true
        try {
            val result = perform(isBusyShown = false) { backend ->
                val shareInvitations = attemptCall { backend.listAllShareInvitation() }
                val waitingVaultKeys = attemptCall { backend.listAllWaitingVaultKey() }
                val organizations = attemptCall { backend.listAllOrganization() }
                val allVault = backend.listAllVault()
                val allRemovedVault = attemptCall { backend.listAllRemovedVault() }.orEmpty()
                // Reads the favorites again, so the lists below mark them even when the event stream missed a change.
                attemptCall<List<FavoriteItem>> { backend.listAllFavoriteItem() }
                val allItemByVaultID = allVault.associate { it.id to backend.listAllItem(it.id) }
                RefreshResult(
                    allVault, allItemByVaultID, attemptCall { backend.listAllPasskey() }.orEmpty(),
                    shareInvitations, allRemovedVault, organizations, waitingVaultKeys,
                )
            }
            // Locked or signed out while listing: locking and signing out emptied the data, so the listing must not bring
            // it back.
            refreshStatus()
            if (result == null || !status.value.isUnlocked) return
            mutableUnlockedData.update { data ->
                data.copy(
                    allVault = result.allVault.sortedWith(vaultOrder),
                    allItemByVaultID = result.allItemByVaultID,
                    allPasskey = result.allPasskey,
                    allReceivedInvitation = result.shareInvitations?.allReceived ?: data.allReceivedInvitation,
                    allWaitingInvitation = result.shareInvitations?.allWaiting ?: data.allWaitingInvitation,
                    // Offline both fail; the last organizations keep naming the vault groups.
                    allOrganization = result.organizations?.allOrganization ?: data.allOrganization,
                    allOrganizationInvitation = result.organizations?.allInvitation ?: data.allOrganizationInvitation,
                    waitingVaultKeys = result.waitingVaultKeys ?: data.waitingVaultKeys,
                    allRemovedVaultNotice = data.allRemovedVaultNotice +
                        result.allRemovedVault.filter { notice -> data.allRemovedVaultNotice.none { it.id == notice.id } },
                )
            }
        } finally {
            mutableIsRefreshing.value = false
        }
    }

    private class RefreshResult(
        val allVault: List<VaultSummary>,
        val allItemByVaultID: Map<String, List<ItemDetail>>,
        val allPasskey: List<StoredPasskey>,
        val shareInvitations: ShareInvitations?,
        val allRemovedVault: List<RemovedVaultNotice>,
        val organizations: Organizations?,
        val waitingVaultKeys: WaitingVaultKeys?,
    )

    /** Refreshes everything after a change elsewhere; changes that come during a refresh make one more refresh. */
    fun handleRemoteChange() {
        if (!status.value.isUnlocked) return
        mutableRemoteChangeCount.update { it + 1 }
        if (mutableIsRefreshing.value) {
            isRefreshRequestedAgain = true
            return
        }
        scope.launch {
            do {
                isRefreshRequestedAgain = false
                refreshAll()
            } while (isRefreshRequestedAgain && status.value.isUnlocked)
        }
    }

    suspend fun copyField(vaultID: String, itemID: String, fieldName: String, label: String) {
        perform(isBusyShown = false) { it.copyField(vaultID, itemID, fieldName) } ?: return
        mutableCopyCount.update { it + 1 }
        showNotice("$label copied. $CLEARING_NOTICE")
    }

    suspend fun copySecretReference(vaultID: String, itemID: String, fieldName: String) {
        perform(isBusyShown = false) { it.copySecretReference(vaultID, itemID, fieldName) } ?: return
        mutableCopyCount.update { it + 1 }
        showNotice("Secret reference copied.")
    }

    /** Copies a secret the app holds itself, such as a website or a generated password, as Go copies fields. */
    fun copyText(text: String, label: String) {
        if (!clipboard.copySecret(text)) return
        mutableCopyCount.update { it + 1 }
        showNotice("$label copied. $CLEARING_NOTICE")
    }

    // Not secret, so it stays on the clipboard: support details and diagnostics.
    fun copyPlainText(text: String, label: String) {
        if (!clipboard.copyPlain(text)) return
        mutableCopyCount.update { it + 1 }
        showNotice("$label copied")
    }

    companion object {
        const val NOTICE_DURATION_MILLISECONDS = 2_500L

        /** Android clears a copy with an inexact alarm, which lands between 90 seconds and about 2.5 minutes. */
        private const val CLEARING_NOTICE = "It clears in about 2 minutes."

        /** The Personal vault first, then the others by name. */
        val vaultOrder: Comparator<VaultSummary> = compareByDescending<VaultSummary> { it.type == VaultSummary.TYPE_PERSONAL }
            .thenBy(String.CASE_INSENSITIVE_ORDER) { it.name }
    }
}

private inline fun <Value> attemptCall(call: () -> Value): Value? = try {
    call()
} catch (_: Exception) {
    null
}
