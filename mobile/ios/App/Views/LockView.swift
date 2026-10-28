import SwiftUI

struct LockView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase
    @State private var isSignOutConfirmationShown = false
    @State private var isLostKeyHelpShown = false

    private var kitMessage: String {
        guard let kit = model.status.emergencyKit else { return "" }
        if model.interruptedKitOperationID == kit.operationId {
            return "Hisar locked while you were saving your Emergency Kit. Unlock to continue."
        }
        return kit.savedBy.isEmpty
            ? "Your Emergency Kit isn't saved yet. Unlock Hisar to save it."
            : "Your Emergency Kit still needs confirming. Unlock Hisar to continue."
    }

    private static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()

    var body: some View {
        UnlockPanel(
            email: model.status.email,
            requestDescription: kitMessage,
            isBiometricReady: model.status.biometricReady,
            isBiometricEnabled: model.status.biometricEnabled,
            isBiometricAvailable: model.status.biometricAvailable,
            lockCount: model.lockCount,
            biometryName: Self.biometryName,
            isBusy: model.isBusy,
            errorMessage: model.errorMessage,
            isOffline: model.status.isOffline,
            allNotice: model.allShareAwaitingUnlock.map(\.message),
            submitPassword: { await model.unlock(password: $0) },
            submitBiometric: { await model.unlockWithBiometric() },
            signOut: { isSignOutConfirmationShown = true },
            showLostKeyHelp: { isLostKeyHelpShown = true }
        )
        .onChange(of: scenePhase) { _, phase in
            if phase == .active {
                Task { await model.refreshShareAwaitingUnlock() }
            }
        }
        .task { await model.refreshShareAwaitingUnlock() }
        .sheet(isPresented: $isLostKeyHelpShown) { LostSecretKeyHelpSheet() }
        .alert(AccountLifecycleLogic.signOutTitle, isPresented: $isSignOutConfirmationShown) {
            Button("Cancel", role: .cancel) {}
            Button("Sign out", role: .destructive) {
                Task { await model.signOut() }
            }
        } message: {
            Text(AccountLifecycleLogic.signOutMessage(isLocked: true, emergencyKit: model.status.emergencyKit))
        }
    }
}

struct LostSecretKeyHelpSheet: View {
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text("It's on your Emergency Kit, the PDF or printout you saved when you created your account. If you saved the Secret Key separately, you can use that too.")
                    Text("Still signed in on another device? Unlock Hisar there with your master password and save a new Emergency Kit: on a Mac in Settings, Security; on an iPhone in Settings, Account, Secret Key and Emergency Kit; on Android in Settings, Account, Secret Key and Emergency Kit. Don't sign out of that device first.")
                    Text("If this device offers Finish account setup for an unfinished operation, try that with the master password you chose for it.")
                    Text("Without your Secret Key, a usable signed-in device or a protected unfinished setup you can resume, neither Hisar nor the server's admin can recover your account.")
                }
                Section("Forgot your master password?") {
                    Text("Hisar can't reset it, and neither can the server's admin. The Emergency Kit doesn't replace it. If you can still open Hisar on a device, don't sign out; preserve the items you can access. If you can no longer unlock any device and cannot find your master password, items only your account can open cannot be recovered. Shared vaults remain accessible to their other members.")
                }
            }
            .applyThemedListStyle()
            .navigationTitle("Can't find your Secret Key?")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("secret-key-help-done")
                }
            }
        }
    }
}
