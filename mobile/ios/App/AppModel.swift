import AuthenticationServices
import Foundation
import SwiftUI
import UIKit

@MainActor
final class AppModel: ObservableObject {
    enum Screen: Equatable {
        case starting
        case failed(String)
        case signedOut
        case locked
        case unlocked
        case updateRequired(UpdateRequirement)
    }

    enum Tab: Hashable {
        case vaults
        case approvals
        case settings
        case search
    }

    @Published private(set) var screen: Screen = .starting
    @Published private(set) var status: VaultStatus = .signedOut
    @Published private(set) var isBusy = false
    @Published private(set) var isRefreshing = false
    @Published var errorMessage: String?
    @Published var noticeMessage: String?
    @Published private(set) var allVault: [VaultSummary] = []
    @Published private(set) var allItemByVaultID: [String: [ItemDetail]] = [:]
    @Published var newSecretKey: String?
    @Published private(set) var isBiometricOfferShown = false
    @Published var pendingSignInLink: SignInLink?
    @Published var pendingInviteLink: InviteLink?
    @Published var inviteLinkMessage: String?
    @Published private(set) var isCheckingMinimumVersion = false
    @Published private(set) var lockCount = 0
    @Published private(set) var allPendingApproval: [PhoneApproval] = []
    @Published private(set) var presentedApproval: PhoneApproval?
    @Published private(set) var isAnsweringApproval = false
    @Published private(set) var isApprovalPasswordRequired = false
    @Published var approvalErrorMessage: String?
    @Published var selectedTab: Tab = .vaults
    /// requestedSettingsDestination is a Settings screen something outside the tab asks it to show, Devices for a tapped
    /// security alert; the Settings tab shows it and clears it.
    @Published var requestedSettingsDestination: SettingsDestination?
    /// settingsAfterUnlock is the Settings screen an invitation link opened while Hisar was locked; the next unlock
    /// shows it, and a sign-out drops it.
    private var settingsAfterUnlock: SettingsDestination?
    /// isNewSecretKeyShown is true while Settings shows a replaced Secret Key, which shows only once.
    private(set) var isNewSecretKeyShown = false
    @Published private(set) var copyCount = 0
    @Published private(set) var isMacApprovalAllowed = true
    @Published private(set) var isWebsiteIconsEnabled = true
    /// Website icons by domain for this unlock; nil while one loads or when the website has none.
    @Published private(set) var websiteIconByDomain: [String: Data] = [:]
    /// The domains asked during this unlock, finished or not, so a website without an icon is asked once.
    private var websiteIconTaskByDomain: [String: Task<Void, Never>] = [:]
    @Published private(set) var theme = VaultTheme.standard
    @Published private(set) var allReceivedInvitation: [ReceivedShareInvitation] = []
    @Published private(set) var allWaitingInvitation: [WaitingShareInvitation] = []
    @Published private(set) var allShareAwaitingUnlock: [ShareAwaitingUnlock] = []
    @Published private(set) var remoteChangeCount = 0
    @Published private(set) var allRemovedVaultNotice: [RemovedVaultNotice] = []
    @Published private(set) var allOrganization: [OrganizationSummary] = []
    @Published private(set) var allOrganizationInvitation: [OrganizationInvitation] = []
    @Published private(set) var waitingVaultKeys = WaitingVaultKeys()
    /// A Credential Exchange import another app started (Settings → Export on the other app, Hisar chosen); the token
    /// waits here until the vaults are unlocked.
    @Published private(set) var pendingCredentialImportToken: UUID?
    @Published private(set) var isCredentialImportShown = false
    @Published private(set) var isReceivingCredentialImport = false
    @Published private(set) var credentialImportPreview: ImportPreview?
    @Published private(set) var credentialImportProgress: ImportProgress?
    @Published private(set) var isImportingCredentials = false
    @Published var credentialImportMessage: String?
    /// isHisarExportImport is true while the open import sheet is a Hisar export the user picked, not another app's
    /// Credential Exchange data; it chooses the sheet's wording.
    @Published private(set) var isHisarExportImport = false
    private var startedRefreshCount: UInt64 = 0
    private var shownRefresh: UInt64 = 0
    private var startedApprovalListCount: UInt64 = 0
    private var shownApprovalList: UInt64 = 0
    @Published private(set) var allPendingRecovery: [PendingRecovery] = []
    @Published var isPendingRecoveryShown = false
    @Published private(set) var pendingRecoveryExport: PendingRecoveryExport?
    private var lifecycleEpoch: UInt64 = 0
    private var pendingDeliveryGeneration: UInt64 = 0

    var receivedInvitationCount: Int { allReceivedInvitation.count }
    var organizationInvitationCount: Int { allOrganizationInvitation.count }

    var autoLockInterval: TimeInterval { TimeInterval(status.autoLockSeconds) }

    private var backend: VaultBackend?
    private var allPasskey: [StoredPasskey] = []
    private let credentialIdentityStore: CredentialIdentityUpdating
    private let getCurrentDate: () -> Date
    private let receiveExportedCredentials: @MainActor (UUID) async throws -> Data
    private var enteredBackgroundAt: Date?
    private var noticeTask: Task<Void, Never>?
    private var allFinishedApprovalID: Set<String> = []
    /// A request the Mac announced while Hisar was in front, waiting for a listing that shows it to open its sheet.
    /// listedAfter is the last listing started before the announcement: a later listing without the request means it is gone.
    private struct AnnouncedApproval {
        var requestID: String
        var listedAfter: UInt64
    }
    private var allAnnouncedApproval: [AnnouncedApproval] = []
    private var isInForeground = true
    private var approvalExpiryTask: Task<Void, Never>?
    private var approvalPollTask: Task<Void, Never>?
    private var isRefreshRequestedAgain = false
    static let approvalPollInterval: TimeInterval = 15

    /// notifications holds Settings → Notifications and what a tapped notification opens.
    private(set) lazy var notifications = PushNotificationsModel(
        center: notificationCenter, registerForRemoteNotifications: registerForRemoteNotifications, getBackend: { [weak self] in self?.backend }
    )
    private let notificationCenter: NotificationScheduling
    private let registerForRemoteNotifications: @MainActor () -> Void

    init(
        credentialIdentityStore: CredentialIdentityUpdating, getCurrentDate: @escaping () -> Date = Date.init,
        receiveExportedCredentials: @escaping @MainActor (UUID) async throws -> Data = { try await AppModel.importExportedCredentials(token: $0) },
        notificationCenter: NotificationScheduling = SystemNotificationCenter(),
        registerForRemoteNotifications: @escaping @MainActor () -> Void = { UIApplication.shared.registerForRemoteNotifications() }
    ) {
        self.credentialIdentityStore = credentialIdentityStore
        self.getCurrentDate = getCurrentDate
        self.receiveExportedCredentials = receiveExportedCredentials
        self.notificationCenter = notificationCenter
        self.registerForRemoteNotifications = registerForRemoteNotifications
    }

    /// Returns the work start() begins, the first refresh and the version check, or nil when the backend failed. The app
    /// lets it run; a test awaits it, so its own changes come after the first lists.
    @discardableResult
    func start(createBackend: () throws -> VaultBackend) -> Task<Void, Never>? {
        do {
            let backend = try createBackend()
            self.backend = backend
            isMacApprovalAllowed = backend.isMacApprovalAllowed()
            isWebsiteIconsEnabled = backend.isWebsiteIconsEnabled()
            theme = backend.getTheme()
            applyStatus(backend.getStatus())
            clearIdentitiesWhenSignedOut()
            let firstRefresh = status.isUnlocked ? Task { await refreshAll() } : nil
            let versionCheck = Task { await checkMinimumVersion() }
            let pendingRecovery = Task { await refreshPendingRecovery() }
            let notificationStart = Task { await notifications.startAtLaunch() }
            return Task {
                await firstRefresh?.value
                await versionCheck.value
                await pendingRecovery.value
                await notificationStart.value
            }
        } catch {
            screen = .failed(describeError(error))
            return nil
        }
    }

    // Offline the backend keeps what it knew, so a failed check never blocks the app.
    func checkMinimumVersion() async {
        guard let backend else { return }
        let epoch = lifecycleEpoch
        isCheckingMinimumVersion = true
        defer { isCheckingMinimumVersion = false }
        guard let newStatus = try? await runInBackground({ backend.checkMinimumVersion() }), epoch == lifecycleEpoch else { return }
        applyStatus(newStatus)
        if newStatus.isUnlocked && newStatus.updateRequired == nil && isInForeground {
            startApprovalPolling()
        }
    }

    func handleUpdateRequired() {
        refreshStatus()
    }

    func openInviteLink(_ link: InviteLink) {
        if !status.isSignedIn {
            pendingInviteLink = link
            return
        }
        guard let invitation = link.invitation else {
            inviteLinkMessage = "You're signed in as \(status.email). Sign out to create an account with this invite."
            return
        }
        // Both servers went through resolveServer, so they compare as written, the host's case aside.
        if link.email != status.email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            || link.server.lowercased() != status.server.lowercased() {
            let serverLabel = link.server.hasPrefix("https://") ? String(link.server.dropFirst("https://".count)) : link.server
            inviteLinkMessage = "This invitation is for \(link.email) on \(serverLabel). You're signed in as \(status.email)."
            return
        }
        let destination: SettingsDestination = invitation == InviteLink.invitationOrganization ? .organizations : .people
        if status.isUnlocked {
            selectedTab = .settings
            requestedSettingsDestination = destination
            return
        }
        settingsAfterUnlock = destination
        inviteLinkMessage = "Unlock Hisar to see the invitation."
    }

