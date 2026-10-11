import Foundation

struct VaultStatus: Codable, Equatable {
    static let stateSignedOut = "signed-out"
    static let stateLocked = "locked"
    static let stateUnlocked = "unlocked"

    var state: String
    var server: String
    var email: String
    var name: String
    var biometricAvailable: Bool
    var biometricEnabled: Bool
    var biometricReady: Bool
    var isOffline: Bool

    static let signedOut = VaultStatus(state: stateSignedOut, server: "", email: "", name: "", biometricAvailable: false, biometricEnabled: false, biometricReady: false, isOffline: false)

    var isSignedIn: Bool { state != Self.stateSignedOut }
    var isUnlocked: Bool { state == Self.stateUnlocked }

    init(state: String, server: String, email: String, name: String, biometricAvailable: Bool, biometricEnabled: Bool, biometricReady: Bool, isOffline: Bool) {
        self.state = state
        self.server = server
        self.email = email
        self.name = name
        self.biometricAvailable = biometricAvailable
        self.biometricEnabled = biometricEnabled
        self.biometricReady = biometricReady
        self.isOffline = isOffline
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        state = try container.decodeIfPresent(String.self, forKey: .state) ?? Self.stateSignedOut
        server = try container.decodeIfPresent(String.self, forKey: .server) ?? ""
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        biometricAvailable = try container.decodeIfPresent(Bool.self, forKey: .biometricAvailable) ?? false
        biometricEnabled = try container.decodeIfPresent(Bool.self, forKey: .biometricEnabled) ?? false
        biometricReady = try container.decodeIfPresent(Bool.self, forKey: .biometricReady) ?? false
        isOffline = try container.decodeIfPresent(Bool.self, forKey: .isOffline) ?? false
    }
}

struct VaultSummary: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var type: String
    var role: String
    var itemCount: Int

    init(id: String, name: String, type: String, role: String, itemCount: Int) {
        self.id = id
        self.name = name
        self.type = type
        self.role = role
        self.itemCount = itemCount
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        type = try container.decodeIfPresent(String.self, forKey: .type) ?? ""
        role = try container.decodeIfPresent(String.self, forKey: .role) ?? ""
        itemCount = try container.decodeIfPresent(Int.self, forKey: .itemCount) ?? 0
    }
}

struct ItemField: Codable, Hashable {
    var name: String
    var value: String
    var concealed: Bool
}

struct ItemDetail: Codable, Identifiable, Hashable {
    static let categoryLogin = "login"
    static let categorySSHKey = "ssh-key"

    var id: String
    var revision: String
    var category: String
    var title: String
    var username: String
    var allURL: [String]
    var password: String
    var notes: String
    var allField: [ItemField]
    var publicKey: String
    var fingerprint: String
    var keyType: String
    var oneTimePassword: String
    var hasOneTimePassword: Bool
    var allPasskey: [ItemPasskey]

    enum CodingKeys: String, CodingKey {
        case id, revision, category, title, username, password, notes, publicKey, fingerprint, keyType, oneTimePassword, hasOneTimePassword
        case allURL = "urls"
        case allField = "fields"
        case allPasskey = "passkeys"
    }

    init(id: String = "", revision: String = "", category: String = ItemDetail.categoryLogin, title: String = "", username: String = "", allURL: [String] = [], password: String = "", notes: String = "", allField: [ItemField] = [], publicKey: String = "", fingerprint: String = "", keyType: String = "", oneTimePassword: String = "", hasOneTimePassword: Bool = false, allPasskey: [ItemPasskey] = []) {
        self.id = id
        self.revision = revision
        self.category = category
        self.title = title
        self.username = username
        self.allURL = allURL
        self.password = password
        self.notes = notes
        self.allField = allField
        self.publicKey = publicKey
        self.fingerprint = fingerprint
        self.keyType = keyType
        self.oneTimePassword = oneTimePassword
        self.hasOneTimePassword = hasOneTimePassword
        self.allPasskey = allPasskey
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? ""
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
        category = try container.decodeIfPresent(String.self, forKey: .category) ?? ""
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        username = try container.decodeIfPresent(String.self, forKey: .username) ?? ""
        allURL = try container.decodeIfPresent([String].self, forKey: .allURL) ?? []
        password = try container.decodeIfPresent(String.self, forKey: .password) ?? ""
        notes = try container.decodeIfPresent(String.self, forKey: .notes) ?? ""
        allField = try container.decodeIfPresent([ItemField].self, forKey: .allField) ?? []
        publicKey = try container.decodeIfPresent(String.self, forKey: .publicKey) ?? ""
        fingerprint = try container.decodeIfPresent(String.self, forKey: .fingerprint) ?? ""
        keyType = try container.decodeIfPresent(String.self, forKey: .keyType) ?? ""
        oneTimePassword = try container.decodeIfPresent(String.self, forKey: .oneTimePassword) ?? ""
        hasOneTimePassword = try container.decodeIfPresent(Bool.self, forKey: .hasOneTimePassword) ?? false
        allPasskey = try container.decodeIfPresent([ItemPasskey].self, forKey: .allPasskey) ?? []
    }

