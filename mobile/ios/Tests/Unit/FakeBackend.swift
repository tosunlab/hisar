import Foundation
@testable import Hisar

/// FakeImportGroup is one source group of the pending import: the titles of its logins and the vault they go to.
struct FakeImportGroup {
    var id: String
    var name: String
    var allTitle: [String]
    var vaultID: String
}

class FakeBackend: VaultBackend {
    static let password = "correct horse battery"
    static let secretKey = "H1-ABC123-AAAAA-BBBBB-CCCCC-DDDDD-EEEEEE"
    static let oneTimePassword = "otpauth://totp/GitHub:alice?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
    static let oneTimeCode = OneTimeCode(code: "287082", expiresAt: "2027-01-15T08:00:30Z", period: 30)
    static let gitHubPasskey = StoredPasskey(
        credentialID: Base64URL.encode(Data([1, 1, 1])), relyingPartyID: "github.com", userName: "alice", userDisplayName: "Alice",
        userHandle: Base64URL.encode(Data("alice-handle".utf8)), vaultID: "personal", itemID: "github", title: "GitHub"
    )

    private let callLock = NSLock()
    var status = VaultStatus.signedOut
    var allVault: [VaultSummary] = []
    var allItemByVaultID: [String: [ItemDetail]] = [:]
    /// Starred items as "vaultID/itemID"; the account keeps them apart from the items.
    var allFavoriteRecordID: Set<String> = []
    var favoriteErrorMessage: String?
    var peopleErrorMessage: String?
    var cancelShareInvitationErrorMessage: String?
    var deletePasskeyErrorMessage: String?
    var isBiometricAvailable = true
    var isBiometricSucceeding = true
    var biometricUnlockError: BackendError?
    var verifyPasskey: (String) -> Bool = { _ in true }
    func cancelPasskeyCeremonies() {}
    var isOffline = false
    var allCopiedField: [String] = []
    var allCallName: [String] = []
    var nextItemNumber = 1
    var allPasskey: [StoredPasskey] = []
    /// Runs when a refresh lists the passkeys, its last call, after the unlock check: a test can sign out or lock while a
    /// refresh that listed everything is still in flight.
    var onListAllPasskey: (() -> Void)?
    var preparePasskeyRegistrationError: BackendError?
    var onPreparePasskeyRegistration: (() -> Void)?
    /// Runs while a pending recovery's Emergency Kit is prepared: a test can lock or sign out meanwhile.
    var onExportPendingRecoveryEmergencyKit: (() -> Void)?
    var allMember: [Member] = []
    var allVaultMemberByVaultID: [String: [Member]] = [:]
    var allInvitedEmail: [String] = []
    var shareInvitations = ShareInvitations()
    var allShareAwaitingUnlock: [ShareAwaitingUnlock] = []
    var isShareAwaitingUnlockFailing = false
    var allAnsweredInvitation: [String] = []
    var allPendingApproval: [PhoneApproval] = []
    var listAllPendingApprovalErrorMessage: String?
    /// Runs when the open requests have been read and before they are returned: a test can hold an older listing while a
    /// newer one runs.
    var onListAllPendingApproval: (() -> Void)?
    var allApprovalAnswer: [String] = []
    var allDevice: [Device] = []
    var deviceListError: String?
    var isMacApprovalAllowedValue = true
    var isWebsiteIconsEnabledValue = true
    var allPushToken: [String] = []
    var allPushPayloadKey: [String] = []
    var pushSettings = PushSettings(isAvailable: false, isServerSending: true, enabledKinds: PushKind.allCases.map(\.rawValue), errorMessage: nil)
    var websiteIconByURL: [String: Data] = [:]
    var preparedIcon: Data?
    var theme = VaultTheme.standard
    var isThemeSaveFailing = false
    var revokeDeviceErrorMessage: String?
    var macApprovalErrorMessage: String?
    /// The backend signs out on the phone even when it could not confirm everything, then reports that.
    var signOutError: BackendError?
    /// What the backend reports when the account is deleted but this phone's copy could not be removed.
    var accountDeletionCleanupError: BackendError?
    var allCopiedReference: [String] = []
    var allPendingRemovedVault: [RemovedVaultNotice] = []
    var allDeletionRequest: [VaultDeletionRequest] = []
    var exportOptions = ExportOptions(isBiometricAvailable: true, minPassphraseLength: 10)
    var currentPassword = FakeBackend.password
    var currentSecretKey = FakeBackend.secretKey
    var deletionPreview = AccountDeletionPreview(email: "alice@example.com", planVersion: "reviewed-plan")
    var isAccountDeleted = false
    var allPendingRecovery: [PendingRecovery] = []
    var pendingRecoveryPassword = FakeBackend.password
    var pendingRecoverySecretKey = FakeBackend.secretKey
    var isPendingRecoveryFactorAvailable = true
    var pendingSignUpResult: SignUpResult?
    var isNewAccountSecretKeyRetained = false
    var newAccountEmergencyKitError: BackendError?
    var allNewAccountEmergencyKitDirectory: [URL] = []
    /// The server signUpWithoutInvite was last asked to create an account on; nil until it is called.
    var signUpWithoutInviteServer: String?
    /// While set, signUpWithoutInvite fails with this sentence: the server's admin has not opened registration.
    var signUpWithoutInviteErrorMessage: String?
    var pendingCredentialChangeResult: CredentialChangeResult?
    /// The organizations the user is in, as GetOrganization answers for each; a refusal is the server's next sentence.
    var allOrganization: [Organization] = []
    var allOrganizationInvitation: [OrganizationInvitation] = []
    var waitingVaultKeys = WaitingVaultKeys()
    var organizationRefusal: String?
    /// While set, listing the organizations or reading one fails with this sentence.
    var listAllOrganizationErrorMessage: String?
    var getOrganizationErrorMessage: String?
    /// The import prepareCredentialExchangeImport keeps, as the logins of each source group it would save; nil when none is pending.
    var allPendingImportGroup: [FakeImportGroup]?
    var pendingImportVaultID = ""
    var allQueuedImport: [(title: String, vaultID: String)] = []
    var importSavedCount = 0
    var importBatchSize = 2
    var allImportedCredentialData: [Data] = []
    /// While set, the next continueImport fails with this sentence and saves nothing.
    var continueImportErrorMessage: String?
    /// What prepareHisarExportImport returns; the default is an encrypted export that waits for its passphrase.
    var hisarExportPreview = FakeBackend.encryptedHisarExportPreview()
    var importPreviewOverride: ImportPreview?
    var importProgressOverride: ImportProgress?
    /// What openImportFile returns once hisarExportPassphrase opens the waiting export.
    var openedHisarExportPreview = FakeBackend.plainHisarExportPreview()
    /// The passphrase that opens the waiting encrypted export.
    var hisarExportPassphrase = "orange cactus harbor"
    private var isHisarExportWaiting = false
    private let approvalLock = NSLock()

    func withApprovalLock<Value>(_ operation: () throws -> Value) rethrows -> Value {
        approvalLock.lock()
        defer { approvalLock.unlock() }
        return try operation()
    }

    fileprivate func record(_ name: String) {
        callLock.lock()
        allCallName.append(name)
        callLock.unlock()
    }

