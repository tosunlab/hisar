import Foundation

protocol VaultBackend: AnyObject {
    func getStatus() -> VaultStatus
    func checkMinimumVersion() -> VaultStatus
    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus
    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult
    /// signUpWithoutInvite is signUp on a server whose admin opened registration: the server's address instead of an invite.
    func signUpWithoutInvite(server: String, email: String, name: String, password: String) throws -> SignUpResult
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
    func confirmShareRecipient(invitationID: String, reviewToken: String) throws
    func listAllShareAwaitingUnlock() throws -> [ShareAwaitingUnlock]
    func listAllVaultMember(vaultID: String) throws -> [Member]
    func getVaultSharing(vaultID: String) throws -> VaultSharing
    func shareVault(vaultID: String, email: String, role: String) throws
    func changeVaultMemberRole(vaultID: String, accountID: String, role: String) throws
    func removeVaultMember(vaultID: String, accountID: String) throws
    func getVaultActions(vaultID: String) throws -> VaultActions
    func renameVault(vaultID: String, name: String) throws -> VaultSummary
    func deleteVault(vaultID: String, request: VaultDeletionRequest) throws -> VaultDeletionResult
    func leaveVault(vaultID: String) throws
    func listAllRemovedVault() throws -> [RemovedVaultNotice]
    func listAllOrganization() throws -> Organizations
    func getOrganization(organizationID: String) throws -> Organization
    func createOrganization(name: String) throws -> OrganizationSummary
    func renameOrganization(organizationID: String, name: String) throws
    func deleteOrganization(organizationID: String) throws
    func inviteToOrganization(organizationID: String, email: String, role: String) throws
    func cancelOrganizationInvitation(organizationID: String, invitationID: String) throws
    func acceptOrganizationInvitation(invitationID: String) throws
    func declineOrganizationInvitation(invitationID: String) throws
    func setOrganizationMemberRole(organizationID: String, accountID: String, role: String) throws
    func removeOrganizationMember(organizationID: String, accountID: String) throws
    func leaveOrganization(organizationID: String) throws
    func createTeam(organizationID: String, name: String) throws -> Team
    func renameTeam(organizationID: String, teamID: String, name: String) throws
    func deleteTeam(organizationID: String, teamID: String) throws
    func addTeamMember(organizationID: String, teamID: String, accountID: String) throws
    func removeTeamMember(organizationID: String, teamID: String, accountID: String) throws
    func giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String) throws
    func takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String) throws
    func createOrganizationVault(organizationID: String, teamID: String, name: String) throws -> VaultSummary
    func listAllWaitingVaultKey() throws -> WaitingVaultKeys
    func getExportOptions() throws -> ExportOptions
    func exportAllData(format: ExportFormat, passphrase: String, password: String, directory: URL) throws -> ExportResult
    /// prepareCredentialExchangeImport reads the credentials another app exported to Hisar (ASExportedCredentialData as
    /// JSON) and keeps them, in memory only, until the import starts or is cancelled.
    func prepareCredentialExchangeImport(vaultID: String, credentialData: Data) throws -> ImportPreview
    /// previewImport shows the prepared import again for another vault, with that vault's duplicates.
    func previewImport(vaultID: String) throws -> ImportPreview
    /// mapImportGroup sends one source group of the prepared import, such as a Credential Exchange collection, to vaultID;
    /// the preview's vaultID is then "" when the groups go to different vaults, and startImport takes it as it is.
    func mapImportGroup(groupID: String, vaultID: String) throws -> ImportPreview
    func startImport(vaultID: String, isSkippingDuplicate: Bool) throws -> ImportProgress
    /// continueImport saves the next items; an error pauses the import, and calling it again continues.
    func continueImport() throws -> ImportProgress
    func cancelImport()
    func listAllItem(vaultID: String) throws -> [ItemDetail]
    func getItem(vaultID: String, itemID: String) throws -> ItemDetail
    func saveItem(vaultID: String, item: ItemDetail) throws -> ItemDetail
    func deleteItem(vaultID: String, itemID: String) throws
    func listAllFavoriteItem() throws -> [FavoriteItem]
    func setItemFavorite(vaultID: String, itemID: String, isFavorite: Bool) throws
    func convertToSecureNote(vaultID: String, itemID: String) throws -> ItemDetail
    func deleteUnreadableItem(vaultID: String, itemID: String, revision: String) throws
    func deleteUnreadableVault(vaultID: String, revision: String) throws
    func repairVaultName(vaultID: String, revision: String, name: String) throws -> VaultSummary
    func describeCard(number: String, expiry: String) throws -> CardDescription
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
    func assertPasskey(relyingPartyID: String, credentialID: Data, clientDataHash: Data, userVerification: String) throws -> PasskeyAssertionResult
    func cancelPasskeyCeremonies()
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
    func deleteAccountWithPlan(password: String, email: String, planVersion: String) throws -> VaultStatus
    func listPendingRecovery() throws -> [PendingRecovery]
    func resumePendingRecovery(operationID: String, password: String) throws -> SignUpResult
    func resumePendingRecoveryWithSecretKey(operationID: String, password: String, secretKey: String, priorSecretKey: String) throws -> SignUpResult
    func discardPreparedRecovery(operationID: String, password: String) throws
    func exportPendingRecoveryEmergencyKit(operationID: String, password: String, directory: URL) throws -> PendingRecoveryExport
    func acknowledgePendingRecovery(operationID: String, password: String) throws
    func dismissSignedOutReason() -> VaultStatus
    func copySecretReference(vaultID: String, itemID: String, fieldName: String) throws -> String
    func isMacApprovalAllowed() -> Bool
    func setMacApprovalAllowed(_ isAllowed: Bool) throws
    func getTheme() -> VaultTheme
    func setTheme(_ theme: VaultTheme) throws -> VaultTheme
    /// prepareItemIcon turns a photo into the PNG an item's icon keeps.
    func prepareItemIcon(_ image: Data) throws -> Data
    /// getWebsiteIcon returns a login website's icon, or nil; it may ask the website, so it runs in the background.
    func getWebsiteIcon(url: String) throws -> Data?
    func isWebsiteIconsEnabled() -> Bool
    func setWebsiteIconsEnabled(_ isEnabled: Bool) throws
    /// setPushToken registers the token APNs gave this launch ("apns" or "apns-sandbox") for the signed-in account; it
    /// reaches the server and the push relay, so it runs in the background.
    func setPushToken(platform: String, token: String) throws
    func setPushKindEnabled(_ kind: PushKind, isEnabled: Bool) throws
    func getPushSettings() throws -> PushSettings
}

struct BackendError: LocalizedError, Equatable {
    var message: String
    var code: String? = nil
    static let passkeyVerificationFailed = "passkey-verification-failed"
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
