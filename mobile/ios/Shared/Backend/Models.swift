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
    static let categorySecureNote = "secure-note"
    static let categoryCreditCard = "credit-card"
    static let categoryIdentity = "identity"
    static let categoryAPICredential = "api-credential"

    var id: String
    var revision: String
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

    enum CodingKeys: String, CodingKey {
        case id, revision, category, title, username, password, notes, publicKey, fingerprint, keyType, oneTimePassword, hasOneTimePassword, canConvertToSecureNote, card, identity, isFavorite, apiCredential
        case allURL = "urls"
        case allTag = "tags"
        case allField = "fields"
        case allPasskey = "passkeys"
    }

    init(id: String = "", revision: String = "", category: String = ItemDetail.categoryLogin, title: String = "", username: String = "", allURL: [String] = [], allTag: [String] = [], password: String = "", notes: String = "", allField: [ItemField] = [], publicKey: String = "", fingerprint: String = "", keyType: String = "", oneTimePassword: String = "", hasOneTimePassword: Bool = false, allPasskey: [ItemPasskey] = [], canConvertToSecureNote: Bool = false, card: ItemCard? = nil, identity: ItemIdentity? = nil, isFavorite: Bool = false, apiCredential: ItemAPICredential? = nil) {
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
        self.canConvertToSecureNote = canConvertToSecureNote
        self.card = card
        self.identity = identity
        self.isFavorite = isFavorite
        self.apiCredential = apiCredential
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? ""
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
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
        canConvertToSecureNote = try container.decodeIfPresent(Bool.self, forKey: .canConvertToSecureNote) ?? false
        card = try container.decodeIfPresent(ItemCard.self, forKey: .card)
        identity = try container.decodeIfPresent(ItemIdentity.self, forKey: .identity)
        isFavorite = try container.decodeIfPresent(Bool.self, forKey: .isFavorite) ?? false
        apiCredential = try container.decodeIfPresent(ItemAPICredential.self, forKey: .apiCredential)
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
}

/// The names core/item stores an identity's values under; copying and secret references use them.
enum IdentityFieldName {
    static let firstName = "First name"
    static let middleName = "Middle name"
    static let lastName = "Last name"
    static let birthDate = "Birth date"
    static let email = "Email"
    static let phone = "Phone"
    static let address = "Address"
    static let passportNumber = "Passport number"
    static let driversLicenseNumber = "Driver's license number"
    static let nationalIDNumber = "National ID number"
}

/// A person's details; the document numbers are optional and concealed.
struct ItemIdentity: Codable, Hashable {
    var firstName = ""
    var middleName = ""
    var lastName = ""
    var birthDate = ""
    var email = ""
    var phone = ""
    var address = ""
    var passportNumber = ""
    var driversLicenseNumber = ""
    var nationalIDNumber = ""

    enum CodingKeys: String, CodingKey {
        case firstName, middleName, lastName, birthDate, email, phone, address, passportNumber, driversLicenseNumber
        case nationalIDNumber = "nationalIdNumber"
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
    var billingAddress = ""
    var brand = ""
    var last4 = ""
    var numberWarning = ""
    var expiryWarning = ""

    enum CodingKeys: String, CodingKey {
        case cardholder, type, number, expiry, securityCode, pin, billingAddress, brand, last4, numberWarning, expiryWarning
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

    /// Groups a card number as it is typed, the way core/item saves it: 4-6-5 for American Express, 4-6-4 for a 14-digit
    /// Diners Club number and fours otherwise. Text with other characters is left as typed.
    static func formatNumberInput(_ value: String) -> String {
        guard value.allSatisfy({ $0.isASCII && ($0.isNumber || $0 == " " || $0 == "-") }) else { return value }
        let digits = Array(value.filter(\.isNumber))
        var allGroupSize = [4]
        if digits.starts(with: ["3", "4"]) || digits.starts(with: ["3", "7"]), digits.count <= 15 {
            allGroupSize = [4, 6, 5]
        } else if String(digits).range(of: "^3(0[0-5]|[689])", options: .regularExpression) != nil, digits.count <= 14 {
            allGroupSize = [4, 6, 4]
        }
        var allGroup: [String] = []
        var start = 0
        while start < digits.count {
            let size = allGroupSize[min(allGroup.count, allGroupSize.count - 1)]
            allGroup.append(String(digits[start..<min(start + size, digits.count)]))
            start += size
        }
        return allGroup.joined(separator: " ")
    }

