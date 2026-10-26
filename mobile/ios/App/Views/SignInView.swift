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
    @State private var isScannerShown = false
    @State private var scannerMessage: String?

    private var isPasswordMismatch: Bool {
        mode == .createAccount && !repeatedPassword.isEmpty && password != repeatedPassword
    }

    private var canSubmit: Bool {
        switch mode {
        case .signIn:
            return !server.isEmpty && !email.isEmpty && !secretKey.isEmpty && !password.isEmpty
        case .createAccount:
            return !inviteLink.isEmpty && !email.isEmpty && !name.isEmpty && !password.isEmpty && password == repeatedPassword
        }
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
                    Text(mode == .signIn ? "Lost your Secret Key? It's on your Emergency Kit." : "Your master password never leaves this iPhone. A forgotten master password can't be recovered by anyone, including Hisar.")
                        .padding(.top, 8)
                        .accessibilityIdentifier(mode == .signIn ? "secret-key-note" : "create-account-note")
                }
            }
            .scrollDismissesKeyboard(.interactively)
            .applyThemedListBackground()
            .navigationTitle(mode == .signIn ? "Welcome back" : "Create account")
        }
        .onChange(of: mode) { _, _ in
            password = ""
            repeatedPassword = ""
            model.clearError()
        }
        .onAppear {
            if !model.status.server.isEmpty { applyServer(model.status.server) }
            applyPendingSignInLink()
            applyPendingInviteLink()
        }
        .onChange(of: model.pendingSignInLink) { _, _ in applyPendingSignInLink() }
        .onChange(of: model.pendingInviteLink) { _, _ in applyPendingInviteLink() }
        .sheet(isPresented: $isScannerShown) {
            EmergencyKitScannerSheet { scannedText in
                isScannerShown = false
                if let link = SignInLink.parse(scannedText) {
                    model.pendingSignInLink = link
                } else {
                    scannerMessage = "This QR code is not from a Hisar Emergency Kit."
                }
            }
        }
        .alert("Scan Emergency Kit", isPresented: Binding(get: { scannerMessage != nil }, set: { if !$0 { scannerMessage = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(scannerMessage ?? "")
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
                TextField("Server", text: $server, prompt: Text(verbatim: SignInView.defaultServer))
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("sign-in-server")
            } else {
                Button("Use a different server…") {
                    isServerShown = true
                }
                .accessibilityIdentifier("sign-in-use-different-server")
            }
        } header: {
            Text("Account")
        } footer: {
            if !isServerShown {
                Text(verbatim: "Signs in to \(SignInView.describeServer(server)).")
            }
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
        Section("Invite") {
            TextField("Invite code or link", text: $inviteLink, prompt: Text("Invite code or link"))
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("sign-up-invite-link")
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
                .accessibilityIdentifier("sign-up-repeated-password")
        } header: {
            Text("Master password")
        } footer: {
            VStack(alignment: .leading, spacing: 6) {
                if isPasswordMismatch {
                    Text("The passwords do not match.")
                        .foregroundStyle(.red)
                }
                Text("Choose a master password you can remember. Hisar never sends it to the server, so nobody can reset it for you.")
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
        guard canSubmit, !model.isBusy else { return }
        Task {
            switch mode {
            case .signIn:
                if await model.signIn(server: server, email: email, secretKey: secretKey, password: password) {
                    password = ""
                    secretKey = ""
                }
            case .createAccount:
                if await model.signUp(inviteLink: inviteLink, email: email, name: name, password: password) {
                    password = ""
                    repeatedPassword = ""
                }
            }
        }
    }
}
