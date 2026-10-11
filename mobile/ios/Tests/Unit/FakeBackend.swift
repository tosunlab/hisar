import Foundation
@testable import Hisar

class FakeBackend: VaultBackend {
    static let password = "correct horse battery"
    static let secretKey = "H1-ABC123-AAAAA-BBBBB-CCCCC-DDDDD-EEEEEE"
    static let oneTimePassword = "otpauth://totp/GitHub:alice?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
    static let oneTimeCode = OneTimeCode(code: "287082", expiresAt: "2027-01-15T08:00:30Z", period: 30)
    static let gitHubPasskey = StoredPasskey(
        credentialID: Base64URL.encode(Data([1, 1, 1])), relyingPartyID: "github.com", userName: "alice", userDisplayName: "Alice",
        userHandle: Base64URL.encode(Data("alice-handle".utf8)), vaultID: "personal", itemID: "github", title: "GitHub"
    )

    private let callLock = NSLock()
    var status = VaultStatus.signedOut
    var allVault: [VaultSummary] = []
    var allItemByVaultID: [String: [ItemDetail]] = [:]
    var isBiometricAvailable = true
    var isBiometricSucceeding = true
    var isOffline = false
    var allCopiedField: [String] = []
    var allCallName: [String] = []
    var nextItemNumber = 1
    var allPasskey: [StoredPasskey] = []
    var allMember: [Member] = []
    var allVaultMemberByVaultID: [String: [Member]] = [:]
    var allInvitedEmail: [String] = []
    var shareInvitations = ShareInvitations()
    var allShareAwaitingUnlock: [ShareAwaitingUnlock] = []
    var isShareAwaitingUnlockFailing = false
    var allAnsweredInvitation: [String] = []
    var allPendingApproval: [PhoneApproval] = []
    var allApprovalAnswer: [String] = []
    var allDevice: [Device] = []
    var isMacApprovalAllowedValue = true
    var theme = VaultTheme.standard
    var isThemeSaveFailing = false
    var allCopiedReference: [String] = []
    var currentPassword = FakeBackend.password
    var currentSecretKey = FakeBackend.secretKey
    var deletionPreview = AccountDeletionPreview(email: "alice@example.com")
    var isAccountDeleted = false
    private let approvalLock = NSLock()

    func withApprovalLock<Value>(_ operation: () throws -> Value) rethrows -> Value {
        approvalLock.lock()
        defer { approvalLock.unlock() }
        return try operation()
    }

    fileprivate func record(_ name: String) {
        callLock.lock()
        allCallName.append(name)
        callLock.unlock()
    }