    /// Writes an expiry as MM/YY while it is typed: "1230" becomes "12/30", "3" becomes "03" and "1/" becomes "01/". A pasted
    /// 12/2030 or 2030-12 becomes 12/30, and text with other characters is left as typed.
    static func formatExpiryInput(_ value: String) -> String {
        guard value.allSatisfy({ $0.isASCII && ($0.isNumber || " /.-".contains($0)) }) else { return value }
        let trimmed = value.trimmingCharacters(in: .whitespaces)
        if let match = trimmed.wholeMatch(of: /(\d{4})\s*[\/.\-]\s*(\d{1,2})/) {
            return "\(pad(match.2))/\(match.1.suffix(2))"
        }
        if let match = trimmed.wholeMatch(of: /(\d{1,2})\s*[\/.\-]\s*(\d*)/) {
            let year = match.2.count == 4 ? match.2.suffix(2) : match.2.prefix(2)
            return "\(pad(match.1))/\(year)"
        }
        var digits = value.filter(\.isNumber)
        if digits.range(of: "^([2-9]|1[3-9])", options: .regularExpression) != nil {
            digits = "0" + digits
        }
        guard digits.count > 2 else { return digits }
        let year = digits.count == 6 ? digits.suffix(2) : digits.dropFirst(2).prefix(2)
        return "\(digits.prefix(2))/\(year)"
    }

    private static func pad(_ month: Substring) -> String {
        month.count == 1 ? "0" + month : String(month)
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
    var message: String { "\(recipientDisplayName) accepted your invitation to a vault — unlock to finish sharing" }

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
}

enum ExportFormat: String, CaseIterable, Identifiable {
    case encrypted
    case plain

    var id: String { rawValue }
    var label: String { self == .encrypted ? "Encrypted export" : "Plain JSON" }
}

struct CredentialChangeResult: Codable, Equatable {
    var status: VaultStatus
    var secretKey: String
    var signedOutDeviceCount: Int
    var keychainError: String
    var emergencyKitPath: String

    init(status: VaultStatus, secretKey: String = "", signedOutDeviceCount: Int = 0, keychainError: String = "", emergencyKitPath: String = "") {
        self.status = status
        self.secretKey = secretKey
        self.signedOutDeviceCount = signedOutDeviceCount
        self.keychainError = keychainError
        self.emergencyKitPath = emergencyKitPath
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        status = try container.decode(VaultStatus.self, forKey: .status)
        secretKey = try container.decodeIfPresent(String.self, forKey: .secretKey) ?? ""
        signedOutDeviceCount = try container.decodeIfPresent(Int.self, forKey: .signedOutDeviceCount) ?? 0
        keychainError = try container.decodeIfPresent(String.self, forKey: .keychainError) ?? ""
        emergencyKitPath = try container.decodeIfPresent(String.self, forKey: .emergencyKitPath) ?? ""
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
    var allOwnedVault: [DeletionVault]
    var allMemberVault: [DeletionVault]
    var shareLinkCount: Int
    var sentInvitationCount: Int
    var receivedInvitationCount: Int
    var deviceCount: Int

    enum CodingKeys: String, CodingKey {
        case email
        case allOwnedVault = "ownedVaults"
        case allMemberVault = "memberVaults"
        case shareLinkCount, sentInvitationCount, receivedInvitationCount, deviceCount
    }

    init(email: String, allOwnedVault: [DeletionVault] = [], allMemberVault: [DeletionVault] = [], shareLinkCount: Int = 0, sentInvitationCount: Int = 0, receivedInvitationCount: Int = 0, deviceCount: Int = 1) {
        self.email = email
        self.allOwnedVault = allOwnedVault
        self.allMemberVault = allMemberVault
        self.shareLinkCount = shareLinkCount
        self.sentInvitationCount = sentInvitationCount
        self.receivedInvitationCount = receivedInvitationCount
        self.deviceCount = deviceCount
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        email = try container.decodeIfPresent(String.self, forKey: .email) ?? ""
        allOwnedVault = try container.decodeIfPresent([DeletionVault].self, forKey: .allOwnedVault) ?? []
        allMemberVault = try container.decodeIfPresent([DeletionVault].self, forKey: .allMemberVault) ?? []
        shareLinkCount = try container.decodeIfPresent(Int.self, forKey: .shareLinkCount) ?? 0
        sentInvitationCount = try container.decodeIfPresent(Int.self, forKey: .sentInvitationCount) ?? 0
        receivedInvitationCount = try container.decodeIfPresent(Int.self, forKey: .receivedInvitationCount) ?? 0
        deviceCount = try container.decodeIfPresent(Int.self, forKey: .deviceCount) ?? 0
    }

    var peopleLosingAccessCount: Int { allOwnedVault.reduce(0) { $0 + $1.otherMemberCount } }
}
