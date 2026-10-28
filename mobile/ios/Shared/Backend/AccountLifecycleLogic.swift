import Foundation

enum AccountLifecycleLogic {
    static let minimumPasswordLength = 10
    static let passwordChangeWarning = "Your other devices will be signed out. Sign in on them with the new master password."
    static let secretKeyReplacementWarning = "Your other devices will be signed out. Sign in on them with your master password and the new Secret Key from the new Emergency Kit. Your old Emergency Kit stops working."
    static let exportAdvice = "Export your items first in Settings → Account → Export data."
    static let signOutTitle = "Sign out of Hisar on this iPhone?"
    static let signOutMessage = "Hisar removes this iPhone's saved Secret Key and offline copy of your vaults. To sign in again, you need your master password and the Secret Key from your Emergency Kit or another signed-in device."
    static let lockedSignOutMessage = signOutMessage + " If you have not saved an Emergency Kit, cancel, unlock Hisar and save one in Settings, Account, Secret Key and Emergency Kit."

    static func isWaitingOnlyForKit(setupState: String, secretKey: String, problems: [String]) -> Bool {
        setupState == "delivery-pending" && !secretKey.isEmpty && problems.isEmpty
    }

    /// The sentence that refuses a new master password: fewer than 10 characters, counted as the user sees them, or core's
    /// rule (`MasterPasswordFeedback`) blocks it. feedback is core's rating of this password, nil while it is on its way.
    static func describeMasterPasswordProblem(_ password: String, feedback: MasterPasswordFeedback?) -> String? {
        if password.isEmpty { return nil }
        if password.count < minimumPasswordLength { return "Use at least \(minimumPasswordLength) characters." }
        if let feedback, feedback.isBlocking { return feedback.message }
        return nil
    }

    /// The hint under a Weak master password that core allows.
    static func describeMasterPasswordHint(feedback: MasterPasswordFeedback?) -> String? {
        guard let feedback, !feedback.isBlocking, feedback.level == "weak" else { return nil }
        return feedback.message
    }

    static func isMasterPasswordAccepted(_ password: String, feedback: MasterPasswordFeedback?) -> Bool {
        !password.isEmpty && feedback != nil && describeMasterPasswordProblem(password, feedback: feedback) == nil
    }

    static func describeNewPasswordProblem(currentPassword: String, newPassword: String, repeatedPassword: String, feedback: MasterPasswordFeedback?) -> String? {
        if let problem = describeMasterPasswordProblem(newPassword, feedback: feedback) { return problem }
        if !newPassword.isEmpty && newPassword == currentPassword { return "The new master password is the same as the current one." }
        if !repeatedPassword.isEmpty && repeatedPassword != newPassword { return "The passwords do not match." }
        return nil
    }

    static func canChangePassword(currentPassword: String, newPassword: String, repeatedPassword: String, feedback: MasterPasswordFeedback?) -> Bool {
        !currentPassword.isEmpty && newPassword == repeatedPassword && isMasterPasswordAccepted(newPassword, feedback: feedback)
            && describeNewPasswordProblem(currentPassword: currentPassword, newPassword: newPassword, repeatedPassword: repeatedPassword, feedback: feedback) == nil
    }

    static func isEmailMatching(typedEmail: String, accountEmail: String) -> Bool {
        let trimmedEmail = typedEmail.trimmingCharacters(in: .whitespacesAndNewlines)
        return !trimmedEmail.isEmpty && trimmedEmail.caseInsensitiveCompare(accountEmail) == .orderedSame
    }

    static func describeSignedOutDevices(_ count: Int) -> String {
        switch count {
        case ..<0: return "Any other devices were signed out; their count could not be recovered."
        case 0: return "No other devices were signed in."
        case 1: return "1 other device was signed out."
        default: return "\(count) other devices were signed out."
        }
    }

    static func describePeople(_ count: Int) -> String {
        count == 1 ? "1 person" : "\(count) people"
    }

    static func describeOwnedVault(_ vault: DeletionVault) -> String {
        if vault.type == "personal" { return "Deleted with its items." }
        if vault.otherMemberCount == 0 { return "Deleted with its items. Nobody else has access." }
        return "Deleted with its items. \(describePeople(vault.otherMemberCount)) lose access."
    }

    static func describeMemberVault(_ vault: DeletionVault) -> String {
        vault.role == "owner" ? "Stays with its other owners. Your access is removed." : "Stays with its owners. Your access is removed."
    }

    static func describeOrganization(_ organization: DeletionOrganization) -> String {
        if organization.isDeleted { return "\(organization.name), its vaults, and their items are deleted with your account" }
        if !organization.newAdminName.isEmpty { return "\(organization.newAdminName) becomes an admin of \(organization.name)" }
        return "You leave \(organization.name)"
    }

    static func describeAllOtherConsequence(_ preview: AccountDeletionPreview) -> [String] {
        var allConsequence: [String] = []
        if preview.shareLinkCount > 0 {
            allConsequence.append(preview.shareLinkCount == 1 ? "1 share link stops working." : "\(preview.shareLinkCount) share links stop working.")
        }
        let invitationCount = preview.sentInvitationCount + preview.receivedInvitationCount
        if invitationCount > 0 {
            allConsequence.append(invitationCount == 1 ? "1 invitation is withdrawn." : "\(invitationCount) invitations are withdrawn.")
        }
        switch preview.deviceCount {
        case ...1: allConsequence.append("This iPhone is signed out.")
        case 2: allConsequence.append("This iPhone and 1 other device are signed out.")
        default: allConsequence.append("This iPhone and \(preview.deviceCount - 1) other devices are signed out.")
        }
        return allConsequence
    }
}
