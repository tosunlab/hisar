import CryptoKit
import Foundation
import Security

/// ApprovalPushVerifier is the signed-in account and its public signing key: what a phone checks an approval preview
/// against while Hisar is locked. It decodes GetApprovalPushVerifier()'s JSON (both empty before the first unlock) and
/// is kept in the shared keychain so the Notification Service Extension can read it without HisarKit.
struct ApprovalPushVerifier: Codable, Equatable {
    var accountId: String
    var signingPublicKey: String

    /// isPresent is true once an account has unlocked at least once, so a preview can be checked.
    var isPresent: Bool { !accountId.isEmpty && !signingPublicKey.isEmpty }
}

/// ApprovalPushVerifierStore keeps the approval push verifier in the keychain, next to the push payload key, where both
/// the app and the extension can read it. Only the app writes it, from the state HisarKit reports.
struct ApprovalPushVerifierStore {
    static let defaultService = "com.tryhisar.hisar.approval-push-verifier"
    static let account = "default"

    private let service: String
    private let accessGroup: String?

    init(service: String = ApprovalPushVerifierStore.defaultService, accessGroup: String?) {
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

    /// load returns the stored verifier, or nil before the app wrote one or when it cannot be read.
    func load() throws -> ApprovalPushVerifier? {
        var query = buildQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data else {
            throw PushKeychainError(status: status, operation: "read the approval push verifier")
        }
        return try? JSONDecoder().decode(ApprovalPushVerifier.self, from: data)
    }

    /// write stores the verifier when it is present and removes it otherwise, so a sign-out leaves nothing behind.
    func write(_ verifier: ApprovalPushVerifier) throws {
        guard verifier.isPresent else { return try delete() }
        let blob = try JSONEncoder().encode(verifier)
        let query = buildQuery()
        let attributes: [String: Any] = [
            kSecValueData as String: blob,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        var status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            status = SecItemAdd(query.merging(attributes) { $1 } as CFDictionary, nil)
        }
        guard status == errSecSuccess || status == errSecDuplicateItem else {
            throw PushKeychainError(status: status, operation: "save the approval push verifier")
        }
    }

    func delete() throws {
        let status = SecItemDelete(buildQuery() as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw PushKeychainError(status: status, operation: "remove the approval push verifier")
        }
    }
}

/// ApprovalPushPreview is the decrypted preview of an approval request the account signed: the lines a notification's
/// title and body show. Unknown fields are ignored and any field may be missing, matching core/approval.PushPreview.
struct ApprovalPushPreview: Decodable {
    var requestId = ""
    var requesterDeviceName = ""
    var expiresAt: Date?
    var kind = ""
    var appName = ""
    var purpose = ""
    var keyName = ""
    var isForwarded = false
    var forwardedFromHost = ""

    enum CodingKeys: String, CodingKey {
        case requestId, requesterDeviceName, expiresAt, kind, appName, purpose, keyName, isForwarded, forwardedFromHost
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        requestId = try container.decodeIfPresent(String.self, forKey: .requestId) ?? ""
        requesterDeviceName = try container.decodeIfPresent(String.self, forKey: .requesterDeviceName) ?? ""
        if let text = try container.decodeIfPresent(String.self, forKey: .expiresAt) {
            expiresAt = PhoneApproval.parseDate(text)
        }
        kind = try container.decodeIfPresent(String.self, forKey: .kind) ?? ""
        appName = try container.decodeIfPresent(String.self, forKey: .appName) ?? ""
        purpose = try container.decodeIfPresent(String.self, forKey: .purpose) ?? ""
        keyName = try container.decodeIfPresent(String.self, forKey: .keyName) ?? ""
        isForwarded = try container.decodeIfPresent(Bool.self, forKey: .isForwarded) ?? false
        forwardedFromHost = try container.decodeIfPresent(String.self, forKey: .forwardedFromHost) ?? ""
    }

    static func decode(_ data: Data) -> ApprovalPushPreview? {
        try? JSONDecoder().decode(ApprovalPushPreview.self, from: data)
    }

    /// phoneApproval builds the PhoneApproval whose bannerTitle and summary are the notification's title and body; the
    /// rest of its fields stay empty, since a preview carries only what a notification shows.
    var phoneApproval: PhoneApproval {
        PhoneApproval(
            requestID: requestId, requesterDeviceName: requesterDeviceName, expiresAt: "", kind: kind,
            appName: appName, purpose: purpose, keyName: keyName, isForwarded: isForwarded, forwardedFromHost: forwardedFromHost
        )
    }
}

/// ApprovalPushText opens an approval push's decrypted message and, once the account's signature over the preview
/// checks out and the request has not expired, returns the notification's title and body. It mirrors Go's
/// approval.OpenPushPreview, so the same push shows the same text on both phones. nil keeps the alert as delivered.
enum ApprovalPushText {
    static let previewSignatureLabel = "hisar/v1/approval-preview"

    /// Message is the decrypted push JSON; only an "approval" message with a preview and signature is handled here.
    struct Message: Decodable {
        var kind: String?
        var approval: Part?

        struct Part: Decodable {
            var preview: String?
            var signature: String?
        }
    }

    static func titleAndBody(decrypted: Data, verifier: ApprovalPushVerifier, now: Date) -> (title: String, body: String)? {
        guard verifier.isPresent,
              let message = try? JSONDecoder().decode(Message.self, from: decrypted),
              message.kind == "approval",
              let part = message.approval,
              let previewText = part.preview, let signatureText = part.signature,
              let previewBytes = Base64URL.decode(previewText),
              let signature = Base64URL.decode(signatureText),
              let publicKeyData = Base64URL.decode(verifier.signingPublicKey),
              verify(publicKeyData: publicKeyData, accountId: verifier.accountId, preview: previewBytes, signature: signature),
              let preview = ApprovalPushPreview.decode(previewBytes),
              let expiresAt = preview.expiresAt, expiresAt > now,
              !preview.kind.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else { return nil }
        let approval = preview.phoneApproval
        return (approval.bannerTitle, approval.summary)
    }

    /// verify checks the account's Ed25519 signature over the preview, exactly as core/crypt.VerifyApprovalPreview does.
    static func verify(publicKeyData: Data, accountId: String, preview: Data, signature: Data) -> Bool {
        guard let key = try? Curve25519.Signing.PublicKey(rawRepresentation: publicKeyData) else { return false }
        return key.isValidSignature(signature, for: encodeContext(previewSignatureLabel, accountId: accountId, preview: preview))
    }

    /// encodeContext length-prefixes each field after the label's UTF-8 bytes, matching core/crypt.encodeContext.
    static func encodeContext(_ label: String, accountId: String, preview: Data) -> Data {
        var encoded = Data(label.utf8)
        for field in [Data(accountId.utf8), preview] {
            var length = UInt32(field.count).bigEndian
            withUnsafeBytes(of: &length) { encoded.append(contentsOf: $0) }
            encoded.append(field)
        }
        return encoded
    }
}
