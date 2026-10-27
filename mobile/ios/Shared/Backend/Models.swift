import Foundation

enum ThemeAppearanceName: String, Codable, CaseIterable {
    case system, light, dark
}

enum ThemeColorName: String, Codable, CaseIterable {
    case neutral, blue, purple, green
}

// VaultTheme mirrors vault.Theme; unknown values fall back to the defaults.
struct VaultTheme: Codable, Equatable {
    var appearance: ThemeAppearanceName
    var color: ThemeColorName

    static let standard = VaultTheme(appearance: .system, color: .neutral)

    init(appearance: ThemeAppearanceName, color: ThemeColorName) {
        self.appearance = appearance
        self.color = color
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        appearance = ThemeAppearanceName(rawValue: (try? container.decode(String.self, forKey: .appearance)) ?? "") ?? .system
        color = ThemeColorName(rawValue: (try? container.decode(String.self, forKey: .color)) ?? "") ?? .neutral
    }
}

struct UpdateRequirement: Codable, Equatable {
    var currentVersion: String
    var minimumVersion: String
    var updateURL: String

    var currentVersionDescription: String {
        currentVersion.isEmpty ? "a development build" : currentVersion
    }
}

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
    var autoLockSeconds: Int
    var signedOutReason = ""
    var updateRequired: UpdateRequirement?

    static let defaultAutoLockSeconds = 300
    static let allAutoLockSeconds = [30, 60, 300, 600, 3600]
    static let signedOut = VaultStatus(state: stateSignedOut, server: "", email: "", name: "", biometricAvailable: false, biometricEnabled: false, biometricReady: false, isOffline: false)

    var isSignedIn: Bool { state != Self.stateSignedOut }
    var isUnlocked: Bool { state == Self.stateUnlocked }

    init(state: String, server: String, email: String, name: String, biometricAvailable: Bool, biometricEnabled: Bool, biometricReady: Bool, isOffline: Bool, autoLockSeconds: Int = VaultStatus.defaultAutoLockSeconds, updateRequired: UpdateRequirement? = nil) {
        self.state = state
        self.server = server
        self.email = email
        self.name = name
        self.biometricAvailable = biometricAvailable
        self.biometricEnabled = biometricEnabled
        self.biometricReady = biometricReady
        self.isOffline = isOffline
        self.autoLockSeconds = autoLockSeconds
        self.updateRequired = updateRequired
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
        let decodedAutoLockSeconds = try container.decodeIfPresent(Int.self, forKey: .autoLockSeconds) ?? 0
        autoLockSeconds = decodedAutoLockSeconds > 0 ? decodedAutoLockSeconds : Self.defaultAutoLockSeconds
        signedOutReason = try container.decodeIfPresent(String.self, forKey: .signedOutReason) ?? ""
        updateRequired = try container.decodeIfPresent(UpdateRequirement.self, forKey: .updateRequired)
    }

    static func describeAutoLock(seconds: Int) -> String {
        switch seconds {
        case ..<60: return "\(seconds) seconds"
        case 60: return "1 minute"
        case 3600: return "1 hour"
        default: return seconds % 3600 == 0 ? "\(seconds / 3600) hours" : "\(seconds / 60) minutes"
        }
    }

    static func describeShortAutoLock(seconds: Int) -> String {
        switch seconds {
        case ..<60: return "\(seconds) s"
        case 3600: return "1 h"
        default: return "\(seconds / 60) min"
        }
    }
}

struct ReadIssue: Codable, Hashable, Identifiable {
    var vaultId: String
    var itemId: String?
    var revision: String?
    var kind: String
    var canRename: Bool
    var canDelete: Bool
    var id: String { "\(vaultId)/\(itemId ?? "")/\(kind)" }
}

struct VaultSummary: Codable, Identifiable, Hashable {
    static let typePersonal = "personal"
    static let typeShared = "shared"
    static let typeOrganization = "organization"

    var id: String
    var name: String
    var type: String
    var role: String
    var itemCount: Int
    /// Set for organization vaults only.
    var organizationID: String
    var readIssues: [ReadIssue] = []
    var isPersonalExtra = false

    var isOrganization: Bool { type == Self.typeOrganization }

    enum CodingKeys: String, CodingKey {
        case id, name, type, role, itemCount
        case readIssues, isPersonalExtra
        case organizationID = "organizationId"
    }

    init(id: String, name: String, type: String, role: String, itemCount: Int, organizationID: String = "") {
        self.id = id
        self.name = name
        self.type = type
        self.role = role
        self.itemCount = itemCount
        self.organizationID = organizationID
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        type = try container.decodeIfPresent(String.self, forKey: .type) ?? ""
        role = try container.decodeIfPresent(String.self, forKey: .role) ?? ""
        itemCount = try container.decodeIfPresent(Int.self, forKey: .itemCount) ?? 0
        organizationID = try container.decodeIfPresent(String.self, forKey: .organizationID) ?? ""
        readIssues = try container.decodeIfPresent([ReadIssue].self, forKey: .readIssues) ?? []
        isPersonalExtra = try container.decodeIfPresent(Bool.self, forKey: .isPersonalExtra) ?? false
    }
}

struct ItemField: Codable, Hashable {
    var name: String
    var value: String
    var concealed: Bool
    private static let builtInReferenceNames: Set<String> = ["username", "password", "url", "notes", "one-time code", "public key", "fingerprint", "connection string"]

    static func nameWarning(_ fields: [ItemField], index: Int) -> String {
        let name = fields[index].name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if builtInReferenceNames.contains(name) {
            return "A secret reference with this name reaches the built-in field."
        }
        return !name.isEmpty && fields.enumerated().contains { $0.offset != index && $0.element.name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == name }
            ? "Secret references need a unique name." : ""
    }
}

struct ItemDetail: Codable, Identifiable, Hashable {
    static let categoryLogin = "login"
    static let categorySSHKey = "ssh-key"
    static let categorySecureNote = "secure-note"
    static let categoryCreditCard = "credit-card"
    static let categoryIdentity = "identity"
    static let categoryAPICredential = "api-credential"
    static let categoryAddress = "address"
    static let categorySoftwareLicense = "software-license"
    static let categoryServer = "server"
    static let categoryDatabase = "database"

    var id: String
    var revision: String
    var readIssue: ReadIssue? = nil
    var category: String
    var title: String
    var username: String
    var allURL: [String]
    var allTag: [String]
    var password: String
    var notes: String
    var allField: [ItemField]
    var publicKey: String
    var fingerprint: String
    var keyType: String
    var oneTimePassword: String
    var hasOneTimePassword: Bool
    var allPasskey: [ItemPasskey]
    /// The Android apps a login fills into, linked on an Android phone; the backend keeps them when the item is saved.
    var allAndroidApp: [ItemAndroidApp]
    /// A login with nothing but notes and custom fields, which the backend can turn into a secure note.
    var canConvertToSecureNote: Bool
    /// A credit card's values, stored as named custom fields; allField then holds only the others. Summaries keep only the
    /// brand and last four digits.
    var card: ItemCard?
    /// An identity's values, stored as named custom fields like a card's. Summaries keep only the name and email.
    var identity: ItemIdentity?
    /// The user starred the item. Favorites belong to the user's account, not the item, so people sharing the vault don't see them.
    var isFavorite: Bool
    /// An API credential's values, stored as named custom fields like an identity's. Summaries keep only the service and
    /// host.
    var apiCredential: ItemAPICredential?
    /// An address's values, stored as named custom fields like an identity's. Summaries keep only the first line of the
    /// street, the city and the country.
    var address: ItemAddress?
    /// A software license's values, stored as named custom fields like an API credential's. Summaries keep only the
    /// product, version, licensed-to name and email.
    var softwareLicense: ItemSoftwareLicense?
    /// A server's host, port and linked SSH key; its username, password and admin console are the item's own. Summaries
    /// keep only the host and port.
    var server: ItemServer?
    /// A database's engine, host, port, name and options; its username and password are the item's own. The backend builds
    /// its connection string. Summaries keep only the engine, host, port and name.
    var database: ItemDatabase?
    /// The icon the user chose, or nil for the automatic one: a login's website icon, else the category's symbol or the
    /// initial.
    var icon: ChosenIcon?

    enum CodingKeys: String, CodingKey {
        case id, revision, category, title, username, password, notes, publicKey, fingerprint, keyType, oneTimePassword, hasOneTimePassword, canConvertToSecureNote, card, identity, isFavorite, apiCredential, address, softwareLicense, server, database, icon
        case readIssue
        case allURL = "urls"
        case allTag = "tags"
        case allField = "fields"
        case allPasskey = "passkeys"
        case allAndroidApp = "androidApps"
    }