    private func applyStatus(_ newStatus: VaultStatus) {
        let wasUnlocked = status.isUnlocked
        let wasSignedIn = status.isSignedIn
        if status.state != newStatus.state || status.email != newStatus.email || status.server != newStatus.server {
            lifecycleEpoch += 1
            clearPendingRecoveryExport()
        }
        status = newStatus
        if wasSignedIn && !newStatus.isSignedIn {
            settingsAfterUnlock = nil
            Task { await credentialIdentityStore.removeAllIdentity() }
        }
        switch newStatus.state {
        case VaultStatus.stateUnlocked: screen = .unlocked
        case VaultStatus.stateLocked: screen = .locked
        default: screen = .signedOut
        }
        if let requirement = newStatus.updateRequired {
            screen = .updateRequired(requirement)
            clearAllApproval()
        }
        if wasUnlocked && !newStatus.isUnlocked {
            allVault = []
            allItemByVaultID = [:]
            allPasskey = []
            lockCount += 1
            allReceivedInvitation = []
            allWaitingInvitation = []
            allRemovedVaultNotice = []
            allOrganization = []
            allOrganizationInvitation = []
            waitingVaultKeys = WaitingVaultKeys()
            newSecretKey = nil
            isBiometricOfferShown = false
            selectedTab = .vaults
            clearAllApproval()
            forgetWebsiteIcons()
            // Locking drops the prepared import in the backend too.
            clearCredentialImport()
        }
        if newStatus.state != VaultStatus.stateLocked {
            allShareAwaitingUnlock = []
        }
        if !wasUnlocked && newStatus.isUnlocked, let destination = settingsAfterUnlock {
            settingsAfterUnlock = nil
            selectedTab = .settings
            requestedSettingsDestination = destination
        }
        if !wasUnlocked && newStatus.isUnlocked && isInForeground && newStatus.updateRequired == nil {
            startApprovalPolling()
        }
        handlePendingCredentialImport()
    }

    private func applyShareInvitations(_ shareInvitations: ShareInvitations) {
        allReceivedInvitation = shareInvitations.allReceived
        allWaitingInvitation = shareInvitations.allWaiting
    }

    private func applyOrganizations(_ organizations: Organizations) {
        allOrganization = organizations.allOrganization
        allOrganizationInvitation = organizations.allInvitation
    }

    // The lock screen's banner: shares people accepted that only this unlocked app can finish. Failures stay silent.
    func refreshShareAwaitingUnlock() async {
        guard let backend, screen == .locked else { return }
        guard let allAwaiting = try? await runInBackground({ try backend.listAllShareAwaitingUnlock() }) else { return }
        if screen == .locked {
            allShareAwaitingUnlock = allAwaiting
        }
    }

    private func perform<Value>(isBusyShown: Bool = true, _ operation: @escaping (VaultBackend) throws -> Value) async -> Value? {
        guard let backend else { return nil }
        if isBusyShown { isBusy = true }
        defer { if isBusyShown { isBusy = false } }
        do {
            let value = try await runInBackground { try operation(backend) }
            errorMessage = nil
            return value
        } catch {
            errorMessage = describeError(error)
            return nil
        }
    }

    func clearError() {
        errorMessage = nil
    }

