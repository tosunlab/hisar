import SwiftUI

struct SignInView: View {
    enum Mode: String, CaseIterable, Identifiable {
        case signIn = "Sign in"
        case createAccount = "Create account"
        var id: String { rawValue }
    }

    static let defaultServer = "https://eu.tryhisar.com"

    @EnvironmentObject private var model: AppModel
    @State private var mode: Mode = .signIn
    @State private var server = SignInView.defaultServer
    @State private var isServerShown = false
    @State private var email = ""
    @State private var secretKey = ""
    @State private var password = ""
    @State private var inviteLink = ""
    @State private var name = ""
    @State private var repeatedPassword = ""
    @FocusState private var isRepeatPasswordFocused: Bool
    @State private var hasValidatedRepeatedPassword = false
    @State private var isScannerShown = false

    private var isPasswordMismatch: Bool {
        mode == .createAccount && hasValidatedRepeatedPassword && password != repeatedPassword
    }

    private var canSubmit: Bool {
        switch mode {
        case .signIn:
            return !server.isEmpty && !email.isEmpty && !secretKey.isEmpty && !password.isEmpty
        case .createAccount:
            return (!isSignUpServerAsked || !server.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty) && !email.isEmpty && !name.isEmpty && !password.isEmpty && password == repeatedPassword
        }
    }

    /// An invite link names its server, so the create-account form asks for one only without an invite.
    private var isSignUpServerAsked: Bool {
        inviteLink.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    VStack(alignment: .leading, spacing: 14) {
                        LogoMark(size: 56)
                        Picker("Mode", selection: $mode) {
                            ForEach(Mode.allCases) { mode in
                                Text(mode.rawValue).tag(mode)
                            }
                        }
                        .pickerStyle(.segmented)
                        .accessibilityIdentifier("sign-in-mode")
                    }
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }
                if !model.status.signedOutReason.isEmpty {
                    Section {
                        Label(model.status.signedOutReason, systemImage: "info.circle")
                            .font(.subheadline)
                            .accessibilityIdentifier("signed-out-reason")
                        Button("OK") {
                            model.dismissSignedOutReason()
                        }
                        .accessibilityIdentifier("dismiss-signed-out-reason")
                    }
                }
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                if mode == .signIn {
                    signInSections
                } else {
                    createAccountSections
                }
                Section {
                    Button(action: submit) {
                        Group {
                            if model.isBusy {
                                ProgressView()
                            } else {
                                Text(mode == .signIn ? "Sign in" : "Create account")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .controlSize(.large)
                    .disabled(!canSubmit || model.isBusy)
                    .accessibilityIdentifier("sign-in-submit")
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                } footer: {
                    if mode == .signIn {
                        Text("Lost your Secret Key? It is on your Emergency Kit. If you can unlock another signed-in device with your master password, save a new Emergency Kit there in Settings.")
                            .padding(.top, 8)
                            .accessibilityIdentifier("secret-key-note")
                    }
                }
            }
            .scrollDismissesKeyboard(.interactively)
            .applyThemedListBackground()
            .navigationTitle(mode == .signIn ? "Welcome back" : "Create account")
        }
        .onChange(of: mode) { _, _ in
            password = ""
            repeatedPassword = ""
            hasValidatedRepeatedPassword = false
            model.clearError()
        }
        .onChange(of: isRepeatPasswordFocused) { _, isFocused in
            if !isFocused && !repeatedPassword.isEmpty {
                hasValidatedRepeatedPassword = true
            }
        }
        .onAppear {
            if !model.status.server.isEmpty { applyServer(model.status.server) }
            applyPendingSignInLink()
            applyPendingInviteLink()
        }
        .onChange(of: model.pendingSignInLink) { _, _ in applyPendingSignInLink() }
        .onChange(of: model.pendingInviteLink) { _, _ in applyPendingInviteLink() }
        .sheet(isPresented: $isScannerShown) {
            EmergencyKitScannerSheet { link in
                model.pendingSignInLink = link
                isScannerShown = false
            }
        }
    }

