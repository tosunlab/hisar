import Foundation

protocol VaultBackend: AnyObject {
    func getStatus() -> VaultStatus
    func checkMinimumVersion() -> VaultStatus
    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus
    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult
    func unlock(password: String) throws -> VaultStatus
    func unlockWithBiometric() throws -> VaultStatus
    func enableBiometricUnlock(password: String) throws -> VaultStatus
    func enableNewAccountBiometricUnlock() throws -> VaultStatus
    func forgetNewAccountSecretKey()
    func disableBiometricUnlock() throws -> VaultStatus
    func lock() -> VaultStatus
    func signOut() throws -> VaultStatus
    func reportActivity()
    func listAllVault() throws -> [VaultSummary]
    func createVault(name: String) throws -> VaultSummary
    func listAllPerson() throws -> [Member]
    func invitePerson(email: String) throws
    func listAllShareInvitation() throws -> ShareInvitations
    func acceptShareInvitation(invitationID: String) throws
    func declineShareInvitation(invitationID: String) throws
    func cancelShareInvitation(invitationID: String) throws
    func listAllShareAwaitingUnlock() throws -> [ShareAwaitingUnlock]
    func listAllVaultMember(vaultID: String) throws -> [Member]
    func getVaultSharing(vaultID: String) throws -> VaultSharing
    func shareVault(vaultID: String, email: String, role: String) throws
    func changeVaultMemberRole(vaultID: String, accountID: String, role: String) throws
    func removeVaultMember(vaultID: String, accountID: String) throws
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
    func listAllPendingApproval() throws -> [PhoneApproval]
    func approveApproval(requestID: String, isRemembered: Bool) throws
    func approveApprovalWithPassword(requestID: String, password: String, isRemembered: Bool) throws
    func denyApproval(requestID: String) throws
    func reconnectEvents()
    func setAutoLock(seconds: Int) throws -> VaultStatus
    func listAllDevice() throws -> [Device]
    func revokeDevice(deviceID: String) throws
    func exportEmergencyKit(password: String, directory: URL) throws -> EmergencyKitExport
    func changeMasterPassword(currentPassword: String, newPassword: String) throws -> CredentialChangeResult
    func replaceSecretKey(password: String, directory: URL) throws -> CredentialChangeResult
    func previewAccountDeletion() throws -> AccountDeletionPreview
    func deleteAccount(password: String, email: String) throws -> VaultStatus
    func dismissSignedOutReason() -> VaultStatus
    func copySecretReference(vaultID: String, itemID: String, fieldName: String) throws -> String
    func isMacApprovalAllowed() -> Bool
    func setMacApprovalAllowed(_ isAllowed: Bool) throws
    func getTheme() -> VaultTheme
    func setTheme(_ theme: VaultTheme) throws -> VaultTheme
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
