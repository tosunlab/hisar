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
        let passkeyVerifier = DeviceOwnerPasskeyVerifier()
        hisar.setPasskeyVerifier(passkeyVerifier)
        hisar.setAppVersion(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "")
        self.hisar = hisar
        self.biometryName = biometric?.getBiometryName() ?? "Face ID"
        allRetainedAdapter = [secretKeyStore, passkeyVerifier, biometric as AnyObject?, clipboard as AnyObject?, eventListener as AnyObject?].compactMap { $0 }
    }

    private static func getDeviceName() -> String {
        let model = ProcessInfo.processInfo.isiOSAppOnMac ? "Mac" : "iPhone"
        return "Hisar on \(model)"
    }

    static func convertError(_ error: Error, biometryName: String) -> BackendError {
        var message = (error as NSError).localizedDescription
        let prefix = BackendError.passkeyVerificationFailed + ": "
        let code = message.hasPrefix(prefix) ? BackendError.passkeyVerificationFailed : nil
        if code != nil { message = String(message.dropFirst(prefix.count)) }
        message = message.replacingOccurrences(of: "Touch ID", with: biometryName)
        if let first = message.first {
            message = first.uppercased() + message.dropFirst()
        }
        return BackendError(message: message, code: code)
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

    func checkMinimumVersion() -> VaultStatus {
        (try? decode(VaultStatus.self, from: hisar.checkMinimumVersion())) ?? getStatus()
    }

    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.sign(in: server, email: email, secretKey: secretKey, password: password, error: $0) })
    }

    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult {
        try decode(SignUpResult.self, from: callReturningText { hisar.signUp(inviteLink, email: email, name: name, password: password, error: $0) })
    }

    func signUpWithoutInvite(server: String, email: String, name: String, password: String) throws -> SignUpResult {
        try decode(SignUpResult.self, from: callReturningText { hisar.signUpWithoutInvite(server, email: email, name: name, password: password, error: $0) })
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

    func enableNewAccountBiometricUnlock() throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.enableNewAccountBiometricUnlock($0) })
    }

    func exportNewAccountEmergencyKit(directory: URL) throws -> EmergencyKitExport {
        try decode(EmergencyKitExport.self, from: callReturningText { hisar.exportNewAccountEmergencyKit(directory.path, error: $0) })
    }

    func acknowledgeNewAccountRecovery() throws {
        try callReturningNothing { try hisar.acknowledgeNewAccountRecovery() }
    }

    func forgetNewAccountSecretKey() {
        hisar.forgetNewAccountSecretKey()
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

    func createVault(name: String) throws -> VaultSummary {
        try decode(VaultSummary.self, from: callReturningText { hisar.createVault(name, error: $0) })
    }

    func listAllPerson() throws -> [Member] {
        try decode([Member]?.self, from: callReturningText { hisar.listAllPerson($0) }) ?? []
    }

    func invitePerson(email: String) throws {
        try callReturningNothing { try hisar.invitePerson(email) }
    }

    func listAllShareInvitation() throws -> ShareInvitations {
        try decode(ShareInvitations.self, from: callReturningText { hisar.listAllShareInvitation($0) })
    }

    func acceptShareInvitation(invitationID: String) throws {
        try callReturningNothing { try hisar.acceptShareInvitation(invitationID) }
    }

    func declineShareInvitation(invitationID: String) throws {
        try callReturningNothing { try hisar.declineShareInvitation(invitationID) }
    }

    func cancelShareInvitation(invitationID: String) throws {
        try callReturningNothing { try hisar.cancelShareInvitation(invitationID) }
    }

    func confirmShareRecipient(invitationID: String, reviewToken: String) throws {
        try callReturningNothing { try hisar.confirmShareRecipient(invitationID, reviewToken: reviewToken) }
    }

    func listAllShareAwaitingUnlock() throws -> [ShareAwaitingUnlock] {
        try decode([ShareAwaitingUnlock]?.self, from: callReturningText { hisar.listAllShareAwaitingUnlock($0) }) ?? []
    }

    func listAllVaultMember(vaultID: String) throws -> [Member] {
        try decode([Member]?.self, from: callReturningText { hisar.listAllVaultMember(vaultID, error: $0) }) ?? []
    }

    func getVaultSharing(vaultID: String) throws -> VaultSharing {
        try decode(VaultSharing.self, from: callReturningText { hisar.getVaultSharing(vaultID, error: $0) })
    }

    func shareVault(vaultID: String, email: String, role: String) throws {
        try callReturningNothing { try hisar.shareVault(vaultID, email: email, role: role) }
    }

    func changeVaultMemberRole(vaultID: String, accountID: String, role: String) throws {
        try callReturningNothing { try hisar.changeVaultMemberRole(vaultID, accountID: accountID, role: role) }
    }

    func removeVaultMember(vaultID: String, accountID: String) throws {
        try callReturningNothing { try hisar.removeVaultMember(vaultID, accountID: accountID) }
    }

    func getVaultActions(vaultID: String) throws -> VaultActions {
        try decode(VaultActions.self, from: callReturningText { hisar.getVaultActions(vaultID, error: $0) })
    }

    func renameVault(vaultID: String, name: String) throws -> VaultSummary {
        try decode(VaultSummary.self, from: callReturningText { hisar.renameVault(vaultID, name: name, error: $0) })
    }

    func deleteVault(vaultID: String, request: VaultDeletionRequest) throws -> VaultDeletionResult {
        let requestJSON = String(decoding: try JSONEncoder().encode(request), as: UTF8.self)
        return try decode(VaultDeletionResult.self, from: callReturningText { hisar.deleteVault(vaultID, requestJSON: requestJSON, error: $0) })
    }

    func leaveVault(vaultID: String) throws {
        try callReturningNothing { try hisar.leaveVault(vaultID) }
    }

    func listAllRemovedVault() throws -> [RemovedVaultNotice] {
        try decode([RemovedVaultNotice]?.self, from: callReturningText { hisar.listAllRemovedVault($0) }) ?? []
    }

    func listAllOrganization() throws -> Organizations {
        try decode(Organizations.self, from: callReturningText { hisar.listAllOrganization($0) })
    }

    func getOrganization(organizationID: String) throws -> Organization {
        try decode(Organization.self, from: callReturningText { hisar.getOrganization(organizationID, error: $0) })
    }

    func createOrganization(name: String) throws -> OrganizationSummary {
        try decode(OrganizationSummary.self, from: callReturningText { hisar.createOrganization(name, error: $0) })
    }

    func renameOrganization(organizationID: String, name: String) throws {
        try callReturningNothing { try hisar.renameOrganization(organizationID, name: name) }
    }

    func deleteOrganization(organizationID: String) throws {
        try callReturningNothing { try hisar.deleteOrganization(organizationID) }
    }

    func inviteToOrganization(organizationID: String, email: String, role: String) throws {
        try callReturningNothing { try hisar.invite(toOrganization: organizationID, email: email, role: role) }
    }

    func cancelOrganizationInvitation(organizationID: String, invitationID: String) throws {
        try callReturningNothing { try hisar.cancelOrganizationInvitation(organizationID, invitationID: invitationID) }
    }

    func acceptOrganizationInvitation(invitationID: String) throws {
        try callReturningNothing { try hisar.acceptOrganizationInvitation(invitationID) }
    }

    func declineOrganizationInvitation(invitationID: String) throws {
        try callReturningNothing { try hisar.declineOrganizationInvitation(invitationID) }
    }

    func setOrganizationMemberRole(organizationID: String, accountID: String, role: String) throws {
        try callReturningNothing { try hisar.setOrganizationMemberRole(organizationID, accountID: accountID, role: role) }
    }

    func removeOrganizationMember(organizationID: String, accountID: String) throws {
        try callReturningNothing { try hisar.removeOrganizationMember(organizationID, accountID: accountID) }
    }

    func leaveOrganization(organizationID: String) throws {
        try callReturningNothing { try hisar.leaveOrganization(organizationID) }
    }

    func createTeam(organizationID: String, name: String) throws -> Team {
        try decode(Team.self, from: callReturningText { hisar.createTeam(organizationID, name: name, error: $0) })
    }

    func renameTeam(organizationID: String, teamID: String, name: String) throws {
        try callReturningNothing { try hisar.renameTeam(organizationID, teamID: teamID, name: name) }
    }

    func deleteTeam(organizationID: String, teamID: String) throws {
        try callReturningNothing { try hisar.deleteTeam(organizationID, teamID: teamID) }
    }

    func addTeamMember(organizationID: String, teamID: String, accountID: String) throws {
        try callReturningNothing { try hisar.addTeamMember(organizationID, teamID: teamID, accountID: accountID) }
    }

    func removeTeamMember(organizationID: String, teamID: String, accountID: String) throws {
        try callReturningNothing { try hisar.removeTeamMember(organizationID, teamID: teamID, accountID: accountID) }
    }

    func giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String) throws {
        try callReturningNothing { try hisar.giveVault(toTeam: organizationID, teamID: teamID, vaultID: vaultID, role: role) }
    }

    func takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String) throws {
        try callReturningNothing { try hisar.takeVault(fromTeam: organizationID, teamID: teamID, vaultID: vaultID) }
    }

    func createOrganizationVault(organizationID: String, teamID: String, name: String) throws -> VaultSummary {
        try decode(VaultSummary.self, from: callReturningText { hisar.createOrganizationVault(organizationID, teamID: teamID, name: name, error: $0) })
    }

    func listAllWaitingVaultKey() throws -> WaitingVaultKeys {
        try decode(WaitingVaultKeys.self, from: callReturningText { hisar.listAllWaitingVaultKey($0) })
    }

    func getExportOptions() throws -> ExportOptions {
        try decode(ExportOptions.self, from: callReturningText { hisar.getExportOptions($0) })
    }

    func exportAllData(format: ExportFormat, passphrase: String, password: String, directory: URL) throws -> ExportResult {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        return try decode(ExportResult.self, from: callReturningText {
            hisar.exportAllData(format.rawValue, passphrase: passphrase, password: password, directory: directory.path, error: $0)
        })
    }

    func prepareCredentialExchangeImport(vaultID: String, credentialData: Data) throws -> ImportPreview {
        try decode(ImportPreview.self, from: callReturningText { hisar.prepareCredentialExchangeImport(vaultID, credentialData: credentialData, error: $0) })
    }

    func prepareHisarExportImport(vaultID: String, fileName: String, content: Data) throws -> ImportPreview {
        // Swift's ObjC importer drops the "Hisar" that matches the HisarkitHisar type, so the selector imports as prepareExportImport.
        try decode(ImportPreview.self, from: callReturningText { hisar.prepareExportImport(vaultID, fileName: fileName, content: content, error: $0) })
    }

    func openImportFile(passphrase: String) throws -> ImportPreview {
        try decode(ImportPreview.self, from: callReturningText { hisar.openImportFile(passphrase, error: $0) })
    }

    func previewImport(vaultID: String) throws -> ImportPreview {
        try decode(ImportPreview.self, from: callReturningText { hisar.previewImport(vaultID, error: $0) })
    }

    func mapImportGroup(groupID: String, vaultID: String) throws -> ImportPreview {
        try decode(ImportPreview.self, from: callReturningText { hisar.mapImportGroup(groupID, vaultID: vaultID, error: $0) })
    }

    func startImport(vaultID: String, isSkippingDuplicate: Bool) throws -> ImportProgress {
        try decode(ImportProgress.self, from: callReturningText { hisar.startImport(vaultID, isSkippingDuplicate: isSkippingDuplicate, error: $0) })
    }

    func continueImport() throws -> ImportProgress {
        try decode(ImportProgress.self, from: callReturningText { hisar.continueImport($0) })
    }

    func cancelImport() {
        hisar.cancelImport()
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

    func deleteUnreadableItem(vaultID: String, itemID: String, revision: String) throws {
        try callReturningNothing { try hisar.deleteUnreadableItem(vaultID, itemID: itemID, revision: revision) }
    }

    func deleteUnreadableVault(vaultID: String, revision: String) throws {
        try callReturningNothing { try hisar.deleteUnreadableVault(vaultID, revision: revision) }
    }

    func repairVaultName(vaultID: String, revision: String, name: String) throws -> VaultSummary {
        try decode(VaultSummary.self, from: callReturningText { hisar.repairVaultName(vaultID, revision: revision, name: name, error: $0) })
    }

    func listAllFavoriteItem() throws -> [FavoriteItem] {
        try decode([FavoriteItem]?.self, from: callReturningText { hisar.listAllFavoriteItem($0) }) ?? []
    }

    func setItemFavorite(vaultID: String, itemID: String, isFavorite: Bool) throws {
        try callReturningNothing { try hisar.setItemFavorite(vaultID, itemID: itemID, isFavorite: isFavorite) }
    }

    func describeCard(number: String, expiry: String) throws -> CardDescription {
        try decode(CardDescription.self, from: callReturningText { hisar.describeCard(number, expiry: expiry, error: $0) })
    }

    func convertToSecureNote(vaultID: String, itemID: String) throws -> ItemDetail {
        try decode(ItemDetail.self, from: callReturningText { hisar.convert(toSecureNote: vaultID, itemID: itemID, error: $0) })
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

    private struct PasskeyRegistrationPayload: Encodable {
        var userVerification: String
        var relyingPartyId: String
        var userName: String
        var userHandle: String
        var clientDataHash: String
        var excludeCredentials: [String]
        var vaultID: String
        var itemID: String
    }

    private static func encodeAllCredentialID(_ allCredentialID: [Data]) -> String {
        let data = (try? JSONEncoder().encode(allCredentialID.map(Base64URL.encode))) ?? Data("[]".utf8)
        return String(decoding: data, as: UTF8.self)
    }

    func listAllPasskey() throws -> [StoredPasskey] {
        try decode([StoredPasskey]?.self, from: callReturningText { hisar.listAllPasskey($0) }) ?? []
    }

    func listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: [Data]) throws -> [StoredPasskey] {
        let allowedJSON = Self.encodeAllCredentialID(allAllowedCredentialID)
        return try decode([StoredPasskey]?.self, from: callReturningText { hisar.listAllPasskey(forRelyingParty: relyingPartyID, allowedCredentialIDsJSON: allowedJSON, error: $0) }) ?? []
    }

    func preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: [Data]) throws -> PasskeyRegistrationOptions {
        let excludedJSON = Self.encodeAllCredentialID(allExcludedCredentialID)
        return try decode(PasskeyRegistrationOptions.self, from: callReturningText { hisar.preparePasskeyRegistration(relyingPartyID, excludedCredentialIDsJSON: excludedJSON, error: $0) })
    }

    func createPasskey(_ request: PasskeyRegistrationRequest, vaultID: String, itemID: String) throws -> PasskeyRegistrationResult {
        let payload = PasskeyRegistrationPayload(
            userVerification: request.userVerification,
            relyingPartyId: request.relyingPartyID, userName: request.userName, userHandle: Base64URL.encode(request.userHandle),
            clientDataHash: Base64URL.encode(request.clientDataHash), excludeCredentials: request.allExcludedCredentialID.map(Base64URL.encode),
            vaultID: vaultID, itemID: itemID
        )
        let registrationJSON = String(decoding: try JSONEncoder().encode(payload), as: UTF8.self)
        return try decode(PasskeyRegistrationResult.self, from: callReturningText { hisar.createPasskey(registrationJSON, error: $0) })
    }

    func assertPasskey(relyingPartyID: String, credentialID: Data, clientDataHash: Data, userVerification: String) throws -> PasskeyAssertionResult {
        let request = ["relyingPartyId": relyingPartyID, "credentialId": Base64URL.encode(credentialID), "clientDataHash": Base64URL.encode(clientDataHash), "userVerification": userVerification]
        let requestJSON = String(decoding: try JSONEncoder().encode(request), as: UTF8.self)
        return try decode(PasskeyAssertionResult.self, from: callReturningText {
            hisar.assertPasskey(withPolicy: requestJSON, error: $0)
        })
    }

    func cancelPasskeyCeremonies() {
        hisar.cancelPasskeyCeremonies()
    }

    func deletePasskey(vaultID: String, itemID: String, credentialID: String) throws -> ItemDetail {
        try decode(ItemDetail.self, from: callReturningText { hisar.deletePasskey(vaultID, itemID: itemID, credentialID: credentialID, error: $0) })
    }

    func listAllPendingApproval() throws -> [PhoneApproval] {
        try decode([PhoneApproval]?.self, from: callReturningText { hisar.listAllPendingApproval($0) }) ?? []
    }

    func approveApproval(requestID: String, isRemembered: Bool) throws {
        try callReturningNothing { try hisar.approveApproval(requestID, isRemembered: isRemembered) }
    }

    func approveApprovalWithPassword(requestID: String, password: String, isRemembered: Bool) throws {
        try callReturningNothing { try hisar.approveApproval(withPassword: requestID, password: password, isRemembered: isRemembered) }
    }

    func denyApproval(requestID: String) throws {
        try callReturningNothing { try hisar.denyApproval(requestID) }
    }

    func reconnectEvents() {
        hisar.reconnectEvents()
    }

    func setAutoLock(seconds: Int) throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.setAutoLock(seconds, error: $0) })
    }

    func listAllDevice() throws -> [Device] {
        try decode([Device]?.self, from: callReturningText { hisar.listAllDevice($0) }) ?? []
    }

    func revokeDevice(deviceID: String) throws {
        try callReturningNothing { try hisar.revokeDevice(deviceID) }
    }

    func exportEmergencyKit(password: String, directory: URL) throws -> EmergencyKitExport {
        try decode(EmergencyKitExport.self, from: callReturningText { hisar.exportEmergencyKit(password, directory: directory.path, error: $0) })
    }

    func changeMasterPassword(currentPassword: String, newPassword: String) throws -> CredentialChangeResult {
        try decode(CredentialChangeResult.self, from: callReturningText { hisar.changeMasterPassword(currentPassword, newPassword: newPassword, error: $0) })
    }

    func replaceSecretKey(password: String, directory: URL) throws -> CredentialChangeResult {
        try decode(CredentialChangeResult.self, from: callReturningText { hisar.replaceSecretKey(password, directory: directory.path, error: $0) })
    }

    func previewAccountDeletion() throws -> AccountDeletionPreview {
        try decode(AccountDeletionPreview.self, from: callReturningText { hisar.previewAccountDeletion($0) })
    }

    func deleteAccountWithPlan(password: String, email: String, planVersion: String) throws -> VaultStatus {
        try decode(VaultStatus.self, from: callReturningText { hisar.deleteAccount(withPlan: password, email: email, planVersion: planVersion, error: $0) })
    }

    func listPendingRecovery() throws -> [PendingRecovery] {
        try decode([PendingRecovery]?.self, from: callReturningText { hisar.listPendingRecovery($0) }) ?? []
    }

    func resumePendingRecovery(operationID: String, password: String) throws -> SignUpResult {
        try decode(SignUpResult.self, from: callReturningText { hisar.resumePendingRecovery(operationID, password: password, error: $0) })
    }

    func resumePendingRecoveryWithSecretKey(operationID: String, password: String, secretKey: String, priorSecretKey: String) throws -> SignUpResult {
        try decode(SignUpResult.self, from: callReturningText {
            hisar.resumePendingRecovery(withSecretKey: operationID, password: password, secretKey: secretKey, priorSecretKey: priorSecretKey, error: $0)
        })
    }

    func discardPreparedRecovery(operationID: String, password: String) throws {
        try callReturningNothing { try hisar.discardPreparedRecovery(operationID, password: password) }
    }

    func exportPendingRecoveryEmergencyKit(operationID: String, password: String, directory: URL) throws -> PendingRecoveryExport {
        try decode(PendingRecoveryExport.self, from: callReturningText {
            hisar.exportPendingRecoveryEmergencyKit(operationID, password: password, directory: directory.path, error: $0)
        })
    }

    func acknowledgePendingRecovery(operationID: String, password: String) throws {
        try callReturningNothing { try hisar.acknowledgePendingRecovery(operationID, password: password) }
    }

    func dismissSignedOutReason() -> VaultStatus {
        (try? decode(VaultStatus.self, from: hisar.dismissSignedOutReason())) ?? getStatus()
    }

    func copySecretReference(vaultID: String, itemID: String, fieldName: String) throws -> String {
        try callReturningText { hisar.copySecretReference(vaultID, itemID: itemID, fieldName: fieldName, error: $0) }
    }

    func isMacApprovalAllowed() -> Bool {
        hisar.isMacApprovalAllowed()
    }

    func setMacApprovalAllowed(_ isAllowed: Bool) throws {
        try callReturningNothing { try hisar.setMacApprovalAllowed(isAllowed) }
    }

    func prepareItemIcon(_ image: Data) throws -> Data {
        let encoded = try callReturningText { hisar.prepareItemIcon(image, error: $0) }
        guard let prepared = Data(base64Encoded: encoded) else {
            throw BackendError(message: "Hisar could not read the answer from its backend.")
        }
        return prepared
    }

    func getWebsiteIcon(url: String) throws -> Data? {
        let encoded = try callReturningText { hisar.getWebsiteIcon(url, error: $0) }
        return encoded.isEmpty ? nil : Data(base64Encoded: encoded)
    }

    func isWebsiteIconsEnabled() -> Bool {
        hisar.isWebsiteIconsEnabled()
    }

    func setWebsiteIconsEnabled(_ isEnabled: Bool) throws {
        try callReturningNothing { try hisar.setWebsiteIconsEnabled(isEnabled) }
    }

    func setPushToken(platform: String, token: String) throws {
        try callReturningNothing { try hisar.setPushToken(platform, token: token) }
    }

    func setPushKindEnabled(_ kind: PushKind, isEnabled: Bool) throws {
        try callReturningNothing { try hisar.setPushKindEnabled(kind.rawValue, isEnabled: isEnabled) }
    }

    func getPushSettings() throws -> PushSettings {
        try decode(PushSettings.self, from: callReturningText { hisar.getPushSettings($0) })
    }

    func getTheme() -> VaultTheme {
        (try? decode(VaultTheme.self, from: hisar.getTheme())) ?? .standard
    }

    func setTheme(_ theme: VaultTheme) throws -> VaultTheme {
        try decode(VaultTheme.self, from: callReturningText { hisar.setTheme(theme.appearance.rawValue, color: theme.color.rawValue, error: $0) })
    }
}
