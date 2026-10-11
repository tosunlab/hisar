import SwiftUI

struct SignInView: View {
    enum Mode: String, CaseIterable, Identifiable {
        case signIn = "Sign in"
        case createAccount = "Create account"
        var id: String { rawValue }
    }

    @EnvironmentObject private var model: AppModel
    @State private var mode: Mode = .signIn
    @State private var server = "https://"
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
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                Wordmark(size: 36)
                    .padding(.top, 24)
                Picker("Mode", selection: $mode) {
                    ForEach(Mode.allCases) { mode in
                        Text(mode.rawValue).tag(mode)
                    }
                }
                .pickerStyle(.segmented)
                .accessibilityIdentifier("sign-in-mode")
                VStack(alignment: .leading, spacing: 6) {
                    Text(mode == .signIn ? "Welcome back" : "Create your Hisar account")
                        .font(.largeTitle.weight(.bold))
                        .foregroundStyle(Theme.text)
                    Text(mode == .signIn ? "Sign in to use Hisar on this iPhone." : "Use the invite link your Hisar admin sent you.")
                        .font(.subheadline)
                        .foregroundStyle(Theme.textMuted)
                }
                if let errorMessage = model.errorMessage {
                    ErrorBanner(message: errorMessage)
                }
                if mode == .signIn {
                    signInFields
                } else {
                    createAccountFields
                }
                Button(action: submit) {
                    if model.isBusy {
                        ProgressView().tint(Theme.primaryText)
                    } else {
                        Text(mode == .signIn ? "Sign in" : "Create account")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(!canSubmit || model.isBusy)
                .accessibilityIdentifier("sign-in-submit")
                if mode == .signIn {
                    Text("Your Secret Key is on your Emergency Kit, or in Settings of a device where you already use Hisar.")
                        .font(.footnote)
                        .foregroundStyle(Theme.textMuted)
                }
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 32)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.background)
        .coverStatusBar()
        .onChange(of: mode) { _, _ in
            password = ""
            repeatedPassword = ""
            model.clearError()
        }
        .onAppear(perform: applyPendingSignInLink)
        .onChange(of: model.pendingSignInLink) { _, _ in applyPendingSignInLink() }
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

    private var signInFields: some View {
        VStack(alignment: .leading, spacing: 16) {
            Button {
                isScannerShown = true
            } label: {
                Label("Scan Emergency Kit QR code", systemImage: "qrcode.viewfinder")
            }
            .buttonStyle(SecondaryButtonStyle())
            .accessibilityIdentifier("scan-emergency-kit")
            LabeledInput(label: "Server", systemImage: "globe") {
                TextField("Server", text: $server, prompt: Text(verbatim: "https://hisar.example.com"))
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("sign-in-server")
            }
            LabeledInput(label: "Email", systemImage: "person") {
                TextField("Email", text: $email, prompt: Text(verbatim: "you@example.com").foregroundStyle(Theme.textMuted))
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("sign-in-email")
            }
            LabeledInput(label: "Secret Key", systemImage: "key") {
                TextField("H1-XXXXXX-XXXXX-…", text: $secretKey)
                    .font(.system(.body, design: .monospaced))
                    .textInputAutocapitalization(.characters)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("sign-in-secret-key")
            }
            LabeledInput(label: "Master password", systemImage: "lock") {
                SecureField("Master password", text: $password)
                    .submitLabel(.go)
                    .onSubmit(submit)
                    .accessibilityIdentifier("sign-in-password")
            }
        }
    }

    private var createAccountFields: some View {
        VStack(alignment: .leading, spacing: 16) {
            LabeledInput(label: "Invite link", systemImage: "link") {
                TextField("Invite link", text: $inviteLink, prompt: Text(verbatim: "https://hisar.example.com/invite/…"))
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("sign-up-invite-link")
            }
            LabeledInput(label: "Email", systemImage: "person") {
                TextField("Email", text: $email, prompt: Text(verbatim: "you@example.com").foregroundStyle(Theme.textMuted))
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("sign-up-email")
            }
            LabeledInput(label: "Your name", systemImage: "person.text.rectangle") {
                TextField("Name", text: $name)
                    .textContentType(.name)
                    .accessibilityIdentifier("sign-up-name")
            }
            LabeledInput(label: "Master password", systemImage: "lock") {
                SecureField("At least 10 characters", text: $password)
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("sign-up-password")
            }
            LabeledInput(label: "Repeat master password", systemImage: "lock") {
                SecureField("Repeat master password", text: $repeatedPassword)
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("sign-up-repeated-password")
            }
            if isPasswordMismatch {
                Text("The passwords do not match.")
                    .font(.footnote)
                    .foregroundStyle(Theme.danger)
            }
            Text("Choose a master password you can remember. Hisar never sends it to the server, so nobody can reset it for you.")
                .font(.footnote)
                .foregroundStyle(Theme.textMuted)
        }
    }

    private func applyPendingSignInLink() {
        guard let link = model.pendingSignInLink else { return }
        model.pendingSignInLink = nil
        mode = .signIn
        if !link.server.isEmpty { server = link.server }
        if !link.email.isEmpty { email = link.email }
        if !link.secretKey.isEmpty { secretKey = link.secretKey }
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
