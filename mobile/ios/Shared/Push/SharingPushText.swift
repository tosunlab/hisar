import Foundation

/// SharingPushDetails is the decrypted payload of a sharing push: who invited the user to what. Unknown fields are
/// ignored and any field may be missing, so every property is optional.
struct SharingPushDetails: Decodable {
    var kind: String?
    var invitation: Invitation?

    struct Invitation: Decodable {
        var type: String?
        var inviterName: String?
        var inviterEmail: String?
        var organizationName: String?
        var role: String?
    }

    /// decode reads the decrypted JSON; it returns nil when the bytes are not the expected object.
    static func decode(_ data: Data) -> SharingPushDetails? {
        try? JSONDecoder().decode(SharingPushDetails.self, from: data)
    }

    /// body composes the alert body, or nil to keep the alert unchanged (unknown type, blank inviter, missing
    /// invitation). It matches the Android text exactly.
    var body: String? {
        guard let invitation else { return nil }
        let inviter = Self.inviterText(name: invitation.inviterName, email: invitation.inviterEmail)
        guard !inviter.isEmpty else { return nil }
        switch invitation.type {
        case "organization":
            guard let organization = invitation.organizationName?.trimmingCharacters(in: .whitespacesAndNewlines),
                  !organization.isEmpty else { return nil }
            return "\(inviter) invited you to join \(organization). Open Hisar to answer it."
        case "vault":
            return "\(inviter) shared a vault with you. Open Hisar to answer it."
        default:
            return nil
        }
    }

    /// inviterText is "<name> (<email>)"; the email alone when the name is blank or equals it case-insensitively; the
    /// name alone when the email is blank; empty when both are blank.
    static func inviterText(name: String?, email: String?) -> String {
        let trimmedName = (name ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedEmail = (email ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmedName.isEmpty { return trimmedEmail }
        if trimmedEmail.isEmpty { return trimmedName }
        if trimmedName.caseInsensitiveCompare(trimmedEmail) == .orderedSame { return trimmedEmail }
        return "\(trimmedName) (\(trimmedEmail))"
    }
}
