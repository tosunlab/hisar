import SwiftUI

struct UnlockPanel: View {
    var email: String
    var requestDescription: String = ""
    var isBiometricReady: Bool
    var isBiometricEnabled: Bool
    var isBiometricAvailable: Bool
    var lockCount: Int
    var biometryName: String
    var isBusy: Bool
    var errorMessage: String?
    var isOffline: Bool = false
    var allNotice: [String] = []
    var submitPassword: (String) async -> Bool
    var submitBiometric: () async -> Void
    var signOut: (() -> Void)?

    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var promptedLockCount = -1
    @State private var password = ""
    @State private var isPasswordFocused = false

    var body: some View {
        Form {
            Section {
                VStack(spacing: 10) {
                    LogoMark(size: 72)
                        .padding(.bottom, 6)
                    Text("Hisar is locked")
                        .font(.title2.weight(.bold))
                    Text(verbatim: email)
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                        .lineLimit(dynamicTypeSize.isAccessibilitySize ? 1 : nil)
                        .minimumScaleFactor(dynamicTypeSize.isAccessibilitySize ? 0.6 : 1)
                        .truncationMode(.middle)
                        .accessibilityLabel(Text(verbatim: email))
                        .accessibilityIdentifier("lock-email")
                    if !requestDescription.isEmpty {
                        Text(requestDescription)
                            .font(.subheadline)
                            .foregroundStyle(.themeSecondary)
                            .multilineTextAlignment(.center)
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.top, 24)
                .listRowBackground(Color.clear)
            }
            if !allNotice.isEmpty {
                Section {
                    ForEach(allNotice, id: \.self) { notice in
                        Label(notice, systemImage: "person.2")
                            .font(.subheadline)
                            .accessibilityIdentifier("lock-notice")
                    }
                }
            }
            if let errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            Section {
                CredentialField("Master password", text: $password)
                    .textContentType(.password)
                    .submitLabel(.go)
                    .focused($isPasswordFocused)
                    .onSubmit(submit)
                    .accessibilityIdentifier("unlock-password")
                    .concealWhen(isBusy)
            } footer: {
                if isBiometricEnabled && !isBiometricReady {
                    Text(isBiometricAvailable
                         ? "Enter your master password to use \(biometryName) again. Hisar requires it at least every 14 days."
                         : "\(biometryName) isn't available right now. Enter your master password.")
                        .accessibilityIdentifier("biometric-password-reason")
                }
                if isOffline {
                    Label("Offline. You can still unlock with the copy on this iPhone.", systemImage: "wifi.slash")
                }
            }
            Section {
                VStack(spacing: 12) {
                    if isBiometricReady {
                        Button {
                            Task { await submitBiometricAndFocus() }
                        } label: {
                            Label("Unlock with \(biometryName)", systemImage: biometryName == "Touch ID" ? "touchid" : "faceid")
                                .frame(maxWidth: .infinity)
                        }
                        .applyProminentGlassStyle()
                        .disabled(isBusy)
                        .accessibilityIdentifier("unlock-biometric")
                    }
                    Button(action: submit) {
                        Group {
                            if isBusy {
                                ProgressView()
                            } else {
                                Text("Unlock")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyGlassButtonStyle(isProminent: !isBiometricReady)
                    .disabled(isBusy || password.isEmpty)
                    .accessibilityIdentifier("unlock-submit")
                    if let signOut {
                        Button("Sign out", action: signOut)
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                            .padding(.top, 8)
                            .accessibilityIdentifier("lock-sign-out")
                    }
                }
                .controlSize(.large)
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
            }
        }
        .scrollDismissesKeyboard(.interactively)
        .applyThemedListBackground()
        .onAppear {
            if !isBiometricReady { isPasswordFocused = true }
            promptBiometricIfReady()
        }
        .onChange(of: scenePhase) { _, _ in promptBiometricIfReady() }
        .onChange(of: isBiometricReady) { _, isReady in
            if !isReady { isPasswordFocused = true }
        }
        .onChange(of: errorMessage) { _, message in
            if message != nil { isPasswordFocused = true }
        }
    }

    private func submitBiometricAndFocus() async {
        await submitBiometric()
        isPasswordFocused = true
    }

    private func promptBiometricIfReady() {
        guard scenePhase == .active, isBiometricReady, !isBusy, promptedLockCount != lockCount else { return }
        promptedLockCount = lockCount
        Task { await submitBiometricAndFocus() }
    }

    private func submit() {
        guard !password.isEmpty, !isBusy else { return }
        let enteredPassword = password
        Task {
            _ = await submitPassword(enteredPassword)
            password = ""
            isPasswordFocused = true
        }
    }
}
