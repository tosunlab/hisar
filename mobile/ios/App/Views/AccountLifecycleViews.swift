import SwiftUI

struct ChangeMasterPasswordView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var currentPassword = ""
    @State private var newPassword = ""
    @State private var repeatedPassword = ""
    @State private var isConfirmationShown = false

    private var problem: String? {
        AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: currentPassword, newPassword: newPassword, repeatedPassword: repeatedPassword)
    }

    private var canSubmit: Bool {
        AccountLifecycleLogic.canChangePassword(currentPassword: currentPassword, newPassword: newPassword, repeatedPassword: repeatedPassword) && !model.isBusy
    }

    var body: some View {
        Form {
            Section {
                Label(AccountLifecycleLogic.passwordChangeWarning, systemImage: "iphone.and.arrow.forward")
                    .accessibilityIdentifier("change-password-warning")
            } footer: {
                Text("Your Secret Key, your Emergency Kit and your items stay the same. Hisar never sends your master password to the server.")
            }
            Section("Current master password") {
                SecureField("Current master password", text: $currentPassword)
                    .textContentType(.password)
                    .accessibilityIdentifier("change-password-current")
            }
            Section {
                SecureField("New master password", text: $newPassword, prompt: Text("At least 10 characters"))
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("change-password-new")
                SecureField("Repeat new master password", text: $repeatedPassword)
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("change-password-repeated")
            } header: {
                Text("New master password")
            } footer: {
                VStack(alignment: .leading, spacing: 6) {
                    if let problem {
                        Text(problem)
                            .foregroundStyle(.red)
                            .accessibilityIdentifier("change-password-problem")
                    }
                    Text("A forgotten master password can't be recovered by anyone, including Hisar.")
                }
            }
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            Section {
                Button {
                    isConfirmationShown = true
                } label: {
                    Group {
                        if model.isBusy {
                            ProgressView()
                        } else {
                            Text("Change master password")
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .applyProminentGlassStyle()
                .controlSize(.large)
                .disabled(!canSubmit)
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
                .accessibilityIdentifier("change-password-submit")
            }
        }
        .applyThemedListBackground()
        .navigationTitle("Change master password")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { model.clearError() }
        .onDisappear(perform: clearAllField)
        .confirmationDialog("Change your master password?", isPresented: $isConfirmationShown, titleVisibility: .visible) {
            Button("Change master password", action: submit)
                .accessibilityIdentifier("change-password-confirm")
        } message: {
            Text(AccountLifecycleLogic.passwordChangeWarning)
        }
    }

    private func clearAllField() {
        currentPassword = ""
        newPassword = ""
        repeatedPassword = ""
    }

    private func submit() {
        let enteredCurrentPassword = currentPassword
        let enteredNewPassword = newPassword
        Task {
            if await model.changeMasterPassword(currentPassword: enteredCurrentPassword, newPassword: enteredNewPassword) != nil {
                clearAllField()
                dismiss()
            }
        }
    }
}

struct ReplaceSecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var password = ""
    @State private var result: CredentialChangeResult?
    @State private var isConfirmationShown = false

    var body: some View {
        Form {
            if let result {
                replacedSections(result)
            } else {
                confirmationSections
            }
        }
        .applyThemedListBackground()
        .navigationTitle("Replace Secret Key")
        .navigationBarTitleDisplayMode(.inline)
        .navigationBarBackButtonHidden(result != nil)
        .onAppear { model.clearError() }
        .onDisappear {
            if let result {
                model.finishSecretKeyReplacement(result)
            }
            result = nil
            password = ""
        }
        .confirmationDialog("Replace your Secret Key?", isPresented: $isConfirmationShown, titleVisibility: .visible) {
            Button("Replace Secret Key", role: .destructive, action: submit)
                .accessibilityIdentifier("replace-secret-key-confirm")
        } message: {
            Text(AccountLifecycleLogic.secretKeyReplacementWarning)
        }
    }

    @ViewBuilder
    private var confirmationSections: some View {
        Section {
            Label(AccountLifecycleLogic.secretKeyReplacementWarning, systemImage: "doc.badge.arrow.up")
                .accessibilityIdentifier("replace-secret-key-warning")
        } footer: {
            Text("Replace it if your Secret Key or Emergency Kit may have been seen by someone else. Your master password and your items stay the same.")
        }
        Section("Master password") {
            SecureField("Master password", text: $password)
                .textContentType(.password)
                .accessibilityIdentifier("replace-secret-key-password")
        }
        if let errorMessage = model.errorMessage {
            Section {
                ErrorBanner(message: errorMessage)
            }
        }
        Section {
            Button {
                isConfirmationShown = true
            } label: {
                Group {
                    if model.isBusy {
                        ProgressView()
                    } else {
                        Text("Replace Secret Key")
                    }
                }
                .frame(maxWidth: .infinity)
            }
            .applyProminentGlassStyle()
            .controlSize(.large)
            .disabled(password.isEmpty || model.isBusy)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
            .accessibilityIdentifier("replace-secret-key-submit")
        }
    }

    @ViewBuilder
    private func replacedSections(_ result: CredentialChangeResult) -> some View {
        Section {
            Text(result.secretKey)
                .font(.body.monospaced())
                .textSelection(.enabled)
                .accessibilityIdentifier("new-secret-key-value")
            Button {
                model.copyText(result.secretKey, label: "Secret Key")
            } label: {
                Label("Copy Secret Key", systemImage: "doc.on.doc")
            }
        } header: {
            Text("New Secret Key")
        } footer: {
            Text("\(AccountLifecycleLogic.describeSignedOutDevices(result.signedOutDeviceCount)) Your old Secret Key and Emergency Kit no longer work.")
        }
        if !result.keychainError.isEmpty {
            Section {
                ErrorBanner(message: result.keychainError)
            }
        }
        if !result.localError.isEmpty {
            Section {
                ErrorBanner(message: result.localError)
            }
        }
        if !result.emergencyKitPath.isEmpty {
            Section {
                ShareLink(item: URL(fileURLWithPath: result.emergencyKitPath), preview: SharePreview("Hisar Emergency Kit", image: Image(systemName: "doc.text"))) {
                    Label("Save or Print Emergency Kit", systemImage: "square.and.arrow.up")
                }
                .accessibilityIdentifier("share-new-emergency-kit")
            } header: {
                Text("New Emergency Kit")
            } footer: {
                Text("The Emergency Kit holds your Secret Key and has a space to write your master password. Save the new one now, and destroy the old one.")
            }
        }
        Section {
            Button {
                model.finishSecretKeyReplacement(result)
                self.result = nil
                dismiss()
            } label: {
                Text("Done")
                    .frame(maxWidth: .infinity)
            }
            .applyProminentGlassStyle()
            .controlSize(.large)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
            .accessibilityIdentifier("replace-secret-key-done")
        }
    }

    private func submit() {
        let enteredPassword = password
        Task {
            if let replaced = await model.replaceSecretKey(password: enteredPassword) {
                password = ""
                result = replaced
            }
        }
    }
}

struct DeleteAccountView: View {
    @EnvironmentObject private var model: AppModel
    @State private var preview: AccountDeletionPreview?
    @State private var isPreviewLoading = true
    @State private var password = ""
    @State private var typedEmail = ""
    @State private var isConfirmationShown = false

    private var canSubmit: Bool {
        !password.isEmpty && AccountLifecycleLogic.isEmailMatching(typedEmail: typedEmail, accountEmail: model.status.email) && !model.isBusy
    }

    var body: some View {
        Form {
            Section {
                Label("Deleting your account can't be undone. \(AccountLifecycleLogic.exportAdvice)", systemImage: "exclamationmark.triangle")
                    .accessibilityIdentifier("delete-account-warning")
            }
            if isPreviewLoading {
                Section {
                    ProgressView()
                        .frame(maxWidth: .infinity)
                }
            } else if let preview {
                consequenceSections(preview)
            }
            Section {
                SecureField("Master password", text: $password)
                    .textContentType(.password)
                    .accessibilityIdentifier("delete-account-password")
                TextField("Email", text: $typedEmail, prompt: Text(verbatim: model.status.email))
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("delete-account-email")
            } header: {
                Text("Confirm")
            } footer: {
                Text("Enter your master password and type \(model.status.email) to confirm. Hisar checks your master password without sending it to the server.")
            }
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            Section {
                Button(role: .destructive) {
                    isConfirmationShown = true
                } label: {
                    Group {
                        if model.isBusy {
                            ProgressView()
                        } else {
                            Text("Delete account")
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(.red)
                .controlSize(.large)
                .disabled(!canSubmit)
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
                .accessibilityIdentifier("delete-account-submit")
            }
        }
        .applyThemedListBackground()
        .navigationTitle("Delete account")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            model.clearError()
            preview = await model.loadAccountDeletionPreview()
            isPreviewLoading = false
        }
        .onDisappear {
            password = ""
            typedEmail = ""
        }
        .confirmationDialog("Delete your Hisar account?", isPresented: $isConfirmationShown, titleVisibility: .visible) {
            Button("Delete account", role: .destructive, action: submit)
                .accessibilityIdentifier("delete-account-confirm")
        } message: {
            Text("Your account, your vaults that have no other owner and their items are deleted for good. This can't be undone.")
        }
    }

    @ViewBuilder
    private func consequenceSections(_ preview: AccountDeletionPreview) -> some View {
        if !preview.allOwnedVault.isEmpty {
            Section("Vaults that are deleted") {
                ForEach(preview.allOwnedVault) { vault in
                    LabeledContent(vault.name, value: AccountLifecycleLogic.describeOwnedVault(vault))
                }
            }
        }
        if !preview.allMemberVault.isEmpty {
            Section("Vaults you leave") {
                ForEach(preview.allMemberVault) { vault in
                    LabeledContent(vault.name, value: AccountLifecycleLogic.describeMemberVault(vault))
                }
            }
        }
        if !preview.allOrganization.isEmpty {
            Section("Organizations") {
                ForEach(preview.allOrganization) { organization in
                    Text(AccountLifecycleLogic.describeOrganization(organization))
                        .accessibilityIdentifier("deletion-organization-\(organization.name)")
                }
            }
        }
        Section("Also") {
            ForEach(AccountLifecycleLogic.describeAllOtherConsequence(preview), id: \.self) { consequence in
                Text(consequence)
            }
        }
    }

    private func submit() {
        let enteredPassword = password
        let enteredEmail = typedEmail
        Task {
            if await model.deleteAccount(password: enteredPassword, email: enteredEmail) {
                password = ""
                typedEmail = ""
            }
        }
    }
}