    func seedSignedIn(state: String = VaultStatus.stateUnlocked) {
        status = VaultStatus(state: state, server: "http://127.0.0.1:18080", email: "alice@example.com", name: "Alice", biometricAvailable: isBiometricAvailable, biometricEnabled: false, biometricReady: false, isOffline: isOffline)
        allVault = [
            VaultSummary(id: "family", name: "Family", type: "shared", role: "editor", itemCount: 1),
            VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2),
        ]
        allItemByVaultID = [
            "personal": [
                ItemDetail(id: "github", revision: "r1", category: "login", title: "GitHub", username: "alice", allURL: ["https://github.com"], password: "github-password", notes: "work account", allField: [ItemField(name: "PIN", value: "1234", concealed: true)], oneTimePassword: Self.oneTimePassword, hasOneTimePassword: true, allPasskey: [ItemPasskey(credentialID: Self.gitHubPasskey.credentialID, relyingPartyID: "github.com", userName: "alice", userDisplayName: "Alice", createdAt: "2026-10-09T12:00:00Z")]),
                ItemDetail(id: "bank", revision: "r1", category: "login", title: "Bank", username: "alice@example.com", allURL: ["bank.example.com"], password: "bank-password"),
            ],
            "family": [
                ItemDetail(id: "netflix", revision: "r1", category: "login", title: "Netflix", username: "family@example.com", allURL: ["https://www.netflix.com/login"], password: "netflix-password"),
            ],
        ]
        allPasskey = [Self.gitHubPasskey]
        allMember = [
            Member(accountID: "alice-id", name: "Alice", email: "alice@example.com", role: Member.roleAdmin, securityCode: "1111 2222", isCurrentUser: true),
            Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: Member.roleMember, securityCode: "3333 4444"),
            Member(accountID: "carol-id", name: "Carol", email: "carol@example.com", role: Member.roleMember, securityCode: "5555 6666"),
        ]
        allVaultMemberByVaultID = [
            "family": [
                Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: Member.vaultRoleOwner, securityCode: "3333 4444"),
                Member(accountID: "alice-id", name: "Alice", email: "alice@example.com", role: Member.vaultRoleEditor, securityCode: "1111 2222", isCurrentUser: true),
            ],
        ]
    }

    private var currentMember: Member? { allMember.first { $0.isCurrentUser } }

    func createVault(name: String) throws -> VaultSummary {
        record("createVault")
        try requireUnlocked()
        let name = name.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { throw BackendError(message: "Enter a vault name") }
        let vault = VaultSummary(id: "vault-\(allVault.count + 1)", name: name, type: "shared", role: Member.vaultRoleOwner, itemCount: 0)
        allVault.append(vault)
        allItemByVaultID[vault.id] = []
        if let currentMember {
            allVaultMemberByVaultID[vault.id] = [Member(accountID: currentMember.accountID, name: currentMember.name, email: currentMember.email, role: Member.vaultRoleOwner, securityCode: currentMember.securityCode, isCurrentUser: true)]
        }
        return vault
    }

    func listAllPerson() throws -> [Member] {
        record("listAllPerson")
        try requireUnlocked()
        if let peopleErrorMessage { throw BackendError(message: peopleErrorMessage) }
        let allSharedAccountID = Set(allVaultMemberByVaultID.values.filter { $0.contains { $0.isCurrentUser } }.flatMap { $0.map(\.accountID) })
        return allMember.filter { $0.isCurrentUser || allSharedAccountID.contains($0.accountID) }
    }

    func invitePerson(email: String) throws {
        record("invitePerson")
        try requireUnlocked()
        guard email.contains("@") else { throw BackendError(message: "Enter the email address of the person") }
        allInvitedEmail.append(email)
    }

    func listAllShareInvitation() throws -> ShareInvitations {
        record("listAllShareInvitation")
        try requireUnlocked()
        return shareInvitations
    }

    func listAllShareAwaitingUnlock() throws -> [ShareAwaitingUnlock] {
        record("listAllShareAwaitingUnlock")
        if isShareAwaitingUnlockFailing { throw BackendError(message: "cannot reach the Hisar server") }
        return status.state == VaultStatus.stateLocked ? allShareAwaitingUnlock : []
    }

    func acceptShareInvitation(invitationID: String) throws {
        record("acceptShareInvitation")
        try answerReceivedInvitation(invitationID: invitationID, answer: "accept")
    }

    func declineShareInvitation(invitationID: String) throws {
        record("declineShareInvitation")
        try answerReceivedInvitation(invitationID: invitationID, answer: "decline")
    }

    private func answerReceivedInvitation(invitationID: String, answer: String) throws {
        try requireUnlocked()
        guard shareInvitations.allReceived.contains(where: { $0.id == invitationID }) else { throw BackendError(message: "This invitation no longer exists") }
        shareInvitations.allReceived.removeAll { $0.id == invitationID }
        allAnsweredInvitation.append("\(invitationID) \(answer)")
    }

    func cancelShareInvitation(invitationID: String) throws {
        record("cancelShareInvitation")
        try requireUnlocked()
        if let cancelShareInvitationErrorMessage { throw BackendError(message: cancelShareInvitationErrorMessage) }
        guard shareInvitations.allSent.contains(where: { $0.id == invitationID }) else { throw BackendError(message: "This invitation no longer exists") }
        shareInvitations.allSent.removeAll { $0.id == invitationID }
    }

    func confirmShareRecipient(invitationID: String, reviewToken: String) throws {
        record("confirmShareRecipient")
        try requireUnlocked()
        guard let invitation = shareInvitations.allSent.first(where: { $0.id == invitationID }),
              invitation.requiresConfirmation == true, invitation.reviewToken == reviewToken, !reviewToken.isEmpty else {
            throw BackendError(message: "Review this invitation again before confirming the recipient.")
        }
        _ = try findSharableVault(vaultID: invitation.vaultID)
        shareInvitations.allSent.removeAll { $0.id == invitationID }
    }

    func listAllVaultMember(vaultID: String) throws -> [Member] {
        record("listAllVaultMember")
        try requireUnlocked()
        guard allVault.contains(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        return allVaultMemberByVaultID[vaultID] ?? []
    }

    func getVaultSharing(vaultID: String) throws -> VaultSharing {
        record("getVaultSharing")
        try requireUnlocked()
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        guard vault.type == "shared" else { throw BackendError(message: "Personal vaults are private and cannot be shared; create a shared vault instead") }
        let isOwner = vault.role == Member.vaultRoleOwner
        return VaultSharing(
            vault: vault,
            isOwner: isOwner,
            allMember: allVaultMemberByVaultID[vaultID] ?? [],
            allInvitation: isOwner ? shareInvitations.allSent.filter { $0.vaultID == vaultID } : [],
            completionError: isOwner ? shareInvitations.completionError : ""
        )
    }

    private func findSharableVault(vaultID: String) throws -> VaultSummary {
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        guard vault.type == "shared" else { throw BackendError(message: "Personal vaults are private and cannot be shared; create a shared vault instead") }
        guard vault.role == Member.vaultRoleOwner else { throw BackendError(message: "Only owners of this vault can share it, change roles or remove members") }
        return vault
    }

    func shareVault(vaultID: String, email: String, role: String) throws {
        record("shareVault")
        try requireUnlocked()
        guard Member.allVaultRole.contains(role) else { throw BackendError(message: "Choose owner, editor or reader") }
        let vault = try findSharableVault(vaultID: vaultID)
        guard email.contains("@") else { throw BackendError(message: "Enter the email address of the person") }
        guard email != currentMember?.email else { throw BackendError(message: "That is your own email; you already have access") }
        guard !(allVaultMemberByVaultID[vault.id] ?? []).contains(where: { $0.email == email }) else { throw BackendError(message: "This person already has access to the vault") }
        shareInvitations.allSent.removeAll { $0.vaultID == vault.id && $0.email == email }
        shareInvitations.allSent.append(SentShareInvitation(id: "invitation-\(shareInvitations.allSent.count + 1)", vaultID: vault.id, vaultName: vault.name, email: email, role: role, state: "pending", expiresAt: "2027-01-22T08:00:00Z"))
    }

    func changeVaultMemberRole(vaultID: String, accountID: String, role: String) throws {
        record("changeVaultMemberRole")
        try requireUnlocked()
        guard Member.allVaultRole.contains(role) else { throw BackendError(message: "Choose owner, editor or reader") }
        let vault = try findSharableVault(vaultID: vaultID)
        guard let index = allVaultMemberByVaultID[vault.id]?.firstIndex(where: { $0.accountID == accountID }) else {
            throw BackendError(message: "This person no longer has access to the vault; share it with them again")
        }
        allVaultMemberByVaultID[vault.id]?[index].role = role
    }

    func removeVaultMember(vaultID: String, accountID: String) throws {
        record("removeVaultMember")
        try requireUnlocked()
        guard accountID != currentMember?.accountID else { throw BackendError(message: "You cannot remove yourself from a vault you own") }
        let vault = try findSharableVault(vaultID: vaultID)
        allVaultMemberByVaultID[vault.id]?.removeAll { $0.accountID == accountID }
    }

    fileprivate func requireUnlocked() throws {
        guard status.isUnlocked else {
            throw BackendError(message: status.isSignedIn ? "Hisar is locked" : "Sign in first")
        }
    }

    func getStatus() -> VaultStatus {
        record("getStatus")
        return status
    }

    // The server's answer to the next version check; nil means it accepts this version.
    var serverUpdateRequirement: UpdateRequirement?
    var onCheckMinimumVersion: (() -> Void)?

    func checkMinimumVersion() -> VaultStatus {
        record("checkMinimumVersion")
        status.updateRequired = serverUpdateRequirement
        let answer = status
        onCheckMinimumVersion?()
        return answer
    }

    func signIn(server: String, email: String, secretKey: String, password: String) throws -> VaultStatus {
        record("signIn")
        guard password == Self.password, secretKey == Self.secretKey else {
            throw BackendError(message: "Incorrect email, master password or Secret Key")
        }
        seedSignedIn()
        status.server = server
        status.email = email
        return status
    }

    func signUp(inviteLink: String, email: String, name: String, password: String) throws -> SignUpResult {
        record("signUp")
        guard inviteLink.contains("/invite/") else { throw BackendError(message: "Invalid invite link") }
        if let pendingSignUpResult { return pendingSignUpResult }
        seedSignedIn()
        allItemByVaultID = ["personal": [], "family": []]
        status.email = email
        status.name = name
        isNewAccountSecretKeyRetained = true
        allPendingRecovery.append(PendingRecovery(operationId: "signup-1", kind: "signup", origin: status.server, accountId: "new-account", email: email, name: name, phase: "delivery-pending", kitSaved: false, problems: [], keychainProtected: true))
        return SignUpResult(status: status, secretKey: Self.secretKey, pendingOperationId: "signup-1", setupState: "delivery-pending")
    }

    func signUpWithoutInvite(server: String, email: String, name: String, password: String) throws -> SignUpResult {
        record("signUpWithoutInvite")
        signUpWithoutInviteServer = server
        if let signUpWithoutInviteErrorMessage { throw BackendError(message: signUpWithoutInviteErrorMessage) }
        if let pendingSignUpResult { return pendingSignUpResult }
        seedSignedIn()
        allItemByVaultID = ["personal": [], "family": []]
        status.server = server
        status.email = email
        status.name = name
        isNewAccountSecretKeyRetained = true
        allPendingRecovery.append(PendingRecovery(operationId: "signup-1", kind: "signup", origin: server, accountId: "new-account", email: email, name: name, phase: "delivery-pending", kitSaved: false, problems: [], keychainProtected: true))
        return SignUpResult(status: status, secretKey: Self.secretKey, pendingOperationId: "signup-1", setupState: "delivery-pending")
    }

    func unlock(password: String) throws -> VaultStatus {
        record("unlock")
        guard password == Self.password else { throw BackendError(message: "The master password is incorrect.") }
        status.state = VaultStatus.stateUnlocked
        if status.biometricEnabled { status.biometricReady = true }
        return status
    }

    func unlockWithBiometric() throws -> VaultStatus {
        record("unlockWithBiometric")
        guard status.biometricReady else { throw BackendError(message: "Unlock with your master password first") }
        if let biometricUnlockError { throw biometricUnlockError }
        guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.", code: BackendError.biometricCancelled) }
        status.state = VaultStatus.stateUnlocked
        return status
    }

    func enableBiometricUnlock(password: String) throws -> VaultStatus {
        record("enableBiometricUnlock")
        try requireUnlocked()
        guard password == Self.password else { throw BackendError(message: "Incorrect email, master password or Secret Key") }
        status.biometricEnabled = true
        status.biometricReady = true
        return status
    }

    func enableNewAccountBiometricUnlock() throws -> VaultStatus {
        record("enableNewAccountBiometricUnlock")
        try requireUnlocked()
        guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.") }
        status.biometricEnabled = true
        status.biometricReady = true
        return status
    }

    func exportNewAccountEmergencyKit(directory: URL) throws -> EmergencyKitExport {
        record("exportNewAccountEmergencyKit")
        allNewAccountEmergencyKitDirectory.append(directory)
        if let newAccountEmergencyKitError { throw newAccountEmergencyKitError }
        try requireUnlocked()
        guard isNewAccountSecretKeyRetained, let index = allPendingRecovery.firstIndex(where: { $0.operationId == "signup-1" }) else {
            throw BackendError(message: "This account's setup needs finishing from the account recovery screen")
        }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent("Hisar Emergency Kit.pdf")
        try Data("%PDF-1.7".utf8).write(to: path)
        allPendingRecovery[index].kitSaved = true
        return EmergencyKitExport(secretKey: Self.secretKey, emergencyKitPath: path.path)
    }

    func acknowledgeNewAccountRecovery() throws {
        record("acknowledgeNewAccountRecovery")
        try requireUnlocked()
        guard isNewAccountSecretKeyRetained, let recovery = allPendingRecovery.first(where: { $0.operationId == "signup-1" }) else {
            throw BackendError(message: "This account's setup needs finishing from the account recovery screen")
        }
        guard recovery.kitSaved else { throw BackendError(message: "Save the Emergency Kit before acknowledging delivery") }
        allPendingRecovery.removeAll { $0.operationId == "signup-1" }
    }

    func forgetNewAccountSecretKey() {
        record("forgetNewAccountSecretKey")
        isNewAccountSecretKeyRetained = false
    }

    func disableBiometricUnlock() throws -> VaultStatus {
        record("disableBiometricUnlock")
        status.biometricEnabled = false
        status.biometricReady = false
        return status
    }

    func lock() -> VaultStatus {
        record("lock")
        if status.isSignedIn { status.state = VaultStatus.stateLocked }
        return status
    }

    func signOut() throws -> VaultStatus {
        record("signOut")
        // As the backend does, signing out keeps only the server, so the sign-in form opens on it.
        let lastServer = status.server
        status = .signedOut
        status.server = lastServer
        if let signOutError { throw signOutError }
        return status
    }

    func reportActivity() {
        record("reportActivity")
    }

    func listAllVault() throws -> [VaultSummary] {
        record("listAllVault")
        try requireUnlocked()
        return allVault
    }

    func listAllItem(vaultID: String) throws -> [ItemDetail] {
        record("listAllItem")
        try requireUnlocked()
        return (allItemByVaultID[vaultID] ?? []).map { item in
            var summary = item
            summary.isFavorite = allFavoriteRecordID.contains(LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: item.id))
            summary.password = ""
            summary.notes = ""
            summary.allField = []
            summary.oneTimePassword = ""
            return summary
        }
    }

    func getItem(vaultID: String, itemID: String) throws -> ItemDetail {
        record("getItem")
        try requireUnlocked()
        guard var item = allItemByVaultID[vaultID]?.first(where: { $0.id == itemID }) else { throw BackendError(message: "Item not found") }
        item.isFavorite = allFavoriteRecordID.contains(LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: itemID))
        return item
    }

    func saveItem(vaultID: String, item: ItemDetail) throws -> ItemDetail {
        record("saveItem")
        try requireUnlocked()
        guard !item.title.trimmingCharacters(in: .whitespaces).isEmpty else { throw BackendError(message: "Enter a title") }
        var savedItem = item
        let oneTimePassword = item.oneTimePassword.trimmingCharacters(in: .whitespacesAndNewlines)
        guard oneTimePassword.isEmpty || oneTimePassword.hasPrefix("otpauth://totp/") || oneTimePassword.replacingOccurrences(of: " ", with: "").allSatisfy({ "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".contains($0.uppercased()) }) else {
            throw BackendError(message: "The one-time code is not a valid secret or otpauth:// link")
        }
        savedItem.oneTimePassword = oneTimePassword.isEmpty ? "" : (oneTimePassword.hasPrefix("otpauth://") ? oneTimePassword : "otpauth://totp/?secret=\(oneTimePassword.uppercased().replacingOccurrences(of: " ", with: ""))")
        savedItem.hasOneTimePassword = !oneTimePassword.isEmpty
        if savedItem.isCreditCard {
            var card = savedItem.card ?? ItemCard()
            let description = try describeCard(number: card.number, expiry: card.expiry)
            card.brand = description.brand
            if card.type.isEmpty {
                card.type = description.brand
            }
            card.last4 = description.last4
            card.numberWarning = description.numberWarning
            savedItem.card = card
        }
        if savedItem.id.isEmpty {
            savedItem.id = "new-\(nextItemNumber)"
            nextItemNumber += 1
        }
        savedItem.revision = UUID().uuidString
        var allItem = allItemByVaultID[vaultID] ?? []
        if let index = allItem.firstIndex(where: { $0.id == savedItem.id }) {
            allItem[index] = savedItem
        } else {
            allItem.append(savedItem)
        }
        allItemByVaultID[vaultID] = allItem
        return savedItem
    }

    /// A stand-in for core/item's checks: Visa by the first digit, and a number of 16 digits ending in 1 counts as valid.
    func describeCard(number: String, expiry: String) throws -> CardDescription {
        record("describeCard")
        let digits = number.filter(\.isNumber)
        let isValid = digits.count == 16 && digits.hasSuffix("1")
        return CardDescription(
            brand: digits.hasPrefix("4") ? "Visa" : "",
            last4: String(digits.suffix(4)),
            numberWarning: number.isEmpty || isValid ? "" : "This doesn't look like a valid card number",
            expiryWarning: ""
        )
    }

    func convertToSecureNote(vaultID: String, itemID: String) throws -> ItemDetail {
        record("convertToSecureNote")
        var item = try getItem(vaultID: vaultID, itemID: itemID)
        guard item.isLogin, item.username.isEmpty, item.password.isEmpty, item.allURL.isEmpty, !item.notes.isEmpty || !item.allField.isEmpty else {
            throw BackendError(message: "only a login with nothing but notes and custom fields can become a secure note")
        }
        item.category = ItemDetail.categorySecureNote
        item.canConvertToSecureNote = false
        item.revision = UUID().uuidString
        replaceItem(vaultID: vaultID, item: item)
        return item
    }

    func listAllFavoriteItem() throws -> [FavoriteItem] {
        record("listAllFavoriteItem")
        try requireUnlocked()
        return allVault.flatMap { vault in
            ((try? listAllItem(vaultID: vault.id)) ?? []).filter(\.isFavorite).map { FavoriteItem(vaultID: vault.id, vaultName: vault.name, vaultType: vault.type, item: $0) }
        }
    }

    func setItemFavorite(vaultID: String, itemID: String, isFavorite: Bool) throws {
        record("setItemFavorite")
        try requireUnlocked()
        if let favoriteErrorMessage { throw BackendError(message: favoriteErrorMessage) }
        let recordID = LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: itemID)
        if isFavorite {
            allFavoriteRecordID.insert(recordID)
        } else {
            allFavoriteRecordID.remove(recordID)
        }
    }

    func deleteItem(vaultID: String, itemID: String) throws {
        record("deleteItem")
        try requireUnlocked()
        allItemByVaultID[vaultID]?.removeAll { $0.id == itemID }
        allPasskey.removeAll { $0.vaultID == vaultID && $0.itemID == itemID }
    }

    func deleteUnreadableItem(vaultID: String, itemID: String, revision: String) throws {
        try requireUnlocked()
        guard allItemByVaultID[vaultID]?.contains(where: { $0.id == itemID && $0.revision == revision && $0.readIssue?.canDelete == true }) == true else {
            throw BackendError(message: "The unreadable record changed")
        }
        allItemByVaultID[vaultID]?.removeAll { $0.id == itemID }
        if let index = allVault.firstIndex(where: { $0.id == vaultID }) { allVault[index].readIssues.removeAll { $0.itemId == itemID } }
    }

    func deleteUnreadableVault(vaultID: String, revision: String) throws {
        try requireUnlocked()
        guard allVault.contains(where: { $0.id == vaultID && $0.readIssues.contains(where: { $0.canDelete && $0.itemId == nil && $0.revision == revision }) }) else {
            throw BackendError(message: "The unreadable vault changed")
        }
        allVault.removeAll { $0.id == vaultID }
        allItemByVaultID.removeValue(forKey: vaultID)
    }

    func repairVaultName(vaultID: String, revision: String, name: String) throws -> VaultSummary {
        try requireUnlocked()
        guard let index = allVault.firstIndex(where: { $0.id == vaultID && $0.readIssues.contains(where: { $0.canRename && $0.revision == revision }) }) else {
            throw BackendError(message: "The unreadable vault changed")
        }
        allVault[index].name = name.trimmingCharacters(in: .whitespaces)
        allVault[index].readIssues.removeAll { $0.itemId == nil }
        return allVault[index]
    }

    func getOneTimeCode(vaultID: String, itemID: String) throws -> OneTimeCode {
        record("getOneTimeCode")
        guard try getItem(vaultID: vaultID, itemID: itemID).hasOneTimePassword else { throw BackendError(message: "One-time password not found") }
        return Self.oneTimeCode
    }

    func getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> OneTimeCode {
        record("getOneTimeCodeForService")
        guard try listAllLoginForService(serviceIdentifier: serviceIdentifier).contains(where: { $0.vaultID == vaultID && $0.itemID == itemID }) else {
            throw BackendError(message: "This login is not saved for this site")
        }
        return try getOneTimeCode(vaultID: vaultID, itemID: itemID)
    }

    func copyField(vaultID: String, itemID: String, fieldName: String) throws {
        record("copyField")
        let item = try getItem(vaultID: vaultID, itemID: itemID)
        allCopiedField.append("\(item.id).\(fieldName)")
    }

    func generatePassword(length: Int, isDigitIncluded: Bool, isSymbolIncluded: Bool) throws -> String {
        record("generatePassword")
        return String(repeating: isSymbolIncluded ? "!" : (isDigitIncluded ? "1" : "a"), count: length)
    }

    func listAllLoginForService(serviceIdentifier: String) throws -> [LoginMatch] {
        record("listAllLoginForService")
        try requireUnlocked()
        guard let host = VaultLogic.extractHost(serviceIdentifier) else { return [] }
        var allLogin: [LoginMatch] = []
        for vault in allVault {
            for item in allItemByVaultID[vault.id] ?? [] where item.allURL.contains(where: { VaultLogic.extractHost($0).map { host.hasSuffix($0) || $0.hasSuffix(host) } ?? false }) {
                allLogin.append(LoginMatch(vaultID: vault.id, itemID: item.id, title: item.title, username: item.username, vaultName: vault.name, hasOneTimePassword: item.hasOneTimePassword, isPersonal: vault.type == "personal"))
            }
        }
        return allLogin
    }

    func getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> LoginCredential {
        record("getLoginForService")
        guard try listAllLoginForService(serviceIdentifier: serviceIdentifier).contains(where: { $0.vaultID == vaultID && $0.itemID == itemID }) else {
            throw BackendError(message: "This login is not saved for this site")
        }
        let item = try getItem(vaultID: vaultID, itemID: itemID)
        return LoginCredential(username: item.username, password: item.password, hasOneTimePassword: item.hasOneTimePassword)
    }
}

