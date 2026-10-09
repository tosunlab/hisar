import Foundation
@testable import Hisar

class FakeBackend: VaultBackend {
    static let password = "correct horse battery"
    static let secretKey = "H1-ABC123-AAAAA-BBBBB-CCCCC-DDDDD-EEEEEE"

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
                ItemDetail(id: "github", revision: "r1", category: "login", title: "GitHub", username: "alice", allURL: ["https://github.com"], password: "github-password", notes: "work account", allField: [ItemField(name: "PIN", value: "1234", concealed: true)]),
                ItemDetail(id: "bank", revision: "r1", category: "login", title: "Bank", username: "alice@example.com", allURL: ["bank.example.com"], password: "bank-password"),
            ],
            "family": [
                ItemDetail(id: "netflix", revision: "r1", category: "login", title: "Netflix", username: "family@example.com", allURL: ["https://www.netflix.com/login"], password: "netflix-password"),
            ],
        ]
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
                allLogin.append(LoginMatch(vaultID: vault.id, itemID: item.id, title: item.title, username: item.username, vaultName: vault.name))
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
        return LoginCredential(username: item.username, password: item.password)
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
