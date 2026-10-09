import SwiftUI

struct LockView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase
    @State private var promptedLockCount = -1
    @State private var isSignOutConfirmationShown = false

    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                Spacer(minLength: 60)
                UnlockPanel(
                    email: model.status.email,
                    isBiometricReady: model.status.biometricReady,
                    biometryName: Self.biometryName,
                    isBusy: model.isBusy,
                    errorMessage: model.errorMessage,
                    submitPassword: { await model.unlock(password: $0) },
                    submitBiometric: { await model.unlockWithBiometric() }
                )
                if model.status.isOffline {
                    Label("Offline. You can still unlock with the copy on this iPhone.", systemImage: "wifi.slash")
                        .font(.footnote)
                        .foregroundStyle(Theme.warning)
                }
                Button("Sign out") { isSignOutConfirmationShown = true }
                    .font(.footnote)
                    .foregroundStyle(Theme.textMuted)
                    .padding(.top, 8)
            }
            .padding(.horizontal, 24)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.background)
        .coverStatusBar()
        .onAppear(perform: promptBiometricIfReady)
        .onChange(of: scenePhase) { _, _ in promptBiometricIfReady() }
        .confirmationDialog("Sign out of Hisar on this iPhone?", isPresented: $isSignOutConfirmationShown, titleVisibility: .visible) {
            Button("Sign out", role: .destructive) {
                Task { await model.signOut() }
            }
        } message: {
            Text("You will need your Secret Key and master password to sign in again.")
        }
    }

    // One automatic prompt per lock, and only while the app is in front.
    private func promptBiometricIfReady() {
        guard scenePhase == .active, model.status.biometricReady, !model.isBusy, promptedLockCount != model.lockCount else { return }
        promptedLockCount = model.lockCount
        Task { await model.unlockWithBiometric() }
    }
}