extension FakeBackend {
    func listAllPasskey() throws -> [StoredPasskey] {
        record("listAllPasskey")
        try requireUnlocked()
        onListAllPasskey?()
        return allPasskey
    }

    func listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: [Data]) throws -> [StoredPasskey] {
        record("listAllPasskeyForRelyingParty")
        try requireUnlocked()
        let allAllowedID = Set(allAllowedCredentialID.map(Base64URL.encode))
        return allPasskey.filter { $0.relyingPartyID == relyingPartyID && (allAllowedID.isEmpty || allAllowedID.contains($0.credentialID)) }
    }

    func preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: [Data]) throws -> PasskeyRegistrationOptions {
        record("preparePasskeyRegistration")
        onPreparePasskeyRegistration?()
        if let preparePasskeyRegistrationError { throw preparePasskeyRegistrationError }
        let allLogin = try listAllLoginForService(serviceIdentifier: "https://\(relyingPartyID)").filter { login in
            allVault.first { $0.id == login.vaultID }?.role != "reader"
        }
        return PasskeyRegistrationOptions(isExcluded: checkIsExcluded(relyingPartyID: relyingPartyID, allExcludedCredentialID: allExcludedCredentialID), allLogin: allLogin)
    }

    private func checkIsExcluded(relyingPartyID: String, allExcludedCredentialID: [Data]) -> Bool {
        let allExcludedID = Set(allExcludedCredentialID.map(Base64URL.encode))
        return allPasskey.contains { $0.relyingPartyID == relyingPartyID && allExcludedID.contains($0.credentialID) }
    }

    func createPasskey(_ request: PasskeyRegistrationRequest, vaultID: String, itemID: String) throws -> PasskeyRegistrationResult {
        record("createPasskey")
        try requireUnlocked()
        if request.userVerification != "discouraged" && !verifyPasskey("Create a passkey for \(request.relyingPartyID)") { throw BackendError(message: "Passkey verification was cancelled.", code: BackendError.passkeyVerificationFailed) }
        guard !checkIsExcluded(relyingPartyID: request.relyingPartyID, allExcludedCredentialID: request.allExcludedCredentialID) else {
            throw BackendError(message: "Hisar already has a passkey for this account")
        }
        guard request.clientDataHash.count == 32 else { throw BackendError(message: "the request is not valid") }
        var targetVaultID = vaultID
        var item: ItemDetail
        if vaultID.isEmpty {
            targetVaultID = "personal"
            item = try saveItem(vaultID: targetVaultID, item: ItemDetail(title: request.relyingPartyID, username: request.userName, allURL: ["https://\(request.relyingPartyID)"]))
        } else {
            guard try preparePasskeyRegistration(relyingPartyID: request.relyingPartyID, allExcludedCredentialID: []).allLogin.contains(where: { $0.vaultID == vaultID && $0.itemID == itemID }) else {
                throw BackendError(message: "this login is not saved for this site")
            }
            item = try getItem(vaultID: vaultID, itemID: itemID)
        }
        let userHandle = Base64URL.encode(request.userHandle)
        let credentialID = Base64URL.encode(Data([UInt8(allPasskey.count + 2), 7, 7]))
        let replacedID = Set(allPasskey.filter { $0.vaultID == targetVaultID && $0.itemID == item.id && $0.relyingPartyID == request.relyingPartyID && $0.userHandle == userHandle }.map(\.credentialID))
        allPasskey.removeAll { replacedID.contains($0.credentialID) }
        allPasskey.append(StoredPasskey(credentialID: credentialID, relyingPartyID: request.relyingPartyID, userName: request.userName, userHandle: userHandle, vaultID: targetVaultID, itemID: item.id, title: item.title))
        item.allPasskey.removeAll { replacedID.contains($0.credentialID) }
        item.allPasskey.append(ItemPasskey(credentialID: credentialID, relyingPartyID: request.relyingPartyID, userName: request.userName))
        replaceItem(vaultID: targetVaultID, item: item)
        return PasskeyRegistrationResult(
            credentialID: credentialID, attestationObject: Base64URL.encode(Data("attestation for \(request.relyingPartyID)".utf8)),
            authenticatorData: Base64URL.encode(Data("authenticator data".utf8)), vaultID: targetVaultID, itemID: item.id
        )
    }

    private func replaceItem(vaultID: String, item: ItemDetail) {
        guard let index = allItemByVaultID[vaultID]?.firstIndex(where: { $0.id == item.id }) else { return }
        allItemByVaultID[vaultID]?[index] = item
    }

    func assertPasskey(relyingPartyID: String, credentialID: Data, clientDataHash: Data, userVerification: String) throws -> PasskeyAssertionResult {
        record("assertPasskey")
        try requireUnlocked()
        if userVerification != "discouraged" && !verifyPasskey("Use your passkey for \(relyingPartyID)") { throw BackendError(message: "Passkey verification was cancelled.", code: BackendError.passkeyVerificationFailed) }
        guard clientDataHash.count == 32 else { throw BackendError(message: "the request is not valid") }
        guard let passkey = allPasskey.first(where: { $0.relyingPartyID == relyingPartyID && $0.credentialID == Base64URL.encode(credentialID) }) else {
            throw BackendError(message: "passkey not found")
        }
        return PasskeyAssertionResult(
            credentialID: passkey.credentialID, authenticatorData: Base64URL.encode(Data("authenticator data for \(relyingPartyID)".utf8)),
            signature: Base64URL.encode(Data("signature over ".utf8) + clientDataHash), userHandle: passkey.userHandle
        )
    }

    func deletePasskey(vaultID: String, itemID: String, credentialID: String) throws -> ItemDetail {
        record("deletePasskey")
        if let deletePasskeyErrorMessage { throw BackendError(message: deletePasskeyErrorMessage) }
        var item = try getItem(vaultID: vaultID, itemID: itemID)
        guard item.allPasskey.contains(where: { $0.credentialID == credentialID }) else { throw BackendError(message: "passkey not found") }
        item.allPasskey.removeAll { $0.credentialID == credentialID }
        allPasskey.removeAll { $0.credentialID == credentialID }
        replaceItem(vaultID: vaultID, item: item)
        return item
    }
}

