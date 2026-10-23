import Foundation
import HisarKit
import Security

struct KeychainError: LocalizedError {
    var status: OSStatus
    var operation: String

    var errorDescription: String? {
        let systemMessage = SecCopyErrorMessageString(status, nil) as String? ?? "error \(status)"
        return "The keychain could not \(operation): \(systemMessage)"
    }
}

final class KeychainSecretKeyStore: NSObject, HisarkitSecretKeyStoreProtocol {
    static let defaultService = "com.tryhisar.hisar.secret-key"

    private let service: String
    private let accessGroup: String?

    init(service: String = KeychainSecretKeyStore.defaultService, accessGroup: String?) {
        self.service = service
        self.accessGroup = accessGroup
    }

    private func buildQuery(accountID: String) -> [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: accountID,
        ]
        if let accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
    }

    func loadSecretKey(_ accountID: String?, error: NSErrorPointer) -> String {
        var query = buildQuery(accountID: accountID ?? "")
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data, let secretKey = String(data: data, encoding: .utf8) else {
            error?.pointee = KeychainError(status: status, operation: "read the Secret Key") as NSError
            return ""
        }
        return secretKey
    }

    // Updating in place keeps the stored Secret Key until the new one is written.
    func storeSecretKey(_ accountID: String?, secretKey: String?) throws {
        let query = buildQuery(accountID: accountID ?? "")
        let attributes: [String: Any] = [
            kSecValueData as String: Data((secretKey ?? "").utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        var status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            status = SecItemAdd(query.merging(attributes) { $1 } as CFDictionary, nil)
        }
        guard status == errSecSuccess else {
            throw KeychainError(status: status, operation: "save the Secret Key")
        }
    }

    func deleteSecretKey(_ accountID: String?) throws {
        let status = SecItemDelete(buildQuery(accountID: accountID ?? "") as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError(status: status, operation: "delete the Secret Key")
        }
    }

    func deleteAllSecretKey() {
        var query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service]
        if let accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        SecItemDelete(query as CFDictionary)
    }
}
