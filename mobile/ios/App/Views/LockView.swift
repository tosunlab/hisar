import SwiftUI

struct LockView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase
    @State private var promptedLockCount = -1
    @State private var isSignOutConfirmationShown = false

    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()

    var body: some View {
        UnlockPanel(
            email: model.status.email,
            isBiometricReady: model.status.biometricReady,
            biometryName: Self.biometryName,
            isBusy: model.isBusy,
            errorMessage: model.errorMessage,
            isOffline: model.status.isOffline,
            allNotice: model.allShareAwaitingUnlock.map(\.message),
            submitPassword: { await model.unlock(password: $0) },
            submitBiometric: { await model.unlockWithBiometric() },
            signOut: { isSignOutConfirmationShown = true }
        )
        .onAppear(perform: promptBiometricIfReady)
        .onChange(of: scenePhase) { _, phase in
            promptBiometricIfReady()
            if phase == .active {
                Task { await model.refreshShareAwaitingUnlock() }
            }
        }
        .task { await model.refreshShareAwaitingUnlock() }
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