// Phone approvals: the Mac's requests are seeded; answers are recorded. Listing is not recorded, it runs in the background.
extension FakeBackend {
    func listAllPendingApproval() throws -> [PhoneApproval] {
        try requireUnlocked()
        if let listAllPendingApprovalErrorMessage { throw BackendError(message: listAllPendingApprovalErrorMessage) }
        let allApproval = withApprovalLock { allPendingApproval }
        onListAllPendingApproval?()
        return allApproval
    }

    func approveApproval(requestID: String, isRemembered: Bool) throws {
        record("approveApproval")
        try requireUnlocked()
        guard isBiometricAvailable else { throw BackendError(message: "Face ID is not available; approve with your master password") }
        guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.") }
        try answerApproval(requestID: requestID, decision: isRemembered ? "approve remembered" : "approve")
    }

    func approveApprovalWithPassword(requestID: String, password: String, isRemembered: Bool) throws {
        record("approveApprovalWithPassword")
        try requireUnlocked()
        guard password == Self.password else { throw BackendError(message: "Incorrect master password.") }
        try answerApproval(requestID: requestID, decision: isRemembered ? "approve remembered" : "approve")
    }

    func reconnectEvents() {
        record("reconnectEvents")
    }

    func denyApproval(requestID: String) throws {
        record("denyApproval")
        try requireUnlocked()
        try answerApproval(requestID: requestID, decision: "deny")
    }