    init(id: String = "", revision: String = "", category: String = ItemDetail.categoryLogin, title: String = "", username: String = "", allURL: [String] = [], allTag: [String] = [], password: String = "", notes: String = "", allField: [ItemField] = [], publicKey: String = "", fingerprint: String = "", keyType: String = "", oneTimePassword: String = "", hasOneTimePassword: Bool = false, allPasskey: [ItemPasskey] = [], allAndroidApp: [ItemAndroidApp] = [], canConvertToSecureNote: Bool = false, card: ItemCard? = nil, identity: ItemIdentity? = nil, isFavorite: Bool = false, apiCredential: ItemAPICredential? = nil, address: ItemAddress? = nil, softwareLicense: ItemSoftwareLicense? = nil, server: ItemServer? = nil, database: ItemDatabase? = nil, icon: ChosenIcon? = nil) {
        self.id = id
        self.revision = revision
        self.category = category
        self.title = title
        self.username = username
        self.allURL = allURL
        self.allTag = allTag
        self.password = password
        self.notes = notes
        self.allField = allField
        self.publicKey = publicKey
        self.fingerprint = fingerprint
        self.keyType = keyType
        self.oneTimePassword = oneTimePassword
        self.hasOneTimePassword = hasOneTimePassword
        self.allPasskey = allPasskey
        self.allAndroidApp = allAndroidApp
        self.canConvertToSecureNote = canConvertToSecureNote
        self.card = card
        self.identity = identity
        self.isFavorite = isFavorite
        self.apiCredential = apiCredential
        self.address = address
        self.softwareLicense = softwareLicense
        self.server = server
        self.database = database
        self.icon = icon
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? ""
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
        readIssue = try container.decodeIfPresent(ReadIssue.self, forKey: .readIssue)
        category = try container.decodeIfPresent(String.self, forKey: .category) ?? ""
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        username = try container.decodeIfPresent(String.self, forKey: .username) ?? ""
        allURL = try container.decodeIfPresent([String].self, forKey: .allURL) ?? []
        allTag = try container.decodeIfPresent([String].self, forKey: .allTag) ?? []
        password = try container.decodeIfPresent(String.self, forKey: .password) ?? ""
        notes = try container.decodeIfPresent(String.self, forKey: .notes) ?? ""
        allField = try container.decodeIfPresent([ItemField].self, forKey: .allField) ?? []
        publicKey = try container.decodeIfPresent(String.self, forKey: .publicKey) ?? ""
        fingerprint = try container.decodeIfPresent(String.self, forKey: .fingerprint) ?? ""
        keyType = try container.decodeIfPresent(String.self, forKey: .keyType) ?? ""
        oneTimePassword = try container.decodeIfPresent(String.self, forKey: .oneTimePassword) ?? ""
        hasOneTimePassword = try container.decodeIfPresent(Bool.self, forKey: .hasOneTimePassword) ?? false
        allPasskey = try container.decodeIfPresent([ItemPasskey].self, forKey: .allPasskey) ?? []
        allAndroidApp = try container.decodeIfPresent([ItemAndroidApp].self, forKey: .allAndroidApp) ?? []
        canConvertToSecureNote = try container.decodeIfPresent(Bool.self, forKey: .canConvertToSecureNote) ?? false
        card = try container.decodeIfPresent(ItemCard.self, forKey: .card)
        identity = try container.decodeIfPresent(ItemIdentity.self, forKey: .identity)
        isFavorite = try container.decodeIfPresent(Bool.self, forKey: .isFavorite) ?? false
        apiCredential = try container.decodeIfPresent(ItemAPICredential.self, forKey: .apiCredential)
        address = try container.decodeIfPresent(ItemAddress.self, forKey: .address)
        softwareLicense = try container.decodeIfPresent(ItemSoftwareLicense.self, forKey: .softwareLicense)
        server = try container.decodeIfPresent(ItemServer.self, forKey: .server)
        database = try container.decodeIfPresent(ItemDatabase.self, forKey: .database)
        icon = try container.decodeIfPresent(ChosenIcon.self, forKey: .icon)
    }

    /// The tags typed into one field, separated by commas, without blanks or repeats.
    static func splitTags(_ text: String) -> [String] {
        var allTag: [String] = []
        for tag in text.split(separator: ",").map({ $0.trimmingCharacters(in: .whitespacesAndNewlines) }) where !tag.isEmpty && !allTag.contains(tag) {
            allTag.append(tag)
        }
        return allTag
    }

    var isLogin: Bool { category.isEmpty || category == Self.categoryLogin }
    var isSSHKey: Bool { category == Self.categorySSHKey }
    var isSecureNote: Bool { category == Self.categorySecureNote }
    var isCreditCard: Bool { category == Self.categoryCreditCard }
    var isIdentity: Bool { category == Self.categoryIdentity }
    var isAPICredential: Bool { category == Self.categoryAPICredential }
    var isAddress: Bool { category == Self.categoryAddress }
    var isSoftwareLicense: Bool { category == Self.categorySoftwareLicense }
    var isServer: Bool { category == Self.categoryServer }
    var isDatabase: Bool { category == Self.categoryDatabase }
    /// AutoFill offers the item's username and password on its websites: a login's, and a server's admin console.
    var offersLoginFill: Bool { isLogin || isServer }
    /// The website whose icon a login shows when it has no icon of its own: its first https URL. Other types keep their
    /// symbol.
    var websiteIconURL: String? {
        guard isLogin, icon == nil else { return nil }
        return allURL.map { $0.trimmingCharacters(in: .whitespaces) }.first { $0.lowercased().hasPrefix("https://") }
    }
}

/// An item's chosen icon: a preset (core/item IconPresets) in one of the icon colors, or a PNG image, which wins. The
/// backend makes the image (prepareItemIcon) and refuses any other.
struct ChosenIcon: Codable, Hashable {
    var preset = ""
    var color = ""
    var image: Data?

    enum CodingKeys: String, CodingKey {
        case preset, color, image
    }

    init(preset: String = "", color: String = "", image: Data? = nil) {
        self.preset = preset
        self.color = color
        self.image = image
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        preset = try container.decodeIfPresent(String.self, forKey: .preset) ?? ""
        color = try container.decodeIfPresent(String.self, forKey: .color) ?? ""
        image = try container.decodeIfPresent(Data.self, forKey: .image)
    }

    /// The preset icon IDs of core/item, in the picker's order.
    static let allPreset = [
        "bank", "card", "wallet", "crypto", "cart", "gift", "mail", "chat", "social", "work", "home", "school",
        "cloud", "server", "database", "code", "terminal", "wifi", "globe", "game", "music", "video", "health",
        "travel", "car", "key", "lock", "shield", "star", "heart",
    ]

    /// The icon color IDs of core/item; "" is the neutral tile.
    static let allColor = ["red", "orange", "yellow", "green", "teal", "blue", "indigo", "purple", "pink", "gray"]

    /// The SF Symbol of each preset.
    static let symbolByPreset: [String: String] = [
        "bank": "building.columns", "card": "creditcard", "wallet": "wallet.bifold", "crypto": "bitcoinsign.circle",
        "cart": "cart", "gift": "gift", "mail": "envelope", "chat": "bubble.left", "social": "at", "work": "briefcase",
        "home": "house", "school": "graduationcap", "cloud": "cloud", "server": "server.rack", "database": "cylinder.split.1x2",
        "code": "chevron.left.forwardslash.chevron.right", "terminal": "terminal", "wifi": "wifi", "globe": "globe",
        "game": "gamecontroller", "music": "music.note", "video": "play.rectangle", "health": "heart.text.square",
        "travel": "airplane", "car": "car", "key": "key", "lock": "lock", "shield": "shield", "star": "star", "heart": "heart",
    ]

    /// The symbol to draw, or nil for an image or a preset from a newer app, which draws the category's symbol.
    var symbol: String? { image == nil ? Self.symbolByPreset[preset] : nil }
}

/// The names core/item stores an address's values under; identities and cards keep their address under the same names, and
/// copying and secret references use them.
enum AddressFieldName {
    static let street = "Street"
    static let city = "City"
    static let region = "Region"
    static let postalCode = "Postal code"
    static let country = "Country"
}

/// A postal address; `street` may hold several lines, such as an apartment on its second line. Identities and cards keep
/// one in the same keys.
struct ItemAddress: Codable, Hashable {
    var street = ""
    var city = ""
    var region = ""
    var postalCode = ""
    var country = ""

    init(street: String = "", city: String = "", region: String = "", postalCode: String = "", country: String = "") {
        self.street = street
        self.city = city
        self.region = region
        self.postalCode = postalCode
        self.country = country
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        street = try container.decodeIfPresent(String.self, forKey: .street) ?? ""
        city = try container.decodeIfPresent(String.self, forKey: .city) ?? ""
        region = try container.decodeIfPresent(String.self, forKey: .region) ?? ""
        postalCode = try container.decodeIfPresent(String.self, forKey: .postalCode) ?? ""
        country = try container.decodeIfPresent(String.self, forKey: .country) ?? ""
    }

    /// The address's line in lists: its city and country, or the first line of its street, such as "Springfield, US".
    var summary: String {
        let cityAndCountry = [city, country].filter { !$0.isEmpty }.joined(separator: ", ")
        if !cityAndCountry.isEmpty { return cityAndCountry }
        let firstLine = street.split(separator: "\n", maxSplits: 1).first.map(String.init) ?? ""
        return firstLine.isEmpty ? "Address" : firstLine
    }

    /// The values as named rows, in the order core/item stores them.
    var allNamedValue: [(name: String, value: String)] {
        [(AddressFieldName.street, street), (AddressFieldName.city, city), (AddressFieldName.region, region), (AddressFieldName.postalCode, postalCode), (AddressFieldName.country, country)]
    }
}

/// The names core/item stores an identity's values under; copying and secret references use them.
enum IdentityFieldName {
    static let firstName = "First name"
    static let middleName = "Middle name"
    static let lastName = "Last name"
    static let birthDate = "Birth date"
    static let email = "Email"
    static let phone = "Phone"
    /// The one-line address an identity had before it kept one in the address fields.
    static let address = "Address"
    static let passportNumber = "Passport number"
    static let driversLicenseNumber = "Driver's license number"
    static let nationalIDNumber = "National ID number"
}