    var isLogin: Bool { category.isEmpty || category == Self.categoryLogin }
    var isSSHKey: Bool { category == Self.categorySSHKey }
}

struct SignUpResult: Codable, Equatable {
    var status: VaultStatus
    var secretKey: String
}

struct LoginMatch: Codable, Hashable, Identifiable {
    var vaultID: String
    var itemID: String
    var title: String
    var username: String
    var vaultName: String
    var hasOneTimePassword: Bool

    init(vaultID: String, itemID: String, title: String, username: String, vaultName: String, hasOneTimePassword: Bool = false) {
        self.vaultID = vaultID
        self.itemID = itemID
        self.title = title
        self.username = username
        self.vaultName = vaultName
        self.hasOneTimePassword = hasOneTimePassword
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        vaultID = try container.decode(String.self, forKey: .vaultID)
        itemID = try container.decode(String.self, forKey: .itemID)
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        username = try container.decodeIfPresent(String.self, forKey: .username) ?? ""
        vaultName = try container.decodeIfPresent(String.self, forKey: .vaultName) ?? ""
        hasOneTimePassword = try container.decodeIfPresent(Bool.self, forKey: .hasOneTimePassword) ?? false
    }

    var id: String { LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: itemID) }

    static func buildRecordIdentifier(vaultID: String, itemID: String) -> String { "\(vaultID)/\(itemID)" }

    static func parseRecordIdentifier(_ recordIdentifier: String) -> (vaultID: String, itemID: String)? {
        let allPart = recordIdentifier.split(separator: "/", maxSplits: 1).map(String.init)
        guard allPart.count == 2, !allPart[0].isEmpty, !allPart[1].isEmpty else { return nil }
        return (allPart[0], allPart[1])
    }
}

struct LoginCredential: Codable, Equatable {
    var username: String
    var password: String
    var hasOneTimePassword: Bool

    init(username: String, password: String, hasOneTimePassword: Bool = false) {
        self.username = username
        self.password = password
        self.hasOneTimePassword = hasOneTimePassword
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        username = try container.decodeIfPresent(String.self, forKey: .username) ?? ""
        password = try container.decodeIfPresent(String.self, forKey: .password) ?? ""
        hasOneTimePassword = try container.decodeIfPresent(Bool.self, forKey: .hasOneTimePassword) ?? false
    }
}

struct OneTimeCode: Codable, Equatable {
    var code: String
    var expiresAt: String
    var period: Int

    var expiryDate: Date? { ISO8601DateFormatter().date(from: expiresAt) }

    var formattedCode: String {
        let middle = code.index(code.startIndex, offsetBy: (code.count + 1) / 2)
        return "\(code[..<middle]) \(code[middle...])".trimmingCharacters(in: .whitespaces)
    }

    func countSecondsLeft(at date: Date) -> Int {
        guard let expiryDate else { return 0 }
        return max(0, Int(expiryDate.timeIntervalSince(date).rounded(.up)))
    }
}

enum Base64URL {
    static func encode(_ data: Data) -> String {
        data.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }

    static func decode(_ text: String) -> Data? {
        var base64 = text.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        base64 += String(repeating: "=", count: (4 - base64.count % 4) % 4)
        return Data(base64Encoded: base64)
    }
}

struct ItemPasskey: Codable, Hashable, Identifiable {
    var credentialID: String
    var relyingPartyID: String
    var userName: String
    var userDisplayName: String
    var createdAt: String

    enum CodingKeys: String, CodingKey {
        case userName, userDisplayName, createdAt
        case credentialID = "credentialId"
        case relyingPartyID = "relyingPartyId"
    }

