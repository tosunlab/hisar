import Foundation
import HisarKit
import UIKit

final class HisarKitBackend: VaultBackend {
    private let hisar: HisarkitHisar
    private let biometryName: String
    private let allRetainedAdapter: [AnyObject]

    init(stateDirectory: URL, secretKeyStore: KeychainSecretKeyStore, biometric: BiometricKeychain?, clipboard: PasteboardClipboard?, eventListener: LockEventRelay?) throws {
        var error: NSError?
        let deviceName = HisarKitBackend.getDeviceName()
        guard let hisar = HisarkitNew(stateDirectory.path, deviceName, secretKeyStore, biometric, clipboard, eventListener, &error) else {
            throw HisarKitBackend.convertError(error ?? NSError(domain: "hisar", code: 1), biometryName: "Face ID")
        }
        self.hisar = hisar
        self.biometryName = biometric?.getBiometryName() ?? "Face ID"
        allRetainedAdapter = [secretKeyStore, biometric as AnyObject?, clipboard as AnyObject?, eventListener as AnyObject?].compactMap { $0 }
    }

    private static func getDeviceName() -> String {
        let model = ProcessInfo.processInfo.isiOSAppOnMac ? "Mac" : "iPhone"
        return "Hisar on \(model)"
    }

    static func convertError(_ error: Error, biometryName: String) -> BackendError {
        var message = (error as NSError).localizedDescription
        message = message.replacingOccurrences(of: "Touch ID", with: biometryName)
        if let first = message.first {
            message = first.uppercased() + message.dropFirst()
        }
        return BackendError(message: message)
    }

    private func callReturningText(_ operation: (NSErrorPointer) -> String) throws -> String {
        var error: NSError?
        let text = operation(&error)
        if let error {
            throw Self.convertError(error, biometryName: biometryName)
        }
        return text
    }

    private func callReturningNothing(_ operation: () throws -> Void) throws {
        do {
            try operation()
        } catch {
            throw Self.convertError(error, biometryName: biometryName)
        }
    }

    private func decode<Value: Decodable>(_ type: Value.Type, from text: String) throws -> Value {
        do {
            return try JSONDecoder().decode(type, from: Data(text.utf8))
        } catch {
            throw BackendError(message: "Hisar could not read the answer from its backend.")
        }
    }

    func getStatus() -> VaultStatus {
        (try? decode(VaultStatus.self, from: hisar.status())) ?? .signedOut
    }

    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.sign(in: server, email: email, secretKey: secretKey, password: password, error: $0) })
    }

    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult {
        try decode(SignUpResult.self, from: callReturningText { hisar.signUp(inviteLink, email: email, name: name, password: password, error: $0) })
    }

    func unlock(password: String) throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.unlock(password, error: $0) })
    }

    func unlockWithBiometric() throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.unlock(withBiometric: $0) })
    }

    func enableBiometricUnlock(password: String) throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.enableBiometricUnlock(password, error: $0) })
    }

    func disableBiometricUnlock() throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.disableBiometricUnlock($0) })
    }

    func lock() -> VaultStatus {
        (try? decode(VaultStatus.self, from: hisar.lock())) ?? getStatus()
    }

    func signOut() throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.signOut($0) })
    }

    func reportActivity() {
        hisar.reportActivity()
    }

    func listAllVault() throws -> [VaultSummary] {
        try decode([VaultSummary]?.self, from: callReturningText { hisar.listAllVault($0) }) ?? []
    }

    func listAllItem(vaultID: String) throws -> [ItemDetail] {
        try decode([ItemDetail]?.self, from: callReturningText { hisar.listAllItem(vaultID, error: $0) }) ?? []
    }

    func getItem(vaultID: String, itemID: String) throws -> ItemDetail {
        try decode(ItemDetail.self, from: callReturningText { hisar.getItem(vaultID, itemID: itemID, error: $0) })
    }

    func saveItem(vaultID: String, item: ItemDetail) throws -> ItemDetail {
        let itemJSON = String(decoding: try JSONEncoder().encode(item), as: UTF8.self)
        return try decode(ItemDetail.self, from: callReturningText { hisar.saveItem(vaultID, itemJSON: itemJSON, error: $0) })
    }

    func deleteItem(vaultID: String, itemID: String) throws {
        try callReturningNothing { try hisar.deleteItem(vaultID, itemID: itemID) }
    }

    func getOneTimeCode(vaultID: String, itemID: String) throws -> OneTimeCode {
        try decode(OneTimeCode.self, from: callReturningText { hisar.getOneTimeCode(vaultID, itemID: itemID, error: $0) })
    }

    func getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> OneTimeCode {
        try decode(OneTimeCode.self, from: callReturningText { hisar.getOneTimeCode(forService: vaultID, itemID: itemID, serviceIdentifier: serviceIdentifier, error: $0) })
    }

    func copyField(vaultID: String, itemID: String, fieldName: String) throws {
        try callReturningNothing { try hisar.copyField(vaultID, itemID: itemID, fieldName: fieldName) }
    }

    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) throws -> String {
        try callReturningText { hisar.generatePassword(length, isDigitIncluded: isDigitIncluded, isSymbolIncluded: isSymbolIncluded, error: $0) }
    }

    func listAllLoginForService(serviceIdentifier: String) throws -> [LoginMatch] {
        try decode([LoginMatch]?.self, from: callReturningText { hisar.listAllLogin(forService: serviceIdentifier, error: $0) }) ?? []
    }

    func getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> LoginCredential {
        try decode(LoginCredential.self, from: callReturningText { hisar.getLoginForService(vaultID, itemID: itemID, serviceIdentifier: serviceIdentifier, error: $0) })
    }
}