/// A person's details; the document numbers are optional and concealed. The address is in `street` to `country`;
/// `address` is a one-line address saved before identities had those, kept as it is.
struct ItemIdentity: Codable, Hashable {
    var firstName = ""
    var middleName = ""
    var lastName = ""
    var birthDate = ""
    var email = ""
    var phone = ""
    var street = ""
    var city = ""
    var region = ""
    var postalCode = ""
    var country = ""
    var address = ""
    var passportNumber = ""
    var driversLicenseNumber = ""
    var nationalIDNumber = ""

    enum CodingKeys: String, CodingKey {
        case firstName, middleName, lastName, birthDate, email, phone, street, city, region, postalCode, country, address, passportNumber, driversLicenseNumber
        case nationalIDNumber = "nationalIdNumber"
    }

    /// The identity's address, for the editor and detail view an address item uses.
    var postalAddress: ItemAddress {
        get { ItemAddress(street: street, city: city, region: region, postalCode: postalCode, country: country) }
        set { (street, city, region, postalCode, country) = (newValue.street, newValue.city, newValue.region, newValue.postalCode, newValue.country) }
    }

    init() {}

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        firstName = try container.decodeIfPresent(String.self, forKey: .firstName) ?? ""
        middleName = try container.decodeIfPresent(String.self, forKey: .middleName) ?? ""
        lastName = try container.decodeIfPresent(String.self, forKey: .lastName) ?? ""
        birthDate = try container.decodeIfPresent(String.self, forKey: .birthDate) ?? ""
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        phone = try container.decodeIfPresent(String.self, forKey: .phone) ?? ""
        postalAddress = try ItemAddress(from: decoder)
        address = try container.decodeIfPresent(String.self, forKey: .address) ?? ""
        passportNumber = try container.decodeIfPresent(String.self, forKey: .passportNumber) ?? ""
        driversLicenseNumber = try container.decodeIfPresent(String.self, forKey: .driversLicenseNumber) ?? ""
        nationalIDNumber = try container.decodeIfPresent(String.self, forKey: .nationalIDNumber) ?? ""
    }

    /// The name parts that are set, such as "Alice M. Example".
    var fullName: String { [firstName, middleName, lastName].filter { !$0.isEmpty }.joined(separator: " ") }

    /// The identity's line in lists: the full name, or the email when it has no name.
    var summary: String { fullName.isEmpty ? (email.isEmpty ? "Identity" : email) : fullName }
}

/// A day without a time or time zone, stored as YYYY-MM-DD: an identity's birth date or an API credential's expiry.
enum CalendarDate {
    private static let formatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(identifier: "UTC")
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter
    }()

    /// The day a stored YYYY-MM-DD date names, as midnight UTC, or nil for any other text. A day that does not exist, such
    /// as 2023-02-31, is nil too: the formatter would roll it over into March.
    static func parse(_ text: String) -> Date? {
        guard let date = formatter.date(from: text), formatter.string(from: date) == text else {
            return nil
        }
        return date
    }

    /// The day as it is stored, YYYY-MM-DD; a date picker in UTC gives midnight UTC of the picked day.
    static func format(_ date: Date) -> String { formatter.string(from: date) }

    /// Today on the device's calendar, as stored. The formatter works in UTC, which is a day ahead or behind near midnight:
    /// at 20:00 on October 15 in Los Angeles it is already October 16 in UTC.
    static func formatToday(now: Date = Date(), calendar: Calendar = .current) -> String {
        let parts = calendar.dateComponents([.year, .month, .day], from: now)
        return String(format: "%04d-%02d-%02d", parts.year ?? 1970, parts.month ?? 1, parts.day ?? 1)
    }

    /// Today on the device's calendar as the UTC day a date picker shows, such as the latest birth date it offers.
    static func getToday(now: Date = Date(), calendar: Calendar = .current) -> Date {
        parse(formatToday(now: now, calendar: calendar)) ?? now
    }
}

/// The names core/item stores an API credential's values under; copying and hisar:// references use them.
enum APICredentialFieldName {
    static let service = "Service"
    static let keyID = "Key ID"
    static let secret = "Secret"
    static let host = "Host"
    static let expiry = "Expiry"
}

/// An API key; the secret is concealed. `expiryWarning` comes from the backend once the expiry has passed.
struct ItemAPICredential: Codable, Hashable {
    var service = ""
    var keyID = ""
    var secret = ""
    var host = ""
    var expiry = ""
    var expiryWarning = ""

    enum CodingKeys: String, CodingKey {
        case service, secret, host, expiry, expiryWarning
        case keyID = "keyId"
    }

    init() {}

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        service = try container.decodeIfPresent(String.self, forKey: .service) ?? ""
        keyID = try container.decodeIfPresent(String.self, forKey: .keyID) ?? ""
        secret = try container.decodeIfPresent(String.self, forKey: .secret) ?? ""
        host = try container.decodeIfPresent(String.self, forKey: .host) ?? ""
        expiry = try container.decodeIfPresent(String.self, forKey: .expiry) ?? ""
        expiryWarning = try container.decodeIfPresent(String.self, forKey: .expiryWarning) ?? ""
    }

    static let expiredWarning = "This credential has expired"

    /// The warning for a YYYY-MM-DD expiry once its day has passed on the device's calendar, as core/item
    /// DescribeAPICredentialExpiry gives it; "" for any other value.
    static func describeExpiry(_ expiry: String, now: Date = Date(), calendar: Calendar = .current) -> String {
        guard CalendarDate.parse(expiry) != nil else {
            return ""
        }
        // YYYY-MM-DD dates sort as text.
        return expiry < CalendarDate.formatToday(now: now, calendar: calendar) ? expiredWarning : ""
    }

    /// The editor's warning, which follows the expiry as it is edited rather than the one the backend gave for the saved
    /// value. A saved value in another form that the backend still reads as a date, such as 2020/01/01, keeps the
    /// backend's warning until it changes.
    static func describeEditedExpiry(_ expiry: String, saved: ItemAPICredential?, now: Date = Date(), calendar: Calendar = .current) -> String {
        if let saved, CalendarDate.parse(expiry) == nil, expiry == saved.expiry {
            return saved.expiryWarning
        }
        return describeExpiry(expiry, now: now, calendar: calendar)
    }

    /// The credential's line in lists: its service and host, such as "Stripe · api.stripe.com".
    var summary: String {
        let parts = [service, host].filter { !$0.isEmpty }
        return parts.isEmpty ? "API credential" : parts.joined(separator: " · ")
    }
}

/// The names core/item stores a server's values under; its username, password and admin console are the item's own.
enum ServerFieldName {
    static let host = "Host"
    static let port = "Port"
    static let sshKey = "SSH key"
}

/// A server's values besides its login data. `sshKeyID` names an SSH key item in the same vault; the backend fills in its
/// title and fingerprint, which are empty when the key is gone.
struct ItemServer: Codable, Hashable {
    var host = ""
    var port = ""
    var sshKeyID = ""
    var sshKeyTitle = ""
    var sshKeyFingerprint = ""
    var portWarning = ""

    enum CodingKeys: String, CodingKey {
        case host, port, sshKeyTitle, sshKeyFingerprint, portWarning
        case sshKeyID = "sshKeyId"
    }

    init(host: String = "", port: String = "", sshKeyID: String = "", sshKeyTitle: String = "", sshKeyFingerprint: String = "", portWarning: String = "") {
        self.host = host
        self.port = port
        self.sshKeyID = sshKeyID
        self.sshKeyTitle = sshKeyTitle
        self.sshKeyFingerprint = sshKeyFingerprint
        self.portWarning = portWarning
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        host = try container.decodeIfPresent(String.self, forKey: .host) ?? ""
        port = try container.decodeIfPresent(String.self, forKey: .port) ?? ""
        sshKeyID = try container.decodeIfPresent(String.self, forKey: .sshKeyID) ?? ""
        sshKeyTitle = try container.decodeIfPresent(String.self, forKey: .sshKeyTitle) ?? ""
        sshKeyFingerprint = try container.decodeIfPresent(String.self, forKey: .sshKeyFingerprint) ?? ""
        portWarning = try container.decodeIfPresent(String.self, forKey: .portWarning) ?? ""
    }

    static let portWarningText = "The port is not a number from 1 to 65535"

    /// The editor's warning for a port that is not a number from 1 to 65535, as core/item gives it; "" for an empty port.
    static func describePort(_ port: String) -> String {
        let trimmedPort = port.trimmingCharacters(in: .whitespaces)
        guard !trimmedPort.isEmpty else { return "" }
        guard trimmedPort.allSatisfy(\.isASCII), trimmedPort.allSatisfy(\.isNumber), let number = Int(trimmedPort), (1...65535).contains(number) else {
            return portWarningText
        }
        return ""
    }

    /// The host and port, such as "build.example.com:2222", with brackets around an IPv6 host.
    var address: String {
        guard !host.isEmpty, !port.isEmpty else { return host }
        return host.contains(":") ? "[\(host)]:\(port)" : "\(host):\(port)"
    }

    /// The server's line in lists: its address and username, as the Mac app shows it.
    func summary(username: String) -> String {
        let parts = [address, username].filter { !$0.isEmpty }
        return parts.isEmpty ? "Server" : parts.joined(separator: " · ")
    }
}

/// The names core/item stores a database's values under; its username and password are the item's own. "connection string"
/// names the connection string the backend builds, for copying and hisar:// references.
enum DatabaseFieldName {
    static let engine = "Engine"
    static let host = "Host"
    static let port = "Port"
    static let name = "Database"
    static let options = "Options"
    static let connectionString = "connection string"
}