    func showNotice(_ message: String) {
        noticeMessage = message
        AccessibilityNotification.Announcement(message).post()
        noticeTask?.cancel()
        noticeTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 2_500_000_000)
            guard !Task.isCancelled else { return }
            self?.noticeMessage = nil
        }
    }

    @discardableResult
    func signIn(server: String, email: String, secretKey: String, password: String) async -> Bool {
        let secretKey = VaultLogic.formatSecretKey(secretKey)
        let server = server.trimmingCharacters(in: .whitespacesAndNewlines)
        let email = email.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let newStatus = await perform({ try $0.signIn(server: server, email: email, secretKey: secretKey, password: password) }) else { return false }
        applyStatus(newStatus)
        await refreshAll()
        return true
    }

    @discardableResult
    func signUp(inviteLink: String, email: String, name: String, password: String) async -> Bool {
        let inviteLink = inviteLink.trimmingCharacters(in: .whitespacesAndNewlines)
        let email = email.trimmingCharacters(in: .whitespacesAndNewlines)
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let result = await perform({ try $0.signUp(inviteLink: inviteLink, email: email, name: name, password: password) }) else { return false }
        return await finishSignUp(result)
    }

    /// signUpWithoutInvite creates an account on a server whose admin opened registration; the server refuses with its
    /// own sentence when sign-ups are closed.
    @discardableResult
    func signUpWithoutInvite(server: String, email: String, name: String, password: String) async -> Bool {
        let server = server.trimmingCharacters(in: .whitespacesAndNewlines)
        let email = email.trimmingCharacters(in: .whitespacesAndNewlines)
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let result = await perform({ try $0.signUpWithoutInvite(server: server, email: email, name: name, password: password) }) else { return false }
        return await finishSignUp(result)
    }

    private func finishSignUp(_ result: SignUpResult) async -> Bool {
        if !result.pendingOperationId.isEmpty {
            refreshStatus()
            // The backend's first sync can report its change before this status arrives, when it is ignored.
            if status.isUnlocked { await refreshAll() }
            await refreshPendingRecovery()
            if !allPendingRecovery.isEmpty { isPendingRecoveryShown = true }
            errorMessage = result.problems.isEmpty ? nil : result.problems.joined(separator: "\n")
            return true
        }
        newSecretKey = result.secretKey
        applyStatus(result.status)
        await refreshAll()
        return true
    }

    func finishNewSecretKey() {
        if status.biometricAvailable && !status.biometricEnabled {
            clearError()
            isBiometricOfferShown = true
        } else {
            dismissNewSecretKey()
        }
    }

    func dismissNewSecretKey() {
        newSecretKey = nil
        isBiometricOfferShown = false
        guard let backend else { return }
        Task { try? await runInBackground { backend.forgetNewAccountSecretKey() } }
    }

    @discardableResult
    func turnOnBiometricUnlockFromOffer() async -> Bool {
        guard let newStatus = await perform({ try $0.enableNewAccountBiometricUnlock() }) else { return false }
        applyStatus(newStatus)
        dismissNewSecretKey()
        showNotice("\(BiometricKeychain(accessGroup: nil).getBiometryName()) unlock is on")
        return true
    }

    @discardableResult
    func unlock(password: String) async -> Bool {
        guard let newStatus = await perform({ try $0.unlock(password: password) }) else { return false }
        applyStatus(newStatus)
        await refreshAll()
        return true
    }

    @discardableResult
    func unlockWithBiometric() async -> Bool {
        guard status.biometricReady, let newStatus = await perform({ try $0.unlockWithBiometric() }) else {
            refreshStatus()
            return false
        }
        applyStatus(newStatus)
        await refreshAll()
        return true
    }

    func lock() {
        guard let backend else { return }
        applyStatus(backend.lock())
    }

    func handleLockedEvent() {
        refreshStatus()
    }

    func refreshStatus() {
        guard let backend else { return }
        applyStatus(backend.getStatus())
    }

    @discardableResult
    func signOut() async -> Bool {
        // Signing out always finishes on this phone; an error says what it could not confirm, so the status still changes.
        guard let newStatus = await perform({ try $0.signOut() }) else {
            refreshStatus()
            return false
        }
        applyStatus(newStatus)
        await credentialIdentityStore.removeAllIdentity()
        return true
    }

    @discardableResult
    func setBiometricUnlock(isEnabled: Bool, password: String) async -> Bool {
        let newStatus: VaultStatus?
        if isEnabled {
            newStatus = await perform { try $0.enableBiometricUnlock(password: password) }
        } else {
            newStatus = await perform { try $0.disableBiometricUnlock() }
        }
        guard let newStatus else { return false }
        applyStatus(newStatus)
        return true
    }

    func reportActivity() {
        guard let backend, status.isUnlocked else { return }
        Task { _ = try? await runInBackground { backend.reportActivity() } }
    }

    func handleDidEnterBackground() {
        enteredBackgroundAt = getCurrentDate()
        isInForeground = false
        stopApprovalPolling()
        let isApprovalWaiting = !allPendingApproval.isEmpty
        Task { await notifications.remindOfWaitingApproval(isWaiting: isApprovalWaiting) }
    }

    /// showNotification opens what a tapped notification is about, once Hisar is unlocked: the Approvals tab for an
    /// approval, Settings → Devices for a security alert, where a device that just signed in shows, and Settings for a
    /// sharing invitation.
    func showNotification(_ kind: PushKind) {
        guard status.isUnlocked else { return }
        notifications.openedKind = nil
        selectedTab = kind == .approval ? .approvals : .settings
        if kind == .approval { Task { await refreshPendingApproval() } }
        if kind == .security { requestedSettingsDestination = .devices }
    }

    func handleWillEnterForeground() {
        notifications.clearApprovalReminder()
        Task { await checkMinimumVersion() }
        isInForeground = true
        defer {
            enteredBackgroundAt = nil
            clearIdentitiesWhenSignedOut()
            if status.isUnlocked, status.updateRequired == nil, let backend {
                backend.reconnectEvents()
                startApprovalPolling()
                handleRemoteChange()
            }
        }
        guard let enteredBackgroundAt else {
            refreshStatus()
            return
        }
        if status.isUnlocked && getCurrentDate().timeIntervalSince(enteredBackgroundAt) >= autoLockInterval {
            lock()
        } else {
            refreshStatus()
        }
    }

    // A remote change that comes while a refresh lists may be missing from its lists, so that refresh lists again, whoever
    // started it: the first refresh, an unlock or a pull to refresh as much as a remote change.
    func refreshAll() async {
        repeat {
            isRefreshRequestedAgain = false
            await refreshAllOnce()
        } while isRefreshRequestedAgain && status.isUnlocked
    }

    // Listing the share invitations also completes the shares people accepted, and listing the waiting vault keys gives
    // team members the keys of the organization vaults this user owns, as the Mac's sync does. Both run before the vaults
    // are listed, so a vault rotated meanwhile is listed with its new key.
    private func refreshAllOnce() async {
        guard status.isUnlocked else { return }
        startedRefreshCount += 1
        let refresh = startedRefreshCount
        isRefreshing = true
        defer { isRefreshing = false }
        guard let result = await perform(isBusyShown: false, { backend -> RefreshResult in
            let shareInvitations = try? backend.listAllShareInvitation()
            let waitingVaultKeys = try? backend.listAllWaitingVaultKey()
            let organizations = try? backend.listAllOrganization()
            let allVault = try backend.listAllVault()
            let allRemovedVault = (try? backend.listAllRemovedVault()) ?? []
            // Reads the favorites again, so the lists below mark them even when the event stream missed a change.
            _ = try? backend.listAllFavoriteItem()
            var allItemByVaultID: [String: [ItemDetail]] = [:]
            for vault in allVault {
                allItemByVaultID[vault.id] = try backend.listAllItem(vaultID: vault.id)
            }
            return RefreshResult(
                allVault: allVault, allItemByVaultID: allItemByVaultID, allPasskey: try? backend.listAllPasskey(),
                shareInvitations: shareInvitations, allRemovedVault: allRemovedVault, organizations: organizations, waitingVaultKeys: waitingVaultKeys
            )
        }) else {
            refreshStatus()
            return
        }
        // Locked or signed out while listing: locking and signing out cleared the items, so the listing must not bring
        // them back, nor put a signed-out account's logins back into AutoFill. A lock keeps what AutoFill already has.
        refreshStatus()
        guard status.isUnlocked else { return }
        // Refreshes can finish out of order: one that started earlier than the lists shown lists older data.
        if refresh > shownRefresh {
            shownRefresh = refresh
            allVault = result.allVault.sorted { Self.sortVault($0, $1) }
            allItemByVaultID = result.allItemByVaultID
            if let passkeys = result.allPasskey { allPasskey = passkeys }
            if let shareInvitations = result.shareInvitations {
                applyShareInvitations(shareInvitations)
            }
            // Offline both fail; the last organizations keep naming the vault groups.
            if let organizations = result.organizations {
                applyOrganizations(organizations)
            }
            if let waitingVaultKeys = result.waitingVaultKeys {
                self.waitingVaultKeys = waitingVaultKeys
            }
        }
        // Removed vaults are listed only once, so even an older refresh keeps them.
        addAllRemovedVaultNotice(result.allRemovedVault)
        if result.allPasskey != nil { await publishAllCredentialIdentity() }
    }

    private struct RefreshResult {
        var allVault: [VaultSummary]
        var allItemByVaultID: [String: [ItemDetail]]
        var allPasskey: [StoredPasskey]?
        var shareInvitations: ShareInvitations?
        var allRemovedVault: [RemovedVaultNotice]
        var organizations: Organizations?
        var waitingVaultKeys: WaitingVaultKeys?
    }

    func handleRemoteChange() {
        guard status.isUnlocked else { return }
        remoteChangeCount += 1
        guard !isRefreshing else {
            isRefreshRequestedAgain = true
            return
        }
        Task { await refreshAll() }
    }

    private func publishAllCredentialIdentity() async {
        guard allVault.allSatisfy({ $0.readIssues.isEmpty }),
              allItemByVaultID.values.allSatisfy({ $0.allSatisfy { $0.readIssue == nil } }) else { return }
        await credentialIdentityStore.replaceAllIdentity(VaultLogic.buildAllCredentialIdentity(listAllItemReference(), allPasskey: allPasskey))
    }

    // A sign-out in the AutoFill extension, or a removal the system refused, leaves identities a signed-out app must not offer.
    private func clearIdentitiesWhenSignedOut() {
        guard !status.isSignedIn else { return }
        Task { await credentialIdentityStore.removeAllIdentity() }
    }

    private static func sortVault(_ first: VaultSummary, _ second: VaultSummary) -> Bool {
        let isFirstPersonal = first.type == "personal"
        let isSecondPersonal = second.type == "personal"
        if isFirstPersonal != isSecondPersonal { return isFirstPersonal }
        return first.name.localizedCaseInsensitiveCompare(second.name) == .orderedAscending
    }

    /// Lists one vault again. Its summary takes the healthy count and the records that did not open from this listing,
    /// so a record that stopped opening is disclosed even though the item list hides it.
    func refreshVault(vaultID: String) async {
        guard let allItem = await perform(isBusyShown: false, { try $0.listAllItem(vaultID: vaultID) }) else {
            refreshStatus()
            return
        }
        refreshStatus()
        guard status.isUnlocked else { return }
        allItemByVaultID[vaultID] = allItem
        if let index = allVault.firstIndex(where: { $0.id == vaultID }) {
            allVault[index].itemCount = allItem.filter { $0.readIssue == nil }.count
            allVault[index].readIssues = allVault[index].readIssues.filter { $0.itemId == nil } + allItem.compactMap(\.readIssue)
        }
    }

    func findVault(vaultID: String) -> VaultSummary? {
        allVault.first { $0.id == vaultID }
    }

    func listAllItemReference(vaultID: String? = nil) -> [VaultItemReference] {
        let allSelectedVault = allVault.filter { vaultID == nil || $0.id == vaultID }
        let allReference = allSelectedVault.flatMap { vault in
            (allItemByVaultID[vault.id] ?? []).filter { $0.readIssue == nil }.map { VaultItemReference(vaultID: vault.id, vaultName: vault.name, item: $0) }
        }
        return VaultLogic.sortAllItem(allReference)
    }

    /// The starred items of every vault, sorted by title; favorites of deleted items are already gone from the lists.
    func listAllFavoriteReference() -> [VaultItemReference] {
        listAllItemReference().filter { $0.item.isFavorite }
    }

    @discardableResult
    func setItemFavorite(vaultID: String, itemID: String, isFavorite: Bool) async -> Bool {
        guard await perform(isBusyShown: false, { try $0.setItemFavorite(vaultID: vaultID, itemID: itemID, isFavorite: isFavorite) }) != nil else { return false }
        if let index = allItemByVaultID[vaultID]?.firstIndex(where: { $0.id == itemID }) {
            allItemByVaultID[vaultID]?[index].isFavorite = isFavorite
        }
        showNotice(isFavorite ? "Added to Favorites." : "Removed from Favorites.")
        return true
    }

    func countItem(vaultID: String) -> Int {
        allItemByVaultID[vaultID]?.filter { $0.readIssue == nil }.count ?? findVault(vaultID: vaultID)?.itemCount ?? 0
    }

    func loadItem(vaultID: String, itemID: String) async -> ItemDetail? {
        await perform(isBusyShown: false) { try $0.getItem(vaultID: vaultID, itemID: itemID) }
    }

    func createVault(name: String) async -> VaultSummary? {
        guard let vault = await perform({ try $0.createVault(name: name) }) else { return nil }
        addVault(vault)
        return vault
    }

    /// Deletes the unreadable record the user reviewed, named by its revision; a changed record is refused.
    func deleteUnreadableRecord(_ issue: ReadIssue) async {
        let deleted: Void? = await perform { backend in
            if let itemID = issue.itemId {
                try backend.deleteUnreadableItem(vaultID: issue.vaultId, itemID: itemID, revision: issue.revision ?? "")
            } else {
                try backend.deleteUnreadableVault(vaultID: issue.vaultId, revision: issue.revision ?? "")
            }
        }
        if deleted != nil { await refreshAll() }
    }

    /// Names a vault whose name did not open, only while its record is still the one the user reviewed.
    func repairVaultName(_ issue: ReadIssue, name: String) async {
        let repaired = await perform { try $0.repairVaultName(vaultID: issue.vaultId, revision: issue.revision ?? "", name: name) }
        if repaired != nil { await refreshAll() }
    }

    private func addVault(_ vault: VaultSummary) {
        allVault.removeAll { $0.id == vault.id }
        allVault.append(vault)
        allVault.sort { Self.sortVault($0, $1) }
        allItemByVaultID[vault.id] = []
    }

    func loadAllPerson() async -> [Member]? {
        await perform(isBusyShown: false) { try $0.listAllPerson() }
    }

    @discardableResult
    func invitePerson(email: String) async -> Bool {
        let email = email.trimmingCharacters(in: .whitespacesAndNewlines)
        return await perform({ try $0.invitePerson(email: email) }) != nil
    }

    func loadShareInvitations() async -> ShareInvitations? {
        guard let shareInvitations = await perform(isBusyShown: false, { try $0.listAllShareInvitation() }) else { return nil }
        applyShareInvitations(shareInvitations)
        return shareInvitations
    }

    @discardableResult
    func acceptShareInvitation(invitationID: String) async -> Bool {
        guard await perform({ try $0.acceptShareInvitation(invitationID: invitationID) }) != nil else { return false }
        allReceivedInvitation.removeAll { $0.id == invitationID }
        await refreshReceivedInvitationCount()
        return true
    }

    @discardableResult
    func declineShareInvitation(invitationID: String) async -> Bool {
        guard await perform({ try $0.declineShareInvitation(invitationID: invitationID) }) != nil else { return false }
        allReceivedInvitation.removeAll { $0.id == invitationID }
        return true
    }

    func loadVaultSharing(vaultID: String) async -> VaultSharing? {
        await perform(isBusyShown: false) { try $0.getVaultSharing(vaultID: vaultID) }
    }

    @discardableResult
    func cancelShareInvitation(invitationID: String) async -> Bool {
        await perform({ try $0.cancelShareInvitation(invitationID: invitationID) }) != nil
    }

    @discardableResult
    func confirmShareRecipient(_ invitation: SentShareInvitation) async -> Bool {
        await perform({ try $0.confirmShareRecipient(invitationID: invitation.id, reviewToken: invitation.reviewToken ?? "") }) != nil
    }

    func loadAllVaultMember(vaultID: String) async -> [Member]? {
        await perform(isBusyShown: false) { try $0.listAllVaultMember(vaultID: vaultID) }
    }

    @discardableResult
    func shareVault(vaultID: String, email: String, role: String) async -> Bool {
        let email = email.trimmingCharacters(in: .whitespacesAndNewlines)
        return await perform({ try $0.shareVault(vaultID: vaultID, email: email, role: role) }) != nil
    }

    @discardableResult
    func changeVaultMemberRole(vaultID: String, accountID: String, role: String) async -> Bool {
        await perform({ try $0.changeVaultMemberRole(vaultID: vaultID, accountID: accountID, role: role) }) != nil
    }

    // Removing a member re-encrypts the vault with a new key, so the items are loaded again afterwards.
    @discardableResult
    func removeVaultMember(vaultID: String, accountID: String) async -> Bool {
        guard await perform({ try $0.removeVaultMember(vaultID: vaultID, accountID: accountID) }) != nil else { return false }
        await refreshVault(vaultID: vaultID)
        return true
    }

    func loadVaultActions(vaultID: String) async -> VaultActions? {
        await perform(isBusyShown: false) { try $0.getVaultActions(vaultID: vaultID) }
    }

    @discardableResult
    func renameVault(vaultID: String, name: String) async -> Bool {
        guard let renamed = await perform({ try $0.renameVault(vaultID: vaultID, name: name) }) else { return false }
        if let index = allVault.firstIndex(where: { $0.id == vaultID }) {
            allVault[index].name = renamed.name
            allVault.sort { Self.sortVault($0, $1) }
        }
        showNotice("Renamed the vault to \(renamed.name)")
        return true
    }

    @discardableResult
    func deleteVault(vaultID: String, request: VaultDeletionRequest) async -> Bool {
        let vaultName = findVault(vaultID: vaultID)?.name ?? "the vault"
        guard let result = await perform({ try $0.deleteVault(vaultID: vaultID, request: request) }) else { return false }
        removeVaultLocally(vaultID: vaultID)
        await refreshAll()
        if result.movedCount > 0 {
            showNotice("Moved \(Self.describeItemCount(result.movedCount)) to \(result.targetVaultName) and deleted \(vaultName).")
        } else {
            showNotice("Deleted \(vaultName).")
        }
        return true
    }

    @discardableResult
    func leaveVault(vaultID: String) async -> Bool {
        let vaultName = findVault(vaultID: vaultID)?.name ?? "the vault"
        guard await perform({ try $0.leaveVault(vaultID: vaultID) }) != nil else { return false }
        removeVaultLocally(vaultID: vaultID)
        await refreshAll()
        showNotice("You left \(vaultName).")
        return true
    }

    private func removeVaultLocally(vaultID: String) {
        allVault.removeAll { $0.id == vaultID }
        allItemByVaultID[vaultID] = nil
        allPasskey.removeAll { $0.vaultID == vaultID }
    }

    private func addAllRemovedVaultNotice(_ allNotice: [RemovedVaultNotice]) {
        for notice in allNotice where !allRemovedVaultNotice.contains(where: { $0.id == notice.id }) {
            allRemovedVaultNotice.append(notice)
        }
    }

    func dismissRemovedVaultNotice(_ notice: RemovedVaultNotice) {
        allRemovedVaultNotice.removeAll { $0.id == notice.id }
    }

    static func describeItemCount(_ count: Int) -> String {
        count == 1 ? "1 item" : "\(count) items"
    }

    // Organizations (§5.11). A change that fails leaves the server's sentence in errorMessage, which the screens show in an
    // alert; changes that move access reload the vaults through the event stream, like any other remote change.
    @discardableResult
    func loadOrganizations() async -> Organizations? {
        guard let organizations = await perform(isBusyShown: false, { try $0.listAllOrganization() }) else { return nil }
        applyOrganizations(organizations)
        return organizations
    }

    func loadOrganization(organizationID: String) async -> Organization? {
        await perform(isBusyShown: false) { try $0.getOrganization(organizationID: organizationID) }
    }

    /// The organization screen's load. Gone only when the organizations no longer list it; otherwise a failure, also of
    /// that list, leaves GetOrganization's sentence in errorMessage for the screen's alert.
    func loadOrganizationScreen(organizationID: String) async -> OrganizationLoad {
        if let organization = await loadOrganization(organizationID: organizationID) {
            return .loaded(organization)
        }
        let sentence = errorMessage
        if let organizations = await loadOrganizations(), !organizations.allOrganization.contains(where: { $0.id == organizationID }) {
            return .gone
        }
        errorMessage = sentence
        return .failed
    }

    func createOrganization(name: String) async -> OrganizationSummary? {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let organization = await perform({ try $0.createOrganization(name: name) }) else { return nil }
        allOrganization.removeAll { $0.id == organization.id }
        allOrganization.append(organization)
        allOrganization.sort { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        return organization
    }

    @discardableResult
    func renameOrganization(organizationID: String, name: String) async -> Bool {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard await perform({ try $0.renameOrganization(organizationID: organizationID, name: name) }) != nil else { return false }
        if let index = allOrganization.firstIndex(where: { $0.id == organizationID }) {
            allOrganization[index].name = name
        }
        return true
    }

    @discardableResult
    func deleteOrganization(organizationID: String) async -> Bool {
        guard await perform({ try $0.deleteOrganization(organizationID: organizationID) }) != nil else { return false }
        allOrganization.removeAll { $0.id == organizationID }
        return true
    }

    @discardableResult
    func inviteToOrganization(organizationID: String, email: String, role: String) async -> Bool {
        let email = email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return await perform({ try $0.inviteToOrganization(organizationID: organizationID, email: email, role: role) }) != nil
    }

    @discardableResult
    func cancelOrganizationInvitation(organizationID: String, invitationID: String) async -> Bool {
        await perform({ try $0.cancelOrganizationInvitation(organizationID: organizationID, invitationID: invitationID) }) != nil
    }

    @discardableResult
    func acceptOrganizationInvitation(invitationID: String) async -> Bool {
        guard await perform({ try $0.acceptOrganizationInvitation(invitationID: invitationID) }) != nil else { return false }
        allOrganizationInvitation.removeAll { $0.id == invitationID }
        await loadOrganizations()
        return true
    }

    @discardableResult
    func declineOrganizationInvitation(invitationID: String) async -> Bool {
        guard await perform({ try $0.declineOrganizationInvitation(invitationID: invitationID) }) != nil else { return false }
        allOrganizationInvitation.removeAll { $0.id == invitationID }
        return true
    }

    @discardableResult
    func setOrganizationMemberRole(organizationID: String, accountID: String, role: String) async -> Bool {
        await perform({ try $0.setOrganizationMemberRole(organizationID: organizationID, accountID: accountID, role: role) }) != nil
    }

    @discardableResult
    func removeOrganizationMember(organizationID: String, accountID: String) async -> Bool {
        await perform({ try $0.removeOrganizationMember(organizationID: organizationID, accountID: accountID) }) != nil
    }

    // Leaving takes the organization's vaults from this user at once, so the vaults are listed again.
    @discardableResult
    func leaveOrganization(organizationID: String) async -> Bool {
        let organizationName = allOrganization.first { $0.id == organizationID }?.name ?? "the organization"
        guard await perform({ try $0.leaveOrganization(organizationID: organizationID) }) != nil else { return false }
        allOrganization.removeAll { $0.id == organizationID }
        await refreshAll()
        showNotice("You left \(organizationName).")
        return true
    }

    func createTeam(organizationID: String, name: String) async -> Team? {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return await perform { try $0.createTeam(organizationID: organizationID, name: name) }
    }

    @discardableResult
    func renameTeam(organizationID: String, teamID: String, name: String) async -> Bool {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return await perform({ try $0.renameTeam(organizationID: organizationID, teamID: teamID, name: name) }) != nil
    }

    @discardableResult
    func deleteTeam(organizationID: String, teamID: String) async -> Bool {
        await perform({ try $0.deleteTeam(organizationID: organizationID, teamID: teamID) }) != nil
    }

    @discardableResult
    func addTeamMember(organizationID: String, teamID: String, accountID: String) async -> Bool {
        await perform({ try $0.addTeamMember(organizationID: organizationID, teamID: teamID, accountID: accountID) }) != nil
    }

    @discardableResult
    func removeTeamMember(organizationID: String, teamID: String, accountID: String) async -> Bool {
        await perform({ try $0.removeTeamMember(organizationID: organizationID, teamID: teamID, accountID: accountID) }) != nil
    }

    @discardableResult
    func giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String) async -> Bool {
        await perform({ try $0.giveVaultToTeam(organizationID: organizationID, teamID: teamID, vaultID: vaultID, role: role) }) != nil
    }

    @discardableResult
    func takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String) async -> Bool {
        await perform({ try $0.takeVaultFromTeam(organizationID: organizationID, teamID: teamID, vaultID: vaultID) }) != nil
    }

    func createOrganizationVault(organizationID: String, teamID: String, name: String) async -> VaultSummary? {
        guard let vault = await perform({ try $0.createOrganizationVault(organizationID: organizationID, teamID: teamID, name: name) }) else { return nil }
        addVault(vault)
        return vault
    }

    /// The new vault's Where choices: a shared vault and each team the user is in. Nil, with the sentence in errorMessage,
    /// when the teams could not be listed: a failed lookup never stands for having no teams.
    func loadVaultLocations() async -> [VaultLocation]? {
        await perform(isBusyShown: false) { backend -> [VaultLocation] in
            let allOrganization = try backend.listAllOrganization().allOrganization.map { try backend.getOrganization(organizationID: $0.id) }
            return OrganizationLogic.listAllVaultLocation(allOrganization)
        }
    }

    func loadExportOptions() async -> ExportOptions? {
        await perform(isBusyShown: false) { try $0.getExportOptions() }
    }

    // The export goes into a folder of its own in the temporary directory; removeExport deletes that folder.
    func exportAllData(format: ExportFormat, passphrase: String, password: String) async -> ExportResult? {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("Export-\(UUID().uuidString)", isDirectory: true)
        let result = await perform { try $0.exportAllData(format: format, passphrase: format == .encrypted ? passphrase : "", password: password, directory: directory) }
        if result == nil {
            try? FileManager.default.removeItem(at: directory)
        }
        return result
    }

    func removeExport(_ result: ExportResult) {
        try? FileManager.default.removeItem(at: URL(fileURLWithPath: result.path).deletingLastPathComponent())
    }

    // Credential Exchange (iOS 26): another password manager hands its logins and passkeys to Hisar. What it exported stays
    // in memory only: the backend keeps the parsed items until the import starts, and cancelImport drops them.
    static let signedOutCredentialImportMessage = "Sign in to Hisar, then export from the other app again."

    /// importExportedCredentials asks the system for what the other app exported, encoded as the Credential Exchange JSON
    /// the backend reads (dates as seconds since 1970).
    static func importExportedCredentials(token: UUID) async throws -> Data {
        let exported = try await ASCredentialImportManager().importCredentials(token: token)
        return try makeCredentialExchangeEncoder().encode(exported)
    }

    /// The encoder of that JSON. iOS 26 reads the exporter's seconds since 1970 as seconds since 2001, so a passkey made in
    /// 2023 arrives dated 2054. No export holds a date after now, so such a date is read back against 2001, which gives the
    /// exporter's number exactly, and a date the system read correctly keeps it.
    static func makeCredentialExchangeEncoder(now: Date = Date()) -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .custom { date, encoder in
            var container = encoder.singleValueContainer()
            try container.encode(credentialExchangeSeconds(of: date, now: now))
        }
        return encoder
    }

    nonisolated static func credentialExchangeSeconds(of date: Date, now: Date) -> Double {
        date > now.addingTimeInterval(24 * 60 * 60) ? date.timeIntervalSinceReferenceDate : date.timeIntervalSince1970
    }

    func receiveCredentialExchange(token: UUID) {
        pendingCredentialImportToken = token
        handlePendingCredentialImport()
    }

    // A Hisar export (.hisar or plain .json) the user picks in Settings → Account. The file picker hands over a
    // security-scoped URL; its bytes go to the backend, which previews the export like a chosen file.
    static let importFileSizeLimit = 100 * 1024 * 1024

    /// importHisarExport reads the Hisar export at url, previews it for the personal vault, and opens the import sheet.
    /// An encrypted export shows the passphrase step first; openCredentialImportFile then opens it.
    func importHisarExport(from url: URL) async {
        guard status.isUnlocked, status.updateRequired == nil, !isCredentialImportShown else { return }
        errorMessage = nil
        isHisarExportImport = true
        isCredentialImportShown = true
        isReceivingCredentialImport = true
        defer { isReceivingCredentialImport = false }
        let fileName = url.lastPathComponent
        let content: Data
        let didAccess = url.startAccessingSecurityScopedResource()
        do {
            defer { if didAccess { url.stopAccessingSecurityScopedResource() } }
            // A file over the limit is refused before it is read into memory.
            if let size = try url.resourceValues(forKeys: [.fileSizeKey]).fileSize, size > Self.importFileSizeLimit {
                if isCredentialImportShown { errorMessage = "the file is larger than 100 MB" }
                return
            }
            content = try Data(contentsOf: url)
        } catch {
            if isCredentialImportShown { errorMessage = "Hisar couldn't read \(fileName). (\(describeError(error)))" }
            return
        }
        guard content.count <= Self.importFileSizeLimit else {
            if isCredentialImportShown { errorMessage = "the file is larger than 100 MB" }
            return
        }
        let allKnownVault = allVault
        let preview = await perform { backend -> ImportPreview in
            let allVault = allKnownVault.isEmpty ? try backend.listAllVault() : allKnownVault
            guard let vault = Self.chooseCredentialImportVault(allVault) else {
                throw BackendError(message: "You have no vault you can add items to.")
            }
            return try backend.prepareHisarExportImport(vaultID: vault.id, fileName: fileName, content: content)
        }
        guard isCredentialImportShown else {
            // Closed or locked while preparing: drop what the backend just prepared, and any sentence about it.
            if preview == nil { errorMessage = nil }
            cancelBackendImport()
            return
        }
        credentialImportPreview = preview
    }

    /// openCredentialImportFile opens the waiting encrypted Hisar export with its passphrase. A wrong one keeps the
    /// export waiting, with the backend's sentence, so the user can try again.
    func openCredentialImportFile(passphrase: String) async {
        guard let preview = credentialImportPreview, preview.isPassphraseRequired, credentialImportProgress == nil else { return }
        guard let opened = await perform({ try $0.openImportFile(passphrase: passphrase) }), isCredentialImportShown else { return }
        credentialImportPreview = opened
    }

    /// The vaults an import can go to: the personal vault and those the user may edit.
    var allCredentialImportVault: [VaultSummary] { allVault.filter(Self.isWritableVault) }

    nonisolated static func isWritableVault(_ vault: VaultSummary) -> Bool {
        vault.type == VaultSummary.typePersonal || vault.role != Member.vaultRoleReader
    }

    /// The vault an import starts in: the personal vault, else the first the user may edit.
    nonisolated static func chooseCredentialImportVault(_ allVault: [VaultSummary]) -> VaultSummary? {
        allVault.first { $0.type == VaultSummary.typePersonal } ?? allVault.first(where: isWritableVault)
    }

    // The token waits for unlocked vaults. Signed out it is dropped: tokens expire within minutes, so the user exports again.
    private func handlePendingCredentialImport() {
        guard let token = pendingCredentialImportToken, backend != nil else { return }
        guard status.isSignedIn else {
            pendingCredentialImportToken = nil
            credentialImportMessage = Self.signedOutCredentialImportMessage
            return
        }
        guard status.isUnlocked, status.updateRequired == nil, !isCredentialImportShown else { return }
        pendingCredentialImportToken = nil
        errorMessage = nil
        isCredentialImportShown = true
        isReceivingCredentialImport = true
        Task { await receiveCredentialImport(token: token) }
    }

    private func receiveCredentialImport(token: UUID) async {
        defer { isReceivingCredentialImport = false }
        let credentialData: Data
        do {
            credentialData = try await receiveExportedCredentials(token)
        } catch {
            if isCredentialImportShown {
                errorMessage = "Hisar couldn't receive the items from the other app. Export them again. (\(describeError(error)))"
            }
            return
        }
        let allKnownVault = allVault
        let preview = await perform { backend -> ImportPreview in
            let allVault = allKnownVault.isEmpty ? try backend.listAllVault() : allKnownVault
            guard let vault = Self.chooseCredentialImportVault(allVault) else {
                throw BackendError(message: "You have no vault you can add items to.")
            }
            return try backend.prepareCredentialExchangeImport(vaultID: vault.id, credentialData: credentialData)
        }
        guard isCredentialImportShown else {
            // Closed or locked while preparing: drop what the backend just prepared, and any sentence about it.
            if preview == nil { errorMessage = nil }
            cancelBackendImport()
            return
        }
        credentialImportPreview = preview
    }

    /// previewCredentialImport shows the import again for another vault, with that vault's duplicates.
    func previewCredentialImport(vaultID: String) async {
        guard credentialImportPreview != nil, credentialImportProgress == nil else { return }
        guard let preview = await perform({ try $0.previewImport(vaultID: vaultID) }), isCredentialImportShown else { return }
        credentialImportPreview = preview
    }

    /// mapCredentialImportGroup sends one source group, such as a collection of the other app, to another vault; the
    /// other groups stay where they are.
    func mapCredentialImportGroup(groupID: String, vaultID: String) async {
        guard credentialImportPreview != nil, credentialImportProgress == nil else { return }
        guard let preview = await perform({ try $0.mapImportGroup(groupID: groupID, vaultID: vaultID) }), isCredentialImportShown else { return }
        credentialImportPreview = preview
    }

    func startCredentialImport(isSkippingDuplicate: Bool) async {
        guard let preview = credentialImportPreview, credentialImportProgress == nil, !isImportingCredentials else { return }
        isImportingCredentials = true
        defer { isImportingCredentials = false }
        guard let progress = await perform(isBusyShown: false, { try $0.startImport(vaultID: preview.vaultID, isSkippingDuplicate: isSkippingDuplicate) }),
              isCredentialImportShown else { return }
        credentialImportProgress = progress
        await continueCredentialImportUntilDone()
    }

    /// continueCredentialImport is Retry: an error pauses the import where it was, and the backend continues from there.
    func continueCredentialImport() async {
        guard credentialImportProgress?.isDone == false, !isImportingCredentials else { return }
        isImportingCredentials = true
        defer { isImportingCredentials = false }
        await continueCredentialImportUntilDone()
    }

    // The backend saves a batch per call. The finished import lists the vaults again, which also gives AutoFill the passkeys.
    private func continueCredentialImportUntilDone() async {
        guard let backend else { return }
        errorMessage = nil
        while isCredentialImportShown, let progress = credentialImportProgress, !progress.isDone {
            do {
                let nextProgress = try await runInBackground { try backend.continueImport() }
                guard isCredentialImportShown else { return }
                credentialImportProgress = nextProgress
            } catch {
                if isCredentialImportShown {
                    errorMessage = describeError(error)
                }
                return
            }
        }
        guard isCredentialImportShown, credentialImportProgress?.isDone == true else { return }
        await refreshAll()
    }

    /// closeCredentialImport is Cancel and Done: it drops the import in the backend. Items a cancelled import already
    /// saved stay, so the vaults are listed again.
    func closeCredentialImport() {
        guard isCredentialImportShown else { return }
        let isPartlySaved = credentialImportProgress.map { !$0.isDone } ?? false
        cancelBackendImport()
        clearCredentialImport()
        errorMessage = nil
        if isPartlySaved {
            Task { await refreshAll() }
        }
        handlePendingCredentialImport()
    }

    // A batch being saved holds the backend until it ends, so the import is dropped in the background.
    private func cancelBackendImport() {
        guard let backend else { return }
        Task { try? await runInBackground { backend.cancelImport() } }
    }

    private func clearCredentialImport() {
        isCredentialImportShown = false
        isReceivingCredentialImport = false
        credentialImportPreview = nil
        credentialImportProgress = nil
        isHisarExportImport = false
    }

    func saveItem(vaultID: String, item: ItemDetail) async -> ItemDetail? {
        guard let savedItem = await perform({ try $0.saveItem(vaultID: vaultID, item: item) }) else { return nil }
        var allItem = allItemByVaultID[vaultID] ?? []
        var summary = savedItem
        summary.password = ""
        summary.notes = ""
        summary.allField = []
        summary.oneTimePassword = ""
        if let card = savedItem.card {
            var cardSummary = ItemCard()
            cardSummary.type = card.type
            cardSummary.brand = card.brand
            cardSummary.last4 = card.last4
            summary.card = cardSummary
        }
        if let identity = savedItem.identity {
            var identitySummary = ItemIdentity()
            identitySummary.firstName = identity.firstName
            identitySummary.middleName = identity.middleName
            identitySummary.lastName = identity.lastName
            identitySummary.email = identity.email
            // As core/app's list summary: search also finds an identity by its address.
            identitySummary.street = identity.street.split(separator: "\n", maxSplits: 1).first.map(String.init) ?? ""
            identitySummary.city = identity.city
            identitySummary.country = identity.country
            summary.identity = identitySummary
        }
        if let credential = savedItem.apiCredential {
            var credentialSummary = ItemAPICredential()
            credentialSummary.service = credential.service
            credentialSummary.keyID = credential.keyID
            credentialSummary.host = credential.host
            summary.apiCredential = credentialSummary
        }
        if let address = savedItem.address {
            // As core/app's list summary: the first line of the street, the city and the country.
            summary.address = ItemAddress(street: address.street.split(separator: "\n", maxSplits: 1).first.map(String.init) ?? "", city: address.city, country: address.country)
        }
        if let server = savedItem.server {
            // As core/app's list summary: the host and port.
            summary.server = ItemServer(host: server.host, port: server.port)
        }
        if let database = savedItem.database {
            // As core/app's list summary: never the connection string or options.
            summary.database = ItemDatabase(engine: database.engine, engineName: database.engineName, host: database.host, port: database.port, name: database.name)
        }
        if let license = savedItem.softwareLicense {
            // As core/app's list summary: never the license key, purchase date or download URL.
            summary.softwareLicense = ItemSoftwareLicense(product: license.product, version: license.version, licensedTo: license.licensedTo, email: license.email)
        }
        if let index = allItem.firstIndex(where: { $0.id == savedItem.id }) {
            allItem[index] = summary
        } else {
            allItem.append(summary)
        }
        allItemByVaultID[vaultID] = allItem
        await publishAllCredentialIdentity()
        return savedItem
    }

    @discardableResult
    func deleteItem(vaultID: String, itemID: String) async -> Bool {
        guard await perform({ try $0.deleteItem(vaultID: vaultID, itemID: itemID) }) != nil else { return false }
        allItemByVaultID[vaultID]?.removeAll { $0.id == itemID }
        allPasskey.removeAll { $0.vaultID == vaultID && $0.itemID == itemID }
        await publishAllCredentialIdentity()
        return true
    }

    /// The brand, last four digits and warnings for a card being typed; nil when the backend cannot answer.
    func describeCard(number: String, expiry: String) async -> CardDescription? {
        guard let backend else { return nil }
        return try? await runInBackground { try backend.describeCard(number: number, expiry: expiry) }
    }

    func convertToSecureNote(vaultID: String, itemID: String) async -> ItemDetail? {
        guard let convertedItem = await perform({ try $0.convertToSecureNote(vaultID: vaultID, itemID: itemID) }) else { return nil }
        if let index = allItemByVaultID[vaultID]?.firstIndex(where: { $0.id == itemID }) {
            allItemByVaultID[vaultID]?[index].category = convertedItem.category
            allItemByVaultID[vaultID]?[index].revision = convertedItem.revision
            allItemByVaultID[vaultID]?[index].canConvertToSecureNote = false
        }
        showNotice("Converted to a secure note.")
        return convertedItem
    }

    func deletePasskey(vaultID: String, itemID: String, credentialID: String) async -> ItemDetail? {
        guard let updatedItem = await perform({ try $0.deletePasskey(vaultID: vaultID, itemID: itemID, credentialID: credentialID) }) else { return nil }
        if let index = allItemByVaultID[vaultID]?.firstIndex(where: { $0.id == itemID }) {
            allItemByVaultID[vaultID]?[index].allPasskey = updatedItem.allPasskey
        }
        allPasskey.removeAll { $0.credentialID == credentialID }
        await publishAllCredentialIdentity()
        showNotice("Passkey deleted.")
        return updatedItem
    }

    func loadOneTimeCode(vaultID: String, itemID: String) async -> OneTimeCode? {
        guard let backend else { return nil }
        return try? await runInBackground { try backend.getOneTimeCode(vaultID: vaultID, itemID: itemID) }
    }

    func copyField(vaultID: String, itemID: String, fieldName: String, label: String) async {
        guard await perform(isBusyShown: false, { try $0.copyField(vaultID: vaultID, itemID: itemID, fieldName: fieldName) }) != nil else { return }
        copyCount += 1
        showNotice("\(label) copied. It clears in 90 seconds.")
    }

    func copySecretReference(vaultID: String, itemID: String, fieldName: String) async {
        guard await perform(isBusyShown: false, { try $0.copySecretReference(vaultID: vaultID, itemID: itemID, fieldName: fieldName) }) != nil else { return }
        copyCount += 1
        showNotice("Secret reference copied.")
    }

    // Not secret, so it stays on the pasteboard and may reach other devices: support details and diagnostics.
    func copyPlainText(_ text: String, label: String) {
        UIPasteboard.general.string = text
        copyCount += 1
        showNotice("\(label) copied")
    }

    func copyText(_ text: String, label: String) {
        guard PasteboardClipboard().setText(text) else { return }
        copyCount += 1
        showNotice("\(label) copied. It clears in 90 seconds.")
    }

    @discardableResult
    func setAutoLock(seconds: Int) async -> Bool {
        guard let newStatus = await perform(isBusyShown: false, { try $0.setAutoLock(seconds: seconds) }) else { return false }
        applyStatus(newStatus)
        return true
    }

    func loadAllDevice() async -> [Device]? {
        await perform(isBusyShown: false) { try $0.listAllDevice() }
    }

    @discardableResult
    func revokeDevice(_ device: Device) async -> Bool {
        guard await perform({ try $0.revokeDevice(deviceID: device.id) }) != nil else { return false }
        showNotice("\(device.name) is signed out.")
        return true
    }

    func exportEmergencyKit(password: String) async -> EmergencyKitExport? {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("EmergencyKit-\(UUID().uuidString)", isDirectory: true)
        return await perform { try $0.exportEmergencyKit(password: password, directory: directory) }
    }

    func removeEmergencyKit(_ export: EmergencyKitExport) {
        removeEmergencyKitFile(path: export.emergencyKitPath)
    }

    /// Exports and Emergency Kits, a pending recovery's included, live in the temporary folder only while their screen is
    /// open, and no screen outlives the process. Whatever a killed process left there is deleted at launch, before any
    /// account opens. The prefixes are those of the folders the exports and kits are written to.
    static func deleteLeftoverKitFiles(in directory: URL = FileManager.default.temporaryDirectory) {
        let allName = (try? FileManager.default.contentsOfDirectory(atPath: directory.path)) ?? []
        for name in allName where ["Export-", "EmergencyKit-", "PendingRecovery-"].contains(where: { name.hasPrefix($0) }) {
            try? FileManager.default.removeItem(at: directory.appendingPathComponent(name))
        }
    }

    func removeEmergencyKitFile(path: String) {
        guard !path.isEmpty else { return }
        try? FileManager.default.removeItem(at: URL(fileURLWithPath: path).deletingLastPathComponent())
    }

    func changeMasterPassword(currentPassword: String, newPassword: String) async -> CredentialChangeResult? {
        guard let result = await perform({ try $0.changeMasterPassword(currentPassword: currentPassword, newPassword: newPassword) }) else { return nil }
        if !result.pendingOperationId.isEmpty {
            refreshStatus()
            await refreshPendingRecovery()
            if !allPendingRecovery.isEmpty { isPendingRecoveryShown = true }
            var seen = Set<String>()
            let problems = (result.problems + [result.localError, result.keychainError]).filter { !$0.isEmpty && seen.insert($0).inserted }
            errorMessage = problems.isEmpty ? nil : problems.joined(separator: "\n")
            return result
        }
        applyStatus(result.status)
        let warning = result.localError.isEmpty ? "" : " \(result.localError)"
        showNotice("Master password changed. \(AccountLifecycleLogic.describeSignedOutDevices(result.signedOutDeviceCount))\(warning)")
        return result
    }

    func replaceSecretKey(password: String) async -> CredentialChangeResult? {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("EmergencyKit-\(UUID().uuidString)", isDirectory: true)
        guard let result = await perform({ try $0.replaceSecretKey(password: password, directory: directory) }) else { return nil }
        if !result.pendingOperationId.isEmpty {
            removeEmergencyKitFile(path: result.emergencyKitPath)
            refreshStatus()
            await refreshPendingRecovery()
            if !allPendingRecovery.isEmpty { isPendingRecoveryShown = true }
            var seen = Set<String>()
            let problems = (result.problems + [result.localError, result.keychainError]).filter { !$0.isEmpty && seen.insert($0).inserted }
            errorMessage = problems.isEmpty ? nil : problems.joined(separator: "\n")
            return result
        }
        isNewSecretKeyShown = true
        applyStatus(result.status)
        return result
    }

    func finishSecretKeyReplacement(_ result: CredentialChangeResult) {
        isNewSecretKeyShown = false
        removeEmergencyKitFile(path: result.emergencyKitPath)
        guard let backend else { return }
        Task { try? await runInBackground { backend.forgetNewAccountSecretKey() } }
    }

    func loadAccountDeletionPreview() async -> AccountDeletionPreview? {
        await perform(isBusyShown: false) { try $0.previewAccountDeletion() }
    }

    @discardableResult
    func deleteAccount(password: String, email: String, planVersion: String) async -> Bool {
        guard let newStatus = await perform({ try $0.deleteAccountWithPlan(password: password, email: email, planVersion: planVersion) }) else {
            refreshStatus()
            return false
        }
        applyStatus(newStatus)
        await credentialIdentityStore.removeAllIdentity()
        newSecretKey = nil
        pendingSignInLink = nil
        showNotice("Your Hisar account was deleted.")
        return true
    }

    func dismissSignedOutReason() {
        guard let backend else { return }
        applyStatus(backend.dismissSignedOutReason())
    }

    func refreshPendingRecovery() async {
        guard let backend else { return }
        let records: [PendingRecovery]
        do {
            records = try await runInBackground { try backend.listPendingRecovery() }
        } catch {
            if errorMessage == nil { errorMessage = describeError(error) }
            return
        }
        let hadNone = allPendingRecovery.isEmpty
        allPendingRecovery = records
        if records.isEmpty { closePendingRecovery() }
        else if hadNone { isPendingRecoveryShown = true }
    }

    func clearPendingRecoveryExport() {
        pendingDeliveryGeneration += 1
        if let export = pendingRecoveryExport { removeEmergencyKitFile(path: export.path) }
        pendingRecoveryExport = nil
    }

    func closePendingRecovery() {
        clearPendingRecoveryExport()
        isPendingRecoveryShown = false
    }

    func resumePendingRecovery(operationID: String, password: String, secretKey: String = "", priorSecretKey: String = "") async {
        let epoch = lifecycleEpoch
        let result = await perform {
            if secretKey.isEmpty { return try $0.resumePendingRecovery(operationID: operationID, password: password) }
            return try $0.resumePendingRecoveryWithSecretKey(
                operationID: operationID, password: password,
                secretKey: VaultLogic.formatSecretKey(secretKey), priorSecretKey: VaultLogic.formatSecretKey(priorSecretKey)
            )
        }
        guard let result, epoch == lifecycleEpoch else {
            let recoveryError = errorMessage
            await refreshPendingRecovery()
            if let recoveryError { errorMessage = recoveryError }
            return
        }
        refreshStatus()
        if status.isUnlocked { await refreshAll() }
        await refreshPendingRecovery()
        if !result.problems.isEmpty { errorMessage = result.problems.joined(separator: "\n") }
    }

    func discardPreparedRecovery(operationID: String, password: String) async {
        guard await perform({ try $0.discardPreparedRecovery(operationID: operationID, password: password) }) != nil else { return }
        clearPendingRecoveryExport()
        await refreshPendingRecovery()
    }

    func exportPendingRecoveryKit(operationID: String, password: String) async {
        clearPendingRecoveryExport()
        let epoch = lifecycleEpoch
        let generation = pendingDeliveryGeneration
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("PendingRecovery-\(UUID().uuidString)", isDirectory: true)
        let result = await perform { try $0.exportPendingRecoveryEmergencyKit(operationID: operationID, password: password, directory: directory) }
        guard let result, epoch == lifecycleEpoch, generation == pendingDeliveryGeneration else {
            try? FileManager.default.removeItem(at: directory)
            // A lock, sign-out or account switch hides the Secret Key; a newer export, another record or closing the
            // screen is the user's own choice and needs no word.
            if result != nil && epoch != lifecycleEpoch {
                errorMessage = "Hisar locked, signed out or switched accounts while it prepared the Emergency Kit. Prepare it again."
            }
            return
        }
        pendingRecoveryExport = result
        await refreshPendingRecovery()
    }

    func acknowledgePendingRecovery(operationID: String, password: String) async {
        let epoch = lifecycleEpoch
        let recovery = allPendingRecovery.first { $0.operationId == operationID }
        guard await perform({ try $0.acknowledgePendingRecovery(operationID: operationID, password: password) }) != nil else { return }
        guard epoch == lifecycleEpoch else { return }
        clearPendingRecoveryExport()
        await refreshPendingRecovery()
        if epoch == lifecycleEpoch, recovery?.kind == "signup", recovery?.phase == "delivery-pending",
           recovery?.manualUnlock == false, recovery?.email == status.email,
           recovery?.origin == status.server, status.isUnlocked && status.biometricAvailable && !status.biometricEnabled {
            newSecretKey = ""
            isBiometricOfferShown = true
        }
    }

    // setTheme shows the theme at once and goes back to the one in use when it cannot be saved.
    func setTheme(_ newTheme: VaultTheme) async {
        let previousTheme = theme
        theme = newTheme
        guard let savedTheme = await perform(isBusyShown: false, { try $0.setTheme(newTheme) }) else {
            theme = previousTheme
            return
        }
        theme = savedTheme
    }

    /// loadWebsiteIcon asks the backend once per domain for the icon of a login's website; tiles read websiteIconByDomain.
    func loadWebsiteIcon(url: String) {
        guard isWebsiteIconsEnabled, let backend, let domain = Self.getWebsiteIconDomain(url),
              websiteIconByDomain[domain] == nil, websiteIconTaskByDomain[domain] == nil else { return }
        websiteIconTaskByDomain[domain] = Task { [weak self] in
            let icon = try? await runInBackground { try backend.getWebsiteIcon(url: url) }
            guard let self, !Task.isCancelled else { return }
            if let icon {
                self.websiteIconByDomain[domain] = icon
            }
        }
    }

    func getWebsiteIcon(url: String) -> Data? {
        Self.getWebsiteIconDomain(url).flatMap { websiteIconByDomain[$0] }
    }

    nonisolated static func getWebsiteIconDomain(_ url: String) -> String? {
        guard let components = URLComponents(string: url.trimmingCharacters(in: .whitespaces)), components.scheme?.lowercased() == "https",
              let host = components.host?.lowercased(), !host.isEmpty else { return nil }
        return host
    }

    private func forgetWebsiteIcons() {
        websiteIconTaskByDomain.values.forEach { $0.cancel() }
        websiteIconTaskByDomain = [:]
        websiteIconByDomain = [:]
    }

    func setWebsiteIconsEnabled(_ isEnabled: Bool) async {
        guard await perform(isBusyShown: false, { try $0.setWebsiteIconsEnabled(isEnabled) }) != nil else { return }
        isWebsiteIconsEnabled = isEnabled
        forgetWebsiteIcons()
    }

    static let unreadablePhotoMessage = "Hisar can't read this photo. Choose another one."

    /// prepareItemIcon turns a chosen photo into the icon image an item keeps, or shows why it can't. Photos come as the
    /// library keeps them (HEIC from the camera, TIFF, PNG…), which the backend does not all read, so the phone decodes
    /// and shrinks the photo first and hands it a PNG or JPEG.
    func prepareItemIcon(_ photo: Data) async -> Data? {
        guard let upload = Self.convertPhotoForIcon(photo) else {
            errorMessage = Self.unreadablePhotoMessage
            return nil
        }
        return await perform { try $0.prepareItemIcon(upload) }
    }

    /// convertPhotoForIcon decodes any image UIKit reads, applies its orientation, scales it down to maxSide and encodes
    /// it as PNG when it has transparency, else as JPEG. It returns nil for data that is no image.
    nonisolated static func convertPhotoForIcon(_ photo: Data, maxSide: CGFloat = 512) -> Data? {
        guard let image = UIImage(data: photo), image.size.width > 0, image.size.height > 0 else { return nil }
        let scale = min(1, maxSide / max(image.size.width, image.size.height))
        let size = CGSize(width: max(1, (image.size.width * scale).rounded()), height: max(1, (image.size.height * scale).rounded()))
        let alphaInfo = image.cgImage?.alphaInfo
        let hasAlpha = [.first, .last, .premultipliedFirst, .premultipliedLast].contains(alphaInfo)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = !hasAlpha
        let resized = UIGraphicsImageRenderer(size: size, format: format).image { _ in
            image.draw(in: CGRect(origin: .zero, size: size))
        }
        return hasAlpha ? resized.pngData() : resized.jpegData(compressionQuality: 0.9)
    }

    @discardableResult
    func setMacApprovalAllowed(_ isAllowed: Bool) async -> Bool {
        guard await perform(isBusyShown: false, { try $0.setMacApprovalAllowed(isAllowed) }) != nil else { return false }
        isMacApprovalAllowed = isAllowed
        if isAllowed {
            await refreshPendingApproval()
        } else {
            for approval in allPendingApproval {
                allFinishedApprovalID.insert(approval.requestID)
            }
            allPendingApproval = []
            allAnnouncedApproval = []
            shownApprovalList = startedApprovalListCount
            presentedApproval = nil
        }
        return true
    }

    func refreshReceivedInvitationCount() async {
        guard let invitations = await perform(isBusyShown: false, { try $0.listAllShareInvitation() }) else { return }
        applyShareInvitations(invitations)
        guard let backend, let organizations = try? await runInBackground({ try backend.listAllOrganization() }), status.isUnlocked else { return }
        applyOrganizations(organizations)
    }

    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) async -> String? {
        await perform(isBusyShown: false) { try $0.generatePassword(length: length, isDigitIncluded: isDigitIncluded, isSymbolIncluded: isSymbolIncluded) }
    }

    // Phone approvals: the Mac's open requests, listed while unlocked and in the foreground.
    func handleApprovalEvent(name: String, requestID: String) {
        switch name {
        case PhoneApproval.eventRequested:
            guard isInForeground else { return }
            // Recorded now, not in the listing's task: a sheet the user closes before that task runs must still cancel it.
            recordAnnouncedApproval(requestID: requestID)
            Task { await refreshPendingApproval() }
        case PhoneApproval.eventAnswered, PhoneApproval.eventResolved:
            removeApproval(requestID: requestID)
        default:
            break
        }
    }

    // refreshPendingApproval lists the open requests. announcedRequestID is a request the Mac announced: the first listing
    // that shows it, this one or a newer one that overtakes it, opens its sheet when none is open.
    func refreshPendingApproval(announcedRequestID: String? = nil) async {
        if let announcedRequestID {
            recordAnnouncedApproval(requestID: announcedRequestID)
        }
        guard status.isUnlocked, isInForeground, isMacApprovalAllowed, let backend else { return }
        startedApprovalListCount += 1
        let approvalList = startedApprovalListCount
        guard let allApproval = try? await runInBackground({ try backend.listAllPendingApproval() }) else { return }
        // Listings run at once and can finish out of order: a poll that started before a request's own listing must not
        // take that request back off the list and close its sheet.
        guard status.isUnlocked, approvalList > shownApprovalList else { return }
        shownApprovalList = approvalList
        let now = getCurrentDate()
        let allOpenApproval = allApproval.filter { !allFinishedApprovalID.contains($0.requestID) && !$0.isExpired(at: now) }
        // Assigning publishes even an equal list, and the poll would then redraw every screen every 15 seconds. A switch
        // whose tap ends during that redraw ignores the tap.
        if allOpenApproval != allPendingApproval {
            allPendingApproval = allOpenApproval
        }
        openAnnouncedApproval(listedIn: approvalList)
        closeSheetOfFinishedApproval()
        startApprovalExpiryCheck()
    }

    private func recordAnnouncedApproval(requestID: String) {
        guard status.isUnlocked, isInForeground, isMacApprovalAllowed else { return }
        allAnnouncedApproval.append(AnnouncedApproval(requestID: requestID, listedAfter: startedApprovalListCount))
    }

    // Opens the first announced request this listing shows, when no sheet is open. A request this listing reflects leaves
    // the queue: it opened, it is listed while another sheet is open (as one that arrived during that sheet always was),
    // or the listing started after its announcement and lacks it, so it was answered, withdrawn or expired.
    private func openAnnouncedApproval(listedIn approvalList: UInt64) {
        for announced in allAnnouncedApproval where approvalList > announced.listedAfter || allPendingApproval.contains(where: { $0.requestID == announced.requestID }) {
            allAnnouncedApproval.removeAll { $0.requestID == announced.requestID }
            if presentedApproval == nil, let approval = allPendingApproval.first(where: { $0.requestID == announced.requestID }) {
                presentApproval(approval)
            }
        }
    }

    func reviewApproval(_ approval: PhoneApproval) {
        guard allPendingApproval.contains(where: { $0.requestID == approval.requestID }) else { return }
        presentApproval(approval)
    }

    // The user closed the sheet: requests announced before that do not open one now; a request announced later still does.
    func closeApprovalSheet() {
        guard !isAnsweringApproval else { return }
        presentedApproval = nil
        allAnnouncedApproval = []
    }

    private func presentApproval(_ approval: PhoneApproval) {
        if presentedApproval?.requestID != approval.requestID {
            isApprovalPasswordRequired = false
            approvalErrorMessage = nil
        }
        presentedApproval = approval
    }

    private func closeSheetOfFinishedApproval() {
        guard let presentedApproval, !allPendingApproval.contains(where: { $0.requestID == presentedApproval.requestID }) else { return }
        self.presentedApproval = nil
        isAnsweringApproval = false
        isApprovalPasswordRequired = false
        approvalErrorMessage = nil
    }

    private func removeApproval(requestID: String) {
        allFinishedApprovalID.insert(requestID)
        allPendingApproval.removeAll { $0.requestID == requestID }
        allAnnouncedApproval.removeAll { $0.requestID == requestID }
        closeSheetOfFinishedApproval()
    }

    func removeAllExpiredApproval() {
        let now = getCurrentDate()
        for approval in allPendingApproval where approval.isExpired(at: now) {
            removeApproval(requestID: approval.requestID)
        }
    }

    private func startApprovalExpiryCheck() {
        guard !allPendingApproval.isEmpty, approvalExpiryTask == nil else { return }
        approvalExpiryTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                guard let self, !Task.isCancelled else { return }
                self.removeAllExpiredApproval()
                if self.allPendingApproval.isEmpty {
                    self.approvalExpiryTask = nil
                    return
                }
            }
        }
    }

    // The event stream can miss requests on a phone (suspension, a dead connection), so the open requests are also polled.
    private func startApprovalPolling() {
        guard approvalPollTask == nil else { return }
        approvalPollTask = Task { [weak self] in
            while !Task.isCancelled, self != nil {
                await self?.refreshPendingApproval()
                try? await Task.sleep(nanoseconds: UInt64(Self.approvalPollInterval * 1_000_000_000))
            }
        }
    }

    private func stopApprovalPolling() {
        approvalPollTask?.cancel()
        approvalPollTask = nil
    }

    private func clearAllApproval() {
        stopApprovalPolling()
        approvalExpiryTask?.cancel()
        approvalExpiryTask = nil
        allPendingApproval = []
        allAnnouncedApproval = []
        // A listing still running started before the clear and must not bring the requests back.
        shownApprovalList = startedApprovalListCount
        presentedApproval = nil
        isAnsweringApproval = false
        isApprovalPasswordRequired = false
        approvalErrorMessage = nil
    }

    // isRemembered is the sheet's switch; it counts only for a request that offers remembering.
    func approvePendingApproval(isRemembered: Bool) async {
        guard let approval = presentedApproval else { return }
        await approve(approval, isRemembered: isRemembered)
    }

    func approvePendingApproval(password: String, isRemembered: Bool) async {
        guard let approval = presentedApproval else { return }
        let isApprovalRemembered = approval.isRememberable && isRemembered
        await answerApproval(approval, successMessage: "Approved on your Mac") { backend, requestID in
            try backend.approveApprovalWithPassword(requestID: requestID, password: password, isRemembered: isApprovalRemembered)
        }
    }

    func denyPendingApproval() async {
        guard let approval = presentedApproval else { return }
        await deny(approval)
    }

    // Approve asks for Face ID; when the master password is needed instead, the request's sheet opens and asks for it.
    // The Vaults card has no remember switch, so it approves as the switch starts: remembered when the Mac's checkbox would be.
    func approve(_ approval: PhoneApproval, isRemembered: Bool? = nil) async {
        let isApprovalRemembered = approval.resolveRememberChoice(isRemembered)
        await answerApproval(approval, successMessage: "Approved on your Mac") { backend, requestID in
            try backend.approveApproval(requestID: requestID, isRemembered: isApprovalRemembered)
        }
    }

    func deny(_ approval: PhoneApproval) async {
        await answerApproval(approval, successMessage: "Denied on your Mac") { backend, requestID in
            try backend.denyApproval(requestID: requestID)
        }
    }

    private func answerApproval(_ approval: PhoneApproval, successMessage: String, _ operation: @escaping (VaultBackend, String) throws -> Void) async {
        guard let backend, !isAnsweringApproval else { return }
        isAnsweringApproval = true
        if presentedApproval?.requestID == approval.requestID {
            approvalErrorMessage = nil
        }
        defer { isAnsweringApproval = false }
        do {
            try await runInBackground { try operation(backend, approval.requestID) }
            removeApproval(requestID: approval.requestID)
            showNotice(successMessage)
        } catch {
            let message = describeError(error)
            switch Self.classifyApprovalError(message) {
            case .gone:
                removeApproval(requestID: approval.requestID)
                showNotice("The request is no longer waiting")
            case .needsPassword:
                presentApproval(approval)
                isApprovalPasswordRequired = true
            case .cancelled:
                break
            case .other:
                presentApproval(approval)
                approvalErrorMessage = message
            }
        }
    }

    enum ApprovalErrorKind {
        case gone
        case needsPassword
        case cancelled
        case other
    }

    static func classifyApprovalError(_ message: String) -> ApprovalErrorKind {
        let lowercased = message.lowercased()
        if lowercased.contains("no longer waiting") {
            return .gone
        }
        if lowercased.contains("was cancelled") {
            return .cancelled
        }
        if lowercased.contains("master password") && !lowercased.contains("incorrect") {
            return .needsPassword
        }
        return .other
    }
}
