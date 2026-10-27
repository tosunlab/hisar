import Foundation
import HisarKit
import LocalAuthentication
import Security

struct BiometricError: LocalizedError {
    var message: String
    var errorDescription: String? { message }
}

final class BiometricKeychain: NSObject, HisarkitBiometricProtocol {
    static let defaultService = "com.tryhisar.hisar.biometric-unlock"

    private let service: String
    private let accessGroup: String?
    private let contextLock = NSLock()
    private var authenticatedContext: LAContext?

    init(service: String = BiometricKeychain.defaultService, accessGroup: String?) {
        self.service = service
        self.accessGroup = accessGroup
    }

    func getBiometryName() -> String {
        let context = LAContext()
        _ = context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
        switch context.biometryType {
        case .touchID: return "Touch ID"
        case .opticID: return "Optic ID"
        default: return "Face ID"
        }
    }

    func isAvailable() -> Bool {
        LAContext().canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
    }

    // LoadKey hands this evaluated context to the keychain, so the user sees one Face ID prompt.
    func authenticate(_ reason: String?) throws {
        let context = LAContext()
        context.localizedFallbackTitle = "Use master password"
        context.localizedCancelTitle = "Cancel"
        let localizedReason = formatReason(reason ?? "")
        let semaphore = DispatchSemaphore(value: 0)
        var evaluationError: Error?
        context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: localizedReason) { isSuccess, error in
            if !isSuccess {
                evaluationError = error ?? BiometricError(message: "\(self.getBiometryName()) did not succeed.")
            }
            semaphore.signal()
        }
        semaphore.wait()
        if let evaluationError {
            throw BiometricError(message: describeEvaluationError(evaluationError))
        }
        contextLock.lock()
        authenticatedContext?.invalidate()
        authenticatedContext = context
        contextLock.unlock()
    }

    private func formatReason(_ reason: String) -> String {
        let text = reason.replacingOccurrences(of: "Touch ID", with: getBiometryName())
        guard let first = text.first else { return "Unlock Hisar" }
        return first.uppercased() + text.dropFirst()
    }

    private func describeEvaluationError(_ error: Error) -> String {
        switch (error as? LAError)?.code {
        case .userCancel, .systemCancel, .appCancel: return "\(BackendError.biometricCancelled): \(getBiometryName()) was cancelled."
        case .userFallback: return "\(BackendError.biometricNeedsPassword): Enter your master password."
        case .biometryLockout: return "\(getBiometryName()) is locked. Unlock with your master password."
        default: return error.localizedDescription
        }
    }

    private func takeAuthenticatedContext() -> LAContext? {
        contextLock.lock()
        defer { contextLock.unlock() }
        let context = authenticatedContext
        authenticatedContext = nil
        return context
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

    func storeKey(_ accountID: String?, key: Data?) throws {
        var accessControlError: Unmanaged<CFError>?
        guard let accessControl = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenUnlockedThisDeviceOnly, .biometryCurrentSet, &accessControlError) else {
            throw BiometricError(message: "Could not protect the key with \(getBiometryName()).")
        }
        let query = buildQuery(accountID: accountID ?? "")
        SecItemDelete(query as CFDictionary)
        var attributes = query
        attributes[kSecValueData as String] = key ?? Data()
        attributes[kSecAttrAccessControl as String] = accessControl
        let status = SecItemAdd(attributes as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw KeychainError(status: status, operation: "save the \(getBiometryName()) key")
        }
    }

    func loadKey(_ accountID: String?) throws -> Data {
        let context = takeAuthenticatedContext() ?? LAContext()
        defer { context.invalidate() }
        var query = buildQuery(accountID: accountID ?? "")
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        query[kSecUseAuthenticationContext as String] = context
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data else {
            throw KeychainError(status: status, operation: "read the \(getBiometryName()) key")
        }
        return data
    }

    func deleteKey(_ accountID: String?) throws {
        let status = SecItemDelete(buildQuery(accountID: accountID ?? "") as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError(status: status, operation: "delete the \(getBiometryName()) key")
        }
    }

    func deleteAllKey() {
        var query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service]
        if let accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        SecItemDelete(query as CFDictionary)
    }
}

final class DeviceOwnerPasskeyVerifier: NSObject, HisarkitPasskeyVerifierProtocol {
    func authenticatePasskey(_ reason: String?) throws {
        let context = LAContext()
        context.touchIDAuthenticationAllowableReuseDuration = 0
        defer { context.invalidate() }
        let semaphore = DispatchSemaphore(value: 0)
        var failure: Error?
        context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason ?? "Verify your passkey") { success, error in
            if !success {
                failure = error ?? NSError(domain: "HisarPasskey", code: 1, userInfo: [NSLocalizedDescriptionKey: "Passkey verification was cancelled."])
            }
            semaphore.signal()
        }
        guard semaphore.wait(timeout: .now() + 120) == .success else {
            throw NSError(domain: "HisarPasskey", code: 2, userInfo: [NSLocalizedDescriptionKey: "Passkey verification expired."])
        }
        if let failure { throw failure }
    }
}