/// A database engine the editor offers and Hisar builds connection strings for, as core/item lists them.
struct DatabaseEngine: Hashable {
    let id: String
    let name: String

    static let allKnown = [
        DatabaseEngine(id: "postgresql", name: "PostgreSQL"),
        DatabaseEngine(id: "mysql", name: "MySQL"),
        DatabaseEngine(id: "mariadb", name: "MariaDB"),
        DatabaseEngine(id: "sqlserver", name: "SQL Server"),
        DatabaseEngine(id: "oracle", name: "Oracle"),
        DatabaseEngine(id: "mongodb", name: "MongoDB"),
        DatabaseEngine(id: "redis", name: "Redis"),
    ]

    static func isKnown(_ engine: String) -> Bool { allKnown.contains { $0.id == engine } }
}

/// A database's values besides its login data. `engine` is a known engine's ID or any other engine as typed, which
/// `engineName` names; `connectionString` is "" for another engine or without a host. The backend reads only engine, host,
/// port, name and options.
struct ItemDatabase: Codable, Hashable {
    var engine = ""
    var engineName = ""
    var host = ""
    var port = ""
    var name = ""
    var options = ""
    var connectionString = ""
    var portWarning = ""

    init(engine: String = "", engineName: String = "", host: String = "", port: String = "", name: String = "", options: String = "", connectionString: String = "", portWarning: String = "") {
        self.engine = engine
        self.engineName = engineName
        self.host = host
        self.port = port
        self.name = name
        self.options = options
        self.connectionString = connectionString
        self.portWarning = portWarning
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        engine = try container.decodeIfPresent(String.self, forKey: .engine) ?? ""
        engineName = try container.decodeIfPresent(String.self, forKey: .engineName) ?? ""
        host = try container.decodeIfPresent(String.self, forKey: .host) ?? ""
        port = try container.decodeIfPresent(String.self, forKey: .port) ?? ""
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        options = try container.decodeIfPresent(String.self, forKey: .options) ?? ""
        connectionString = try container.decodeIfPresent(String.self, forKey: .connectionString) ?? ""
        portWarning = try container.decodeIfPresent(String.self, forKey: .portWarning) ?? ""
    }

    /// The host and port, such as "db.example.com:5432", with brackets around an IPv6 host, as a server's.
    var address: String { ItemServer(host: host, port: port).address }

    /// The address and database name, such as "db.example.com:5432/app".
    var location: String {
        guard !name.isEmpty else { return address }
        return address.isEmpty ? name : "\(address)/\(name)"
    }

    /// The database's line in lists: its engine, address and name, as the Mac app shows it.
    var summary: String {
        let parts = [engineName, location].filter { !$0.isEmpty }
        return parts.isEmpty ? "Database" : parts.joined(separator: " · ")
    }
}

/// The names core/item stores a software license's values under; copying and hisar:// references use them.
enum SoftwareLicenseFieldName {
    static let product = "Product"
    static let version = "Version"
    static let licenseKey = "License key"
    static let licensedTo = "Licensed to"
    static let email = "Email"
    static let purchaseDate = "Purchase date"
    static let downloadURL = "Download URL"
}

/// A software license; the license key is concealed. The download URL is a field, not the item's website, so AutoFill
/// never offers the license on that site.
struct ItemSoftwareLicense: Codable, Hashable {
    var product = ""
    var version = ""
    var licenseKey = ""
    var licensedTo = ""
    var email = ""
    var purchaseDate = ""
    var downloadURL = ""

    enum CodingKeys: String, CodingKey {
        case product, version, licenseKey, licensedTo, email, purchaseDate
        case downloadURL = "downloadUrl"
    }

    init(product: String = "", version: String = "", licenseKey: String = "", licensedTo: String = "", email: String = "", purchaseDate: String = "", downloadURL: String = "") {
        self.product = product
        self.version = version
        self.licenseKey = licenseKey
        self.licensedTo = licensedTo
        self.email = email
        self.purchaseDate = purchaseDate
        self.downloadURL = downloadURL
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        product = try container.decodeIfPresent(String.self, forKey: .product) ?? ""
        version = try container.decodeIfPresent(String.self, forKey: .version) ?? ""
        licenseKey = try container.decodeIfPresent(String.self, forKey: .licenseKey) ?? ""
        licensedTo = try container.decodeIfPresent(String.self, forKey: .licensedTo) ?? ""
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        purchaseDate = try container.decodeIfPresent(String.self, forKey: .purchaseDate) ?? ""
        downloadURL = try container.decodeIfPresent(String.self, forKey: .downloadURL) ?? ""
    }

    /// The license's line in lists: its version and who it is licensed to, such as "Version 100 · Alice Example", or its
    /// email or product when it has neither, as the Mac app shows it.
    var summary: String {
        let versionText = version.first?.isNumber == true ? "Version \(version)" : version
        let parts = [versionText, licensedTo.isEmpty ? email : licensedTo].filter { !$0.isEmpty }
        if !parts.isEmpty { return parts.joined(separator: " · ") }
        return product.isEmpty ? "Software license" : product
    }
}

/// One of the user's starred items with the vault it is in; the item carries no secrets, as in a vault's list.
struct FavoriteItem: Codable, Hashable, Identifiable {
    var vaultID: String
    var vaultName: String
    var vaultType: String
    var item: ItemDetail

    var id: String { LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: item.id) }

    enum CodingKeys: String, CodingKey {
        case vaultName, vaultType, item
        case vaultID = "vaultId"
    }
}

/// The names core/item stores a card's values under; copying and secret references use them.
enum CardFieldName {
    static let cardholder = "Cardholder"
    static let type = "Type"
    static let number = "Card number"
    static let expiry = "Expiry"
    static let securityCode = "Security code"
    static let pin = "PIN"
    /// The one-line address a card had before it kept one in the address fields.
    static let billingAddress = "Billing address"
}

/// A card's values. `type` is the brand saved with the card, which the user may choose; `brand` is the one its number
/// points to.
struct ItemCard: Codable, Hashable {
    var cardholder = ""
    var type = ""
    var number = ""
    var expiry = ""
    var securityCode = ""
    var pin = ""
    var street = ""
    var city = ""
    var region = ""
    var postalCode = ""
    var country = ""
    /// A one-line address saved before cards kept one in `street` to `country`, kept as it is.
    var billingAddress = ""
    var brand = ""
    var last4 = ""
    var numberWarning = ""
    var expiryWarning = ""

    enum CodingKeys: String, CodingKey {
        case cardholder, type, number, expiry, securityCode, pin, street, city, region, postalCode, country, billingAddress, brand, last4, numberWarning, expiryWarning
    }

    /// The card's billing address, for the editor and detail view an address item uses.
    var postalAddress: ItemAddress {
        get { ItemAddress(street: street, city: city, region: region, postalCode: postalCode, country: country) }
        set { (street, city, region, postalCode, country) = (newValue.street, newValue.city, newValue.region, newValue.postalCode, newValue.country) }
    }

    init() {}

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        cardholder = try container.decodeIfPresent(String.self, forKey: .cardholder) ?? ""
        type = try container.decodeIfPresent(String.self, forKey: .type) ?? ""
        number = try container.decodeIfPresent(String.self, forKey: .number) ?? ""
        expiry = try container.decodeIfPresent(String.self, forKey: .expiry) ?? ""
        securityCode = try container.decodeIfPresent(String.self, forKey: .securityCode) ?? ""
        pin = try container.decodeIfPresent(String.self, forKey: .pin) ?? ""
        postalAddress = try ItemAddress(from: decoder)
        billingAddress = try container.decodeIfPresent(String.self, forKey: .billingAddress) ?? ""
        brand = try container.decodeIfPresent(String.self, forKey: .brand) ?? ""
        last4 = try container.decodeIfPresent(String.self, forKey: .last4) ?? ""
        numberWarning = try container.decodeIfPresent(String.self, forKey: .numberWarning) ?? ""
        expiryWarning = try container.decodeIfPresent(String.self, forKey: .expiryWarning) ?? ""
    }

    /// The brands an editor offers as a card's type, in the order of core/item ListCardBrands.
    static let allBrand = ["Visa", "Mastercard", "American Express", "Discover", "Diners Club", "JCB", "UnionPay", "Maestro", "Troy"]

    /// The card's line in lists: its type (or the brand its number points to) and last four digits, such as "Visa •••• 4242".
    var summary: String {
        let parts = [type.isEmpty ? brand : type, last4.isEmpty ? "" : "•••• \(last4)"].filter { !$0.isEmpty }
        return parts.isEmpty ? "Credit card" : parts.joined(separator: " ")
    }
}

struct CardDescription: Codable, Equatable {
    var brand = ""
    var last4 = ""
    var numberWarning = ""
    var expiryWarning = ""
}

struct SignUpResult: Codable, Equatable {
    var status: VaultStatus
    var secretKey: String
    var pendingOperationId: String = ""
    var setupState: String = ""
    var problems: [String] = []

    init(status: VaultStatus, secretKey: String, pendingOperationId: String = "", setupState: String = "", problems: [String] = []) {
        self.status = status
        self.secretKey = secretKey
        self.pendingOperationId = pendingOperationId
        self.setupState = setupState
        self.problems = problems
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        status = try values.decode(VaultStatus.self, forKey: .status)
        secretKey = try values.decodeIfPresent(String.self, forKey: .secretKey) ?? ""
        pendingOperationId = try values.decodeIfPresent(String.self, forKey: .pendingOperationId) ?? ""
        setupState = try values.decodeIfPresent(String.self, forKey: .setupState) ?? ""
        problems = try values.decodeIfPresent([String].self, forKey: .problems) ?? []
    }
}

