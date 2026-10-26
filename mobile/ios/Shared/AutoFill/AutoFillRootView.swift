import SwiftUI

struct AutoFillRootView: View {
    @ObservedObject var model: AutoFillModel

    var body: some View {
        NavigationStack {
            ZStack {
                model.theme.palette.groupedBackground.color.ignoresSafeArea()
                content
            }
            .navigationTitle(model.screen == .list || model.screen == .passkeyRegistration ? "Hisar" : "")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { model.cancel() }
                        .accessibilityIdentifier("autofill-cancel")
                }
            }
        }
        .applyTheme(model.theme)
    }

    @ViewBuilder
    private var content: some View {
        switch model.screen {
        case .starting:
            ProgressView()
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
        case .updateRequired:
            MessageView(systemImage: "arrow.down.app", title: "Update required", message: "Hisar needs an update. Open Hisar to update it.")
        }
    }
}

private struct MessageView: View {
    var systemImage: String
    var title: String
    var message: String

    var body: some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text(message))
    }
}

private struct AutoFillLockView: View {
    @ObservedObject var model: AutoFillModel
    @State private var promptedLockCount = -1
    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()

    var body: some View {
        UnlockPanel(
            email: model.status.email,
            requestDescription: model.serviceName.isEmpty ? "" : requestDescription,
            isBiometricReady: model.status.biometricReady,
            biometryName: Self.biometryName,
            isBusy: model.isBusy,
            errorMessage: model.errorMessage,
            submitPassword: { await model.unlock(password: $0) },
            submitBiometric: { await model.unlockWithBiometric() }
        )
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
                        .foregroundStyle(.themeSecondary)
                        
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
                        ProgressView()
                    } else if model.allVisibleOtherLogin.isEmpty {
                        Text("No other logins match.")
                            .foregroundStyle(.themeSecondary)
                            
                    }
                    ForEach(model.allVisibleOtherLogin) { login in
                        buildLoginRow(login)
                    }
                }
            }
        }
        .applyThemedListStyle()
        // The rows wait while a login fills or the logins load; the search field never does. Searching starts the load of every
        // login, and a disabled field drops what is typed meanwhile.
        .disabled(model.isBusy)
        .searchable(text: $model.searchText, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search all logins")
    }

    private var sectionTitle: String {
        let noun = model.requestKind == .oneTimeCode ? "One-time codes" : "Logins"
        return model.serviceName.isEmpty ? "Matching \(noun.lowercased())" : "\(noun) for \(model.serviceName)"
    }

    private var emptyMessage: String {
        switch (model.requestKind, model.searchText.isEmpty) {
        case (.oneTimeCode, true): "No login for this site has a one-time code. Search all logins above."
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
                        .foregroundStyle(.primary)
                    Text(passkey.userName.isEmpty ? passkey.userDisplayName : passkey.userName)
                        .font(.caption)
                        .foregroundStyle(.themeSecondary)
                }
                Spacer()
                Image(systemName: "person.badge.key")
                    .foregroundStyle(.themeSecondary)
                    .accessibilityLabel("Passkey")
            }
            .contentShape(Rectangle())
        }
        
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
                        .foregroundStyle(.primary)
                    Text(login.username.isEmpty ? login.vaultName : "\(login.username) · \(login.vaultName)")
                        .font(.caption)
                        .foregroundStyle(.themeSecondary)
                }
                Spacer()
                if login.hasOneTimePassword {
                    Image(systemName: "timer")
                        .foregroundStyle(.themeSecondary)
                        .accessibilityLabel("Has a one-time code")
                }
            }
            .contentShape(Rectangle())
        }
        
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
                        .font(.largeTitle)
                        .foregroundStyle(.themeSecondary)
                    Text("Save passkey")
                        .font(.title3.weight(.semibold))
                        .foregroundStyle(.primary)
                    if let request = model.passkeyRegistrationRequest {
                        Text(request.userName.isEmpty ? request.relyingPartyID : "\(request.userName) on \(request.relyingPartyID)")
                            .multilineTextAlignment(.center)
                            .foregroundStyle(.themeSecondary)
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
                                .frame(maxWidth: .infinity)
                        }
                        .applyProminentGlassStyle()
                        .controlSize(.large)
                        .listRowInsets(EdgeInsets())
                        .listRowBackground(Color.clear)
                        .accessibilityIdentifier("autofill-save-passkey")
                    }
                }
            } else {
                ProgressView().frame(maxWidth: .infinity).listRowBackground(Color.clear)
            }
        }
        .applyThemedListStyle()
        .disabled(model.isBusy)
    }

    private func buildExcludedSection() -> some View {
        Section {
            Text("Hisar already has a passkey for this account. Sign in with it instead.")
                .foregroundStyle(.primary)
                
                .accessibilityIdentifier("autofill-passkey-excluded")
            Button("OK") { model.rejectExcludedPasskeyRegistration() }
                
                .accessibilityIdentifier("autofill-passkey-excluded-ok")
        }
    }

    private func buildLoginSection(options: PasskeyRegistrationOptions) -> some View {
        Section {
            ForEach(options.allLogin) { login in
                buildChoiceRow(title: login.title, subtitle: (login.username.isEmpty ? login.vaultName : "\(login.username) · \(login.vaultName)") + (login.isPersonal ? "" : " · Shared. Other members can sign in with this passkey."), loginID: login.id)
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
                        .foregroundStyle(.primary)
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(.themeSecondary)
                }
                Spacer()
                if model.selectedRegistrationLoginID == loginID {
                    Image(systemName: "checkmark")
                        .foregroundStyle(.tint)
                        .accessibilityLabel("Selected")
                }
            }
            .contentShape(Rectangle())
        }
        
        .accessibilityAddTraits(model.selectedRegistrationLoginID == loginID ? .isSelected : [])
    }
}