    init(credentialID: String, relyingPartyID: String, userName: String, userDisplayName: String = "", createdAt: String = "") {
        self.credentialID = credentialID
        self.relyingPartyID = relyingPartyID
        self.userName = userName
        self.userDisplayName = userDisplayName
        self.createdAt = createdAt
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        credentialID = try container.decode(String.self, forKey: .credentialID)
        relyingPartyID = try container.decodeIfPresent(String.self, forKey: .relyingPartyID) ?? ""
        userName = try container.decodeIfPresent(String.self, forKey: .userName) ?? ""
        userDisplayName = try container.decodeIfPresent(String.self, forKey: .userDisplayName) ?? ""
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
    }

    var id: String { credentialID }
    var createdDate: Date? { ISO8601DateFormatter().date(from: createdAt) }
}

struct StoredPasskey: Codable, Hashable, Identifiable {
    var credentialID: String
    var relyingPartyID: String
    var userName: String
    var userDisplayName: String
    var userHandle: String
    var vaultID: String
    var itemID: String
    var title: String

    enum CodingKeys: String, CodingKey {
        case userName, userDisplayName, userHandle, vaultID, itemID, title
        case credentialID = "credentialId"
        case relyingPartyID = "relyingPartyId"
    }

    init(credentialID: String, relyingPartyID: String, userName: String, userDisplayName: String = "", userHandle: String, vaultID: String, itemID: String, title: String) {
        self.credentialID = credentialID
        self.relyingPartyID = relyingPartyID
        self.userName = userName
        self.userDisplayName = userDisplayName
        self.userHandle = userHandle
        self.vaultID = vaultID
        self.itemID = itemID
        self.title = title
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        credentialID = try container.decode(String.self, forKey: .credentialID)
        relyingPartyID = try container.decodeIfPresent(String.self, forKey: .relyingPartyID) ?? ""
        userName = try container.decodeIfPresent(String.self, forKey: .userName) ?? ""
        userDisplayName = try container.decodeIfPresent(String.self, forKey: .userDisplayName) ?? ""
        userHandle = try container.decodeIfPresent(String.self, forKey: .userHandle) ?? ""
        vaultID = try container.decode(String.self, forKey: .vaultID)
        itemID = try container.decode(String.self, forKey: .itemID)
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
    }

    var id: String { credentialID }
    var recordIdentifier: String { LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: itemID) }
}

struct PasskeyRegistrationOptions: Codable, Equatable {
    var isExcluded: Bool
    var allLogin: [LoginMatch]

    enum CodingKeys: String, CodingKey {
        case isExcluded
        case allLogin = "logins"
    }

    init(isExcluded: Bool, allLogin: [LoginMatch]) {
        self.isExcluded = isExcluded
        self.allLogin = allLogin
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        isExcluded = try container.decodeIfPresent(Bool.self, forKey: .isExcluded) ?? false
        allLogin = try container.decodeIfPresent([LoginMatch].self, forKey: .allLogin) ?? []
    }
}

struct PasskeyAssertionRequest: Equatable {
    var relyingPartyID: String
    var clientDataHash: Data
    var allAllowedCredentialID: [Data] = []
}

struct PasskeyRegistrationRequest: Equatable {
    static let algorithmES256 = -7

    var relyingPartyID: String
    var userName: String
    var userHandle: Data
    var clientDataHash: Data
    var allSupportedAlgorithm: [Int] = []
    var allExcludedCredentialID: [Data] = []

    var isAlgorithmSupported: Bool {
        allSupportedAlgorithm.isEmpty || allSupportedAlgorithm.contains(Self.algorithmES256)
    }
}

struct PasskeyRegistrationResult: Codable, Equatable {
    var credentialID: String
    var attestationObject: String
    var authenticatorData: String
    var vaultID: String
    var itemID: String

    enum CodingKeys: String, CodingKey {
        case attestationObject, authenticatorData, vaultID, itemID
        case credentialID = "credentialId"
    }
}

struct PasskeyAssertionResult: Codable, Equatable {
    var credentialID: String
    var authenticatorData: String
    var signature: String
    var userHandle: String

    enum CodingKeys: String, CodingKey {
        case authenticatorData, signature, userHandle
        case credentialID = "credentialId"
    }
}

struct PasskeyAssertionCredential: Equatable {
    var relyingPartyID: String
    var clientDataHash: Data
    var credentialID: Data
    var authenticatorData: Data
    var signature: Data
    var userHandle: Data