struct PendingRecovery: Codable, Equatable, Identifiable {
    var operationId: String
    var kind: String
    var origin: String
    var accountId: String
    var email: String
    var name: String
    var phase: String
    var kitSaved: Bool
    var problems: [String]
    var manualUnlock: Bool = false
    var keychainProtected: Bool = false
    var id: String { operationId }
}

struct PendingRecoveryExport: Codable, Equatable {
    var secretKey: String
    var path: String
}

struct LoginMatch: Codable, Hashable, Identifiable {
    var vaultID: String
    var itemID: String
    var title: String
    var username: String
    var vaultName: String
    var hasOneTimePassword: Bool
    var isPersonal = false

    init(vaultID: String, itemID: String, title: String, username: String, vaultName: String, hasOneTimePassword: Bool = false, isPersonal: Bool = false) {
        self.vaultID = vaultID
        self.itemID = itemID
        self.title = title
        self.username = username
        self.vaultName = vaultName
        self.hasOneTimePassword = hasOneTimePassword
        self.isPersonal = isPersonal
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        vaultID = try container.decode(String.self, forKey: .vaultID)
        itemID = try container.decode(String.self, forKey: .itemID)
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        username = try container.decodeIfPresent(String.self, forKey: .username) ?? ""
        vaultName = try container.decodeIfPresent(String.self, forKey: .vaultName) ?? ""
        hasOneTimePassword = try container.decodeIfPresent(Bool.self, forKey: .hasOneTimePassword) ?? false
        isPersonal = try container.decodeIfPresent(Bool.self, forKey: .isPersonal) ?? false
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

struct ItemAndroidApp: Codable, Hashable, Identifiable {
    var packageName: String
    /// The signing certificate's SHA-256 as upper-case hex bytes joined by colons.
    var certificateSHA256: String

    enum CodingKeys: String, CodingKey {
        case packageName
        case certificateSHA256 = "certificateSha256"
    }

    var id: String { packageName + " " + certificateSHA256 }
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
    var userVerification = "preferred"
    var isUserVerificationDiscouraged: Bool { userVerification == "discouraged" }
}

struct PasskeyRegistrationRequest: Equatable {
    static let algorithmES256 = -7

    var relyingPartyID: String
    var userName: String
    var userHandle: Data
    var clientDataHash: Data
    var allSupportedAlgorithm: [Int] = []
    var allExcludedCredentialID: [Data] = []
    var userVerification = "preferred"
    var isUserVerificationDiscouraged: Bool { userVerification == "discouraged" }

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

// Member is a person you share a vault with (role admin or member on the server), someone with access to a vault (role
// owner, editor or reader) or a member of an organization (role admin or member of the organization).
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
    var requiresConfirmation: Bool? = nil
    var recipientID: String? = nil
    var securityCode: String? = nil
    var reviewToken: String? = nil

    enum CodingKeys: String, CodingKey {
        case id, vaultName, email, role, state, expiresAt
        case vaultID = "vaultId"
        case requiresConfirmation, securityCode, reviewToken
        case recipientID = "recipientId"
    }
}

// WaitingShareInvitation is a share this user accepted that the owner's app has not finished yet.
struct WaitingShareInvitation: Codable, Identifiable, Hashable {
    var id: String
    var vaultID: String
    var role: String
    var invitedByName: String
    var invitedByEmail: String
    var acceptedAt: String

    var inviterName: String { invitedByName.isEmpty || invitedByName == invitedByEmail ? invitedByEmail : invitedByName }

    enum CodingKeys: String, CodingKey {
        case id, role, invitedByName, invitedByEmail, acceptedAt
        case vaultID = "vaultId"
    }

    func describe(now: Date) -> String {
        let since = formatTimeAgo(acceptedAt, now: now)
        return "Waiting for \(inviterName)'s Hisar to finish sharing" + (since.isEmpty ? "" : " · accepted \(since)")
    }
}

// ShareAwaitingUnlock is a share someone accepted that this locked app finishes once unlocked; the vault name is encrypted until then.
struct ShareAwaitingUnlock: Codable, Identifiable, Hashable {
    var invitationID: String
    var recipientName: String
    var recipientEmail: String
    var acceptedAt: String

    var id: String { invitationID }
    var recipientDisplayName: String { recipientName.isEmpty || recipientName == recipientEmail ? recipientEmail : recipientName }
    var message: String { "\(recipientDisplayName) accepted your invitation to a vault. Unlock to continue sharing." }

    enum CodingKeys: String, CodingKey {
        case recipientName, recipientEmail, acceptedAt
        case invitationID = "invitationId"
    }
}

// formatTimeAgo is a short "since" for banners: "just now", "5 min ago", "2 h ago", "yesterday", "3 days ago".
func formatTimeAgo(_ timestamp: String, now: Date) -> String {
    guard let date = ISO8601DateFormatter().date(from: timestamp) else { return "" }
    let minuteCount = Int(now.timeIntervalSince(date) / 60)
    if minuteCount < 1 { return "just now" }
    if minuteCount < 60 { return "\(minuteCount) min ago" }
    let hourCount = minuteCount / 60
    if hourCount < 24 { return "\(hourCount) h ago" }
    let dayCount = hourCount / 24
    return dayCount == 1 ? "yesterday" : "\(dayCount) days ago"
}

struct ShareInvitations: Codable, Hashable {
    var allReceived: [ReceivedShareInvitation]
    var allSent: [SentShareInvitation]
    var allWaiting: [WaitingShareInvitation]
    var completionError: String

    var allRequiringReview: [SentShareInvitation] {
        allSent.filter { $0.state == SentShareInvitation.stateAccepted && $0.requiresConfirmation == true }
    }

    enum CodingKeys: String, CodingKey {
        case allReceived = "received"
        case allSent = "sent"
        case allWaiting = "waiting"
        case completionError
    }

    init(allReceived: [ReceivedShareInvitation] = [], allSent: [SentShareInvitation] = [], allWaiting: [WaitingShareInvitation] = [], completionError: String = "") {
        self.allReceived = allReceived
        self.allSent = allSent
        self.allWaiting = allWaiting
        self.completionError = completionError
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        allReceived = try container.decodeIfPresent([ReceivedShareInvitation].self, forKey: .allReceived) ?? []
        allSent = try container.decodeIfPresent([SentShareInvitation].self, forKey: .allSent) ?? []
        allWaiting = try container.decodeIfPresent([WaitingShareInvitation].self, forKey: .allWaiting) ?? []
        completionError = try container.decodeIfPresent(String.self, forKey: .completionError) ?? ""
    }
}

struct VaultSharing: Codable, Hashable {
    var vault: VaultSummary
    var isOwner: Bool
    var allMember: [Member]
    var allInvitation: [SentShareInvitation]
    var completionError: String

    enum CodingKeys: String, CodingKey {
        case vault, isOwner, completionError
        case allMember = "members"
        case allInvitation = "invitations"
    }

    init(vault: VaultSummary, isOwner: Bool, allMember: [Member] = [], allInvitation: [SentShareInvitation] = [], completionError: String = "") {
        self.vault = vault
        self.isOwner = isOwner
        self.allMember = allMember
        self.allInvitation = allInvitation
        self.completionError = completionError
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        vault = try container.decode(VaultSummary.self, forKey: .vault)
        isOwner = try container.decodeIfPresent(Bool.self, forKey: .isOwner) ?? false
        allMember = try container.decodeIfPresent([Member].self, forKey: .allMember) ?? []
        allInvitation = try container.decodeIfPresent([SentShareInvitation].self, forKey: .allInvitation) ?? []
        completionError = try container.decodeIfPresent(String.self, forKey: .completionError) ?? ""
    }
}

// Device is a signed-in device of the account; lastSeenAt is RFC 3339.
/// PushKind is what a push notification is about; the push carries nothing else.
enum PushKind: String, Codable, CaseIterable, Hashable {
    case approval
    case security
    case sharing
}

/// PushSettings is app.PushSettings: whether notifications can reach this iPhone, and the kinds turned on.
struct PushSettings: Codable, Equatable {
    var isAvailable: Bool
    var isServerSending: Bool
    var enabledKinds: [String]
    var errorMessage: String?

    static let unavailable = PushSettings(isAvailable: false, isServerSending: false, enabledKinds: PushKind.allCases.map(\.rawValue), errorMessage: nil)

    func isEnabled(_ kind: PushKind) -> Bool {
        enabledKinds.contains(kind.rawValue)
    }
}

struct Device: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var platform: String
    var createdAt: String
    var lastSeenAt: String
    var isCurrent: Bool

    init(id: String, name: String, platform: String, createdAt: String = "", lastSeenAt: String = "", isCurrent: Bool = false) {
        self.id = id
        self.name = name
        self.platform = platform
        self.createdAt = createdAt
        self.lastSeenAt = lastSeenAt
        self.isCurrent = isCurrent
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        platform = try container.decodeIfPresent(String.self, forKey: .platform) ?? ""
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
        lastSeenAt = try container.decodeIfPresent(String.self, forKey: .lastSeenAt) ?? ""
        isCurrent = try container.decodeIfPresent(Bool.self, forKey: .isCurrent) ?? false
    }

    var lastSeenDate: Date? { PhoneApproval.parseDate(lastSeenAt) }
    var createdDate: Date? { PhoneApproval.parseDate(createdAt) }

    private var operatingSystem: String { platform.split(separator: "/").first.map(String.init) ?? "" }
    private var client: String { platform.split(separator: "/").dropFirst().first.map(String.init) ?? "" }

    var platformDescription: String {
        let system: String
        switch operatingSystem {
        case "ios": system = "iOS"
        case "android": system = "Android"
        case "darwin", "macos": system = "macOS"
        case "linux": system = "Linux"
        case "windows": system = "Windows"
        default: system = operatingSystem.isEmpty ? "Unknown" : operatingSystem
        }
        switch client {
        case "mobile": return "\(system) app"
        case "desktop": return "\(system) app"
        case "cli": return "Command line on \(system)"
        case "extension", "browser": return "Browser extension on \(system)"
        default: return client.isEmpty ? system : "\(system), \(client)"
        }
    }

    var systemImage: String {
        if client == "cli" { return "apple.terminal" }
        if client == "extension" || client == "browser" { return "puzzlepiece.extension" }
        switch operatingSystem {
        case "ios": return "iphone"
        case "android": return "smartphone"
        case "darwin", "macos": return "laptopcomputer"
        default: return "desktopcomputer"
        }
    }
}

struct EmergencyKitExport: Codable, Equatable {
    var secretKey: String
    var emergencyKitPath: String
}

// VaultActions says who may rename, delete or leave a vault and what deleting it means; members leave out the user.
struct VaultActions: Codable, Hashable {
    var vault: VaultSummary
    var canRename: Bool
    var canDelete: Bool
    var deleteBlockedReason: String
    var canLeave: Bool
    var leaveBlockedReason: String
    var allMemberLosingAccess: [Member]
    var allTargetVault: [VaultSummary]

