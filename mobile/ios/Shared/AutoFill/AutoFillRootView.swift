import SwiftUI

struct AutoFillRootView: View {
    @ObservedObject var model: AutoFillModel

    var body: some View {
        NavigationStack {
            ZStack {
                Theme.background.ignoresSafeArea()
                content
            }
            .navigationTitle(model.screen == .list || model.screen == .passkeyRegistration ? "Hisar" : "")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { model.cancel() }
                        .accessibilityIdentifier("autofill-cancel")
                }
            }
        }
        .preferredColorScheme(.dark)
        .tint(Theme.primary)
    }

    @ViewBuilder
    private var content: some View {
        switch model.screen {
        case .starting:
            ProgressView().tint(Theme.textMuted)
        case .failed(let message):
            MessageView(systemImage: "exclamationmark.triangle", title: "Hisar could not start", message: message)
        case .signedOut:
            MessageView(systemImage: "person.crop.circle.badge.questionmark", title: "Sign in first", message: "Open the Hisar app and sign in, then use AutoFill again.")
        case .configuration:
            MessageView(systemImage: "checkmark.shield", title: "Hisar AutoFill is on", message: "Tap a password field in Safari or another app and choose Hisar to fill a login.")
        case .locked:
            AutoFillLockView(model: model)
        case .list:
            AutoFillListView(model: model)
        case .passkeyRegistration:
            AutoFillPasskeyRegistrationView(model: model)
        }
    }
}

private struct MessageView: View {
    var systemImage: String
    var title: String
    var message: String

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: systemImage)
                .font(.system(size: 40))
                .foregroundStyle(Theme.textSecondary)
            Text(title)
                .font(.title3.weight(.semibold))
                .foregroundStyle(Theme.text)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(Theme.textMuted)
        }
        .padding(32)
    }
}

private struct AutoFillLockView: View {
    @ObservedObject var model: AutoFillModel
    @State private var promptedLockCount = -1
    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()

    var body: some View {
        ScrollView {
            VStack(spacing: 16) {
                if !model.serviceName.isEmpty {
                    Text(requestDescription)
                        .font(.subheadline)
                        .foregroundStyle(Theme.textMuted)
                        .padding(.top, 24)
                }
                UnlockPanel(
                    email: model.status.email,
                    isBiometricReady: model.status.biometricReady,
                    biometryName: Self.biometryName,
                    isBusy: model.isBusy,
                    errorMessage: model.errorMessage,
                    submitPassword: { await model.unlock(password: $0) },
                    submitBiometric: { await model.unlockWithBiometric() }
                )
            }
            .padding(.horizontal, 24)
            .padding(.top, 24)
        }
        .onAppear {
            guard model.status.biometricReady, promptedLockCount != model.lockCount else { return }
            promptedLockCount = model.lockCount
            Task { await model.unlockWithBiometric() }
        }
    }
}

extension AutoFillLockView {
    fileprivate var requestDescription: String {
        if model.passkeyRegistrationRequest != nil { return "Save a passkey for \(model.serviceName)" }
        if model.requestKind == .oneTimeCode { return "Fill a one-time code for \(model.serviceName)" }
        if model.passkeyAssertionRequest != nil && model.allServiceIdentifier.isEmpty { return "Sign in with a passkey for \(model.serviceName)" }
        return "Fill a login for \(model.serviceName)"
    }
}

private struct AutoFillListView: View {
    @ObservedObject var model: AutoFillModel