    func seedSignedIn(state: String = VaultStatus.stateUnlocked) {
        status = VaultStatus(state: state, server: "http://127.0.0.1:18080", email: "alice@example.com", name: "Alice", biometricAvailable: isBiometricAvailable, biometricEnabled: false, biometricReady: false, isOffline: isOffline)
        allVault = [
            VaultSummary(id: "family", name: "Family", type: "shared", role: "editor", itemCount: 1),
            VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2),
        ]
        allItemByVaultID = [
            "personal": [
                ItemDetail(id: "github", revision: "r1", category: "login", title: "GitHub", username: "alice", allURL: ["https://github.com"], password: "github-password", notes: "work account", allField: [ItemField(name: "PIN", value: "1234", concealed: true)], oneTimePassword: Self.oneTimePassword, hasOneTimePassword: true, allPasskey: [ItemPasskey(credentialID: Self.gitHubPasskey.credentialID, relyingPartyID: "github.com", userName: "alice", userDisplayName: "Alice", createdAt: "2026-10-09T12:00:00Z")]),
                ItemDetail(id: "bank", revision: "r1", category: "login", title: "Bank", username: "alice@example.com", allURL: ["bank.example.com"], password: "bank-password"),
            ],
            "family": [
                ItemDetail(id: "netflix", revision: "r1", category: "login", title: "Netflix", username: "family@example.com", allURL: ["https://www.netflix.com/login"], password: "netflix-password"),
            ],
        ]
        allPasskey = [Self.gitHubPasskey]
        allMember = [
            Member(accountID: "alice-id", name: "Alice", email: "alice@example.com", role: Member.roleAdmin, securityCode: "1111 2222", isCurrentUser: true),
            Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: Member.roleMember, securityCode: "3333 4444"),
            Member(accountID: "carol-id", name: "Carol", email: "carol@example.com", role: Member.roleMember, securityCode: "5555 6666"),
        ]
        allVaultMemberByVaultID = [
            "family": [
                Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: Member.vaultRoleOwner, securityCode: "3333 4444"),
                Member(accountID: "alice-id", name: "Alice", email: "alice@example.com", role: Member.vaultRoleEditor, securityCode: "1111 2222", isCurrentUser: true),
            ],
        ]
    }

    private var currentMember: Member? { allMember.first { $0.isCurrentUser } }

    func createVault(name: String) throws -> VaultSummary {
        record("createVault")
        try requireUnlocked()
        let name = name.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { throw BackendError(message: "Enter a vault name") }
        let vault = VaultSummary(id: "vault-\(allVault.count + 1)", name: name, type: "shared", role: Member.vaultRoleOwner, itemCount: 0)
        allVault.append(vault)
        allItemByVaultID[vault.id] = []
        if let currentMember {
            allVaultMemberByVaultID[vault.id] = [Member(accountID: currentMember.accountID, name: currentMember.name, email: currentMember.email, role: Member.vaultRoleOwner, securityCode: currentMember.securityCode, isCurrentUser: true)]
        }
        return vault
    }

    func listAllPerson() throws -> [Member] {
        record("listAllPerson")
        try requireUnlocked()
        let allSharedAccountID = Set(allVaultMemberByVaultID.values.filter { $0.contains { $0.isCurrentUser } }.flatMap { $0.map(\.accountID) })
        return allMember.filter { $0.isCurrentUser || allSharedAccountID.contains($0.accountID) }
    }

    func invitePerson(email: String) throws {
        record("invitePerson")
        try requireUnlocked()
        guard email.contains("@") else { throw BackendError(message: "Enter the email address of the person") }
        allInvitedEmail.append(email)
    }

    func listAllShareInvitation() throws -> ShareInvitations {
        record("listAllShareInvitation")
        try requireUnlocked()
        return shareInvitations
    }

    func listAllShareAwaitingUnlock() throws -> [ShareAwaitingUnlock] {
        record("listAllShareAwaitingUnlock")
        if isShareAwaitingUnlockFailing { throw BackendError(message: "cannot reach the Hisar server") }
        return status.state == VaultStatus.stateLocked ? allShareAwaitingUnlock : []
    }

    func acceptShareInvitation(invitationID: String) throws {
        record("acceptShareInvitation")
        try answerReceivedInvitation(invitationID: invitationID, answer: "accept")
    }

    func declineShareInvitation(invitationID: String) throws {
        record("declineShareInvitation")
        try answerReceivedInvitation(invitationID: invitationID, answer: "decline")
    }

    private func answerReceivedInvitation(invitationID: String, answer: String) throws {
        try requireUnlocked()
        guard shareInvitations.allReceived.contains(where: { $0.id == invitationID }) else { throw BackendError(message: "This invitation no longer exists") }
        shareInvitations.allReceived.removeAll { $0.id == invitationID }
        allAnsweredInvitation.append("\(invitationID) \(answer)")
    }

    func cancelShareInvitation(invitationID: String) throws {
        record("cancelShareInvitation")
        try requireUnlocked()
        guard shareInvitations.allSent.contains(where: { $0.id == invitationID }) else { throw BackendError(message: "This invitation no longer exists") }
        shareInvitations.allSent.removeAll { $0.id == invitationID }
    }

    func listAllVaultMember(vaultID: String) throws -> [Member] {
        record("listAllVaultMember")
        try requireUnlocked()
        guard allVault.contains(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        return allVaultMemberByVaultID[vaultID] ?? []
    }

    func getVaultSharing(vaultID: String) throws -> VaultSharing {
        record("getVaultSharing")
        try requireUnlocked()
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        guard vault.type == "shared" else { throw BackendError(message: "Personal vaults are private and cannot be shared; create a shared vault instead") }
        let isOwner = vault.role == Member.vaultRoleOwner
        return VaultSharing(
            vault: vault,
            isOwner: isOwner,
            allMember: allVaultMemberByVaultID[vaultID] ?? [],
            allInvitation: isOwner ? shareInvitations.allSent.filter { $0.vaultID == vaultID } : [],
            completionError: isOwner ? shareInvitations.completionError : ""
        )
    }

    private func findSharableVault(vaultID: String) throws -> VaultSummary {
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        guard vault.type == "shared" else { throw BackendError(message: "Personal vaults are private and cannot be shared; create a shared vault instead") }
        guard vault.role == Member.vaultRoleOwner else { throw BackendError(message: "Only owners of this vault can share it, change roles or remove members") }
        return vault
    }

    func shareVault(vaultID: String, email: String, role: String) throws {
        record("shareVault")
        try requireUnlocked()
        guard Member.allVaultRole.contains(role) else { throw BackendError(message: "Choose owner, editor or reader") }
        let vault = try findSharableVault(vaultID: vaultID)
        guard email.contains("@") else { throw BackendError(message: "Enter the email address of the person") }
        guard email != currentMember?.email else { throw BackendError(message: "That is your own email; you already have access") }
        guard !(allVaultMemberByVaultID[vault.id] ?? []).contains(where: { $0.email == email }) else { throw BackendError(message: "This person already has access to the vault") }
        shareInvitations.allSent.removeAll { $0.vaultID == vault.id && $0.email == email }
        shareInvitations.allSent.append(SentShareInvitation(id: "invitation-\(shareInvitations.allSent.count + 1)", vaultID: vault.id, vaultName: vault.name, email: email, role: role, state: "pending", expiresAt: "2027-01-22T08:00:00Z"))
    }

    func changeVaultMemberRole(vaultID: String, accountID: String, role: String) throws {
        record("changeVaultMemberRole")
        try requireUnlocked()
        guard Member.allVaultRole.contains(role) else { throw BackendError(message: "Choose owner, editor or reader") }
        let vault = try findSharableVault(vaultID: vaultID)
        guard let index = allVaultMemberByVaultID[vault.id]?.firstIndex(where: { $0.accountID == accountID }) else {
            throw BackendError(message: "This person no longer has access to the vault; share it with them again")
        }
        allVaultMemberByVaultID[vault.id]?[index].role = role
    }

    func removeVaultMember(vaultID: String, accountID: String) throws {
        record("removeVaultMember")
        try requireUnlocked()
        guard accountID != currentMember?.accountID else { throw BackendError(message: "You cannot remove yourself from a vault you own") }
        let vault = try findSharableVault(vaultID: vaultID)
        allVaultMemberByVaultID[vault.id]?.removeAll { $0.accountID == accountID }
    }

    fileprivate func requireUnlocked() throws {
        guard status.isUnlocked else {
            throw BackendError(message: status.isSignedIn ? "Hisar is locked" : "Sign in first")
        }
    }

    func getStatus() -> VaultStatus {
        record("getStatus")
        return status
    }

    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus {
        record("signIn")
        guard password == Self.password, secretKey == Self.secretKey else {
            throw BackendError(message: "Incorrect email, master password or Secret Key")
        }
        seedSignedIn()
        status.server = server
        status.email = email
        return status
    }

    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult {
        record("signUp")
        guard inviteLink.contains("/invite/") else { throw BackendError(message: "Invalid invite link") }
        seedSignedIn()
        allItemByVaultID = ["personal": [], "family": []]
        status.email = email
        status.name = name
        return SignUpResult(status: status, secretKey: Self.secretKey)
    }

    func unlock(password: String) throws -> VaultStatus {
        record("unlock")
        guard password == Self.password else { throw BackendError(message: "Incorrect email, master password or Secret Key") }
        status.state = VaultStatus.stateUnlocked
        if status.biometricEnabled { status.biometricReady = true }
        return status
    }

    func unlockWithBiometric() throws -> VaultStatus {
        record("unlockWithBiometric")
        guard status.biometricReady else { throw BackendError(message: "Unlock with your master password first") }
        guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.") }
        status.state = VaultStatus.stateUnlocked
        return status
    }

    func enableBiometricUnlock(password: String) throws -> VaultStatus {
        record("enableBiometricUnlock")
        try requireUnlocked()
        guard password == Self.password else { throw BackendError(message: "Incorrect email, master password or Secret Key") }
        status.biometricEnabled = true
        status.biometricReady = true
        return status
    }

    func enableNewAccountBiometricUnlock() throws -> VaultStatus {
        record("enableNewAccountBiometricUnlock")
        try requireUnlocked()
        guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.") }
        status.biometricEnabled = true
        status.biometricReady = true
        return status
    }

    func forgetNewAccountSecretKey() {
        record("forgetNewAccountSecretKey")
    }

    func disableBiometricUnlock() throws -> VaultStatus {
        record("disableBiometricUnlock")
        status.biometricEnabled = false
        status.biometricReady = false
        return status
    }

    func lock() -> VaultStatus {
        record("lock")
        if status.isSignedIn { status.state = VaultStatus.stateLocked }
        return status
    }

    func signOut() throws -> VaultStatus {
        record("signOut")
        status = .signedOut
        return status
    }

    func reportActivity() {
        record("reportActivity")
    }

    func listAllVault() throws -> [VaultSummary] {
        record("listAllVault")
        try requireUnlocked()
        return allVault
    }

    func listAllItem(vaultID: String) throws -> [ItemDetail] {
        record("listAllItem")
        try requireUnlocked()
        return (allItemByVaultID[vaultID] ?? []).map { item in
            var summary = item
            summary.password = ""
            summary.notes = ""
            summary.allField = []
            summary.oneTimePassword = ""
            return summary
        }
    }

    func getItem(vaultID: String, itemID: String) throws -> ItemDetail {
        record("getItem")
        try requireUnlocked()
        guard let item = allItemByVaultID[vaultID]?.first(where: { $0.id == itemID }) else { throw BackendError(message: "Item not found") }
        return item
    }

    func saveItem(vaultID: String, item: ItemDetail) throws -> ItemDetail {
        record("saveItem")
        try requireUnlocked()
        guard !item.title.trimmingCharacters(in: .whitespaces).isEmpty else { throw BackendError(message: "Enter a title") }
        var savedItem = item
        let oneTimePassword = item.oneTimePassword.trimmingCharacters(in: .whitespacesAndNewlines)
        guard oneTimePassword.isEmpty || oneTimePassword.hasPrefix("otpauth://totp/") || oneTimePassword.replacingOccurrences(of: " ", with: "").allSatisfy({ "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".contains($0.uppercased()) }) else {
            throw BackendError(message: "The one-time password is not a valid secret or otpauth:// link")
        }
        savedItem.oneTimePassword = oneTimePassword.isEmpty ? "" : (oneTimePassword.hasPrefix("otpauth://") ? oneTimePassword : "otpauth://totp/?secret=\(oneTimePassword.uppercased().replacingOccurrences(of: " ", with: ""))")
        savedItem.hasOneTimePassword = !oneTimePassword.isEmpty
        if savedItem.id.isEmpty {
            savedItem.id = "new-\(nextItemNumber)"
            nextItemNumber += 1
        }
        savedItem.revision = UUID().uuidString
        var allItem = allItemByVaultID[vaultID] ?? []
        if let index = allItem.firstIndex(where: { $0.id == savedItem.id }) {
            allItem[index] = savedItem
        } else {
            allItem.append(savedItem)
        }
        allItemByVaultID[vaultID] = allItem
        return savedItem
    }

    func deleteItem(vaultID: String, itemID: String) throws {
        record("deleteItem")
        try requireUnlocked()
        allItemByVaultID[vaultID]?.removeAll { $0.id == itemID }
        allPasskey.removeAll { $0.vaultID == vaultID && $0.itemID == itemID }
    }

    func getOneTimeCode(vaultID: String, itemID: String) throws -> OneTimeCode {
        record("getOneTimeCode")
        guard try getItem(vaultID: vaultID, itemID: itemID).hasOneTimePassword else { throw BackendError(message: "One-time password not found") }
        return Self.oneTimeCode
    }

    func getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> OneTimeCode {
        record("getOneTimeCodeForService")
        guard try listAllLoginForService(serviceIdentifier: serviceIdentifier).contains(where: { $0.vaultID == vaultID && $0.itemID == itemID }) else {
            throw BackendError(message: "This login is not saved for this site")
        }
        return try getOneTimeCode(vaultID: vaultID, itemID: itemID)
    }

    func copyField(vaultID: String, itemID: String, fieldName: String) throws {
        record("copyField")
        let item = try getItem(vaultID: vaultID, itemID: itemID)
        allCopiedField.append("\(item.id).\(fieldName)")
    }

    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) throws -> String {
        record("generatePassword")
        return String(repeating: isSymbolIncluded ? "!" : (isDigitIncluded ? "1" : "a"), count: length)
    }

    func listAllLoginForService(serviceIdentifier: String) throws -> [LoginMatch] {
        record("listAllLoginForService")
        try requireUnlocked()
        guard let host = VaultLogic.extractHost(serviceIdentifier) else { return [] }
        var allLogin: [LoginMatch] = []
        for vault in allVault {
            for item in allItemByVaultID[vault.id] ?? [] where item.allURL.contains(where: { VaultLogic.extractHost($0).map { host.hasSuffix($0) || $0.hasSuffix(host) } ?? false }) {
                allLogin.append(LoginMatch(vaultID: vault.id, itemID: item.id, title: item.title, username: item.username, vaultName: vault.name, hasOneTimePassword: item.hasOneTimePassword))
            }
        }
        return allLogin
    }

    func getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> LoginCredential {
        record("getLoginForService")
        guard try listAllLoginForService(serviceIdentifier: serviceIdentifier).contains(where: { $0.vaultID == vaultID && $0.itemID == itemID }) else {
            throw BackendError(message: "This login is not saved for this site")
        }
        let item = try getItem(vaultID: vaultID, itemID: itemID)
        return LoginCredential(username: item.username, password: item.password, hasOneTimePassword: item.hasOneTimePassword)
    }
}

