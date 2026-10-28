package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FavoriteItem
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.MasterPasswordFeedback
import com.tryhisar.hisar.model.PasswordStrength
import com.tryhisar.hisar.model.PushKind
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
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
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
    /** Whether sign-in and sign-up take an http:// server: only debug builds do, for a server on the developer's machine. */
    isPlainHttpAllowed: Boolean = false,
    notificationPlatform: NotificationPlatform = NotificationPlatform.None,
) : ModelContext {
    private var backend: VaultBackend? = null
    private val mutableStatus = MutableStateFlow(VaultStatus.signedOut)
    private val mutableScreen = MutableStateFlow<Screen>(Screen.Starting)
    private val mutableUnlockedData = MutableStateFlow(UnlockedData())
    private val mutableIsInForeground = MutableStateFlow(true)
    private val mutableIsBusy = MutableStateFlow(false)
    private val mutableIsRefreshing = MutableStateFlow(false)
    private val mutableIsCheckingMinimumVersion = MutableStateFlow(false)
    private val mutableNotice = MutableStateFlow<Notice?>(null)
    private val mutableLockCount = MutableStateFlow(0)
    private val mutableCopyCount = MutableStateFlow(0)
    private val mutableRemoteChangeCount = MutableStateFlow(0)
    private val allStatusListener = mutableListOf<(VaultStatus, VaultStatus) -> Unit>()
    private var enteredBackgroundAt: Instant? = null
    private var isRefreshRequestedAgain = false
    private var startedRefreshCount = 0L
    private var shownRefresh = 0L
    private var statusEpoch = 0L
    private var invitationSettingsAfterUnlock: SettingsDestination? = null

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
    val notice: StateFlow<Notice?> = mutableNotice.asStateFlow()

    /**
     * A Settings screen something outside the tab asks it to show: AutoFill for the system's Autofill service screen,
     * Devices for a tapped security alert. The Settings tab shows it and clears it.
     */
    val requestedSettings = MutableStateFlow<SettingsDestination?>(null)

    /** Grows with every lock; the lock screen offers biometrics once per value. */
    val lockCount: StateFlow<Int> = mutableLockCount.asStateFlow()

    /** Grows with every copy; the root plays a haptic for each. */
    val copyCount: StateFlow<Int> = mutableCopyCount.asStateFlow()

    /** Grows with every remote change while unlocked; open screens reload what they show on it. */
    val remoteChangeCount: StateFlow<Int> = mutableRemoteChangeCount.asStateFlow()
    val selectedTab = MutableStateFlow(Tab.VAULTS)

    val signIn = SignInModel(this, isPlainHttpAllowed)
    val lock = LockModel(this)
    val settings = SettingsModel(this)
    val pendingRecovery = PendingRecoveryModel(this) { signIn.openEmergencyKit() }
    val credentialImport = CredentialImportModel(this)
    val itemEditor = ItemEditorModel(this)
    val favorites = FavoritesModel(this)
    val websiteIcons = WebsiteIconModel(this)
    val item = ItemModel(this)
    val vaults = VaultsModel(this)
    val invitations = InvitationsModel(this)
    val navigation = VaultsNavigation(this, selectedTab, itemEditor)
    val sharing = SharingModel(this, vaults, invitations)
    val organizations = OrganizationsModel(this, invitations)
    val approvals = ApprovalsModel(this, now)
    val notifications = NotificationsModel(this, notificationPlatform)

    val autoLockInterval: Duration get() = Duration.ofSeconds(status.value.autoLockSeconds.toLong())
    private var isStarting = false
    private var isCollectingBackendEvents = false

    /** Builds the backend, then shows the screen its status asks for; a backend that cannot start shows why. */
    suspend fun start(createBackend: () -> VaultBackend) {
        if (isStarting || backend != null) return
        isStarting = true
        mutableScreen.value = Screen.Starting
        notifications.clearApprovalReminder()
        // Events are not replayed, so they are collected before the backend is first asked for its status.
        if (!isCollectingBackendEvents) {
            isCollectingBackendEvents = true
            scope.launch(start = CoroutineStart.UNDISPATCHED) { backendEvents.collect(::handleEvent) }
        }
        val started = try {
            withContext(ioDispatcher) {
                val backend = createBackend()
                StartedBackend(backend, backend.isMacApprovalAllowed(), backend.isWebsiteIconsEnabled(), backend.getTheme(), backend.getStatus())
            }
        } catch (error: Exception) {
            isStarting = false
            if (error is CancellationException) throw error
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
        scope.launch { notifications.refresh() }
        refreshPendingRecovery()
        isStarting = false
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
        val epoch = statusEpoch
        mutableIsCheckingMinimumVersion.value = true
        try {
            attempt { it.checkMinimumVersion() }?.let { if (epoch == statusEpoch) applyStatus(it) }
        } finally {
            mutableIsCheckingMinimumVersion.value = false
        }
    }

    override fun addStatusListener(listener: (previous: VaultStatus, current: VaultStatus) -> Unit) {
        allStatusListener += listener
    }

    override fun applyStatus(newStatus: VaultStatus) {
        val previous = mutableStatus.value
        if (previous.state != newStatus.state || previous.email != newStatus.email || previous.server != newStatus.server) statusEpoch++
        mutableStatus.value = newStatus
        mutableScreen.value = when {
            newStatus.updateRequired != null -> Screen.UpdateRequired(newStatus.updateRequired)
            newStatus.state == VaultStatus.STATE_UNLOCKED -> Screen.Unlocked
            newStatus.state == VaultStatus.STATE_LOCKED -> Screen.Locked
            else -> Screen.SignedOut
        }
        if (previous.isUnlocked && !newStatus.isUnlocked) {
            mutableNotice.value = null
            mutableUnlockedData.value = UnlockedData()
            mutableLockCount.update { it + 1 }
            selectedTab.value = Tab.VAULTS
        }
        if (!previous.isUnlocked && newStatus.isUnlocked) {
            invitationSettingsAfterUnlock?.let {
                invitationSettingsAfterUnlock = null
                selectedTab.value = Tab.SETTINGS
                requestedSettings.value = it
            }
        }
        if (newStatus.state == VaultStatus.STATE_SIGNED_OUT) invitationSettingsAfterUnlock = null
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
        } catch (error: CancellationException) {
            throw error
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
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            null
        }
    }

    override fun showNotice(message: String, action: Notice.Action?) {
        mutableNotice.value = Notice(message, action)
    }

    fun dismissNotice(notice: Notice) {
        if (mutableNotice.value === notice) mutableNotice.value = null
    }

    suspend fun performNoticeAction(notice: Notice) {
        if (mutableNotice.value !== notice) return
        mutableNotice.value = null
        notice.action?.perform()
    }

    override suspend fun refreshStatus() {
        attempt { it.getStatus() }?.let(::applyStatus)
    }

    override suspend fun refreshPendingRecovery(isOpening: Boolean, operationID: String?) {
        pendingRecovery.refresh(isOpening, operationID)
        if (isOpening && pendingRecovery.allRecovery.value.isNotEmpty()) pendingRecovery.open()
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
        notifications.remindOfWaitingApproval(approvals.allPendingApproval.value.mapNotNull { it.expiryDate }.maxOrNull())
    }

    /**
     * Opens what a tapped notification is about, once Hisar is unlocked: the Approvals tab for an approval, Settings →
     * Devices for a security alert, where a device that just signed in shows, and Settings for a sharing invitation.
     */
    fun showNotification(kind: PushKind) {
        if (!status.value.isUnlocked) return
        notifications.openedKind.value = null
        selectedTab.value = if (kind == PushKind.APPROVAL) Tab.APPROVALS else Tab.SETTINGS
        if (kind == PushKind.SECURITY) requestedSettings.value = SettingsDestination.DEVICES
    }

    override fun openInvitationSettings(destination: SettingsDestination) {
        if (status.value.isUnlocked) {
            selectedTab.value = Tab.SETTINGS
            requestedSettings.value = destination
            return
        }
        invitationSettingsAfterUnlock = destination
        showNotice("Unlock Hisar to see the invitation.")
    }

    suspend fun handleWillEnterForeground() {
        notifications.clearApprovalReminder()
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

    // A remote change that comes while a refresh lists may be missing from its lists, so that refresh lists again, whoever
    // started it: the first refresh, an unlock or a pull to refresh as much as a remote change.
    override suspend fun refreshAll() {
        do {
            isRefreshRequestedAgain = false
            refreshAllOnce()
        } while (isRefreshRequestedAgain && status.value.isUnlocked)
    }

    // Listing the share invitations also completes the shares people accepted, and listing the waiting vault keys gives
    // team members the keys of the organization vaults this user owns, as the Mac's sync does. Both run before the vaults
    // are listed, so a vault rotated meanwhile is listed with its new key.
    private suspend fun refreshAllOnce() {
        if (!status.value.isUnlocked) return
        val refresh = ++startedRefreshCount
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
                    allVault, allItemByVaultID, attemptCall { backend.listAllPasskey() },
                    shareInvitations, allRemovedVault, organizations, waitingVaultKeys,
                )
            }
            // Locked or signed out while listing: locking and signing out emptied the data, so the listing must not bring
            // it back.
            refreshStatus()
            if (result == null || !status.value.isUnlocked) return
            // Refreshes can finish out of order: one that started earlier than the lists shown lists older data. Removed
            // vaults are listed only once, so even an older refresh keeps them.
            val isNewer = refresh > shownRefresh
            if (isNewer) shownRefresh = refresh
            mutableUnlockedData.update { data ->
                val allRemovedVaultNotice = data.allRemovedVaultNotice +
                    result.allRemovedVault.filter { notice -> data.allRemovedVaultNotice.none { it.id == notice.id } }
                if (!isNewer) return@update data.copy(allRemovedVaultNotice = allRemovedVaultNotice)
                data.copy(
                    allVault = result.allVault.sortedWith(vaultOrder),
                    allItemByVaultID = result.allItemByVaultID,
                    allPasskey = result.allPasskey ?: data.allPasskey,
                    allReceivedInvitation = result.shareInvitations?.allReceived ?: data.allReceivedInvitation,
                    allWaitingInvitation = result.shareInvitations?.allWaiting ?: data.allWaitingInvitation,
                    allReviewInvitation = result.shareInvitations?.allRequiringReview ?: data.allReviewInvitation,
                    // Offline both fail; the last organizations keep naming the vault groups.
                    allOrganization = result.organizations?.allOrganization ?: data.allOrganization,
                    allOrganizationInvitation = result.organizations?.allInvitation ?: data.allOrganizationInvitation,
                    waitingVaultKeys = result.waitingVaultKeys ?: data.waitingVaultKeys,
                    allRemovedVaultNotice = allRemovedVaultNotice,
                )
            }
        } finally {
            mutableIsRefreshing.value = false
        }
    }

    private class RefreshResult(
        val allVault: List<VaultSummary>,
        val allItemByVaultID: Map<String, List<ItemDetail>>,
        val allPasskey: List<StoredPasskey>?,
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
        scope.launch { refreshAll() }
    }

    suspend fun copyField(vaultID: String, itemID: String, fieldName: String, label: String) {
        perform(isBusyShown = false) { it.copyField(vaultID, itemID, fieldName) } ?: return
        mutableCopyCount.update { it + 1 }
        showNotice("$label copied. $CLEARING_NOTICE")
    }

    suspend fun copySecretReference(vaultID: String, itemID: String, fieldName: String) {
        val reference = perform(isBusyShown = false) { it.copySecretReference(vaultID, itemID, fieldName) } ?: return
        mutableCopyCount.update { it + 1 }
        showNotice("Copied $reference.")
    }

    /** core/item's rating of a password for the strength meter, or the empty rating without a backend. */
    suspend fun ratePassword(password: String): PasswordStrength = attempt { it.ratePassword(password) } ?: PasswordStrength()

    /** core/item's rule for a new master password; a password it could not rate counts as refused. */
    suspend fun checkMasterPassword(password: String): MasterPasswordFeedback =
        attempt { it.checkMasterPassword(password) } ?: MasterPasswordFeedback(isBlocking = true)

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
        /** Android clears a copy with an inexact alarm, which lands between 90 seconds and about 2.5 minutes. */
        private const val CLEARING_NOTICE = "It clears in about 2 minutes."

        /** The Personal vault first, then the others by name. */
        val vaultOrder: Comparator<VaultSummary> = compareByDescending<VaultSummary> { it.type == VaultSummary.TYPE_PERSONAL }
            .thenBy(String.CASE_INSENSITIVE_ORDER) { it.name }
    }
}

private inline fun <Value> attemptCall(call: () -> Value): Value? = try {
    call()
} catch (error: CancellationException) {
    throw error
} catch (_: Exception) {
    null
}
