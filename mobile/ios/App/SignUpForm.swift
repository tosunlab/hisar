import SwiftUI

@MainActor
final class SignUpForm: ObservableObject {
    enum PolicyState: Equatable {
        case checking
        case ready(SignUpPolicy)
        case failed(String)
    }
    enum Field: Hashable { case invite, name, email, password, repeatedPassword }

    @Published private(set) var server = InviteLink.defaultServer
    @Published var name = "" { didSet { errors[.name] = nil } }
    @Published var email = "" { didSet { errors[.email] = nil } }
    @Published var password = "" { didSet { errors[.password] = nil } }
    @Published var repeatedPassword = ""
    @Published var inviteText = "" { didSet { errors[.invite] = nil } }
    @Published private(set) var invitation: InviteLink?
    @Published private(set) var policy = PolicyState.checking
    @Published private(set) var isCheckingVisible = false
    @Published private(set) var invitationMessage: String?
    @Published private(set) var errors: [Field: String] = [:]
    @Published private(set) var keepsSubmittedFields = false
    private var generation = 0

    var canonicalServer: String { InviteLink.resolveServer(server) ?? server }
    var host: String { SignInView.describeServer(canonicalServer) }
    var boundEmail: String? { invitation.flatMap { $0.email.isEmpty ? nil : $0.email } }
    var mode: String {
        if case .ready(let policy) = policy { return policy.mode }
        return ""
    }
    var needsInvitation: Bool { mode == "invite" || mode == "setup" }
    var showsInviteField: Bool { invitation == nil && (needsInvitation || mode == "unknown") }
    var showsAccountFields: Bool {
        invitation != nil || keepsSubmittedFields || mode == "open" || mode == "unknown"
    }
    var isChecking: Bool { invitation == nil && policy == .checking }
    var canSubmit: Bool {
        if invitation != nil { return true }
        if case .ready = policy { return true }
        return false
    }

    func changeServer(_ value: String) {
        guard value != server else { return }
        generation += 1
        if let invitation, InviteLink.resolveServer(value) != invitation.server {
            invitationMessage = "This invitation is for \(SignInView.describeServer(invitation.server)), so Hisar removed it."
            self.invitation = nil
            inviteText = ""
        } else { invitationMessage = nil }
        server = value
        errors = [:]
        keepsSubmittedFields = false
        policy = .checking
        isCheckingVisible = false
    }

    func applyInvitation(_ link: InviteLink) {
        generation += 1
        server = link.server
        invitation = link
        inviteText = link.invite
        invitationMessage = nil
        errors = [:]
        isCheckingVisible = false
    }

    func parseInvitation() {
        guard let link = InviteLink.parseInput(inviteText, server: canonicalServer) else {
            invitation = nil
            return
        }
        applyInvitation(link)
    }

    func removeInvitation() {
        generation += 1
        invitation = nil
        inviteText = ""
        invitationMessage = nil
        errors = [:]
        policy = .checking
    }

    func checkPolicy(using model: AppModel, debounce: Bool = false) async {
        generation += 1
        let requestedGeneration = generation
        let requestedServer = canonicalServer
        guard invitation == nil else { return }
        policy = .checking
        isCheckingVisible = false
        if debounce {
            do { try await Task.sleep(for: .milliseconds(500)) } catch { return }
        }
        guard requestedGeneration == generation, !Task.isCancelled else { return }
        let spinner = Task {
            try? await Task.sleep(for: .milliseconds(400))
            if !Task.isCancelled && requestedGeneration == generation { isCheckingVisible = true }
        }
        defer { spinner.cancel() }
        do {
            let answer = try await model.getSignUpPolicy(server: requestedServer)
            guard requestedGeneration == generation, !Task.isCancelled,
                  answer.server == canonicalServer else { return }
            policy = .ready(answer)
        } catch {
            guard requestedGeneration == generation, !Task.isCancelled else { return }
            policy = .failed("Hisar can't reach \(host). Check your internet connection and the server address.")
        }
        isCheckingVisible = false
    }

    func clearErrors() { errors = [:] }

    func validateRepeatedPassword() {
        errors[.repeatedPassword] = !repeatedPassword.isEmpty && password != repeatedPassword
            ? "The master passwords don't match." : nil
    }

    func validate(feedback: MasterPasswordFeedback?) -> Field? {
        errors = [:]
        if invitation == nil && (needsInvitation || !inviteText.isEmpty) { errors[.invite] = "Enter an invite link or code." }
        if name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { errors[.name] = "Enter your name." }
        if (boundEmail ?? email).trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { errors[.email] = "Enter your email." }
        if password.count < AccountLifecycleLogic.minimumPasswordLength {
            errors[.password] = "Use a master password with at least 10 characters."
        } else if let feedback, feedback.isBlocking { errors[.password] = feedback.message }
        if password != repeatedPassword { errors[.repeatedPassword] = "The master passwords don't match." }
        return [.invite, .name, .email, .password, .repeatedPassword].first { errors[$0] != nil }
    }

    func submit(using model: AppModel) async -> Bool {
        guard canSubmit else { return false }
        let succeeded: Bool
        if let invitation {
            succeeded = await model.signUp(inviteLink: invitation.invite, email: boundEmail ?? email, name: name, password: password)
        } else {
            succeeded = await model.signUpWithoutInvite(server: canonicalServer, email: email, name: name, password: password)
        }
        if succeeded { password = ""; repeatedPassword = ""; return true }
        let refusal = model.errorMessage ?? ""
        if refusal.localizedCaseInsensitiveContains("sign-ups are not open") {
            await checkPolicy(using: model)
            policy = .ready(SignUpPolicy(server: canonicalServer, host: host, mode: "invite"))
            keepsSubmittedFields = true
            invitationMessage = "Sign-ups on \(host) need an invitation now. Ask someone with an account there to invite you."
            model.clearError()
        } else if refusal.localizedCaseInsensitiveContains("invit") {
            invitationMessage = "This invitation can't be used. It may have expired or been used already. Ask for a new one."
            model.clearError()
        }
        return false
    }
}
