import Foundation

enum PendingRecoveryCopy {
    private static let headings = [
        "signup": "Finish creating your account",
        "replace-secret-key": "Finish replacing your Secret Key"
    ]
    private static let statuses = [
        "awaiting-kit": "Save and confirm your new Emergency Kit before Hisar sends this change.",
        "prepared": "Your Emergency Kit is confirmed saved. Continue to send this change.",
        "manual-unlock": "Sign in with your master password and the Secret Key from your saved Emergency Kit.",
        "outcome-unknown": "Hisar could not confirm whether the server finished this change. Continue to check. Do not start again."
    ]
    private static let kindStatuses = [
        "delivery-pending": [
            "signup": "Your account is ready. Save and confirm your Emergency Kit to finish.",
            "replace-secret-key": "Your Secret Key was replaced. Save and confirm your new Emergency Kit to finish."
        ],
        "needs-finishing": [
            "signup": "Your account was created, but setup did not finish on this device. Continue with the same master password.",
            "replace-secret-key": "Your Secret Key was replaced, but setup did not finish on this device. Continue with the same master password."
        ]
    ]

    static func heading(_ kind: String) -> String { headings[kind] ?? "Finish account setup" }

    static func status(kind: String, phase: String) -> String {
        guard headings[kind] != nil else { return "Finish account setup" }
        return statuses[phase] ?? kindStatuses[phase]?[kind] ?? "Finish account setup"
    }

    static func host(_ origin: String) -> String { URL(string: origin)?.host() ?? origin }
}