    enum CodingKeys: String, CodingKey {
        case vault, canRename, canDelete, deleteBlockedReason, canLeave, leaveBlockedReason
        case allMemberLosingAccess = "membersLosingAccess"
        case allTargetVault = "targetVaults"
    }

    init(vault: VaultSummary, canRename: Bool = false, canDelete: Bool = false, deleteBlockedReason: String = "", canLeave: Bool = false, leaveBlockedReason: String = "", allMemberLosingAccess: [Member] = [], allTargetVault: [VaultSummary] = []) {
        self.vault = vault
        self.canRename = canRename
        self.canDelete = canDelete
        self.deleteBlockedReason = deleteBlockedReason
        self.canLeave = canLeave
        self.leaveBlockedReason = leaveBlockedReason
        self.allMemberLosingAccess = allMemberLosingAccess
        self.allTargetVault = allTargetVault
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        vault = try container.decode(VaultSummary.self, forKey: .vault)
        canRename = try container.decodeIfPresent(Bool.self, forKey: .canRename) ?? false
        canDelete = try container.decodeIfPresent(Bool.self, forKey: .canDelete) ?? false
        deleteBlockedReason = try container.decodeIfPresent(String.self, forKey: .deleteBlockedReason) ?? ""
        canLeave = try container.decodeIfPresent(Bool.self, forKey: .canLeave) ?? false
        leaveBlockedReason = try container.decodeIfPresent(String.self, forKey: .leaveBlockedReason) ?? ""
        allMemberLosingAccess = try container.decodeIfPresent([Member].self, forKey: .allMemberLosingAccess) ?? []
        allTargetVault = try container.decodeIfPresent([VaultSummary].self, forKey: .allTargetVault) ?? []
    }
}

// VaultDeletionRequest: moveToVaultID moves every item first; isDeletingItems needs confirmedName, the vault's name.
struct VaultDeletionRequest: Codable, Equatable {
    var moveToVaultID: String = ""
    var isDeletingItems = false
    var confirmedName = ""

    enum CodingKeys: String, CodingKey {
        case moveToVaultID = "moveToVaultId"
        case isDeletingItems, confirmedName
    }
}

struct VaultDeletionResult: Codable, Equatable {
    var movedCount: Int
    var targetVaultName: String

    init(movedCount: Int = 0, targetVaultName: String = "") {
        self.movedCount = movedCount
        self.targetVaultName = targetVaultName
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        movedCount = try container.decodeIfPresent(Int.self, forKey: .movedCount) ?? 0
        targetVaultName = try container.decodeIfPresent(String.self, forKey: .targetVaultName) ?? ""
    }
}

// RemovedVaultNotice is a vault someone else deleted, or that the user lost access to.
struct RemovedVaultNotice: Codable, Hashable, Identifiable {
    var id: String
    var name: String
    var message: String
}

struct ExportOptions: Codable, Equatable {
    var isBiometricAvailable: Bool
    var minPassphraseLength: Int
}

struct ExportResult: Codable, Equatable {
    var path: String
    var vaultCount: Int
    var itemCount: Int
    var warning: String? = nil
}

enum ExportFormat: String, CaseIterable, Identifiable {
    case encrypted
    case plain

    var id: String { rawValue }
    var label: String { self == .encrypted ? "Encrypted export" : "Plain JSON" }
}

/// ImportSkippedEntry is an item an import leaves out or failed to save, with why.
struct ImportSkippedEntry: Codable, Hashable {
    var title: String
    var reason: String
}

/// ImportDuplicate is an item the chosen vault already has. passkeyCount is how many of its passkeys the existing login
/// lacks; skipping duplicates adds them to it.
struct ImportDuplicate: Codable, Hashable {
    var title: String
    var username: String
    var url: String
    var detail: String
    var passkeyCount: Int

    init(title: String, username: String = "", url: String = "", detail: String = "", passkeyCount: Int = 0) {
        self.title = title
        self.username = username
        self.url = url
        self.detail = detail
        self.passkeyCount = passkeyCount
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        username = try container.decodeIfPresent(String.self, forKey: .username) ?? ""
        url = try container.decodeIfPresent(String.self, forKey: .url) ?? ""
        detail = try container.decodeIfPresent(String.self, forKey: .detail) ?? ""
        passkeyCount = try container.decodeIfPresent(Int.self, forKey: .passkeyCount) ?? 0
    }
}

/// ImportPreview is what an import would bring into vaultID. For a Credential Exchange import fileName names the app the
/// items came from and filePath is empty.
struct ImportSourceGroup: Codable, Equatable, Identifiable {
    var id: String
    var name: String
    var isPrivate: Bool
    var itemCount: Int
    var vaultID: String
    var vaultName: String
    var members: [Member]

    enum CodingKeys: String, CodingKey {
        case id, name, isPrivate, itemCount, vaultName, members
        case vaultID = "vaultId"
    }
}

struct ImportPreview: Codable, Equatable {
    static let formatCredentialExchange = "credential-exchange"
    static let formatHisarExport = "hisar-export"
    static let formatHisarEncryptedExport = "hisar-encrypted-export"

    var fileName: String
    var filePath: String = ""
    var format: String = ""
    var formatName: String = ""
    var vaultID: String
    var vaultName: String
    var itemCount: Int = 0
    var loginCount: Int = 0
    var secureNoteCount: Int = 0
    var sshKeyCount: Int = 0
    var creditCardCount: Int = 0
    var identityCount: Int = 0
    var apiCredentialCount: Int = 0
    var addressCount: Int = 0
    var softwareLicenseCount: Int = 0
    var serverCount: Int = 0
    var databaseCount: Int = 0
    var passkeyCount: Int = 0
    var allSkipped: [ImportSkippedEntry] = []
    var allDuplicate: [ImportDuplicate] = []
    var groups: [ImportSourceGroup] = []
    /// isPassphraseRequired: the file is an encrypted Hisar export; openImportFile opens it with the export passphrase.
    var isPassphraseRequired = false
    /// notice describes a Hisar export that is a cached or partial copy; shown as a warning in the preview.
    var notice = ""
    /// isSourceKept: the file stays on disk (an encrypted export, a backup, or a copy that may not hold everything).
    var isSourceKept = false
    /// exportedFrom and exportedAt name the account and time a Hisar export was made; either may be empty.
    var exportedFrom = ""
    var exportedAt = ""

    enum CodingKeys: String, CodingKey {
        case fileName, filePath, format, formatName, vaultName, itemCount, loginCount, secureNoteCount, sshKeyCount
        case creditCardCount, identityCount, apiCredentialCount, addressCount, softwareLicenseCount, serverCount
        case databaseCount, passkeyCount, allSkipped, allDuplicate
        case groups, isPassphraseRequired, notice, isSourceKept, exportedFrom, exportedAt
        case vaultID = "vaultId"
    }

    init(fileName: String, vaultID: String, vaultName: String, itemCount: Int = 0, loginCount: Int = 0, passkeyCount: Int = 0,
         allSkipped: [ImportSkippedEntry] = [], allDuplicate: [ImportDuplicate] = []) {
        self.fileName = fileName
        self.format = Self.formatCredentialExchange
        self.formatName = "Credential Exchange"
        self.vaultID = vaultID
        self.vaultName = vaultName
        self.itemCount = itemCount
        self.loginCount = loginCount
        self.passkeyCount = passkeyCount
        self.allSkipped = allSkipped
        self.allDuplicate = allDuplicate
    }

