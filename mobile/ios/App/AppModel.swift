import Foundation
import SwiftUI

@MainActor
final class AppModel: ObservableObject {
    enum Screen: Equatable {
        case starting
        case failed(String)
        case signedOut
        case locked
        case unlocked
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
    @Published var pendingSignInLink: SignInLink?
    @Published private(set) var lockCount = 0
    @Published private(set) var allPendingApproval: [PhoneApproval] = []
    @Published private(set) var presentedApproval: PhoneApproval?
    @Published private(set) var isAnsweringApproval = false
    @Published private(set) var isApprovalPasswordRequired = false
    @Published var approvalErrorMessage: String?
    @Published var selectedTab: Tab = .vaults
    @Published private(set) var copyCount = 0
    @Published private(set) var isMacApprovalAllowed = true
    @Published private(set) var allReceivedInvitation: [ReceivedShareInvitation] = []
    @Published private(set) var remoteChangeCount = 0

    var receivedInvitationCount: Int { allReceivedInvitation.count }

    var autoLockInterval: TimeInterval { TimeInterval(status.autoLockSeconds) }

    private var backend: VaultBackend?
    private var allPasskey: [StoredPasskey] = []
    private let credentialIdentityStore: CredentialIdentityUpdating
    private let getCurrentDate: () -> Date
    private var enteredBackgroundAt: Date?
    private var noticeTask: Task<Void, Never>?
    private var allFinishedApprovalID: Set<String> = []
    private var allSeenApprovalID: Set<String> = []
    private var isInForeground = true
    private var approvalExpiryTask: Task<Void, Never>?
    private var approvalPollTask: Task<Void, Never>?
    private var isRefreshRequestedAgain = false
    static let approvalPollInterval: TimeInterval = 15

    init(credentialIdentityStore: CredentialIdentityUpdating, getCurrentDate: @escaping () -> Date = Date.init) {
        self.credentialIdentityStore = credentialIdentityStore
        self.getCurrentDate = getCurrentDate
    }

    func start(createBackend: () throws -> VaultBackend) {
        do {
            let backend = try createBackend()
            self.backend = backend
            isMacApprovalAllowed = backend.isMacApprovalAllowed()
            applyStatus(backend.getStatus())
            if status.isUnlocked {
                Task { await refreshAll() }
            }
        } catch {
            screen = .failed(describeError(error))
        }
    }

