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

struct CredentialIdentityRecord: Hashable {
    var host: String
    var username: String
    var recordIdentifier: String
}

enum VaultLogic {
    static func filterAllItem(_ allItem: [VaultItemReference], searchText: String) -> [VaultItemReference] {
        let allWord = searchText.lowercased().split(whereSeparator: \.isWhitespace).map(String.init)
        guard !allWord.isEmpty else { return allItem }
        return allItem.filter { reference in
            let searchable = ([reference.item.title, reference.item.username, reference.vaultName] + reference.item.allURL).joined(separator: " ").lowercased()
            return allWord.allSatisfy { searchable.contains($0) }
        }
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

    static func buildAllCredentialIdentity(_ allItem: [VaultItemReference]) -> [CredentialIdentityRecord] {
        var allIdentity: [CredentialIdentityRecord] = []
        var allSeen = Set<CredentialIdentityRecord>()
        for reference in allItem where reference.item.isLogin {
            let username = reference.item.username.trimmingCharacters(in: .whitespaces)
            guard !username.isEmpty else { continue }
            for url in reference.item.allURL {
                guard let host = extractHost(url) else { continue }
                let identity = CredentialIdentityRecord(host: host, username: username, recordIdentifier: reference.id)
                if allSeen.insert(identity).inserted {
                    allIdentity.append(identity)
                }
            }
        }
        return allIdentity
    }

    static func formatSecretKey(_ text: String) -> String {
        text.uppercased().filter { !$0.isWhitespace }
    }
}