extension FakeBackend {
    func listAllPasskey() throws -> [StoredPasskey] {
        record("listAllPasskey")
        try requireUnlocked()
        return allPasskey
    }

    func listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: [Data]) throws -> [StoredPasskey] {
        record("listAllPasskeyForRelyingParty")
        try requireUnlocked()
        let allAllowedID = Set(allAllowedCredentialID.map(Base64URL.encode))
        return allPasskey.filter { $0.relyingPartyID == relyingPartyID && (allAllowedID.isEmpty || allAllowedID.contains($0.credentialID)) }
    }

    func preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: [Data]) throws -> PasskeyRegistrationOptions {
        record("preparePasskeyRegistration")
        let allLogin = try listAllLoginForService(serviceIdentifier: "https://\(relyingPartyID)").filter { login in
            allVault.first { $0.id == login.vaultID }?.role != "reader"
        }
        return PasskeyRegistrationOptions(isExcluded: checkIsExcluded(relyingPartyID: relyingPartyID, allExcludedCredentialID: allExcludedCredentialID), allLogin: allLogin)
    }

    private func checkIsExcluded(relyingPartyID: String, allExcludedCredentialID: [Data]) -> Bool {
        let allExcludedID = Set(allExcludedCredentialID.map(Base64URL.encode))
        return allPasskey.contains { $0.relyingPartyID == relyingPartyID && allExcludedID.contains($0.credentialID) }
    }

    func createPasskey(_ request: PasskeyRegistrationRequest, vaultID: String, itemID: String) throws -> PasskeyRegistrationResult {
        record("createPasskey")
        try requireUnlocked()
        guard !checkIsExcluded(relyingPartyID: request.relyingPartyID, allExcludedCredentialID: request.allExcludedCredentialID) else {
            throw BackendError(message: "Hisar already has a passkey for this account")
        }
        guard request.clientDataHash.count == 32 else { throw BackendError(message: "the request is not valid") }
        var targetVaultID = vaultID
        var item: ItemDetail
        if vaultID.isEmpty {
            targetVaultID = "personal"
            item = try saveItem(vaultID: targetVaultID, item: ItemDetail(title: request.relyingPartyID, username: request.userName, allURL: ["https://\(request.relyingPartyID)"]))
        } else {
            guard try preparePasskeyRegistration(relyingPartyID: request.relyingPartyID, allExcludedCredentialID: []).allLogin.contains(where: { $0.vaultID == vaultID && $0.itemID == itemID }) else {
                throw BackendError(message: "this login is not saved for this site")
            }
            item = try getItem(vaultID: vaultID, itemID: itemID)
        }
        let userHandle = Base64URL.encode(request.userHandle)
        let credentialID = Base64URL.encode(Data([UInt8(allPasskey.count + 2), 7, 7]))
        let replacedID = Set(allPasskey.filter { $0.vaultID == targetVaultID && $0.itemID == item.id && $0.relyingPartyID == request.relyingPartyID && $0.userHandle == userHandle }.map(\.credentialID))
        allPasskey.removeAll { replacedID.contains($0.credentialID) }
        allPasskey.append(StoredPasskey(credentialID: credentialID, relyingPartyID: request.relyingPartyID, userName: request.userName, userHandle: userHandle, vaultID: targetVaultID, itemID: item.id, title: item.title))
        item.allPasskey.removeAll { replacedID.contains($0.credentialID) }
        item.allPasskey.append(ItemPasskey(credentialID: credentialID, relyingPartyID: request.relyingPartyID, userName: request.userName))
        replaceItem(vaultID: targetVaultID, item: item)
        return PasskeyRegistrationResult(
            credentialID: credentialID, attestationObject: Base64URL.encode(Data("attestation for \(request.relyingPartyID)".utf8)),
            authenticatorData: Base64URL.encode(Data("authenticator data".utf8)), vaultID: targetVaultID, itemID: item.id
        )
    }

    private func replaceItem(vaultID: String, item: ItemDetail) {
        guard let index = allItemByVaultID[vaultID]?.firstIndex(where: { $0.id == item.id }) else { return }
        allItemByVaultID[vaultID]?[index] = item
    }

    func assertPasskey(relyingPartyID: String, credentialID: Data, clientDataHash: Data) throws -> PasskeyAssertionResult {
        record("assertPasskey")
        try requireUnlocked()
        guard clientDataHash.count == 32 else { throw BackendError(message: "the request is not valid") }
        guard let passkey = allPasskey.first(where: { $0.relyingPartyID == relyingPartyID && $0.credentialID == Base64URL.encode(credentialID) }) else {
            throw BackendError(message: "passkey not found")
        }
        return PasskeyAssertionResult(
            credentialID: passkey.credentialID, authenticatorData: Base64URL.encode(Data("authenticator data for \(relyingPartyID)".utf8)),
            signature: Base64URL.encode(Data("signature over ".utf8) + clientDataHash), userHandle: passkey.userHandle
        )
    }

    func deletePasskey(vaultID: String, itemID: String, credentialID: String) throws -> ItemDetail {
        record("deletePasskey")
        var item = try getItem(vaultID: vaultID, itemID: itemID)
        guard item.allPasskey.contains(where: { $0.credentialID == credentialID }) else { throw BackendError(message: "passkey not found") }
        item.allPasskey.removeAll { $0.credentialID == credentialID }
        allPasskey.removeAll { $0.credentialID == credentialID }
        replaceItem(vaultID: vaultID, item: item)
        return item
    }
}