    private func applyStatus(_ newStatus: VaultStatus) {
        let wasUnlocked = status.isUnlocked
        status = newStatus
        switch newStatus.state {
        case VaultStatus.stateUnlocked: screen = .unlocked
        case VaultStatus.stateLocked: screen = .locked
        default: screen = .signedOut
        }
        if wasUnlocked && !newStatus.isUnlocked {
            allVault = []
            allItemByVaultID = [:]
            allPasskey = []
            lockCount += 1
            allReceivedInvitation = []
            selectedTab = .vaults
            clearAllApproval()
        }
        if !wasUnlocked && newStatus.isUnlocked && isInForeground {
            startApprovalPolling()
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
        newSecretKey = result.secretKey
        applyStatus(result.status)
        await refreshAll()
        return true
    }

    func dismissNewSecretKey() {
        newSecretKey = nil
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
        guard let newStatus = await perform({ try $0.signOut() }) else { return false }
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
    }

    func handleWillEnterForeground() {
        isInForeground = true
        defer {
            enteredBackgroundAt = nil
            if status.isUnlocked, let backend {
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

    // Listing the share invitations also completes the shares people accepted, as the Mac's sync does.
    func refreshAll() async {
        guard status.isUnlocked else { return }
        isRefreshing = true
        defer { isRefreshing = false }
        guard let result = await perform(isBusyShown: false, { backend -> ([VaultSummary], [String: [ItemDetail]], [StoredPasskey], ShareInvitations?) in
            let shareInvitations = try? backend.listAllShareInvitation()
            let allVault = try backend.listAllVault()
            var allItemByVaultID: [String: [ItemDetail]] = [:]
            for vault in allVault {
                allItemByVaultID[vault.id] = try backend.listAllItem(vaultID: vault.id)
            }
            return (allVault, allItemByVaultID, (try? backend.listAllPasskey()) ?? [], shareInvitations)
        }) else {
            refreshStatus()
            return
        }
        allVault = result.0.sorted { Self.sortVault($0, $1) }
        allItemByVaultID = result.1
        allPasskey = result.2
        if let shareInvitations = result.3 {
            allReceivedInvitation = shareInvitations.allReceived
        }
        refreshStatus()
        await publishAllCredentialIdentity()
    }

    func handleRemoteChange() {
        guard status.isUnlocked else { return }
        remoteChangeCount += 1
        guard !isRefreshing else {
            isRefreshRequestedAgain = true
            return
        }
        Task {
            repeat {
                isRefreshRequestedAgain = false
                await refreshAll()
            } while isRefreshRequestedAgain && status.isUnlocked
        }
    }

    private func publishAllCredentialIdentity() async {
        await credentialIdentityStore.replaceAllIdentity(VaultLogic.buildAllCredentialIdentity(listAllItemReference(), allPasskey: allPasskey))
    }

    private static func sortVault(_ first: VaultSummary, _ second: VaultSummary) -> Bool {
        let isFirstPersonal = first.type == "personal"
        let isSecondPersonal = second.type == "personal"
        if isFirstPersonal != isSecondPersonal { return isFirstPersonal }
        return first.name.localizedCaseInsensitiveCompare(second.name) == .orderedAscending
    }

    func refreshVault(vaultID: String) async {
        guard let allItem = await perform(isBusyShown: false, { try $0.listAllItem(vaultID: vaultID) }) else {
            refreshStatus()
            return
        }
        allItemByVaultID[vaultID] = allItem
        refreshStatus()
    }

    func findVault(vaultID: String) -> VaultSummary? {
        allVault.first { $0.id == vaultID }
    }

    func listAllItemReference(vaultID: String? = nil) -> [VaultItemReference] {
        let allSelectedVault = allVault.filter { vaultID == nil || $0.id == vaultID }
        let allReference = allSelectedVault.flatMap { vault in
            (allItemByVaultID[vault.id] ?? []).map { VaultItemReference(vaultID: vault.id, vaultName: vault.name, item: $0) }
        }
        return VaultLogic.sortAllItem(allReference)
    }

    func countItem(vaultID: String) -> Int {
        allItemByVaultID[vaultID]?.count ?? findVault(vaultID: vaultID)?.itemCount ?? 0
    }

    func loadItem(vaultID: String, itemID: String) async -> ItemDetail? {
        await perform(isBusyShown: false) { try $0.getItem(vaultID: vaultID, itemID: itemID) }
    }

    func createVault(name: String) async -> VaultSummary? {
        guard let vault = await perform({ try $0.createVault(name: name) }) else { return nil }
        allVault.removeAll { $0.id == vault.id }
        allVault.append(vault)
        allVault.sort { Self.sortVault($0, $1) }
        allItemByVaultID[vault.id] = []
        return vault
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
        allReceivedInvitation = shareInvitations.allReceived
        return shareInvitations
    }

    @discardableResult
    func acceptShareInvitation(invitationID: String) async -> Bool {
        guard await perform({ try $0.acceptShareInvitation(invitationID: invitationID) }) != nil else { return false }
        allReceivedInvitation.removeAll { $0.id == invitationID }
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

    func saveItem(vaultID: String, item: ItemDetail) async -> ItemDetail? {
        guard let savedItem = await perform({ try $0.saveItem(vaultID: vaultID, item: item) }) else { return nil }
        var allItem = allItemByVaultID[vaultID] ?? []
        var summary = savedItem
        summary.password = ""
        summary.notes = ""
        summary.allField = []
        summary.oneTimePassword = ""
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
        try? FileManager.default.removeItem(at: URL(fileURLWithPath: export.emergencyKitPath).deletingLastPathComponent())
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
            presentedApproval = nil
        }
        return true
    }

    func refreshReceivedInvitationCount() async {
        guard let invitations = await perform(isBusyShown: false, { try $0.listAllShareInvitation() }) else { return }
        allReceivedInvitation = invitations.allReceived
    }

    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) async -> String? {
        await perform(isBusyShown: false) { try $0.generatePassword(length: length, isDigitIncluded: isDigitIncluded, isSymbolIncluded: isSymbolIncluded) }
    }

    // Phone approvals: the Mac's open requests, listed while unlocked and in the foreground.
    func handleApprovalEvent(name: String, requestID: String) {
        switch name {
        case PhoneApproval.eventRequested:
            guard isInForeground else { return }
            Task { await refreshPendingApproval(isOpeningNewRequest: true) }
        case PhoneApproval.eventAnswered, PhoneApproval.eventResolved:
            removeApproval(requestID: requestID)
        default:
            break
        }
    }

    // refreshPendingApproval lists the open requests; with isOpeningNewRequest, a request seen for the first time opens the sheet.
    func refreshPendingApproval(isOpeningNewRequest: Bool = false) async {
        guard status.isUnlocked, isInForeground, isMacApprovalAllowed, let backend else { return }
        guard let allApproval = try? await runInBackground({ try backend.listAllPendingApproval() }) else { return }
        guard status.isUnlocked else { return }
        let now = getCurrentDate()
        let allOpenApproval = allApproval.filter { !allFinishedApprovalID.contains($0.requestID) && !$0.isExpired(at: now) }
        let firstNewApproval = allOpenApproval.first { !allSeenApprovalID.contains($0.requestID) }
        allSeenApprovalID.formUnion(allOpenApproval.map(\.requestID))
        allPendingApproval = allOpenApproval
        if isOpeningNewRequest, presentedApproval == nil, let firstNewApproval {
            presentApproval(firstNewApproval)
        }
        closeSheetOfFinishedApproval()
        startApprovalExpiryCheck()
    }

    func reviewApproval(_ approval: PhoneApproval) {
        guard allPendingApproval.contains(where: { $0.requestID == approval.requestID }) else { return }
        presentApproval(approval)
    }

    func closeApprovalSheet() {
        guard !isAnsweringApproval else { return }
        presentedApproval = nil
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
        presentedApproval = nil
        isAnsweringApproval = false
        isApprovalPasswordRequired = false
        approvalErrorMessage = nil
    }

    func approvePendingApproval() async {
        guard let approval = presentedApproval else { return }
        await approve(approval)
    }

    func approvePendingApproval(password: String) async {
        guard let approval = presentedApproval else { return }
        await answerApproval(approval, successMessage: "Approved on your Mac") { backend, requestID in
            try backend.approveApprovalWithPassword(requestID: requestID, password: password)
        }
    }

    func denyPendingApproval() async {
        guard let approval = presentedApproval else { return }
        await deny(approval)
    }

    // Approve asks for Face ID; when the master password is needed instead, the request's sheet opens and asks for it.
    func approve(_ approval: PhoneApproval) async {
        await answerApproval(approval, successMessage: "Approved on your Mac") { backend, requestID in
            try backend.approveApproval(requestID: requestID)
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
