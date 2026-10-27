import CryptoKit
import Foundation
import Security

/// Base64url without padding, the encoding the server and HisarKit use for the push subscription keys and payload.
enum Base64URL {
    static func encode(_ data: Data) -> String {
        data.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    static func decode(_ string: String) -> Data? {
        var text = string.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        let remainder = text.count % 4
        if remainder != 0 { text += String(repeating: "=", count: 4 - remainder) }
        return Data(base64Encoded: text)
    }
}

/// PushKeychainError reports a keychain failure without importing HisarKit, so the extension can use this file.
struct PushKeychainError: LocalizedError {
    var status: OSStatus
    var operation: String
    var errorDescription: String? {
        let systemMessage = SecCopyErrorMessageString(status, nil) as String? ?? "error \(status)"
        return "The keychain could not \(operation): \(systemMessage)"
    }
}

/// The subscription key pair a push payload is encrypted to: a P-256 key-agreement private key and a 16-byte
/// authentication secret, RFC 8291. Generated once and kept in the shared keychain where both the app and the
/// Notification Service Extension can read it, accessible after first unlock so decryption works while locked.
struct PushPayloadKey {
    var privateKey: P256.KeyAgreement.PrivateKey
    var auth: Data

    /// The uncompressed P-256 public point (65 bytes), the server's "p256dh".
    var publicKeyData: Data { privateKey.publicKey.x963Representation }
}

/// PushPayloadKeyStore keeps the push payload key in the keychain, creating it the first time it is asked for.
struct PushPayloadKeyStore {
    static let defaultService = "com.tryhisar.hisar.push-payload-key"
    static let account = "default"
    static let authSize = 16

    private let service: String
    private let accessGroup: String?

    init(service: String = PushPayloadKeyStore.defaultService, accessGroup: String?) {
        self.service = service
        self.accessGroup = accessGroup
    }

    private func buildQuery() -> [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: Self.account,
        ]
        if let accessGroup { query[kSecAttrAccessGroup as String] = accessGroup }
        return query
    }

    /// loadOrCreate returns the stored key, generating and saving one the first time. The stored blob is the 32-byte
    /// private scalar followed by the 16-byte authentication secret.
    func loadOrCreate() throws -> PushPayloadKey {
        if let key = try load() { return key }
        let key = PushPayloadKey(privateKey: P256.KeyAgreement.PrivateKey(), auth: randomAuth())
        try save(key)
        // Reading it back guards against a lost race where another process stored a different key meanwhile.
        return try load() ?? key
    }

    /// load returns the stored key, or nil before the app made one.
    func load() throws -> PushPayloadKey? {
        var query = buildQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data else {
            throw PushKeychainError(status: status, operation: "read the push payload key")
        }
        guard data.count == 32 + Self.authSize else { return nil }
        let privateKey = try P256.KeyAgreement.PrivateKey(rawRepresentation: data.prefix(32))
        return PushPayloadKey(privateKey: privateKey, auth: Data(data.suffix(Self.authSize)))
    }

    private func save(_ key: PushPayloadKey) throws {
        var blob = key.privateKey.rawRepresentation
        blob.append(key.auth)
        let query = buildQuery()
        let attributes: [String: Any] = [
            kSecValueData as String: blob,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        var status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            status = SecItemAdd(query.merging(attributes) { $1 } as CFDictionary, nil)
        }
        // Another process creating the same item first is fine; load() then returns its key.
        guard status == errSecSuccess || status == errSecDuplicateItem else {
            throw PushKeychainError(status: status, operation: "save the push payload key")
        }
    }

    private func randomAuth() -> Data {
        var bytes = [UInt8](repeating: 0, count: Self.authSize)
        _ = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        return Data(bytes)
    }
}