// Phone approvals: the Mac's requests are seeded; answers are recorded. Listing is not recorded, it runs in the background.
extension FakeBackend {
    func listAllPendingApproval() throws -> [PhoneApproval] {
        try requireUnlocked()
        return withApprovalLock { allPendingApproval }
    }

    func approveApproval(requestID: String, isRemembered: Bool) throws {
        record("approveApproval")
        try requireUnlocked()
        guard isBiometricAvailable else { throw BackendError(message: "Face ID is not available; approve with your master password") }
        guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.") }
        try answerApproval(requestID: requestID, decision: isRemembered ? "approve remembered" : "approve")
    }

    func approveApprovalWithPassword(requestID: String, password: String, isRemembered: Bool) throws {
        record("approveApprovalWithPassword")
        try requireUnlocked()
        guard password == Self.password else { throw BackendError(message: "Incorrect master password.") }
        try answerApproval(requestID: requestID, decision: isRemembered ? "approve remembered" : "approve")
    }

    func reconnectEvents() {
        record("reconnectEvents")
    }

    func denyApproval(requestID: String) throws {
        record("denyApproval")
        try requireUnlocked()
        try answerApproval(requestID: requestID, decision: "deny")
    }

    private func answerApproval(requestID: String, decision: String) throws {
        try withApprovalLock {
            guard allPendingApproval.contains(where: { $0.requestID == requestID }) else {
                throw BackendError(message: "This request is no longer waiting for an answer")
            }
            allPendingApproval.removeAll { $0.requestID == requestID }
            allApprovalAnswer.append("\(decision) \(requestID)")
        }
    }
}

