import Foundation

struct VaultItemReference: Hashable, Identifiable {
    var vaultID: String
    var vaultName: String
    var item: ItemDetail

    var id: String { LoginMatch.buildRecordIdentifier(vaultID: vaultID, itemID: item.id) }
}

struct SignInLink: Equatable {
    var server: String
    var email: String
    var secretKey: String

    // hisar://sign-in?server=…&email=…&secretKey=… from the Emergency Kit QR code.
    static func parse(_ text: String) -> SignInLink? {
        guard let components = URLComponents(string: text.trimmingCharacters(in: .whitespacesAndNewlines)),
              components.scheme?.lowercased() == "hisar",
              components.host?.lowercased() == "sign-in" else { return nil }
        func readValue(_ name: String) -> String {
            components.queryItems?.first { $0.name == name }?.value?.trimmingCharacters(in: .whitespaces) ?? ""
        }
        let link = SignInLink(server: readValue("server"), email: readValue("email"), secretKey: readValue("secretKey"))
        return link.server.isEmpty && link.email.isEmpty && link.secretKey.isEmpty ? nil : link
    }
}

struct InviteLink: Equatable {
    static let defaultServer = "https://api.tryhisar.com"

    // What Create account accepts: the bare code for the default server, otherwise the invite link on its server.
    var invite: String

    // hisar://invite/<code>?server=<origin> from the invite page's Open in Hisar button.
    static func parse(_ text: String) -> InviteLink? {
        guard let components = URLComponents(string: text.trimmingCharacters(in: .whitespacesAndNewlines)),
              components.scheme?.lowercased() == "hisar",
              components.host?.lowercased() == "invite" else { return nil }
        let code = String(components.path.dropFirst())
        guard !code.isEmpty, code.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil else { return nil }
        let serverText = components.queryItems?.first { $0.name == "server" }?.value ?? ""
        if serverText.isEmpty {
            return InviteLink(invite: code)
        }
        guard let server = resolveServer(serverText) else { return nil }
        return InviteLink(invite: server == defaultServer ? code : "\(server)/invite/\(code)")
    }

    private static func resolveServer(_ text: String) -> String? {
        var server = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let hasScheme = server.contains("://")
        while server.hasSuffix("/") {
            server.removeLast()
        }
        if !server.isEmpty && !hasScheme {
            server = "https://" + server
        }
        guard let components = URLComponents(string: server),
              let scheme = components.scheme?.lowercased(), scheme == "https" || scheme == "http",
              let host = components.host, !host.isEmpty else { return nil }
        return server
    }
}

struct CredentialIdentityRecord: Hashable {
    enum Kind: Hashable {
        case password
        case oneTimeCode
        case passkey
    }

    var kind: Kind = .password
    var host: String
    var username: String
    var recordIdentifier: String
    var credentialID = Data()
    var userHandle = Data()
}

struct TagSummary: Hashable, Identifiable {
    var name: String
    var itemCount: Int

    var id: String { name }
}

enum VaultLogic {
    static func filterAllItem(_ allItem: [VaultItemReference], searchText: String) -> [VaultItemReference] {
        let allWord = searchText.lowercased().split(whereSeparator: \.isWhitespace).map(String.init)
        guard !allWord.isEmpty else { return allItem }
        return allItem.filter { reference in
            // An identity's list summary keeps its name, email and address, an API credential's its service, key ID and
            // host, an address's first street line, city and country. The list shows some of them under the title; search
            // reads them all, so an identity imported with an address is found by its city.
            let identityText = reference.item.identity.map { [$0.fullName, $0.email, $0.street, $0.city, $0.country] } ?? []
            let credentialText = reference.item.apiCredential.map { [$0.service, $0.keyID, $0.host] } ?? []
            let addressText = reference.item.address.map { [$0.street, $0.city, $0.country] } ?? []
            // A software license's summary keeps its product, version, licensed-to name and email, never the key.
            let licenseText = reference.item.softwareLicense.map { [$0.product, $0.version, $0.licensedTo, $0.email] } ?? []
            let searchable = ([reference.item.title, reference.item.username, reference.vaultName] + reference.item.allURL + reference.item.allTag + identityText + credentialText + addressText + licenseText)
                .joined(separator: " ").lowercased()
            return allWord.allSatisfy { searchable.contains($0) }
        }
    }

