import SwiftUI

struct SignInView: View {
    enum Mode: String, CaseIterable, Identifiable {
        case signIn = "Sign in"
        case createAccount = "Create account"
        var id: String { rawValue }
    }
    enum Field: Hashable { case server, invite, name, email, secretKey, password, repeatedPassword }
    static let defaultServer = InviteLink.defaultServer

    @EnvironmentObject private var model: AppModel
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @StateObject private var form = SignUpForm()
    @State private var mode = Mode.signIn
    @State private var hasChosenEntry = false
    @State private var isServerShown = false
    @State private var email = ""
    @State private var secretKey = ""
    @State private var password = ""
    @State private var scannedLink: SignInLink?
    @State private var signInErrors: [Field: String] = [:]
    @FocusState private var focusedField: Field?
    @State private var focusedCredential: Field?
    @State private var isScannerShown = false
    @State private var isLostKeyHelpShown = false
    @State private var ratedPassword = ""
    @State private var passwordFeedback: MasterPasswordFeedback?
    @State private var isSubmitting = false

    private var showsWelcome: Bool {
        !hasChosenEntry && model.status.server.isEmpty && model.allPendingRecovery.isEmpty
            && model.pendingInviteLink == nil && model.pendingSignInLink == nil
    }
    private var heading: String {
        mode == .createAccount ? "Create account" : (model.status.server.isEmpty ? "Sign in" : "Welcome back")
    }
    private var serverBinding: Binding<String> {
        Binding(get: { form.server }, set: { form.changeServer($0); model.clearError() })
    }

    private func focus(_ field: Field) -> Binding<Bool> {
        Binding(get: { focusedCredential == field }, set: { isFocused in
            if isFocused {
                focusedField = nil
                focusedCredential = field
            } else if focusedCredential == field {
                focusedCredential = nil
            }
        })
    }

    private func focusField(_ field: Field?) {
        if field == .password || field == .repeatedPassword {
            focusedField = nil
            focusedCredential = field
        } else {
            focusedCredential = nil
            focusedField = field
        }
    }

