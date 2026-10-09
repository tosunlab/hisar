import SwiftUI

struct UnlockPanel: View {
    var email: String
    var isBiometricReady: Bool
    var biometryName: String
    var isBusy: Bool
    var errorMessage: String?
    var submitPassword: (String) async -> Bool
    var submitBiometric: () async -> Void

    @State private var password = ""
    @FocusState private var isPasswordFocused: Bool

    var body: some View {
        VStack(spacing: 20) {
            LogoMark(size: 72)
                .padding(.bottom, 4)
            VStack(spacing: 6) {
                Text("Hisar is locked")
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(Theme.text)
                Text(email)
                    .font(.subheadline)
                    .foregroundStyle(Theme.textMuted)
                    .accessibilityIdentifier("lock-email")
            }
            if let errorMessage {
                ErrorBanner(message: errorMessage)
            }
            if isBiometricReady {
                Button {
                    Task { await submitBiometric() }
                } label: {
                    Label("Unlock with \(biometryName)", systemImage: biometryName == "Touch ID" ? "touchid" : "faceid")
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(isBusy)
                .accessibilityIdentifier("unlock-biometric")
            }
            LabeledInput(label: "Master password", systemImage: "lock") {
                SecureField("Master password", text: $password)
                    .submitLabel(.go)
                    .focused($isPasswordFocused)
                    .onSubmit(submit)
                    .accessibilityIdentifier("unlock-password")
            }
            Button(action: submit) {
                if isBusy {
                    ProgressView().tint(isBiometricReady ? Theme.text : Theme.primaryText)
                } else {
                    Text("Unlock")
                }
            }
            .modifier(UnlockButtonStyle(isSecondary: isBiometricReady))
            .disabled(isBusy || password.isEmpty)
            .accessibilityIdentifier("unlock-submit")
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

private struct UnlockButtonStyle: ViewModifier {
    var isSecondary: Bool

    func body(content: Content) -> some View {
        if isSecondary {
            content.buttonStyle(SecondaryButtonStyle())
        } else {
            content.buttonStyle(PrimaryButtonStyle())
        }
    }
}