    private func answerApproval(requestID: String, decision: String) throws {
        try withApprovalLock {
            guard allPendingApproval.contains(where: { $0.requestID == requestID }) else {
                throw BackendError(message: "This request is no longer waiting for an answer")
            }
            allPendingApproval.removeAll { $0.requestID == requestID }
            allApprovalAnswer.append("\(decision) \(requestID)")
        }
    }
}

// Settings: auto-lock, devices, the Emergency Kit, secret references and the Mac approval switch.
extension FakeBackend {
    func setAutoLock(seconds: Int) throws -> VaultStatus {
        record("setAutoLock")
        guard VaultStatus.allAutoLockSeconds.contains(seconds) else {
            throw BackendError(message: "choose 30 seconds, 1 minute, 5 minutes, 10 minutes or 1 hour")
        }
        status.autoLockSeconds = seconds
        return status
    }

    func listAllDevice() throws -> [Device] {
        record("listAllDevice")
        try requireUnlocked()
        if let deviceListError { throw BackendError(message: deviceListError) }
        return allDevice
    }

    func revokeDevice(deviceID: String) throws {
        record("revokeDevice")
        try requireUnlocked()
        if let message = revokeDeviceErrorMessage { throw BackendError(message: message) }
        guard let device = allDevice.first(where: { $0.id == deviceID }) else { throw BackendError(message: "this device is already signed out") }
        guard !device.isCurrent else { throw BackendError(message: "sign out this device instead") }
        allDevice.removeAll { $0.id == deviceID }
    }


