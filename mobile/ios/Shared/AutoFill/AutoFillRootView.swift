import SwiftUI

struct AutoFillRootView: View {
    @ObservedObject var model: AutoFillModel

    var body: some View {
        NavigationStack {
            ZStack {
                Theme.background.ignoresSafeArea()
                content
            }
            .navigationTitle(model.screen == .list ? "Hisar" : "")
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
                    Text(model.requestKind == .oneTimeCode ? "Fill a one-time code for \(model.serviceName)" : "Fill a login for \(model.serviceName)")
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
