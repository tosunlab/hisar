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

    func listAllVaultMember(vaultID: String) throws -> [Member] {
        try decode([Member]?.self, from: callReturningText { hisar.listAllVaultMember(vaultID, error: $0) }) ?? []
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

    private struct PasskeyRegistrationPayload: Encodable {
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
            relyingPartyId: request.relyingPartyID, userName: request.userName, userHandle: Base64URL.encode(request.userHandle),
            clientDataHash: Base64URL.encode(request.clientDataHash), excludeCredentials: request.allExcludedCredentialID.map(Base64URL.encode),
            vaultID: vaultID, itemID: itemID
        )
        let registrationJSON = String(decoding: try JSONEncoder().encode(payload), as: UTF8.self)
        return try decode(PasskeyRegistrationResult.self, from: callReturningText { hisar.createPasskey(registrationJSON, error: $0) })
    }

    func assertPasskey(relyingPartyID: String, credentialID: Data, clientDataHash: Data) throws -> PasskeyAssertionResult {
        try decode(PasskeyAssertionResult.self, from: callReturningText {
            hisar.assertPasskey(relyingPartyID, credentialID: Base64URL.encode(credentialID), clientDataHash: Base64URL.encode(clientDataHash), error: $0)
        })
    }

    func deletePasskey(vaultID: String, itemID: String, credentialID: String) throws -> ItemDetail {
        try decode(ItemDetail.self, from: callReturningText { hisar.deletePasskey(vaultID, itemID: itemID, credentialID: credentialID, error: $0) })
    }

    func listAllPendingApproval() throws -> [PhoneApproval] {
        try decode([PhoneApproval]?.self, from: callReturningText { hisar.listAllPendingApproval($0) }) ?? []
    }

    func approveApproval(requestID: String) throws {
        try callReturningNothing { try hisar.approveApproval(requestID) }
    }

    func approveApprovalWithPassword(requestID: String, password: String) throws {
        try callReturningNothing { try hisar.approveApproval(withPassword: requestID, password: password) }
    }

    func denyApproval(requestID: String) throws {
        try callReturningNothing { try hisar.denyApproval(requestID) }
    }
}
