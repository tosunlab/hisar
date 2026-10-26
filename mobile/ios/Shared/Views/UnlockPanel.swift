import SwiftUI

struct UnlockPanel: View {
    var email: String
    var requestDescription: String = ""
    var isBiometricReady: Bool
    var biometryName: String
    var isBusy: Bool
    var errorMessage: String?
    var isOffline: Bool = false
    var allNotice: [String] = []
    var submitPassword: (String) async -> Bool
    var submitBiometric: () async -> Void
    var signOut: (() -> Void)?

    @State private var password = ""
    @FocusState private var isPasswordFocused: Bool

    var body: some View {
        Form {
            Section {
                VStack(spacing: 10) {
                    LogoMark(size: 72)
                        .padding(.bottom, 6)
                    Text("Hisar is locked")
                        .font(.title2.weight(.bold))
                    Text(email)
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
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
                SecureField("Master password", text: $password)
                    .textContentType(.password)
                    .submitLabel(.go)
                    .focused($isPasswordFocused)
                    .onSubmit(submit)
                    .accessibilityIdentifier("unlock-password")
            } footer: {
                if isOffline {
                    Label("Offline. You can still unlock with the copy on this iPhone.", systemImage: "wifi.slash")
                }
            }
            Section {
                VStack(spacing: 12) {
                    if isBiometricReady {
                        Button {
                            Task { await submitBiometric() }
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
        }
        .onChange(of: isBiometricReady) { _, isReady in
            if !isReady { isPasswordFocused = true }
        }
        .onChange(of: errorMessage) { _, message in
            if message != nil { isPasswordFocused = true }
        }
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