    func getVaultActions(vaultID: String) throws -> VaultActions {
        record("getVaultActions")
        try requireUnlocked()
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "vault not found") }
        var actions = VaultActions(vault: vault, canRename: vault.role == Member.vaultRoleOwner)
        let allMember = allVaultMemberByVaultID[vaultID] ?? []
        actions.allMemberLosingAccess = allMember.filter { !$0.isCurrentUser }
        if vault.type == "personal" {
            actions.deleteBlockedReason = "Your Personal vault can't be deleted."
            actions.leaveBlockedReason = "You can't leave your Personal vault."
        } else if vault.role != Member.vaultRoleOwner {
            actions.deleteBlockedReason = "Only an owner of this vault can delete it. You can leave it instead."
            actions.canLeave = true
        } else {
            actions.canDelete = true
            actions.leaveBlockedReason = "You are its only owner."
            actions.allTargetVault = allVault.filter { $0.id != vaultID && $0.role != Member.vaultRoleReader }
        }
        return actions
    }

    func renameVault(vaultID: String, name: String) throws -> VaultSummary {
        record("renameVault")
        try requireUnlocked()
        let name = name.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { throw BackendError(message: "Enter a vault name") }
        guard let index = allVault.firstIndex(where: { $0.id == vaultID }), allVault[index].role == Member.vaultRoleOwner else {
            throw BackendError(message: "Only an owner of the vault can do that")
        }
        allVault[index].name = name
        return allVault[index]
    }

    func deleteVault(vaultID: String, request: VaultDeletionRequest) throws -> VaultDeletionResult {
        record("deleteVault")
        try requireUnlocked()
        allDeletionRequest.append(request)
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "vault not found") }
        let allItem = allItemByVaultID[vaultID] ?? []
        var result = VaultDeletionResult()
        if !request.moveToVaultID.isEmpty {
            guard let target = allVault.first(where: { $0.id == request.moveToVaultID }) else { throw BackendError(message: "vault not found") }
            allItemByVaultID[target.id, default: []].append(contentsOf: allItem)
            result = VaultDeletionResult(movedCount: allItem.count, targetVaultName: target.name)
        } else if request.isDeletingItems {
            guard request.confirmedName.trimmingCharacters(in: .whitespaces) == vault.name else {
                throw BackendError(message: "Type the vault's name to delete it together with its items")
            }
        } else if !allItem.isEmpty {
            throw BackendError(message: "This vault has items")
        }
        allVault.removeAll { $0.id == vaultID }
        allItemByVaultID[vaultID] = nil
        return result
    }

    func leaveVault(vaultID: String) throws {
        record("leaveVault")
        try requireUnlocked()
        allVault.removeAll { $0.id == vaultID }
        allItemByVaultID[vaultID] = nil
    }

    func listAllRemovedVault() throws -> [RemovedVaultNotice] {
        record("listAllRemovedVault")
        try requireUnlocked()
        let allNotice = allPendingRemovedVault
        allPendingRemovedVault = []
        return allNotice
    }

    func getExportOptions() throws -> ExportOptions {
        record("getExportOptions")
        return exportOptions
    }

    func exportAllData(format: ExportFormat, passphrase: String, password: String, directory: URL) throws -> ExportResult {
        record("exportAllData")
        try requireUnlocked()
        if password.isEmpty {
            guard isBiometricSucceeding else { throw BackendError(message: "Face ID was cancelled.") }
        } else if password != Self.password {
            throw BackendError(message: "Incorrect email, master password or Secret Key")
        }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent(format == .plain ? "Hisar export (not encrypted).json" : "Hisar export.hisar")
        try Data("{}".utf8).write(to: path)
        return ExportResult(path: path.path, vaultCount: allVault.count, itemCount: allItemByVaultID.values.map(\.count).reduce(0, +))
    }

    private func checkImportVault(_ vaultID: String) throws -> VaultSummary {
        guard let vault = allVault.first(where: { $0.id == vaultID }) else { throw BackendError(message: "Vault not found") }
        guard vault.type == VaultSummary.typePersonal || vault.role != Member.vaultRoleReader else { throw BackendError(message: "You can only view this vault") }
        return vault
    }

    /// buildImportPreview is the pending import as core/app previews it: vaultID is the vault every group goes to, or ""
    /// when the groups go to different vaults.
    private func buildImportPreview() throws -> ImportPreview {
        guard let allGroup = allPendingImportGroup else { throw BackendError(message: "There is no import to continue") }
        let allVaultID = Set(allGroup.map(\.vaultID))
        let commonVault = allVaultID.count == 1 ? try checkImportVault(allGroup[0].vaultID) : nil
        var allDuplicate: [ImportDuplicate] = []
        var allSourceGroup: [ImportSourceGroup] = []
        for group in allGroup {
            let vault = try checkImportVault(group.vaultID)
            let allExistingTitle = Set((allItemByVaultID[vault.id] ?? []).map(\.title))
            allDuplicate += group.allTitle.filter { allExistingTitle.contains($0) }.map { ImportDuplicate(title: $0) }
            allSourceGroup.append(ImportSourceGroup(id: group.id, name: group.name, isPrivate: true, itemCount: group.allTitle.count, vaultID: vault.id, vaultName: vault.name,
                                                    members: allVaultMemberByVaultID[vault.id] ?? []))
        }
        let itemCount = allGroup.map(\.allTitle.count).reduce(0, +)
        var preview = ImportPreview(fileName: "Other Manager", vaultID: commonVault?.id ?? "", vaultName: commonVault?.name ?? "the mapped vaults", itemCount: itemCount,
                                    loginCount: itemCount, allDuplicate: allDuplicate)
        preview.groups = allSourceGroup
        return importPreviewOverride ?? preview
    }

    /// prepareCredentialExchangeImport reads credentialData as the titles of the logins to import, or as one list of
    /// titles per source group, like the collections of a Credential Exchange.
    func prepareCredentialExchangeImport(vaultID: String, credentialData: Data) throws -> ImportPreview {
        record("prepareCredentialExchangeImport")
        try requireUnlocked()
        if let allTitle = try? JSONDecoder().decode([String].self, from: credentialData) {
            allPendingImportGroup = [FakeImportGroup(id: "private", name: "Private or unspecified source", allTitle: allTitle, vaultID: vaultID)]
        } else {
            let allGroupTitle = try JSONDecoder().decode([[String]].self, from: credentialData)
            allPendingImportGroup = allGroupTitle.enumerated().map { FakeImportGroup(id: "group-\($0.offset)", name: "Collection \($0.offset + 1)", allTitle: $0.element, vaultID: vaultID) }
        }
        allImportedCredentialData.append(credentialData)
        return try buildImportPreview()
    }

    /// The default encrypted Hisar export: only the file name and format, and it waits for its passphrase.
    static func encryptedHisarExportPreview() -> ImportPreview {
        var preview = ImportPreview(fileName: "Vault export.hisar", vaultID: "", vaultName: "")
        preview.format = ImportPreview.formatHisarEncryptedExport
        preview.formatName = "Hisar encrypted export"
        preview.isPassphraseRequired = true
        preview.isSourceKept = true
        return preview
    }

    /// A plain Hisar export preview: one group per exported vault, previewed at once.
    static func plainHisarExportPreview() -> ImportPreview {
        var preview = ImportPreview(fileName: "Vault export.json", vaultID: "personal", vaultName: "Personal", itemCount: 3, loginCount: 3)
        preview.format = ImportPreview.formatHisarExport
        preview.formatName = "Hisar export"
        preview.exportedFrom = "owner@example.com"
        preview.exportedAt = "2026-01-02T03:04:05Z"
        preview.groups = [ImportSourceGroup(id: "personal", name: "Personal", isPrivate: false, itemCount: 3, vaultID: "personal", vaultName: "Personal", members: [])]
        return preview
    }

    /// prepareHisarExportImport returns the configured preview. An encrypted export then waits for openImportFile.
    func prepareHisarExportImport(vaultID: String, fileName: String, content: Data) throws -> ImportPreview {
        record("prepareHisarExportImport")
        try requireUnlocked()
        isHisarExportWaiting = hisarExportPreview.isPassphraseRequired
        return hisarExportPreview
    }

    /// openImportFile opens the waiting export only for hisarExportPassphrase; any other keeps it waiting.
    func openImportFile(passphrase: String) throws -> ImportPreview {
        record("openImportFile")
        try requireUnlocked()
        guard isHisarExportWaiting else { throw BackendError(message: "There is no import to continue") }
        guard passphrase == hisarExportPassphrase else { throw BackendError(message: "that passphrase doesn't open this export") }
        isHisarExportWaiting = false
        return openedHisarExportPreview
    }

    func previewImport(vaultID: String) throws -> ImportPreview {
        record("previewImport")
        try requireUnlocked()
        guard allPendingImportGroup?.count == 1 else { throw BackendError(message: "Choose a destination for each source group") }
        _ = try checkImportVault(vaultID)
        allPendingImportGroup?[0].vaultID = vaultID
        return try buildImportPreview()
    }

    func mapImportGroup(groupID: String, vaultID: String) throws -> ImportPreview {
        record("mapImportGroup")
        try requireUnlocked()
        guard let index = allPendingImportGroup?.firstIndex(where: { $0.id == groupID }) else { throw BackendError(message: "Source group not found; choose the import file again") }
        _ = try checkImportVault(vaultID)
        allPendingImportGroup?[index].vaultID = vaultID
        return try buildImportPreview()
    }

    func startImport(vaultID: String, isSkippingDuplicate: Bool) throws -> ImportProgress {
        record("startImport")
        try requireUnlocked()
        let preview = try buildImportPreview()
        guard vaultID == preview.vaultID else { throw BackendError(message: "Review the import destinations again") }
        allQueuedImport = []
        for group in allPendingImportGroup ?? [] {
            let allExistingTitle = Set((allItemByVaultID[group.vaultID] ?? []).map(\.title))
            allQueuedImport += group.allTitle.filter { !isSkippingDuplicate || !allExistingTitle.contains($0) }.map { (title: $0, vaultID: group.vaultID) }
        }
        pendingImportVaultID = vaultID
        importSavedCount = 0
        return buildImportProgress()
    }

    func continueImport() throws -> ImportProgress {
        record("continueImport")
        try requireUnlocked()
        guard allPendingImportGroup != nil else { throw BackendError(message: "There is no import to continue") }
        if let continueImportErrorMessage {
            self.continueImportErrorMessage = nil
            throw BackendError(message: continueImportErrorMessage)
        }
        for queued in allQueuedImport.prefix(importBatchSize) {
            allItemByVaultID[queued.vaultID, default: []].append(ItemDetail(id: "imported-\(nextItemNumber)", revision: "r1", category: "login", title: queued.title))
            nextItemNumber += 1
            importSavedCount += 1
        }
        allQueuedImport = Array(allQueuedImport.dropFirst(importBatchSize))
        return buildImportProgress()
    }

    private func buildImportProgress() -> ImportProgress {
        let vaultName = allVault.first { $0.id == pendingImportVaultID }?.name ?? "the mapped vaults"
        if allQueuedImport.isEmpty, let importProgressOverride { return importProgressOverride }
        return ImportProgress(fileName: "Other Manager", vaultID: pendingImportVaultID, vaultName: vaultName, savedCount: importSavedCount,
                              totalCount: importSavedCount + allQueuedImport.count, isDone: allQueuedImport.isEmpty)
    }

    func cancelImport() {
        record("cancelImport")
        allPendingImportGroup = nil
        allQueuedImport = []
    }

    func exportEmergencyKit(password: String, directory: URL) throws -> EmergencyKitExport {
        record("exportEmergencyKit")
        try requireUnlocked()
        guard password == Self.password else { throw BackendError(message: "Incorrect master password.") }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent("Hisar Emergency Kit.pdf")
        try Data("%PDF-1.7".utf8).write(to: path)
        return EmergencyKitExport(secretKey: Self.secretKey, emergencyKitPath: path.path)
    }

    func changeMasterPassword(currentPassword: String, newPassword: String) throws -> CredentialChangeResult {
        record("changeMasterPassword")
        if let pendingCredentialChangeResult { return pendingCredentialChangeResult }
        try requireUnlocked()
        guard currentPassword == self.currentPassword else { throw BackendError(message: "The master password is not correct") }
        guard newPassword.count >= 10 else { throw BackendError(message: "Use a master password with at least 10 characters") }
        self.currentPassword = newPassword
        return CredentialChangeResult(status: status, signedOutDeviceCount: 1)
    }

    func replaceSecretKey(password: String, directory: URL) throws -> CredentialChangeResult {
        record("replaceSecretKey")
        if let pendingCredentialChangeResult { return pendingCredentialChangeResult }
        try requireUnlocked()
        guard password == currentPassword else { throw BackendError(message: "The master password is not correct") }
        currentSecretKey = "H1-ABC123-ZZZZZ-YYYYY-XXXXX-WWWWW-VVVVVV"
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent("Hisar Emergency Kit.pdf")
        try Data("%PDF-1.7".utf8).write(to: path)
        return CredentialChangeResult(status: status, secretKey: currentSecretKey, signedOutDeviceCount: 2, emergencyKitPath: path.path)
    }

    func previewAccountDeletion() throws -> AccountDeletionPreview {
        record("previewAccountDeletion")
        try requireUnlocked()
        return deletionPreview
    }

    func deleteAccountWithPlan(password: String, email: String, planVersion: String) throws -> VaultStatus {
        record("deleteAccountWithPlan")
        try requireUnlocked()
        guard !planVersion.isEmpty, planVersion == deletionPreview.planVersion else { throw BackendError(message: "The deletion plan changed. Review it again.") }
        guard email.trimmingCharacters(in: .whitespaces).lowercased() == status.email.lowercased() else { throw BackendError(message: "Type the email of this account to confirm") }
        guard password == currentPassword else { throw BackendError(message: "The master password is not correct") }
        isAccountDeleted = true
        status = .signedOut
        if let accountDeletionCleanupError { throw accountDeletionCleanupError }
        return status
    }

    func listPendingRecovery() throws -> [PendingRecovery] {
        record("listPendingRecovery")
        return allPendingRecovery
    }

    func resumePendingRecovery(operationID: String, password: String) throws -> SignUpResult {
        record("resumePendingRecovery")
        guard password == pendingRecoveryPassword,
              let index = allPendingRecovery.firstIndex(where: { $0.operationId == operationID }) else {
            throw BackendError(message: "The recovery password is not correct.")
        }
        guard allPendingRecovery[index].phase != "awaiting-kit" else { throw BackendError(message: "Save and confirm the Emergency Kit before continuing.") }
        guard allPendingRecovery[index].keychainProtected || isPendingRecoveryFactorAvailable else {
            throw BackendError(message: "Enter the Secret Key from your saved Emergency Kit.")
        }
        allPendingRecovery[index].phase = "delivery-pending"
        return SignUpResult(status: status, secretKey: pendingRecoverySecretKey, pendingOperationId: operationID, setupState: "delivery-pending", problems: allPendingRecovery[index].problems)
    }

    func resumePendingRecoveryWithSecretKey(operationID: String, password: String, secretKey: String, priorSecretKey: String) throws -> SignUpResult {
        record("resumePendingRecoveryWithSecretKey")
        guard secretKey == pendingRecoverySecretKey,
              priorSecretKey.isEmpty || priorSecretKey == currentSecretKey else {
            throw BackendError(message: "The Secret Key does not match this recovery.")
        }
        isPendingRecoveryFactorAvailable = true
        return try resumePendingRecovery(operationID: operationID, password: password)
    }

    func discardPreparedRecovery(operationID: String, password: String) throws {
        record("discardPreparedRecovery")
        guard password == pendingRecoveryPassword,
              let recovery = allPendingRecovery.first(where: { $0.operationId == operationID }),
              recovery.phase == "awaiting-kit" || recovery.phase == "prepared" else {
            throw BackendError(message: "Only an uncommitted setup can be discarded.")
        }
        allPendingRecovery.removeAll { $0.operationId == operationID }
    }

    func exportPendingRecoveryEmergencyKit(operationID: String, password: String, directory: URL) throws -> PendingRecoveryExport {
        record("exportPendingRecoveryEmergencyKit")
        onExportPendingRecoveryEmergencyKit?()
        guard password == pendingRecoveryPassword,
              let index = allPendingRecovery.firstIndex(where: { $0.operationId == operationID }),
              allPendingRecovery[index].phase != "outcome-unknown" else {
            throw BackendError(message: "Resume this recovery before preparing the Emergency Kit.")
        }
        guard allPendingRecovery[index].keychainProtected || isPendingRecoveryFactorAvailable else {
            throw BackendError(message: "Enter the Secret Key from your saved Emergency Kit.")
        }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let path = directory.appendingPathComponent("Hisar Emergency Kit.pdf")
        try Data("%PDF-1.7".utf8).write(to: path)
        allPendingRecovery[index].kitSaved = true
        return PendingRecoveryExport(secretKey: pendingRecoverySecretKey, path: path.path)
    }

    func acknowledgePendingRecovery(operationID: String, password: String) throws {
        record("acknowledgePendingRecovery")
        guard password == pendingRecoveryPassword,
              let recovery = allPendingRecovery.first(where: { $0.operationId == operationID }),
              (recovery.phase == "delivery-pending" || recovery.phase == "awaiting-kit"), recovery.kitSaved else {
            throw BackendError(message: "Save the new Emergency Kit before finishing recovery.")
        }
        if recovery.phase == "awaiting-kit" || recovery.manualUnlock {
            let index = allPendingRecovery.firstIndex { $0.operationId == operationID }!
            allPendingRecovery[index].phase = recovery.phase == "awaiting-kit" ? "prepared" : "manual-unlock"
        } else {
            allPendingRecovery.removeAll { $0.operationId == operationID }
        }
    }

    func dismissSignedOutReason() -> VaultStatus {
        record("dismissSignedOutReason")
        status.signedOutReason = ""
        return status
    }

    func copySecretReference(vaultID: String, itemID: String, fieldName: String) throws -> String {
        record("copySecretReference")
        let item = try getItem(vaultID: vaultID, itemID: itemID)
        let vaultName = allVault.first { $0.id == vaultID }?.name ?? vaultID
        let reference = "hisar://\(vaultName)/\(item.title)/\(fieldName)"
        allCopiedReference.append(reference)
        return reference
    }

    func isMacApprovalAllowed() -> Bool {
        isMacApprovalAllowedValue
    }

    func setMacApprovalAllowed(_ isAllowed: Bool) throws {
        record("setMacApprovalAllowed")
        if let message = macApprovalErrorMessage { throw BackendError(message: message) }
        isMacApprovalAllowedValue = isAllowed
    }

    func prepareItemIcon(_ image: Data) throws -> Data {
        record("prepareItemIcon")
        guard let preparedIcon else { throw BackendError(message: "choose a PNG, JPEG, GIF or WebP image") }
        return preparedIcon
    }

    func getWebsiteIcon(url: String) throws -> Data? {
        record("getWebsiteIcon")
        return isWebsiteIconsEnabledValue ? websiteIconByURL[url] : nil
    }

    func isWebsiteIconsEnabled() -> Bool {
        isWebsiteIconsEnabledValue
    }

    func setWebsiteIconsEnabled(_ isEnabled: Bool) throws {
        record("setWebsiteIconsEnabled")
        isWebsiteIconsEnabledValue = isEnabled
    }

    func setPushToken(platform: String, token: String) throws {
        record("setPushToken")
        allPushToken.append("\(platform):\(token)")
        pushSettings.isAvailable = pushSettings.isServerSending
    }

    func setPushPayloadKey(p256dh: String, auth: String) throws {
        record("setPushPayloadKey")
        allPushPayloadKey.append("\(p256dh):\(auth)")
    }

    func setPushKindEnabled(_ kind: PushKind, isEnabled: Bool) throws {
        record("setPushKindEnabled")
        pushSettings.enabledKinds.removeAll { $0 == kind.rawValue }
        if isEnabled { pushSettings.enabledKinds.append(kind.rawValue) }
    }

    func getPushSettings() throws -> PushSettings {
        pushSettings
    }

    func getTheme() -> VaultTheme {
        theme
    }

    func setTheme(_ theme: VaultTheme) throws -> VaultTheme {
        record("setTheme")
        guard !isThemeSaveFailing else { throw BackendError(message: "Could not save hisar.json") }
        self.theme = theme
        return theme
    }
}