// Settings: auto-lock, devices, the Emergency Kit, secret references and the Mac approval switch.
extension FakeBackend {
    func setAutoLock(seconds: Int) throws -> VaultStatus {
        record("setAutoLock")
        guard VaultStatus.allAutoLockSeconds.contains(seconds) else {
            throw BackendError(message: "choose 30 seconds, 1 minute, 5 minutes, 10 minutes or 1 hour")
        }
        status.autoLockSeconds = seconds
        return status
    }

    func listAllDevice() throws -> [Device] {
        record("listAllDevice")
        try requireUnlocked()
        return allDevice
    }

    func revokeDevice(deviceID: String) throws {
        record("revokeDevice")
        try requireUnlocked()
        guard let device = allDevice.first(where: { $0.id == deviceID }) else { throw BackendError(message: "this device is already signed out") }
        guard !device.isCurrent else { throw BackendError(message: "sign out this device instead") }
        allDevice.removeAll { $0.id == deviceID }
    }

    func exportEmergencyKit(password: String, directory: URL) throws -> EmergencyKitExport {
        record("exportEmergencyKit")
        try requireUnlocked()
        guard password == Self.password else { throw BackendError(message: "Incorrect master password.") }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent("Hisar Emergency Kit.pdf")
        try Data("%PDF-1.7".utf8).write(to: path)
        return EmergencyKitExport(secretKey: Self.secretKey, emergencyKitPath: path.path)
    }