    @ViewBuilder
    private var signInSections: some View {
        Section {
            Button {
                isScannerShown = true
            } label: {
                Label("Scan Emergency Kit QR code", systemImage: "qrcode.viewfinder")
            }
            .accessibilityIdentifier("scan-emergency-kit")
        } footer: {
            Text("The QR code fills in your server, email and Secret Key.")
        }
        Section {
            TextField("Email", text: $email, prompt: Text(verbatim: "you@example.com"))
                .textContentType(.username)
                .keyboardType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("sign-in-email")
            if isServerShown {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Server")
                        .font(.subheadline)
                    TextField("Server", text: $server, prompt: Text(verbatim: SignInView.defaultServer))
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("sign-in-server")
                }
                if !SignInView.isDefaultServer(server) {
                    Button("Use eu.tryhisar.com") {
                        server = Self.defaultServer
                        isServerShown = false
                    }
                    .accessibilityIdentifier("sign-in-use-default-server")
                }
            } else {
                Button("Use a different server…") {
                    isServerShown = true
                }
                .accessibilityIdentifier("sign-in-use-different-server")
            }
        } header: {
            Text("Account")
        } footer: {
            Text(verbatim: "Signs in to \(SignInView.describeServer(server)).")
        }
        Section("Secret Key and master password") {
            TextField("Secret Key", text: $secretKey, prompt: Text(verbatim: "H1-XXXXXX-XXXXX-…"))
                .font(.body.monospaced())
                .textInputAutocapitalization(.characters)
                .autocorrectionDisabled()
                .accessibilityIdentifier("sign-in-secret-key")
            SecureField("Master password", text: $password)
                .textContentType(.password)
                .submitLabel(.go)
                .onSubmit(submit)
                .accessibilityIdentifier("sign-in-password")
        }
    }

    @ViewBuilder
    private var createAccountSections: some View {
        Section {
            TextField("Invite code or link (optional)", text: $inviteLink, prompt: Text("Invite code or link (optional)"))
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("sign-up-invite-link")
        } header: {
            Text("Invite")
        } footer: {
            Text("Leave it empty if the server lets anyone sign up.")
        }
        if isSignUpServerAsked {
            Section {
                if isServerShown {
                    TextField("Server", text: $server, prompt: Text(verbatim: SignInView.defaultServer))
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("sign-up-server")
                } else {
                    Button("Use a different server…") {
                        isServerShown = true
                    }
                    .accessibilityIdentifier("sign-up-use-different-server")
                }
            } header: {
                Text("Server")
            } footer: {
                if !isServerShown {
                    Text(verbatim: "Creates your account on \(SignInView.describeServer(server)).")
                }
            }
        }
        Section("You") {
            TextField("Email", text: $email, prompt: Text(verbatim: "you@example.com"))
                .keyboardType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("sign-up-email")
            TextField("Your name", text: $name)
                .textContentType(.name)
                .accessibilityIdentifier("sign-up-name")
        }
        Section {
            SecureField("Master password", text: $password, prompt: Text("At least 10 characters"))
                .textContentType(.newPassword)
                .accessibilityIdentifier("sign-up-password")
            SecureField("Repeat master password", text: $repeatedPassword)
                .textContentType(.newPassword)
                .focused($isRepeatPasswordFocused)
                .accessibilityIdentifier("sign-up-repeated-password")
        } header: {
            Text("Master password")
        } footer: {
            VStack(alignment: .leading, spacing: 6) {
                if isPasswordMismatch {
                    Text("The passwords do not match.")
                        .foregroundStyle(.red)
                }
                Text("Choose a master password you can remember. A forgotten master password can't be recovered by anyone, including Hisar.")
                    .accessibilityIdentifier("create-account-note")
            }
        }
    }

    static func describeServer(_ server: String) -> String {
        server.replacingOccurrences(of: "https://", with: "")
    }

    static func isDefaultServer(_ server: String) -> Bool {
        let trimmedServer = server.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmedServer.isEmpty || trimmedServer.trimmingCharacters(in: CharacterSet(charactersIn: "/")) == defaultServer
    }

    /// applyServer fills in a remembered or scanned server; it stays behind "Use a different server…", and the footer names it.
    private func applyServer(_ newServer: String) {
        server = newServer
    }

    private func applyPendingSignInLink() {
        guard let link = model.pendingSignInLink else { return }
        model.pendingSignInLink = nil
        mode = .signIn
        if !link.server.isEmpty { applyServer(link.server) }
        if !link.email.isEmpty { email = link.email }
        if !link.secretKey.isEmpty { secretKey = link.secretKey }
    }

    private func applyPendingInviteLink() {
        guard let link = model.pendingInviteLink else { return }
        model.pendingInviteLink = nil
        mode = .createAccount
        inviteLink = link.invite
    }

    private func submit() {
        if mode == .createAccount { hasValidatedRepeatedPassword = true }
        guard canSubmit, !model.isBusy else { return }
        Task {
            switch mode {
            case .signIn:
                if await model.signIn(server: server, email: email, secretKey: secretKey, password: password) {
                    password = ""
                    secretKey = ""
                }
            case .createAccount:
                let isSignedUp = if isSignUpServerAsked {
                    await model.signUpWithoutInvite(server: server, email: email, name: name, password: password)
                } else {
                    await model.signUp(inviteLink: inviteLink, email: email, name: name, password: password)
                }
                if isSignedUp {
                    password = ""
                    repeatedPassword = ""
                }
            }
        }
    }
}