// Organizations (§5.11): the user's organizations with their members and teams; admins change them, members only read.
extension FakeBackend {
    func listAllOrganization() throws -> Organizations {
        record("listAllOrganization")
        try requireUnlocked()
        if let listAllOrganizationErrorMessage { throw BackendError(message: listAllOrganizationErrorMessage) }
        return Organizations(
            allOrganization: allOrganization.map { OrganizationSummary(id: $0.id, name: $0.name, role: $0.role, memberCount: $0.allMember.count) },
            allInvitation: allOrganizationInvitation
        )
    }

    func getOrganization(organizationID: String) throws -> Organization {
        record("getOrganization")
        try requireUnlocked()
        if let getOrganizationErrorMessage { throw BackendError(message: getOrganizationErrorMessage) }
        guard var organization = allOrganization.first(where: { $0.id == organizationID }) else {
            throw BackendError(message: "That organization, team, person or vault was not found, or you are no longer in the organization")
        }
        for teamIndex in organization.allTeam.indices {
            for vaultIndex in organization.allTeam[teamIndex].allVault.indices {
                let vaultID = organization.allTeam[teamIndex].allVault[vaultIndex].vaultID
                let vault = allVault.first { $0.id == vaultID }
                organization.allTeam[teamIndex].allVault[vaultIndex].name = vault?.name ?? ""
                organization.allTeam[teamIndex].allVault[vaultIndex].canOpen = vault != nil
            }
        }
        if !organization.isAdmin {
            organization.allInvitation = []
        }
        return organization
    }