    func changeMasterPassword(currentPassword: String, newPassword: String) throws -> CredentialChangeResult {
        record("changeMasterPassword")
        try requireUnlocked()
        guard currentPassword == self.currentPassword else { throw BackendError(message: "The master password is not correct") }
        guard newPassword.count >= 10 else { throw BackendError(message: "Use a master password with at least 10 characters") }
        self.currentPassword = newPassword
        return CredentialChangeResult(status: status, signedOutDeviceCount: 1)
    }

    func replaceSecretKey(password: String, directory: URL) throws -> CredentialChangeResult {
        record("replaceSecretKey")
        try requireUnlocked()
        guard password == currentPassword else { throw BackendError(message: "The master password is not correct") }
        currentSecretKey = "H1-ABC123-ZZZZZ-YYYYY-XXXXX-WWWWW-VVVVVV"
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent("Hisar Emergency Kit.pdf")
        try Data("%PDF-1.7".utf8).write(to: path)
        return CredentialChangeResult(status: status, secretKey: currentSecretKey, signedOutDeviceCount: 2, emergencyKitPath: path.path)
    }

    func previewAccountDeletion() throws -> AccountDeletionPreview {
        record("previewAccountDeletion")
        try requireUnlocked()
        return deletionPreview
    }

    func deleteAccount(password: String, email: String) throws -> VaultStatus {
        record("deleteAccount")
        try requireUnlocked()
        guard email.trimmingCharacters(in: .whitespaces).lowercased() == status.email.lowercased() else { throw BackendError(message: "Type the email of this account to confirm") }
        guard password == currentPassword else { throw BackendError(message: "The master password is not correct") }
        isAccountDeleted = true
        status = .signedOut
        return status
    }