    var body: some View {
        List {
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                        .listRowInsets(EdgeInsets())
                        .listRowBackground(Color.clear)
                }
            }
            if !model.allVisibleMatchingPasskey.isEmpty, let request = model.passkeyAssertionRequest {
                Section {
                    ForEach(model.allVisibleMatchingPasskey) { passkey in
                        buildPasskeyRow(passkey)
                    }
                } header: {
                    Text("Passkeys for \(request.relyingPartyID)")
                        .textCase(nil)
                }
            }
            Section {
                if model.allVisibleMatchingLogin.isEmpty {
                    Text(emptyMessage)
                        .foregroundStyle(Theme.textMuted)
                        .listRowBackground(Theme.surface)
                }
                ForEach(model.allVisibleMatchingLogin) { login in
                    buildLoginRow(login)
                }
            } header: {
                Text(sectionTitle)
                    .textCase(nil)
            }
            if !model.searchText.isEmpty {
                Section("All logins") {
                    if !model.isOtherLoginLoaded {
                        ProgressView().tint(Theme.textMuted).listRowBackground(Theme.surface)
                    } else if model.allVisibleOtherLogin.isEmpty {
                        Text("No other logins match.")
                            .foregroundStyle(Theme.textMuted)
                            .listRowBackground(Theme.surface)
                    }
                    ForEach(model.allVisibleOtherLogin) { login in
                        buildLoginRow(login)
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .applyListTheme()
        .searchable(text: $model.searchText, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search all logins")
        .disabled(model.isBusy)
    }

    private var sectionTitle: String {
        let noun = model.requestKind == .oneTimeCode ? "One-time codes" : "Logins"
        return model.serviceName.isEmpty ? "Matching \(noun.lowercased())" : "\(noun) for \(model.serviceName)"
    }

    private var emptyMessage: String {
        switch (model.requestKind, model.searchText.isEmpty) {
        case (.oneTimeCode, true): "No login for this site has a one-time password. Search all logins above."
        case (.oneTimeCode, false): "No matching one-time codes for this site."
        case (.password, true): "No logins saved for this site. Search all logins above."
        case (.password, false): "No matching logins for this site."
        }
    }

    private func buildPasskeyRow(_ passkey: StoredPasskey) -> some View {
        Button {
            Task { await model.selectPasskey(passkey) }
        } label: {
            HStack(spacing: 14) {
                ItemIcon(item: ItemDetail(title: passkey.title))
                VStack(alignment: .leading, spacing: 2) {
                    Text(passkey.title)
                        .foregroundStyle(Theme.text)
                    Text(passkey.userName.isEmpty ? passkey.userDisplayName : passkey.userName)
                        .font(.caption)
                        .foregroundStyle(Theme.textMuted)
                }
                Spacer()
                Image(systemName: "person.badge.key")
                    .foregroundStyle(Theme.textMuted)
                    .accessibilityLabel("Passkey")
            }
            .contentShape(Rectangle())
        }
        .listRowBackground(Theme.surface)
        .accessibilityIdentifier("autofill-passkey-\(passkey.title)")
    }

    private func buildLoginRow(_ login: LoginMatch) -> some View {
        Button {
            Task { await model.select(login) }
        } label: {
            HStack(spacing: 14) {
                ItemIcon(item: ItemDetail(title: login.title))
                VStack(alignment: .leading, spacing: 2) {
                    Text(login.title)
                        .foregroundStyle(Theme.text)
                    Text(login.username.isEmpty ? login.vaultName : "\(login.username) · \(login.vaultName)")
                        .font(.caption)
                        .foregroundStyle(Theme.textMuted)
                }
                Spacer()
                if login.hasOneTimePassword {
                    Image(systemName: "timer")
                        .foregroundStyle(Theme.textMuted)
                        .accessibilityLabel("Has a one-time code")
                }
            }
            .contentShape(Rectangle())
        }
        .listRowBackground(Theme.surface)
        .accessibilityIdentifier("autofill-login-\(login.title)")
    }
}

private struct AutoFillPasskeyRegistrationView: View {
    @ObservedObject var model: AutoFillModel

    var body: some View {
        List {
            Section {
                VStack(spacing: 10) {
                    Image(systemName: "person.badge.key")
                        .font(.system(size: 36))
                        .foregroundStyle(Theme.textSecondary)
                    Text("Save passkey")
                        .font(.title3.weight(.semibold))
                        .foregroundStyle(Theme.text)
                    if let request = model.passkeyRegistrationRequest {
                        Text(request.userName.isEmpty ? request.relyingPartyID : "\(request.userName) on \(request.relyingPartyID)")
                            .multilineTextAlignment(.center)
                            .foregroundStyle(Theme.textMuted)
                            .accessibilityIdentifier("autofill-passkey-account")
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 8)
                .listRowBackground(Color.clear)
            }
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                        .listRowInsets(EdgeInsets())
                        .listRowBackground(Color.clear)
                }
            }
            if let options = model.passkeyRegistrationOptions {
                if options.isExcluded {
                    buildExcludedSection()
                } else {
                    buildLoginSection(options: options)
                    Section {
                        Button {
                            Task { await model.savePasskey() }
                        } label: {
                            Text("Save passkey")
                                .font(.body.weight(.semibold))
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(PrimaryButtonStyle())
                        .listRowInsets(EdgeInsets())
                        .listRowBackground(Color.clear)
                        .accessibilityIdentifier("autofill-save-passkey")
                    }
                }
            } else {
                ProgressView().tint(Theme.textMuted).frame(maxWidth: .infinity).listRowBackground(Color.clear)
            }
        }
        .listStyle(.insetGrouped)
        .applyListTheme()
        .disabled(model.isBusy)
    }

    private func buildExcludedSection() -> some View {
        Section {
            Text("Hisar already has a passkey for this account. Sign in with it instead.")
                .foregroundStyle(Theme.text)
                .listRowBackground(Theme.surface)
                .accessibilityIdentifier("autofill-passkey-excluded")
            Button("OK") { model.rejectExcludedPasskeyRegistration() }
                .listRowBackground(Theme.surface)
                .accessibilityIdentifier("autofill-passkey-excluded-ok")
        }
    }

    private func buildLoginSection(options: PasskeyRegistrationOptions) -> some View {
        Section {
            ForEach(options.allLogin) { login in
                buildChoiceRow(title: login.title, subtitle: login.username.isEmpty ? login.vaultName : "\(login.username) · \(login.vaultName)", loginID: login.id)
                    .accessibilityIdentifier("autofill-passkey-login-\(login.title)")
            }
            buildChoiceRow(title: "New login", subtitle: "In your Personal vault", loginID: nil)
                .accessibilityIdentifier("autofill-passkey-new-login")
        } header: {
            Text("Save to")
                .textCase(nil)
        }
    }

    private func buildChoiceRow(title: String, subtitle: String, loginID: String?) -> some View {
        Button {
            model.selectedRegistrationLoginID = loginID
        } label: {
            HStack(spacing: 14) {
                ItemIcon(item: ItemDetail(title: title))
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .foregroundStyle(Theme.text)
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(Theme.textMuted)
                }
                Spacer()
                if model.selectedRegistrationLoginID == loginID {
                    Image(systemName: "checkmark")
                        .foregroundStyle(Theme.primary)
                        .accessibilityLabel("Selected")
                }
            }
            .contentShape(Rectangle())
        }
        .listRowBackground(Theme.surface)
        .accessibilityAddTraits(model.selectedRegistrationLoginID == loginID ? .isSelected : [])
    }
}