    // Go writes empty lists as null.
    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        fileName = try container.decodeIfPresent(String.self, forKey: .fileName) ?? ""
        filePath = try container.decodeIfPresent(String.self, forKey: .filePath) ?? ""
        format = try container.decodeIfPresent(String.self, forKey: .format) ?? ""
        formatName = try container.decodeIfPresent(String.self, forKey: .formatName) ?? ""
        vaultID = try container.decodeIfPresent(String.self, forKey: .vaultID) ?? ""
        vaultName = try container.decodeIfPresent(String.self, forKey: .vaultName) ?? ""
        itemCount = try container.decodeIfPresent(Int.self, forKey: .itemCount) ?? 0
        loginCount = try container.decodeIfPresent(Int.self, forKey: .loginCount) ?? 0
        secureNoteCount = try container.decodeIfPresent(Int.self, forKey: .secureNoteCount) ?? 0
        sshKeyCount = try container.decodeIfPresent(Int.self, forKey: .sshKeyCount) ?? 0
        creditCardCount = try container.decodeIfPresent(Int.self, forKey: .creditCardCount) ?? 0
        identityCount = try container.decodeIfPresent(Int.self, forKey: .identityCount) ?? 0
        apiCredentialCount = try container.decodeIfPresent(Int.self, forKey: .apiCredentialCount) ?? 0
        addressCount = try container.decodeIfPresent(Int.self, forKey: .addressCount) ?? 0
        softwareLicenseCount = try container.decodeIfPresent(Int.self, forKey: .softwareLicenseCount) ?? 0
        serverCount = try container.decodeIfPresent(Int.self, forKey: .serverCount) ?? 0
        databaseCount = try container.decodeIfPresent(Int.self, forKey: .databaseCount) ?? 0
        passkeyCount = try container.decodeIfPresent(Int.self, forKey: .passkeyCount) ?? 0
        allSkipped = try container.decodeIfPresent([ImportSkippedEntry].self, forKey: .allSkipped) ?? []
        allDuplicate = try container.decodeIfPresent([ImportDuplicate].self, forKey: .allDuplicate) ?? []
        groups = try container.decodeIfPresent([ImportSourceGroup].self, forKey: .groups) ?? []
        isPassphraseRequired = try container.decodeIfPresent(Bool.self, forKey: .isPassphraseRequired) ?? false
        notice = try container.decodeIfPresent(String.self, forKey: .notice) ?? ""
        isSourceKept = try container.decodeIfPresent(Bool.self, forKey: .isSourceKept) ?? false
        exportedFrom = try container.decodeIfPresent(String.self, forKey: .exportedFrom) ?? ""
        exportedAt = try container.decodeIfPresent(String.self, forKey: .exportedAt) ?? ""
    }

    /// Whether this preview is a Hisar export rather than another app's Credential Exchange data.
    var isHisarExport: Bool { format == Self.formatHisarExport || format == Self.formatHisarEncryptedExport }

    /// The "Exported from <email> on <date>" line, with the date in the platform's medium style; nil when no date is known.
    var exportedDescription: String? {
        guard !exportedAt.isEmpty, let date = ISO8601DateFormatter().date(from: exportedAt) else { return nil }
        let dateText = date.formatted(date: .abbreviated, time: .omitted)
        return exportedFrom.isEmpty ? "Exported on \(dateText)" : "Exported from \(exportedFrom) on \(dateText)"
    }

    /// The preview's non-zero counts, as the import sheet lists them.
    var allCountRow: [ImportCountRow] {
        [
            ImportCountRow(label: "Logins", count: loginCount), ImportCountRow(label: "Passkeys", count: passkeyCount),
            ImportCountRow(label: "Secure notes", count: secureNoteCount), ImportCountRow(label: "Cards", count: creditCardCount),
            ImportCountRow(label: "Identities", count: identityCount), ImportCountRow(label: "SSH keys", count: sshKeyCount),
            ImportCountRow(label: "API credentials", count: apiCredentialCount), ImportCountRow(label: "Addresses", count: addressCount),
            ImportCountRow(label: "Software licenses", count: softwareLicenseCount), ImportCountRow(label: "Servers", count: serverCount),
            ImportCountRow(label: "Databases", count: databaseCount),
        ].filter { $0.count > 0 }
    }

    /// How many passkeys skipping the duplicates still adds to the logins the vault already has.
    var duplicatePasskeyCount: Int { allDuplicate.map(\.passkeyCount).reduce(0, +) }
}

struct ImportCountRow: Hashable, Identifiable {
    var label: String
    var count: Int

    var id: String { label }
}

/// ImportProgress is a running import: continueImport saves the next items until isDone.
struct ImportProgress: Codable, Equatable {
    var fileName: String
    var filePath: String = ""
    var vaultID: String
    var vaultName: String
    var savedCount: Int
    var totalCount: Int
    var isDone: Bool
    var allFailed: [ImportSkippedEntry] = []
    /// isSourceKept: the file is not offered for deletion, as in ImportPreview.
    var isSourceKept = false

    enum CodingKeys: String, CodingKey {
        case fileName, filePath, vaultName, savedCount, totalCount, isDone, allFailed, isSourceKept
        case vaultID = "vaultId"
    }

    init(fileName: String, vaultID: String, vaultName: String, savedCount: Int, totalCount: Int, isDone: Bool, allFailed: [ImportSkippedEntry] = []) {
        self.fileName = fileName
        self.vaultID = vaultID
        self.vaultName = vaultName
        self.savedCount = savedCount
        self.totalCount = totalCount
        self.isDone = isDone
        self.allFailed = allFailed
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        fileName = try container.decodeIfPresent(String.self, forKey: .fileName) ?? ""
        filePath = try container.decodeIfPresent(String.self, forKey: .filePath) ?? ""
        vaultID = try container.decodeIfPresent(String.self, forKey: .vaultID) ?? ""
        vaultName = try container.decodeIfPresent(String.self, forKey: .vaultName) ?? ""
        savedCount = try container.decodeIfPresent(Int.self, forKey: .savedCount) ?? 0
        totalCount = try container.decodeIfPresent(Int.self, forKey: .totalCount) ?? 0
        isDone = try container.decodeIfPresent(Bool.self, forKey: .isDone) ?? false
        allFailed = try container.decodeIfPresent([ImportSkippedEntry].self, forKey: .allFailed) ?? []
        isSourceKept = try container.decodeIfPresent(Bool.self, forKey: .isSourceKept) ?? false
    }
}

struct CredentialChangeResult: Codable, Equatable {
    var status: VaultStatus
    var secretKey: String
    var signedOutDeviceCount: Int
    var keychainError: String
    var localError: String
    var emergencyKitPath: String
    var pendingOperationId: String = ""
    var setupState: String = ""
    var problems: [String] = []

    var isWaitingOnlyForKit: Bool {
        AccountLifecycleLogic.isWaitingOnlyForKit(setupState: setupState, secretKey: secretKey, problems: problems)
            && localError.isEmpty && keychainError.isEmpty
    }

    init(status: VaultStatus, secretKey: String = "", signedOutDeviceCount: Int = 0, keychainError: String = "", localError: String = "", emergencyKitPath: String = "", pendingOperationId: String = "", setupState: String = "", problems: [String] = []) {
        self.status = status
        self.secretKey = secretKey
        self.signedOutDeviceCount = signedOutDeviceCount
        self.keychainError = keychainError
        self.localError = localError
        self.emergencyKitPath = emergencyKitPath
        self.pendingOperationId = pendingOperationId
        self.setupState = setupState
        self.problems = problems
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        status = try container.decode(VaultStatus.self, forKey: .status)
        secretKey = try container.decodeIfPresent(String.self, forKey: .secretKey) ?? ""
        signedOutDeviceCount = try container.decodeIfPresent(Int.self, forKey: .signedOutDeviceCount) ?? 0
        keychainError = try container.decodeIfPresent(String.self, forKey: .keychainError) ?? ""
        localError = try container.decodeIfPresent(String.self, forKey: .localError) ?? ""
        emergencyKitPath = try container.decodeIfPresent(String.self, forKey: .emergencyKitPath) ?? ""
        pendingOperationId = try container.decodeIfPresent(String.self, forKey: .pendingOperationId) ?? ""
        setupState = try container.decodeIfPresent(String.self, forKey: .setupState) ?? ""
        problems = try container.decodeIfPresent([String].self, forKey: .problems) ?? []
    }
}

struct DeletionVault: Codable, Equatable, Identifiable {
    var id: String
    var name: String
    var type: String
    var role: String
    var otherMemberCount: Int
}

struct AccountDeletionPreview: Codable, Equatable {
    var email: String
    var planVersion: String
    var allOwnedVault: [DeletionVault]
    var allMemberVault: [DeletionVault]
    var shareLinkCount: Int
    var sentInvitationCount: Int
    var receivedInvitationCount: Int
    var deviceCount: Int
    var allOrganization: [DeletionOrganization]

    enum CodingKeys: String, CodingKey {
        case email, planVersion
        case allOwnedVault = "ownedVaults"
        case allMemberVault = "memberVaults"
        case shareLinkCount, sentInvitationCount, receivedInvitationCount, deviceCount
        case allOrganization = "organizations"
    }