    init?(relyingPartyID: String, clientDataHash: Data, result: PasskeyAssertionResult) {
        guard let credentialID = Base64URL.decode(result.credentialID), let authenticatorData = Base64URL.decode(result.authenticatorData),
              let signature = Base64URL.decode(result.signature), let userHandle = Base64URL.decode(result.userHandle) else { return nil }
        self.relyingPartyID = relyingPartyID
        self.clientDataHash = clientDataHash
        self.credentialID = credentialID
        self.authenticatorData = authenticatorData
        self.signature = signature
        self.userHandle = userHandle
    }
}

struct PasskeyRegistrationCredential: Equatable {
    var relyingPartyID: String
    var clientDataHash: Data
    var credentialID: Data
    var attestationObject: Data

    init?(relyingPartyID: String, clientDataHash: Data, result: PasskeyRegistrationResult) {
        guard let credentialID = Base64URL.decode(result.credentialID), let attestationObject = Base64URL.decode(result.attestationObject) else { return nil }
        self.relyingPartyID = relyingPartyID
        self.clientDataHash = clientDataHash
        self.credentialID = credentialID
        self.attestationObject = attestationObject
    }
}

// Member is a person you share a vault with (role admin or member on the server) or someone with access to a vault (role owner, editor or reader).
struct Member: Codable, Identifiable, Hashable {
    static let roleAdmin = "admin"
    static let roleMember = "member"
    static let vaultRoleOwner = "owner"
    static let vaultRoleEditor = "editor"
    static let vaultRoleReader = "reader"
    static let allVaultRole = [vaultRoleOwner, vaultRoleEditor, vaultRoleReader]

    var accountID: String
    var name: String
    var email: String
    var role: String
    var securityCode: String
    var isCurrentUser: Bool
    var isKeyChanged: Bool

    var id: String { accountID }

    enum CodingKeys: String, CodingKey {
        case accountID = "accountId"
        case name, email, role, securityCode, isCurrentUser, isKeyChanged
    }

    init(accountID: String, name: String, email: String, role: String, securityCode: String = "", isCurrentUser: Bool = false, isKeyChanged: Bool = false) {
        self.accountID = accountID
        self.name = name
        self.email = email
        self.role = role
        self.securityCode = securityCode
        self.isCurrentUser = isCurrentUser
        self.isKeyChanged = isKeyChanged
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        accountID = try container.decodeIfPresent(String.self, forKey: .accountID) ?? ""
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        role = try container.decodeIfPresent(String.self, forKey: .role) ?? ""
        securityCode = try container.decodeIfPresent(String.self, forKey: .securityCode) ?? ""
        isCurrentUser = try container.decodeIfPresent(Bool.self, forKey: .isCurrentUser) ?? false
        isKeyChanged = try container.decodeIfPresent(Bool.self, forKey: .isKeyChanged) ?? false
    }
}

struct ReceivedShareInvitation: Codable, Identifiable, Hashable {
    var id: String
    var vaultID: String
    var role: String
    var invitedByName: String
    var invitedByEmail: String
    var expiresAt: String

    var inviterName: String { invitedByName.isEmpty ? invitedByEmail : invitedByName }

    enum CodingKeys: String, CodingKey {
        case id, role, invitedByName, invitedByEmail, expiresAt
        case vaultID = "vaultId"
    }
}

struct SentShareInvitation: Codable, Identifiable, Hashable {
    static let stateAccepted = "accepted"

    var id: String
    var vaultID: String
    var vaultName: String
    var email: String
    var role: String
    var state: String
    var expiresAt: String

    enum CodingKeys: String, CodingKey {
        case id, vaultName, email, role, state, expiresAt
        case vaultID = "vaultId"
    }
}

struct ShareInvitations: Codable, Hashable {
    var allReceived: [ReceivedShareInvitation]
    var allSent: [SentShareInvitation]
    var completionError: String

    enum CodingKeys: String, CodingKey {
        case allReceived = "received"
        case allSent = "sent"
        case completionError
    }

    init(allReceived: [ReceivedShareInvitation] = [], allSent: [SentShareInvitation] = [], completionError: String = "") {
        self.allReceived = allReceived
        self.allSent = allSent
        self.completionError = completionError
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        allReceived = try container.decodeIfPresent([ReceivedShareInvitation].self, forKey: .allReceived) ?? []
        allSent = try container.decodeIfPresent([SentShareInvitation].self, forKey: .allSent) ?? []
        completionError = try container.decodeIfPresent(String.self, forKey: .completionError) ?? ""
    }
}