    var body: some View {
        NavigationStack {
            if showsWelcome {
                welcome
            } else {
                Form {
                    Section {
                        if dynamicTypeSize.isAccessibilitySize {
                            Text(heading)
                                .font(.largeTitle.bold())
                                .accessibilityAddTraits(.isHeader)
                                .accessibilityIdentifier("sign-in-heading")
                            VStack(alignment: .leading) {
                                ForEach(Mode.allCases) { option in
                                    Button(option.rawValue) { mode = option }
                                        .frame(minHeight: 44)
                                        .accessibilityAddTraits(mode == option ? .isSelected : [])
                                }
                            }
                            .accessibilityIdentifier("sign-in-mode")
                        } else {
                            Picker("Mode", selection: $mode) {
                                ForEach(Mode.allCases) { Text($0.rawValue).tag($0) }
                            }
                            .pickerStyle(.segmented)
                            .accessibilityIdentifier("sign-in-mode")
                        }
                    }
                    if !model.status.signedOutReason.isEmpty {
                        Section {
                            Text(model.status.signedOutReason).accessibilityIdentifier("signed-out-reason")
                            Button("OK") { model.dismissSignedOutReason() }
                                .accessibilityIdentifier("dismiss-signed-out-reason")
                        }
                    }
                    if mode == .signIn { signInSections } else { createAccountSections }
                    if mode == .signIn || form.canSubmit {
                        Section {
                            if let error = model.errorMessage { ErrorBanner(message: error) }
                            Button(action: submit) {
                                Text(model.isBusy ? (mode == .signIn ? "Signing in…" : "Creating account…") : mode.rawValue)
                                    .frame(maxWidth: .infinity)
                            }
                            .applyProminentGlassStyle()
                            .controlSize(.large)
                            .disabled(model.isBusy || isSubmitting || (mode == .createAccount && form.isChecking))
                            .accessibilityIdentifier("sign-in-submit")
                        }
                    }
                    if mode == .signIn {
                        Section {
                            Button("Can't find your Secret Key?") { isLostKeyHelpShown = true }
                                .accessibilityIdentifier("secret-key-note")
                        }
                    }
                }
                .scrollDismissesKeyboard(.interactively)
                .applyThemedListBackground()
                .navigationTitle(dynamicTypeSize.isAccessibilitySize ? "Hisar" : heading)
                .navigationBarTitleDisplayMode(dynamicTypeSize.isAccessibilitySize ? .inline : .large)
            }
        }
        .onAppear {
            if !model.status.server.isEmpty { form.changeServer(model.status.server) }
            applyPendingSignInLink()
            applyPendingInviteLink()
        }
        .onChange(of: mode) { _, _ in
            hasChosenEntry = true
            password = ""
            form.password = ""
            form.repeatedPassword = ""
            signInErrors = [:]
            form.clearErrors()
            if mode == .signIn { email = form.email } else { form.email = email }
            model.clearError()
        }
        .task(id: "\(mode.rawValue)\n\(form.server)\n\(hasChosenEntry)") {
            if mode == .createAccount && hasChosenEntry { await form.checkPolicy(using: model, debounce: isServerShown) }
        }
        .onChange(of: focusedField) { old, new in
            if old == .server && new != .server && mode == .createAccount && form.isChecking {
                Task { await form.checkPolicy(using: model) }
            }
            if new != nil { focusedCredential = nil }
        }
        .onChange(of: focusedCredential) { old, new in
            if old == .repeatedPassword && new != .repeatedPassword { form.validateRepeatedPassword() }
        }
        .onChange(of: form.repeatedPassword) { _, _ in
            if form.errors[.repeatedPassword] != nil { form.validateRepeatedPassword() }
        }
        .onChange(of: email) { _, _ in signInErrors[.email] = nil }
        .onChange(of: secretKey) { _, _ in signInErrors[.secretKey] = nil }
        .onChange(of: password) { _, _ in signInErrors[.password] = nil }
        .onChange(of: form.mode) { _, _ in
            if form.needsInvitation && form.invitation == nil { focusedField = .invite }
        }
        .onChange(of: form.invitation) { _, invitation in
            if invitation != nil { focusedField = .name }
        }
        .onChange(of: model.pendingSignInLink) { _, _ in applyPendingSignInLink() }
        .onChange(of: model.pendingInviteLink) { _, _ in applyPendingInviteLink() }
        .sheet(isPresented: $isScannerShown) {
            EmergencyKitScannerSheet { link in
                model.pendingSignInLink = link
                isScannerShown = false
            }
        }
        .sheet(isPresented: $isLostKeyHelpShown) { LostSecretKeyHelpSheet() }
    }

    private var welcome: some View {
        VStack(alignment: .leading, spacing: 24) {
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    LogoMark(size: 72)
                    Text("Welcome to Hisar").font(.largeTitle.bold()).accessibilityAddTraits(.isHeader)
                    Text("Passwords, SSH keys and secrets for developers, end-to-end encrypted.")
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            Button {
                mode = .createAccount
                hasChosenEntry = true
            } label: {
                Text("Create account").frame(maxWidth: .infinity, minHeight: 44)
            }
                .applyProminentGlassStyle()
                .accessibilityIdentifier("welcome-create-account")
            Button {
                mode = .signIn
                hasChosenEntry = true
            } label: {
                Text("I already have an account").frame(maxWidth: .infinity, minHeight: 44)
            }
                .accessibilityIdentifier("welcome-sign-in")
        }
        .padding(24)
    }

    @ViewBuilder
    private var serverSection: some View {
        Section {
            Text("\(mode == .signIn ? "Signs in to" : "Creates your account on") \(form.host).")
            if isServerShown {
                TextField("Server", text: serverBinding)
                    .keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                    .focused($focusedField, equals: .server)
                    .accessibilityIdentifier(mode == .signIn ? "sign-in-server" : "sign-up-server")
                    .onSubmit { Task { await form.checkPolicy(using: model) } }
                if !Self.isDefaultServer(form.server) {
                    Button("Use eu.tryhisar.com") { form.changeServer(Self.defaultServer); isServerShown = false }
                        .accessibilityIdentifier("sign-in-use-default-server")
                }
            } else {
                Button("Use a different server…") { isServerShown = true }
                    .accessibilityIdentifier(mode == .signIn ? "sign-in-use-different-server" : "sign-up-use-different-server")
            }
        }
    }

