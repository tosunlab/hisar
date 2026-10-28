import SwiftUI

struct ChangeMasterPasswordView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var currentPassword = ""
    @State private var newPassword = ""
    @State private var repeatedPassword = ""
    @State private var isConfirmationShown = false
    @State private var ratedPassword = ""
    @State private var newPasswordFeedback: MasterPasswordFeedback?

    private var problem: String? {
        AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: currentPassword, newPassword: newPassword, repeatedPassword: repeatedPassword, feedback: newPasswordFeedback)
    }

    private var newPasswordProblem: String? {
        AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: currentPassword, newPassword: newPassword, repeatedPassword: newPassword, feedback: newPasswordFeedback)
    }

    private var canSubmit: Bool {
        AccountLifecycleLogic.canChangePassword(
            currentPassword: currentPassword, newPassword: newPassword, repeatedPassword: repeatedPassword,
            feedback: ratedPassword == newPassword ? newPasswordFeedback : nil
        ) && !model.isBusy
    }

    var body: some View {
        Form {
            Section {
                CredentialField("Current master password", text: $currentPassword, secretName: "current master password")
                    .textContentType(.password)
                    .accessibilityIdentifier("change-password-current")
                    .concealWhen(model.isBusy || isConfirmationShown)
            } header: {
                Text("Current master password")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                Text("Your Secret Key, your Emergency Kit and your items stay the same. Hisar never sends your master password to the server.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                CredentialField("New master password", text: $newPassword, prompt: "At least 10 characters", secretName: "new master password")
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("change-password-new")
                    .concealWhen(model.isBusy || isConfirmationShown)
                    .validationError(newPasswordProblem)
                CredentialField("Repeat new master password", text: $repeatedPassword, secretName: "repeated new master password")
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("change-password-repeated")
                    .concealWhen(model.isBusy || isConfirmationShown)
                    .validationError(newPasswordProblem == nil ? problem : nil)
            } header: {
                Text("New master password")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                VStack(alignment: .leading, spacing: 6) {
                    PasswordStrengthMeter(strength: newPasswordFeedback?.strength ?? PasswordStrength())
                    if let problem {
                        Label {
                            Text(problem).foregroundStyle(.primary)
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.red)
                        }
                            .accessibilityIdentifier("change-password-problem")
                    } else if let hint = AccountLifecycleLogic.describeMasterPasswordHint(feedback: newPasswordFeedback) {
                        Text(hint)
                            .foregroundStyle(.themeSecondary)
                    }
                    Text("A forgotten master password can't be recovered by anyone, including Hisar.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            .task(id: newPassword) {
                let typedPassword = newPassword
                let feedback = await model.checkMasterPassword(typedPassword)
                guard !Task.isCancelled else { return }
                ratedPassword = typedPassword
                newPasswordFeedback = feedback
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
    @State private var isConfirmationShown = false

    var body: some View {
        Form {
            confirmationSections
        }
        .applyThemedListBackground()
        .navigationTitle("Replace Secret Key")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { model.clearError() }
        .onDisappear { password = "" }
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
            CredentialField("Master password", text: $password)
                .textContentType(.password)
                .accessibilityIdentifier("replace-secret-key-password")
                .concealWhen(model.isBusy || isConfirmationShown)
        } header: {
            Text("Master password")
                .foregroundStyle(.themeSecondary)
        } footer: {
            Text("Replace it if your Secret Key or Emergency Kit may have been seen by someone else. Your master password and your items stay the same.")
                .foregroundStyle(.themeSecondary)
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


    private func submit() {
        let enteredPassword = password
        Task {
            if await model.replaceSecretKey(password: enteredPassword) != nil {
                password = ""
                dismiss()
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
        preview?.planVersion.isEmpty == false && !isPreviewLoading && !password.isEmpty && AccountLifecycleLogic.isEmailMatching(typedEmail: typedEmail, accountEmail: model.status.email) && !model.isBusy
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
                CredentialField("Master password", text: $password)
                    .textContentType(.password)
                    .accessibilityIdentifier("delete-account-password")
                    .concealWhen(model.isBusy || isConfirmationShown)
                TextField("Email", text: $typedEmail, prompt: Text(verbatim: model.status.email))
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("delete-account-email")
            } header: {
                Text("Confirm")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                Text("Enter your master password and type \(model.status.email) to confirm. Hisar checks your master password without sending it to the server.")
                    .foregroundStyle(.themeSecondary)
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
                .buttonStyle(.glassProminent)
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
            Section {
                ForEach(preview.allOwnedVault) { vault in
                    LabeledContent(vault.name, value: AccountLifecycleLogic.describeOwnedVault(vault))
                }
            } header: {
                Text("Vaults that are deleted")
                    .foregroundStyle(.themeSecondary)
            }
        }
        if !preview.allMemberVault.isEmpty {
            Section {
                ForEach(preview.allMemberVault) { vault in
                    LabeledContent(vault.name, value: AccountLifecycleLogic.describeMemberVault(vault))
                }
            } header: {
                Text("Vaults you leave")
                    .foregroundStyle(.themeSecondary)
            }
        }
        if !preview.allOrganization.isEmpty {
            Section {
                ForEach(preview.allOrganization) { organization in
                    Text(AccountLifecycleLogic.describeOrganization(organization))
                        .accessibilityIdentifier("deletion-organization-\(organization.name)")
                }
            } header: {
                Text("Organizations")
                    .foregroundStyle(.themeSecondary)
            }
        }
        Section {
            ForEach(AccountLifecycleLogic.describeAllOtherConsequence(preview), id: \.self) { consequence in
                Text(consequence)
            }
        } header: {
            Text("Also")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func submit() {
        guard let reviewed = preview, !reviewed.planVersion.isEmpty else { return }
        let enteredPassword = password
        let enteredEmail = typedEmail
        password = ""
        typedEmail = ""
        isConfirmationShown = false
        preview = nil
        Task {
            let deleted = await model.deleteAccount(password: enteredPassword, email: enteredEmail, planVersion: reviewed.planVersion)
            if !deleted && model.status.isUnlocked {
                let deletionError = model.errorMessage
                isPreviewLoading = true
                preview = await model.loadAccountDeletionPreview()
                isPreviewLoading = false
                model.errorMessage = deletionError
            }
        }
    }
}

struct PendingRecoveryView: View {
    @EnvironmentObject private var model: AppModel
    private var selectedID: String? { model.selectedPendingRecoveryID }
    @State private var password = ""
    @State private var secretKey = ""
    @State private var priorSecretKey = ""
    @State private var isDiscardConfirmationShown = false

    private func clearCredentials() {
        password = ""
        secretKey = ""
        priorSecretKey = ""
        isDiscardConfirmationShown = false
    }

    private func handleOutcome(_ succeeded: Bool, isFinal: Bool = false) {
        if succeeded && isFinal { clearCredentials() }
        else if !succeeded && model.lastErrorCode == BackendError.pendingRecoveryPassword { password = "" }
    }

    private var selected: PendingRecovery? {
        model.allPendingRecovery.first { $0.operationId == selectedID }
    }


    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Save and confirm your Emergency Kit to finish this setup.")
                }
                if model.allPendingRecovery.count > 1 {
                    Section {
                        ForEach(model.allPendingRecovery) { recovery in
                            Button {
                                if selectedID != recovery.operationId {
                                    clearCredentials()
                                    model.selectPendingRecovery(recovery.operationId)
                                }
                            } label: {
                                HStack {
                                    VStack(alignment: .leading) {
                                        Text(PendingRecoveryCopy.heading(recovery.kind))
                                        Text("\(recovery.email) · \(PendingRecoveryCopy.host(recovery.origin))").font(.caption)
                                    }
                                    Spacer()
                                    Image(systemName: "chevron.right").accessibilityHidden(true)
                                }
                            }
                            .accessibilityIdentifier("pending-recovery-\(recovery.operationId)")
                        }
                    } header: {
                        Text("Choose an account")
                            .foregroundStyle(.themeSecondary)
                    }
                }
                if let recovery = selected {
                    Section {
                        LabeledContent("Server", value: recovery.origin)
                        Text(PendingRecoveryCopy.status(kind: recovery.kind, phase: recovery.phase))
                            .accessibilityIdentifier("pending-recovery-phase")
                        if !recovery.keychainProtected {
                            Text("The OS keychain is unavailable. Save the Emergency Kit before continuing. After a restart, enter the Secret Key from that kit. Hisar does not store the Secret Key in this recovery record.")
                            if recovery.phase == "awaiting-kit" || recovery.phase == "prepared" {
                                Text("No server change has been sent for this setup. If Hisar restarted before you saved a kit, discard this setup and start again.")
                            }
                        }
                        if !recovery.keychainProtected {
                            SecureField("Secret Key from saved Emergency Kit", text: $secretKey)
                                .accessibilityIdentifier("pending-recovery-input-secret-key")
                            if recovery.kind != "signup" {
                                SecureField("Previous Secret Key, if requested", text: $priorSecretKey)
                                    .accessibilityIdentifier("pending-recovery-input-prior-secret-key")
                            }
                        }
                        ForEach(recovery.problems, id: \.self) { Text($0) }
                        CredentialField("Master password", text: $password)
                            .textContentType(.password)
                            .accessibilityIdentifier("pending-recovery-password")
                            .concealWhen(model.isBusy || isDiscardConfirmationShown)
                        if password.isEmpty {
                            Text("Enter your master password to continue.")
                                .font(.footnote)
                                .foregroundStyle(.themeSecondary)
                        }
                        if model.status.state != VaultStatus.stateSignedOut && (model.status.email != recovery.email || model.status.server != recovery.origin) {
                            Text("Continuing signs this device in to \(recovery.email) on \(PendingRecoveryCopy.host(recovery.origin)).")
                        }
                        if ["manual-unlock", "prepared", "needs-finishing", "outcome-unknown"].contains(recovery.phase) {
                            Button(recovery.phase == "manual-unlock" ? "Sign in with saved Emergency Kit" : "Continue") {
                                let enteredPassword = password
                                let enteredKey = secretKey
                                let enteredPriorKey = priorSecretKey
                                Task {
                                    let succeeded = await model.resumePendingRecovery(operationID: recovery.operationId, password: enteredPassword, secretKey: enteredKey, priorSecretKey: enteredPriorKey)
                                    handleOutcome(succeeded)
                                }
                            }
                            .applyProminentGlassStyle()
                            .disabled(password.isEmpty || model.isBusy)
                            .accessibilityIdentifier("pending-recovery-resume")
                        } else if model.pendingRecoveryExport == nil {
                            Button("Prepare Emergency Kit") {
                                let enteredPassword = password
                                Task { handleOutcome(await model.exportPendingRecoveryKit(operationID: recovery.operationId, password: enteredPassword)) }
                            }
                            .disabled(password.isEmpty || model.isBusy || recovery.phase == "outcome-unknown")
                            .applyProminentGlassStyle()
                            .accessibilityIdentifier("pending-recovery-export")
                        }
                    } header: {
                        Text(PendingRecoveryCopy.heading(recovery.kind))
                            .foregroundStyle(.themeSecondary)
                    } footer: {
                        Text("Preparing a kit does not confirm that you saved it.")
                            .foregroundStyle(.themeSecondary)
                    }
                    if let export = model.pendingRecoveryExport {
                        SecretKeySection(secretKey: export.secretKey, valueIdentifier: "pending-recovery-secret-key", copyIdentifier: "pending-recovery-copy-secret-key")
                        Section {
                            EmergencyKitShareButton(url: URL(fileURLWithPath: export.path), label: "Save or print Emergency Kit", onPresent: model.reportActivity) { method in
                                Task { await model.recordPendingKitDelivery(method: method, path: export.path) }
                            }
                            .applyGlassButtonStyle(isProminent: recovery.savedBy?.isEmpty != false)
                            .accessibilityIdentifier("pending-recovery-share-kit")
                            if recovery.savedBy?.isEmpty != false {
                                Text("Save, print or share the Emergency Kit first.")
                                    .font(.footnote)
                                    .foregroundStyle(.themeSecondary)
                                    .accessibilityIdentifier("pending-recovery-handoff-hint")
                            }
                        } header: {
                            Text("Emergency Kit")
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                    Section {
                        if recovery.savedBy?.isEmpty == false && ["delivery-pending", "awaiting-kit"].contains(recovery.phase) {
                            Button("I saved my Emergency Kit") {
                                let enteredPassword = password
                                Task {
                                    handleOutcome(await model.acknowledgePendingRecovery(operationID: recovery.operationId, password: enteredPassword), isFinal: recovery.phase == "delivery-pending")
                                }
                            }
                            .applyProminentGlassStyle()
                            .disabled(password.isEmpty || model.isBusy || !recovery.kitSaved)
                            .accessibilityIdentifier("pending-recovery-acknowledge")
                        }
                        if recovery.phase == "awaiting-kit" || recovery.phase == "prepared" {
                            Button("Cancel this setup", role: .destructive) { isDiscardConfirmationShown = true }
                                .disabled(password.isEmpty || model.isBusy)
                                .accessibilityIdentifier("pending-recovery-discard")
                        }
                    }
                }
                if model.isBusy {
                    Section {
                        ProgressView("Finishing account setup")
                            .accessibilityIdentifier("pending-recovery-progress")
                    }
                }
                if let message = model.errorMessage { Section { ErrorBanner(message: message) } }
                Section {
                    Button("Finish later") { clearCredentials(); model.closePendingRecovery() }
                        .accessibilityIdentifier("pending-recovery-later")
                }
            }
            .applyThemedListBackground()
            .navigationTitle("Finish account setup")
        }
        .confirmationDialog("Cancel this setup?", isPresented: $isDiscardConfirmationShown, titleVisibility: .visible) {
            Button("Cancel this setup", role: .destructive) {
                guard let recovery = selected else { return }
                let enteredPassword = password
                Task { handleOutcome(await model.discardPreparedRecovery(operationID: recovery.operationId, password: enteredPassword), isFinal: true) }
            }
            .accessibilityIdentifier("pending-recovery-discard-confirm")
        } message: {
            Text("No server change has been sent. This removes only the local prepared setup so you can start again.")
        }
        .onChange(of: model.status.state) { _, state in
            if state == VaultStatus.stateLocked || state == VaultStatus.stateSignedOut { clearCredentials() }
        }
        .onChange(of: selectedID) { _, _ in
            clearCredentials()
        }
        .onChange(of: model.status.email) { _, _ in clearCredentials() }
        .onChange(of: model.status.server) { _, _ in clearCredentials() }
        .onDisappear {
            clearCredentials()
            model.clearPendingRecoveryExport()
        }
    }
}