    init(email: String, planVersion: String = "", allOwnedVault: [DeletionVault] = [], allMemberVault: [DeletionVault] = [], shareLinkCount: Int = 0, sentInvitationCount: Int = 0, receivedInvitationCount: Int = 0, deviceCount: Int = 1, allOrganization: [DeletionOrganization] = []) {
        self.email = email
        self.planVersion = planVersion
        self.allOwnedVault = allOwnedVault
        self.allMemberVault = allMemberVault
        self.shareLinkCount = shareLinkCount
        self.sentInvitationCount = sentInvitationCount
        self.receivedInvitationCount = receivedInvitationCount
        self.deviceCount = deviceCount
        self.allOrganization = allOrganization
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        planVersion = try container.decodeIfPresent(String.self, forKey: .planVersion) ?? ""
        allOwnedVault = try container.decodeIfPresent([DeletionVault].self, forKey: .allOwnedVault) ?? []
        allMemberVault = try container.decodeIfPresent([DeletionVault].self, forKey: .allMemberVault) ?? []
        shareLinkCount = try container.decodeIfPresent(Int.self, forKey: .shareLinkCount) ?? 0
        sentInvitationCount = try container.decodeIfPresent(Int.self, forKey: .sentInvitationCount) ?? 0
        receivedInvitationCount = try container.decodeIfPresent(Int.self, forKey: .receivedInvitationCount) ?? 0
        deviceCount = try container.decodeIfPresent(Int.self, forKey: .deviceCount) ?? 0
        allOrganization = try container.decodeIfPresent([DeletionOrganization].self, forKey: .allOrganization) ?? []
    }

    var peopleLosingAccessCount: Int { allOwnedVault.reduce(0) { $0 + $1.otherMemberCount } }
}

// DeletionOrganization: isDeleted when the user is its last member; newAdminName is who becomes admin when the user is its
// last admin.
struct DeletionOrganization: Codable, Equatable, Identifiable {
    var id: String
    var name: String
    var isDeleted: Bool
    var newAdminName: String

    init(id: String, name: String, isDeleted: Bool = false, newAdminName: String = "") {
        self.id = id
        self.name = name
        self.isDeleted = isDeleted
        self.newAdminName = newAdminName
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? ""
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        isDeleted = try container.decodeIfPresent(Bool.self, forKey: .isDeleted) ?? false
        newAdminName = try container.decodeIfPresent(String.self, forKey: .newAdminName) ?? ""
    }
}

// The organization models mirror core/app's Organizations, Organization and WaitingVaultKeys (§5.11). Organization members
// are Members: they carry the same fields, security code included.
enum OrganizationRole {
    static let admin = "admin"
    static let member = "member"
    static let all = [member, admin]

    static func describe(_ role: String) -> String {
        role == admin ? "Admin" : "Member"
    }

    /// "an admin" or "a member", as in "Ada invited you to Acme as a member".
    static func describeWithArticle(_ role: String) -> String {
        role == admin ? "an admin" : "a member"
    }
}

struct OrganizationSummary: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var role: String
    var memberCount: Int

    var isAdmin: Bool { role == OrganizationRole.admin }

    init(id: String, name: String, role: String, memberCount: Int) {
        self.id = id
        self.name = name
        self.role = role
        self.memberCount = memberCount
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        role = try container.decodeIfPresent(String.self, forKey: .role) ?? ""
        memberCount = try container.decodeIfPresent(Int.self, forKey: .memberCount) ?? 0
    }
}

// OrganizationInvitation is an invitation to this user's email to join an organization; expiresAt is RFC 3339.
struct OrganizationInvitation: Codable, Identifiable, Hashable {
    var id: String
    var organizationID: String
    var organizationName: String
    var role: String
    var invitedByName: String
    var invitedByEmail: String
    var expiresAt: String

    var inviterName: String { invitedByName.isEmpty ? invitedByEmail : invitedByName }
    var message: String { "\(inviterName) invited you to \(organizationName) as \(OrganizationRole.describeWithArticle(role))" }

    enum CodingKeys: String, CodingKey {
        case id, organizationName, role, invitedByName, invitedByEmail, expiresAt
        case organizationID = "organizationId"
    }
}

struct Organizations: Codable, Hashable {
    var allOrganization: [OrganizationSummary]
    var allInvitation: [OrganizationInvitation]

    enum CodingKeys: String, CodingKey {
        case allOrganization = "organizations"
        case allInvitation = "invitations"
    }

    init(allOrganization: [OrganizationSummary] = [], allInvitation: [OrganizationInvitation] = []) {
        self.allOrganization = allOrganization
        self.allInvitation = allInvitation
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        allOrganization = try container.decodeIfPresent([OrganizationSummary].self, forKey: .allOrganization) ?? []
        allInvitation = try container.decodeIfPresent([OrganizationInvitation].self, forKey: .allInvitation) ?? []
    }
}

struct TeamMember: Codable, Identifiable, Hashable {
    var accountID: String
    var name: String
    var email: String
    var isCurrentUser: Bool

    var id: String { accountID }
    var displayName: String {
        let name = name.isEmpty ? email : name
        return isCurrentUser ? "\(name) (you)" : name
    }

    enum CodingKeys: String, CodingKey {
        case accountID = "accountId"
        case name, email, isCurrentUser
    }

    init(accountID: String, name: String, email: String, isCurrentUser: Bool = false) {
        self.accountID = accountID
        self.name = name
        self.email = email
        self.isCurrentUser = isCurrentUser
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        accountID = try container.decodeIfPresent(String.self, forKey: .accountID) ?? ""
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        isCurrentUser = try container.decodeIfPresent(Bool.self, forKey: .isCurrentUser) ?? false
    }
}

// TeamVault is a vault the team was given. Vault names are encrypted, so name is empty when canOpen is false: this device
// has no key for it (yet).
struct TeamVault: Codable, Identifiable, Hashable {
    static let unopenableName = "A vault you can't open"

    var vaultID: String
    var name: String
    var role: String
    var canOpen: Bool

    var id: String { vaultID }
    var displayName: String { canOpen ? name : Self.unopenableName }

    enum CodingKeys: String, CodingKey {
        case vaultID = "vaultId"
        case name, role, canOpen
    }

    init(vaultID: String, name: String, role: String, canOpen: Bool) {
        self.vaultID = vaultID
        self.name = name
        self.role = role
        self.canOpen = canOpen
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        vaultID = try container.decodeIfPresent(String.self, forKey: .vaultID) ?? ""
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        role = try container.decodeIfPresent(String.self, forKey: .role) ?? ""
        canOpen = try container.decodeIfPresent(Bool.self, forKey: .canOpen) ?? false
    }
}

struct Team: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var allMember: [TeamMember]
    var allVault: [TeamVault]

    var hasCurrentUser: Bool { allMember.contains { $0.isCurrentUser } }

    enum CodingKeys: String, CodingKey {
        case id, name
        case allMember = "members"
        case allVault = "vaults"
    }

    init(id: String, name: String, allMember: [TeamMember] = [], allVault: [TeamVault] = []) {
        self.id = id
        self.name = name
        self.allMember = allMember
        self.allVault = allVault
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        allMember = try container.decodeIfPresent([TeamMember].self, forKey: .allMember) ?? []
        allVault = try container.decodeIfPresent([TeamVault].self, forKey: .allVault) ?? []
    }
}

// SentOrganizationInvitation is a pending invitation to the organization; only admins see them. Times are RFC 3339.
struct SentOrganizationInvitation: Codable, Identifiable, Hashable {
    var id: String
    var email: String
    var role: String
    var createdAt: String
    var expiresAt: String
}

struct Organization: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var role: String
    var isAdmin: Bool
    var allMember: [Member]
    var allTeam: [Team]
    /// Empty unless isAdmin.
    var allInvitation: [SentOrganizationInvitation]

    enum CodingKeys: String, CodingKey {
        case id, name, role, isAdmin
        case allMember = "members"
        case allTeam = "teams"
        case allInvitation = "invitations"
    }

    init(id: String, name: String, role: String, isAdmin: Bool, allMember: [Member] = [], allTeam: [Team] = [], allInvitation: [SentOrganizationInvitation] = []) {
        self.id = id
        self.name = name
        self.role = role
        self.isAdmin = isAdmin
        self.allMember = allMember
        self.allTeam = allTeam
        self.allInvitation = allInvitation
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        role = try container.decodeIfPresent(String.self, forKey: .role) ?? ""
        isAdmin = try container.decodeIfPresent(Bool.self, forKey: .isAdmin) ?? false
        allMember = try container.decodeIfPresent([Member].self, forKey: .allMember) ?? []
        allTeam = try container.decodeIfPresent([Team].self, forKey: .allTeam) ?? []
        allInvitation = try container.decodeIfPresent([SentOrganizationInvitation].self, forKey: .allInvitation) ?? []
    }
}

// WaitingVaultKey is an organization vault this user's teams give them that no owner's Hisar has delivered yet.
struct WaitingVaultKey: Codable, Identifiable, Hashable {
    var vaultID: String
    var organizationID: String
    var organizationName: String
    var role: String

    var id: String { vaultID }

    enum CodingKeys: String, CodingKey {
        case vaultID = "vaultId"
        case organizationID = "organizationId"
        case organizationName, role
    }
}

// WaitingVaultKeys: deliveryError and rotationError say why this app could not give someone a vault key or rotate a vault
// it owns, e.g. a changed signing key.
struct WaitingVaultKeys: Codable, Hashable {
    var allWaiting: [WaitingVaultKey]
    var deliveryError: String
    var rotationError: String

    enum CodingKeys: String, CodingKey {
        case allWaiting = "waiting"
        case deliveryError, rotationError
    }

    init(allWaiting: [WaitingVaultKey] = [], deliveryError: String = "", rotationError: String = "") {
        self.allWaiting = allWaiting
        self.deliveryError = deliveryError
        self.rotationError = rotationError
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        allWaiting = try container.decodeIfPresent([WaitingVaultKey].self, forKey: .allWaiting) ?? []
        deliveryError = try container.decodeIfPresent(String.self, forKey: .deliveryError) ?? ""
        rotationError = try container.decodeIfPresent(String.self, forKey: .rotationError) ?? ""
    }
}