    @ViewBuilder
    private var signInSections: some View {
        Section {
            Button { isScannerShown = true } label: {
                Label("Scan Emergency Kit QR code", systemImage: "qrcode.viewfinder")
            }
            .accessibilityIdentifier("scan-emergency-kit")
            Text("The QR code fills in your server, email and Secret Key.").font(.footnote)
        }
        if let scannedLink {
            Section {
                Text("\(scannedLink.email) on \(Self.describeServer(scannedLink.server))")
                Text("Secret Key added from your Emergency Kit.")
                Button("Enter the details instead") { self.scannedLink = nil }
            }
        } else {
            serverSection
            Section("Or enter the details") {
                Text("All fields are required.").font(.footnote)
                TextField("Email", text: $email, prompt: Text(verbatim: "you@example.com"))
                    .textContentType(.username).keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never).autocorrectionDisabled()
                    .focused($focusedField, equals: .email)
                    .accessibilityLabel("Email, required")
                    .accessibilityIdentifier("sign-in-email")
                fieldError(signInErrors[.email])
                TextField("Secret Key", text: $secretKey, prompt: Text("H1-…"))
                    .font(.body.monospaced()).textInputAutocapitalization(.characters).autocorrectionDisabled()
                    .focused($focusedField, equals: .secretKey)
                    .accessibilityLabel("Secret Key, required")
                    .accessibilityIdentifier("sign-in-secret-key")
                fieldError(signInErrors[.secretKey])
                Text("Starts with H1-. It's on your Emergency Kit.").font(.footnote)
            }
        }
        Section {
            CredentialField("Master password, required", text: $password, prompt: "Master password")
                .textContentType(.password).focused(focus(.password)).submitLabel(.go).onSubmit(submit)
                .accessibilityIdentifier("sign-in-password").concealWhen(model.isBusy)
            fieldError(signInErrors[.password])
        }
    }

    @ViewBuilder
    private var createAccountSections: some View {
        serverSection
        if let message = form.invitationMessage { Section { Text(message) } }
        if let invitation = form.invitation {
            Section {
                Text(invitation.email.isEmpty ? "Invitation" : "Invitation for \(invitation.email)")
                Text("on \(Self.describeServer(invitation.server))")
                Button("Remove invitation") {
                    form.removeInvitation()
                    Task { await form.checkPolicy(using: model) }
                }
                .accessibilityIdentifier("sign-up-remove-invitation")
            }
        } else if case .failed(let message) = form.policy {
            Section {
                ErrorBanner(message: message)
                Button("Try again") { Task { await form.checkPolicy(using: model) } }
                    .accessibilityIdentifier("sign-up-policy-retry")
            }
        } else if form.isChecking {
            if form.isCheckingVisible { Section { ProgressView("Checking sign-up options on \(form.host)…") } }
        } else if form.showsInviteField {
            Section {
                if form.mode == "invite" {
                    Text("You need an invitation to create an account on \(form.host). Ask someone with an account there to invite you.")
                } else if form.mode == "setup" {
                    Text("\(form.host) is still being set up. You need an invitation from its admin.")
                }
                TextField(form.needsInvitation ? "Invite link or code" : "Invite link or code (if you have one)", text: $form.inviteText)
                    .textInputAutocapitalization(.never).autocorrectionDisabled()
                    .focused($focusedField, equals: .invite)
                    .accessibilityLabel(form.needsInvitation ? "Invite link or code, required" : "Invite link or code (if you have one)")
                    .accessibilityIdentifier("sign-up-invite-link")
                    .task(id: form.inviteText) {
                        do { try await Task.sleep(for: .milliseconds(500)) } catch { return }
                        form.parseInvitation()
                    }
                fieldError(form.errors[.invite])
                if form.mode == "unknown" {
                    Text("This server doesn't say whether it needs an invitation. If it does, Hisar tells you when you create the account.").font(.footnote)
                }
            }
        }
        if form.showsAccountFields {
            Section("You") {
                Text("All fields are required.").font(.footnote)
                TextField("Name", text: $form.name, prompt: Text("Your name"))
                    .textContentType(.name).focused($focusedField, equals: .name)
                    .accessibilityLabel("Name, required").accessibilityIdentifier("sign-up-name")
                fieldError(form.errors[.name])
                if let boundEmail = form.boundEmail {
                    Text(boundEmail).accessibilityIdentifier("sign-up-bound-email")
                    Text("This invitation is for this address.").font(.footnote)
                } else {
                    TextField("Email", text: $form.email, prompt: Text(verbatim: "you@example.com"))
                        .keyboardType(.emailAddress).textInputAutocapitalization(.never).autocorrectionDisabled()
                        .focused($focusedField, equals: .email)
                        .accessibilityLabel("Email, required").accessibilityIdentifier("sign-up-email")
                    fieldError(form.errors[.email])
                }
            }
            Section("Master password") {
                CredentialField("Master password, required", text: $form.password, prompt: "At least 10 characters")
                    .textContentType(.newPassword).focused(focus(.password))
                    .accessibilityIdentifier("sign-up-password").concealWhen(model.isBusy)
                fieldError(form.errors[.password])
                CredentialField("Repeat master password, required", text: $form.repeatedPassword, prompt: "Repeat master password", secretName: "repeated master password")
                    .textContentType(.newPassword).focused(focus(.repeatedPassword)).onSubmit(submit)
                    .accessibilityIdentifier("sign-up-repeated-password").concealWhen(model.isBusy)
                fieldError(form.errors[.repeatedPassword])
                PasswordStrengthMeter(strength: passwordFeedback?.strength ?? PasswordStrength())
                if let problem = AccountLifecycleLogic.describeMasterPasswordProblem(form.password, feedback: passwordFeedback) {
                    Text(problem).accessibilityIdentifier("sign-up-password-problem")
                } else if let hint = AccountLifecycleLogic.describeMasterPasswordHint(feedback: passwordFeedback) { Text(hint) }
                Text("Choose a master password you can remember. Hisar never sends it to the server, so nobody can reset it for you.")
                    .font(.footnote).accessibilityIdentifier("create-account-note")
                Text("Next, Hisar creates your Secret Key. You don't choose one.").font(.footnote)
            }
            .task(id: form.password) {
                let typedPassword = form.password
                let feedback = await model.checkMasterPassword(typedPassword)
                guard !Task.isCancelled else { return }
                ratedPassword = typedPassword
                passwordFeedback = feedback
            }
        }
    }

    @ViewBuilder private func fieldError(_ message: String?) -> some View {
        if let message { Label(message, systemImage: "exclamationmark.triangle").font(.footnote) }
    }

    static func describeServer(_ server: String) -> String { server.replacingOccurrences(of: "https://", with: "") }
    static func isDefaultServer(_ server: String) -> Bool {
        server.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || InviteLink.resolveServer(server) == defaultServer
    }

    private func applyPendingSignInLink() {
        guard let link = model.pendingSignInLink else { return }
        model.pendingSignInLink = nil
        mode = .signIn
        hasChosenEntry = true
        form.changeServer(link.server)
        email = link.email
        secretKey = link.secretKey
        scannedLink = link
    }

    private func applyPendingInviteLink() {
        guard let link = model.pendingInviteLink else { return }
        model.pendingInviteLink = nil
        mode = .createAccount
        hasChosenEntry = true
        form.applyInvitation(link)
    }

    private func submit() {
        guard !model.isBusy, !isSubmitting else { return }
        if mode == .signIn {
            signInErrors = [:]
            if email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { signInErrors[.email] = "Enter your email." }
            if secretKey.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { signInErrors[.secretKey] = "Enter your Secret Key." }
            if password.isEmpty { signInErrors[.password] = "Enter your master password." }
            if let field = [Field.email, .secretKey, .password].first(where: { signInErrors[$0] != nil }) {
                focusField(field)
                UIAccessibility.post(notification: .announcement, argument: signInErrors[field])
                return
            }
            isSubmitting = true
            Task {
                defer { isSubmitting = false }
                if await model.signIn(server: form.canonicalServer, email: email, secretKey: secretKey, password: password) {
                    password = ""; secretKey = ""
                }
            }
        } else {
            guard form.canSubmit else { return }
            if let field = form.validate(feedback: ratedPassword == form.password ? passwordFeedback : nil) {
                focusField([.invite: .invite, .name: .name, .email: .email, .password: .password, .repeatedPassword: .repeatedPassword][field])
                UIAccessibility.post(notification: .announcement, argument: form.errors[field])
                return
            }
            isSubmitting = true
            Task {
                defer { isSubmitting = false }
                if ratedPassword != form.password { passwordFeedback = await model.checkMasterPassword(form.password); ratedPassword = form.password }
                guard form.validate(feedback: passwordFeedback) == nil else { focusField(.password); return }
                _ = await form.submit(using: model)
            }
        }
    }
}