    func dismissSignedOutReason() -> VaultStatus {
        record("dismissSignedOutReason")
        status.signedOutReason = ""
        return status
    }

    func copySecretReference(vaultID: String, itemID: String, fieldName: String) throws -> String {
        record("copySecretReference")
        let item = try getItem(vaultID: vaultID, itemID: itemID)
        let vaultName = allVault.first { $0.id == vaultID }?.name ?? vaultID
        let reference = "hisar://\(vaultName)/\(item.title)/\(fieldName)"
        allCopiedReference.append(reference)
        return reference
    }

    func isMacApprovalAllowed() -> Bool {
        isMacApprovalAllowedValue
    }

    func setMacApprovalAllowed(_ isAllowed: Bool) throws {
        record("setMacApprovalAllowed")
        isMacApprovalAllowedValue = isAllowed
    }

    func getTheme() -> VaultTheme {
        theme
    }

    func setTheme(_ theme: VaultTheme) throws -> VaultTheme {
        record("setTheme")
        guard !isThemeSaveFailing else { throw BackendError(message: "Could not save hisar.json") }
        self.theme = theme
        return theme
    }
}

final class FakeCredentialIdentityStore: CredentialIdentityUpdating {
    var allIdentity: [CredentialIdentityRecord] = []
    var replaceCount = 0

    func replaceAllIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        self.allIdentity = allIdentity
        replaceCount += 1
    }

    func removeAllIdentity() async {
        allIdentity = []
    }
}
