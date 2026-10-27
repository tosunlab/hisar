import SwiftUI

struct LockView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase
    @State private var isSignOutConfirmationShown = false

    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()

    var body: some View {
        UnlockPanel(
            email: model.status.email,
            isBiometricReady: model.status.biometricReady,
            lockCount: model.lockCount,
            biometryName: Self.biometryName,
            isBusy: model.isBusy,
            errorMessage: model.errorMessage,
            isOffline: model.status.isOffline,
            allNotice: model.allShareAwaitingUnlock.map(\.message),
            submitPassword: { await model.unlock(password: $0) },
            submitBiometric: { await model.unlockWithBiometric() },
            signOut: { isSignOutConfirmationShown = true }
        )
        .onChange(of: scenePhase) { _, phase in
            if phase == .active {
                Task { await model.refreshShareAwaitingUnlock() }
            }
        }
        .task { await model.refreshShareAwaitingUnlock() }
        .confirmationDialog(AccountLifecycleLogic.signOutTitle, isPresented: $isSignOutConfirmationShown, titleVisibility: .visible) {
            Button("Sign out", role: .destructive) {
                Task { await model.signOut() }
            }
        } message: {
            Text(AccountLifecycleLogic.lockedSignOutMessage)
        }
    }
}
