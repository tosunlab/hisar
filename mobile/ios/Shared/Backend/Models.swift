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

    enum CodingKeys: String, CodingKey {
        case id, revision, category, title, username, password, notes, publicKey, fingerprint, keyType
        case allURL = "urls"
        case allField = "fields"
    }

    init(id: String = "", revision: String = "", category: String = ItemDetail.categoryLogin, title: String = "", username: String = "", allURL: [String] = [], password: String = "", notes: String = "", allField: [ItemField] = [], publicKey: String = "", fingerprint: String = "", keyType: String = "") {
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
}