    static func filterAllItem(_ allItem: [VaultItemReference], tag: String) -> [VaultItemReference] {
        allItem.filter { $0.item.allTag.contains(tag) }
    }

    /// Every tag of the items with how many carry it, sorted by name.
    static func listAllTag(_ allItem: [VaultItemReference]) -> [TagSummary] {
        var countByTag: [String: Int] = [:]
        for reference in allItem {
            for tag in reference.item.allTag {
                countByTag[tag, default: 0] += 1
            }
        }
        return countByTag.map { TagSummary(name: $0.key, itemCount: $0.value) }.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    static func sortAllItem(_ allItem: [VaultItemReference]) -> [VaultItemReference] {
        allItem.sorted {
            let order = $0.item.title.localizedCaseInsensitiveCompare($1.item.title)
            return order == .orderedSame ? $0.item.username.localizedCaseInsensitiveCompare($1.item.username) == .orderedAscending : order == .orderedAscending
        }
    }

    static func convertToOpenableURL(_ text: String) -> URL? {
        let trimmedText = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedText.isEmpty else { return nil }
        let fullText = trimmedText.contains("://") ? trimmedText : "https://\(trimmedText)"
        guard let url = URL(string: fullText), let scheme = url.scheme?.lowercased(), ["http", "https"].contains(scheme), url.host != nil else { return nil }
        return url
    }

    static func extractHost(_ text: String) -> String? {
        convertToOpenableURL(text)?.host?.lowercased()
    }

    static func describeServer(_ server: String) -> String {
        guard let url = convertToOpenableURL(server), let host = url.host else { return server }
        return url.port.map { "\(host):\($0)" } ?? host
    }

    static func buildAllCredentialIdentity(_ allItem: [VaultItemReference], allPasskey: [StoredPasskey] = []) -> [CredentialIdentityRecord] {
        var allIdentity: [CredentialIdentityRecord] = []
        var allSeen = Set<CredentialIdentityRecord>()
        for reference in allItem where reference.item.isLogin {
            let username = reference.item.username.trimmingCharacters(in: .whitespaces)
            for url in reference.item.allURL {
                guard let host = extractHost(url) else { continue }
                var allCandidate: [CredentialIdentityRecord] = []
                if !username.isEmpty {
                    allCandidate.append(CredentialIdentityRecord(host: host, username: username, recordIdentifier: reference.id))
                }
                if reference.item.hasOneTimePassword {
                    let label = username.isEmpty ? reference.item.title : username
                    allCandidate.append(CredentialIdentityRecord(kind: .oneTimeCode, host: host, username: label, recordIdentifier: reference.id))
                }
                for identity in allCandidate where allSeen.insert(identity).inserted {
                    allIdentity.append(identity)
                }
            }
        }
        let allLoginID = Set(allItem.filter(\.item.isLogin).map(\.id))
        for passkey in allPasskey where allLoginID.contains(passkey.recordIdentifier) {
            guard let credentialID = Base64URL.decode(passkey.credentialID), let userHandle = Base64URL.decode(passkey.userHandle),
                  !credentialID.isEmpty, !userHandle.isEmpty else { continue }
            let identity = CredentialIdentityRecord(kind: .passkey, host: passkey.relyingPartyID, username: passkey.userName, recordIdentifier: passkey.recordIdentifier, credentialID: credentialID, userHandle: userHandle)
            if allSeen.insert(identity).inserted {
                allIdentity.append(identity)
            }
        }
        return allIdentity
    }

    static func formatSecretKey(_ text: String) -> String {
        text.uppercased().filter { !$0.isWhitespace }
    }
}
