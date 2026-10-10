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

    private func record(_ name: String) {
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
    }

    private func requireUnlocked() throws {
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
