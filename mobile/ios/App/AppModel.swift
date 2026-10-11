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

    static let autoLockInterval: TimeInterval = 5 * 60

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
    @Published private(set) var pendingApproval: PhoneApproval?
    @Published private(set) var isAnsweringApproval = false
    @Published private(set) var isApprovalPasswordRequired = false
    @Published var approvalErrorMessage: String?

    private var backend: VaultBackend?
    private var allPasskey: [StoredPasskey] = []
    private let credentialIdentityStore: CredentialIdentityUpdating
    private let getCurrentDate: () -> Date
    private var enteredBackgroundAt: Date?
    private var noticeTask: Task<Void, Never>?
    private var allPendingApproval: [PhoneApproval] = []
    private var allSkippedApprovalID: Set<String> = []
    private var isInForeground = true
    private var approvalExpiryTask: Task<Void, Never>?

    init(credentialIdentityStore: CredentialIdentityUpdating, getCurrentDate: @escaping () -> Date = Date.init) {
        self.credentialIdentityStore = credentialIdentityStore
        self.getCurrentDate = getCurrentDate
    }

    func start(createBackend: () throws -> VaultBackend) {
        do {
            let backend = try createBackend()
            self.backend = backend
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
            clearAllApproval()
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
    }

    func handleWillEnterForeground() {
        isInForeground = true
        defer {
            enteredBackgroundAt = nil
            Task { await refreshPendingApproval() }
        }
        guard let enteredBackgroundAt else {
            refreshStatus()
            return
        }
        if status.isUnlocked && getCurrentDate().timeIntervalSince(enteredBackgroundAt) >= Self.autoLockInterval {
            lock()
        } else {
            refreshStatus()
        }
    }

    func refreshAll() async {
        guard status.isUnlocked else { return }
        isRefreshing = true
        defer { isRefreshing = false }
        guard let result = await perform(isBusyShown: false, { backend -> ([VaultSummary], [String: [ItemDetail]], [StoredPasskey]) in
            let allVault = try backend.listAllVault()
            var allItemByVaultID: [String: [ItemDetail]] = [:]
            for vault in allVault {
                allItemByVaultID[vault.id] = try backend.listAllItem(vaultID: vault.id)
            }
            return (allVault, allItemByVaultID, (try? backend.listAllPasskey()) ?? [])
        }) else {
            refreshStatus()
            return
        }
        allVault = result.0.sorted { Self.sortVault($0, $1) }
        allItemByVaultID = result.1
        allPasskey = result.2
        refreshStatus()
        await publishAllCredentialIdentity()
        await refreshPendingApproval()
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
        await perform(isBusyShown: false) { try $0.listAllShareInvitation() }
    }

    @discardableResult
    func acceptShareInvitation(invitationID: String) async -> Bool {
        await perform({ try $0.acceptShareInvitation(invitationID: invitationID) }) != nil
    }

    @discardableResult
    func declineShareInvitation(invitationID: String) async -> Bool {
        await perform({ try $0.declineShareInvitation(invitationID: invitationID) }) != nil
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
        showNotice("\(label) copied. It clears in 90 seconds.")
    }

    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) async -> String? {
        await perform(isBusyShown: false) { try $0.generatePassword(length: length, isDigitIncluded: isDigitIncluded, isSymbolIncluded: isSymbolIncluded) }
    }

    // Phone approvals: only the Mac's open requests, only while unlocked and in the foreground.
    func handleApprovalEvent(name: String, requestID: String) {
        switch name {
        case PhoneApproval.eventRequested:
            guard isInForeground else { return }
            Task { await refreshPendingApproval() }
        case PhoneApproval.eventAnswered, PhoneApproval.eventResolved:
            removeApproval(requestID: requestID)
        default:
            break
        }
    }

    func refreshPendingApproval() async {
        guard status.isUnlocked, isInForeground, let backend else { return }
        guard let allApproval = try? await runInBackground({ try backend.listAllPendingApproval() }) else { return }
        guard status.isUnlocked else { return }
        let now = getCurrentDate()
        allPendingApproval = allApproval.filter { !allSkippedApprovalID.contains($0.requestID) && !$0.isExpired(at: now) }
        showNextApproval()
    }

    private func showNextApproval() {
        let nextApproval = allPendingApproval.first
        if nextApproval?.requestID != pendingApproval?.requestID {
            isApprovalPasswordRequired = false
            approvalErrorMessage = nil
        }
        pendingApproval = nextApproval
        approvalExpiryTask?.cancel()
        guard let nextApproval, let expiryDate = nextApproval.expiryDate else { return }
        let delay = max(0, expiryDate.timeIntervalSince(getCurrentDate()))
        approvalExpiryTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
            guard !Task.isCancelled else { return }
            self?.removeApproval(requestID: nextApproval.requestID)
        }
    }

    private func removeApproval(requestID: String) {
        allSkippedApprovalID.insert(requestID)
        allPendingApproval.removeAll { $0.requestID == requestID }
        if pendingApproval?.requestID == requestID {
            isAnsweringApproval = false
        }
        showNextApproval()
    }

    private func clearAllApproval() {
        approvalExpiryTask?.cancel()
        allPendingApproval = []
        pendingApproval = nil
        isAnsweringApproval = false
        isApprovalPasswordRequired = false
        approvalErrorMessage = nil
    }

    // skipPendingApproval hides the request on this iPhone; the Mac keeps asking until it is answered there or expires.
    func skipPendingApproval() {
        guard let approval = pendingApproval else { return }
        removeApproval(requestID: approval.requestID)
    }

    func approvePendingApproval() async {
        await answerPendingApproval(successMessage: "Approved on your Mac") { backend, requestID in
            try backend.approveApproval(requestID: requestID)
        }
    }

    func approvePendingApproval(password: String) async {
        await answerPendingApproval(successMessage: "Approved on your Mac") { backend, requestID in
            try backend.approveApprovalWithPassword(requestID: requestID, password: password)
        }
    }

    func denyPendingApproval() async {
        await answerPendingApproval(successMessage: "Denied on your Mac") { backend, requestID in
            try backend.denyApproval(requestID: requestID)
        }
    }

    private func answerPendingApproval(successMessage: String, _ operation: @escaping (VaultBackend, String) throws -> Void) async {
        guard let backend, let approval = pendingApproval, !isAnsweringApproval else { return }
        isAnsweringApproval = true
        approvalErrorMessage = nil
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
                isApprovalPasswordRequired = true
            case .cancelled:
                break
            case .other:
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