    func createOrganization(name: String) throws -> OrganizationSummary {
        record("createOrganization")
        try requireUnlocked()
        guard OrganizationLogic.isValidName(name), let currentMember else { throw BackendError(message: "Enter a name of 1 to 100 characters") }
        var admin = currentMember
        admin.role = OrganizationRole.admin
        let organization = Organization(id: "organization-\(allOrganization.count + 1)", name: name, role: OrganizationRole.admin, isAdmin: true, allMember: [admin])
        allOrganization.append(organization)
        return OrganizationSummary(id: organization.id, name: name, role: organization.role, memberCount: 1)
    }

    func renameOrganization(organizationID: String, name: String) throws {
        record("renameOrganization")
        try changeOrganizationAsAdmin(organizationID) { $0.name = name }
    }

    func deleteOrganization(organizationID: String) throws {
        record("deleteOrganization")
        try changeOrganizationAsAdmin(organizationID) { _ in }
        guard !allVault.contains(where: { $0.organizationID == organizationID }) else { throw BackendError(message: "The organization still has vaults; delete them first") }
        allOrganization.removeAll { $0.id == organizationID }
    }

    func inviteToOrganization(organizationID: String, email: String, role: String) throws {
        record("inviteToOrganization")
        try changeOrganizationAsAdmin(organizationID) { organization in
            organization.allInvitation.append(SentOrganizationInvitation(id: "organization-invitation-\(organization.allInvitation.count + 1)", email: email, role: role, createdAt: "2027-01-15T08:00:00Z", expiresAt: "2027-01-22T08:00:00Z"))
        }
    }

    func cancelOrganizationInvitation(organizationID: String, invitationID: String) throws {
        record("cancelOrganizationInvitation")
        try changeOrganizationAsAdmin(organizationID) { $0.allInvitation.removeAll { $0.id == invitationID } }
    }

    func acceptOrganizationInvitation(invitationID: String) throws {
        record("acceptOrganizationInvitation")
        try requireUnlocked()
        guard let invitation = allOrganizationInvitation.first(where: { $0.id == invitationID }), let currentMember else { throw BackendError(message: "This invitation no longer exists") }
        allOrganizationInvitation.removeAll { $0.id == invitationID }
        var member = currentMember
        member.role = invitation.role
        allOrganization.append(Organization(id: invitation.organizationID, name: invitation.organizationName, role: invitation.role, isAdmin: invitation.role == OrganizationRole.admin, allMember: [member]))
    }

    func declineOrganizationInvitation(invitationID: String) throws {
        record("declineOrganizationInvitation")
        try requireUnlocked()
        guard allOrganizationInvitation.contains(where: { $0.id == invitationID }) else { throw BackendError(message: "This invitation no longer exists") }
        allOrganizationInvitation.removeAll { $0.id == invitationID }
    }

    func setOrganizationMemberRole(organizationID: String, accountID: String, role: String) throws {
        record("setOrganizationMemberRole")
        try changeOrganizationAsAdmin(organizationID) { organization in
            guard let index = organization.allMember.firstIndex(where: { $0.accountID == accountID }) else { return }
            organization.allMember[index].role = role
        }
    }

    func removeOrganizationMember(organizationID: String, accountID: String) throws {
        record("removeOrganizationMember")
        try changeOrganizationAsAdmin(organizationID) { organization in
            organization.allMember.removeAll { $0.accountID == accountID }
            for index in organization.allTeam.indices {
                organization.allTeam[index].allMember.removeAll { $0.accountID == accountID }
            }
        }
    }

    func leaveOrganization(organizationID: String) throws {
        record("leaveOrganization")
        try requireUnlocked()
        allOrganization.removeAll { $0.id == organizationID }
        allVault.removeAll { $0.organizationID == organizationID }
    }

    func createTeam(organizationID: String, name: String) throws -> Team {
        record("createTeam")
        let team = Team(id: "team-\(name.lowercased())", name: name)
        try changeOrganizationAsAdmin(organizationID) { organization in
            organization.allTeam.append(team)
        }
        return team
    }

    func renameTeam(organizationID: String, teamID: String, name: String) throws {
        record("renameTeam")
        try changeTeam(organizationID, teamID) { $0.name = name }
    }

    func deleteTeam(organizationID: String, teamID: String) throws {
        record("deleteTeam")
        try changeOrganizationAsAdmin(organizationID) { $0.allTeam.removeAll { $0.id == teamID } }
    }

    func addTeamMember(organizationID: String, teamID: String, accountID: String) throws {
        record("addTeamMember")
        guard let member = allOrganization.first(where: { $0.id == organizationID })?.allMember.first(where: { $0.accountID == accountID }) else {
            throw BackendError(message: "That organization, team, person or vault was not found, or you are no longer in the organization")
        }
        try changeTeam(organizationID, teamID) { $0.allMember.append(TeamMember(accountID: accountID, name: member.name, email: member.email, isCurrentUser: member.isCurrentUser)) }
    }

    func removeTeamMember(organizationID: String, teamID: String, accountID: String) throws {
        record("removeTeamMember")
        try changeTeam(organizationID, teamID) { $0.allMember.removeAll { $0.accountID == accountID } }
    }

    func giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String) throws {
        record("giveVaultToTeam")
        try changeTeam(organizationID, teamID) { team in
            team.allVault.removeAll { $0.vaultID == vaultID }
            team.allVault.append(TeamVault(vaultID: vaultID, name: "", role: role, canOpen: false))
        }
    }

    func takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String) throws {
        record("takeVaultFromTeam")
        try changeTeam(organizationID, teamID) { $0.allVault.removeAll { $0.vaultID == vaultID } }
    }

    func createOrganizationVault(organizationID: String, teamID: String, name: String) throws -> VaultSummary {
        record("createOrganizationVault")
        try requireUnlocked()
        guard let team = allOrganization.first(where: { $0.id == organizationID })?.allTeam.first(where: { $0.id == teamID }), team.hasCurrentUser else {
            throw BackendError(message: "Only members of the team can create a vault for it")
        }
        let vault = VaultSummary(id: "vault-\(allVault.count + 1)", name: name, type: VaultSummary.typeOrganization, role: Member.vaultRoleOwner, itemCount: 0, organizationID: organizationID)
        allVault.append(vault)
        allItemByVaultID[vault.id] = []
        try changeTeam(organizationID, teamID, isAdminRequired: false) { $0.allVault.append(TeamVault(vaultID: vault.id, name: "", role: Member.vaultRoleOwner, canOpen: false)) }
        return vault
    }

    func listAllWaitingVaultKey() throws -> WaitingVaultKeys {
        record("listAllWaitingVaultKey")
        try requireUnlocked()
        return waitingVaultKeys
    }

    private func changeOrganizationAsAdmin(_ organizationID: String, isAdminRequired: Bool = true, _ change: (inout Organization) -> Void) throws {
        try requireUnlocked()
        if let organizationRefusal {
            self.organizationRefusal = nil
            throw BackendError(message: organizationRefusal)
        }
        guard let index = allOrganization.firstIndex(where: { $0.id == organizationID }) else {
            throw BackendError(message: "That organization, team, person or vault was not found, or you are no longer in the organization")
        }
        guard allOrganization[index].isAdmin || !isAdminRequired else { throw BackendError(message: "Only an admin of this organization can do that") }
        change(&allOrganization[index])
    }

    private func changeTeam(_ organizationID: String, _ teamID: String, isAdminRequired: Bool = true, _ change: (inout Team) -> Void) throws {
        try changeOrganizationAsAdmin(organizationID, isAdminRequired: isAdminRequired) { organization in
            guard let index = organization.allTeam.firstIndex(where: { $0.id == teamID }) else { return }
            change(&organization.allTeam[index])
        }
    }
}

extension FakeBackend {
    /// Holds a refresh at its last listing while change runs, as when the device is signed out, locks or hears of a
    /// remote change meanwhile.
    @MainActor
    func finishRefresh(of model: AppModel, after change: () async -> Void) async {
        let isListing = DispatchSemaphore(value: 0)
        let mayFinish = DispatchSemaphore(value: 0)
        onListAllPasskey = {
            isListing.signal()
            mayFinish.wait()
        }
        let refresh = Task { await model.refreshAll() }
        while isListing.wait(timeout: .now()) == .timedOut {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        await change()
        onListAllPasskey = nil
        mayFinish.signal()
        await refresh.value
    }
}

/// Main-actor isolated: AppModel calls it from several tasks at once (a status change and sign-out both clear it),
/// and the tests read its state on the main actor.
@MainActor
final class FakeCredentialIdentityStore: CredentialIdentityUpdating {
    var allIdentity: [CredentialIdentityRecord] = []
    var replaceCount = 0

    func replaceAllIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        self.allIdentity = allIdentity
        replaceCount += 1
    }

    func removeAllIdentity() async {
        allIdentity = []
    }
}
