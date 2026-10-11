import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var isBiometricPasswordPromptShown = false
    @State private var biometricPassword = ""
    @State private var isSignOutConfirmationShown = false
    @State private var isGeneratorShown = false
    @State private var isAutoFillEnabled = false

    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()
    private static let isAutoFillIncluded = HisarConfiguration.current.isAppGroupAvailable

    private var versionText: String {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?"
        let build = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "?"
        return "\(version) (\(build))"
    }

    var body: some View {
        NavigationStack {
            List {
                Section("Account") {
                    buildRow(label: "Name", value: model.status.name)
                    buildRow(label: "Email", value: model.status.email)
                    buildRow(label: "Server", value: model.status.server)
                }
                Section {
                    Toggle(isOn: Binding(get: { model.status.biometricEnabled }, set: setBiometricUnlock)) {
                        Label("Unlock with \(Self.biometryName)", systemImage: Self.biometryName == "Touch ID" ? "touchid" : "faceid")
                    }
                    .tint(Theme.success)
                    .disabled(!model.status.biometricAvailable || model.isBusy)
                    .listRowBackground(Theme.surface)
                    .accessibilityIdentifier("biometric-toggle")
                    buildRow(label: "Auto-lock", value: "After 5 minutes")
                } header: {
                    Text("Security")
                } footer: {
                    Text(model.status.biometricAvailable
                         ? "Turning on \(Self.biometryName) needs your master password. Hisar still asks for the master password at least every 14 days."
                         : "\(Self.biometryName) is not set up on this iPhone.")
                }
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                            .listRowInsets(EdgeInsets())
                            .listRowBackground(Color.clear)
                    }
                }
                Section {
                    NavigationLink {
                        PeopleView()
                            .environmentObject(model)
                    } label: {
                        Label("People", systemImage: "person.2")
                            .foregroundStyle(Theme.text)
                    }
                    .listRowBackground(Theme.surface)
                    .accessibilityIdentifier("open-people")
                } footer: {
                    Text("People you share vaults with, share invitations, and invites to Hisar.")
                }
                Section {
                    Button {
                        isGeneratorShown = true
                    } label: {
                        Label("Password generator", systemImage: "wand.and.stars")
                            .foregroundStyle(Theme.text)
                    }
                    .listRowBackground(Theme.surface)
                }
                Section {
                    if !Self.isAutoFillIncluded {
                        Label("Not available in this build", systemImage: "key.viewfinder")
                            .foregroundStyle(Theme.textMuted)
                            .listRowBackground(Theme.surface)
                    } else if isAutoFillEnabled {
                        Label("Hisar fills passwords in apps and Safari", systemImage: "checkmark.circle")
                            .foregroundStyle(Theme.text)
                            .listRowBackground(Theme.surface)
                    } else {
                        Button {
                            Task {
                                isAutoFillEnabled = await AutoFillSettings.requestToTurnOn()
                                if isAutoFillEnabled {
                                    await model.refreshAll()
                                }
                            }
                        } label: {
                            Label("Turn on Hisar AutoFill", systemImage: "key.viewfinder")
                                .foregroundStyle(Theme.text)
                        }
                        .listRowBackground(Theme.surface)
                        .accessibilityIdentifier("turn-on-autofill")
                    }
                } header: {
                    Text("AutoFill")
                } footer: {
                    Text(Self.isAutoFillIncluded
                         ? "You can also turn it on in the Settings app: General, AutoFill & Passwords."
                         : "This build is signed with a free Apple ID, which cannot include the AutoFill extension.")
                }
                Section("About") {
                    buildRow(label: "Version", value: versionText)
                    Link(destination: URL(string: "https://github.com/tryhisar/hisar")!) {
                        Label("Source code", systemImage: "chevron.left.forwardslash.chevron.right")
                            .foregroundStyle(Theme.text)
                    }
                    .listRowBackground(Theme.surface)
                }
                Section {
                    Button("Sign out", role: .destructive) {
                        isSignOutConfirmationShown = true
                    }
                    .frame(maxWidth: .infinity)
                    .listRowBackground(Theme.surface)
                    .accessibilityIdentifier("sign-out")
                }
            }
            .listStyle(.insetGrouped)
            .applyListTheme()
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("settings-done")
                }
            }
            .alert("Turn on \(Self.biometryName)", isPresented: $isBiometricPasswordPromptShown) {
                SecureField("Master password", text: $biometricPassword)
                    .accessibilityIdentifier("biometric-password")
                Button("Cancel", role: .cancel) { biometricPassword = "" }
                Button("Turn on") {
                    let password = biometricPassword
                    biometricPassword = ""
                    Task { await model.setBiometricUnlock(isEnabled: true, password: password) }
                }
            } message: {
                Text("Enter your master password to unlock with \(Self.biometryName).")
            }
            .confirmationDialog("Sign out of Hisar on this iPhone?", isPresented: $isSignOutConfirmationShown, titleVisibility: .visible) {
                Button("Sign out", role: .destructive) {
                    Task {
                        if await model.signOut() {
                            dismiss()
                        }
                    }
                }
            } message: {
                Text("You will need your Secret Key and master password to sign in again.")
            }
            .sheet(isPresented: $isGeneratorShown) {
                PasswordGeneratorView(isUseShown: false) { _ in }
                    .environmentObject(model)
            }
            .onAppear { model.clearError() }
            .task { isAutoFillEnabled = await AutoFillSettings.checkIsEnabled() }
        }
        .preferredColorScheme(.dark)
    }

    private func setBiometricUnlock(_ isEnabled: Bool) {
        if isEnabled {
            isBiometricPasswordPromptShown = true
        } else {
            Task { await model.setBiometricUnlock(isEnabled: false, password: "") }
        }
    }

    private func buildRow(label: String, value: String) -> some View {
        HStack {
            Text(label)
                .foregroundStyle(Theme.textMuted)
            Spacer()
            Text(value)
                .foregroundStyle(Theme.text)
                .lineLimit(1)
                .truncationMode(.middle)
        }
        .listRowBackground(Theme.surface)
    }
}
