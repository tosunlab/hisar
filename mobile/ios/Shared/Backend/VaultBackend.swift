import Foundation

protocol VaultBackend: AnyObject {
    func getStatus() -> VaultStatus
    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus
    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult
    func unlock(password: String) throws -> VaultStatus
    func unlockWithBiometric() throws -> VaultStatus
    func enableBiometricUnlock(password: String) throws -> VaultStatus
    func disableBiometricUnlock() throws -> VaultStatus
    func lock() -> VaultStatus
    func signOut() throws -> VaultStatus
    func reportActivity()
    func listAllVault() throws -> [VaultSummary]
    func listAllItem(vaultID: String) throws -> [ItemDetail]
    func getItem(vaultID: String, itemID: String) throws -> ItemDetail
    func saveItem(vaultID: String, item: ItemDetail) throws -> ItemDetail
    func deleteItem(vaultID: String, itemID: String) throws
    func getOneTimeCode(vaultID: String, itemID: String) throws -> OneTimeCode
    func getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> OneTimeCode
    func copyField(vaultID: String, itemID: String, fieldName: String) throws
    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) throws -> String
    func listAllLoginForService(serviceIdentifier: String) throws -> [LoginMatch]
    func getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> LoginCredential
    func listAllPasskey() throws -> [StoredPasskey]
    func listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: [Data]) throws -> [StoredPasskey]
    func preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: [Data]) throws -> PasskeyRegistrationOptions
    func createPasskey(_ request: PasskeyRegistrationRequest, vaultID: String, itemID: String) throws -> PasskeyRegistrationResult
    func assertPasskey(relyingPartyID: String, credentialID: Data, clientDataHash: Data) throws -> PasskeyAssertionResult
    func deletePasskey(vaultID: String, itemID: String, credentialID: String) throws -> ItemDetail
}

struct BackendError: LocalizedError, Equatable {
    var message: String
    var errorDescription: String? { message }
}

private let backgroundQueue = DispatchQueue(label: "com.tryhisar.hisar.backend", qos: .userInitiated, attributes: .concurrent)

// Go does network I/O and Argon2, so it never runs on the main thread.
func runInBackground<Value>(_ operation: @escaping () throws -> Value) async throws -> Value {
    try await withCheckedThrowingContinuation { continuation in
        backgroundQueue.async {
            continuation.resume(with: Result { try operation() })
        }
    }
}

func describeError(_ error: Error) -> String {
    if let backendError = error as? BackendError {
        return backendError.message
    }
    let message = (error as NSError).localizedDescription
    return message.isEmpty ? "Something went wrong." : message
}
