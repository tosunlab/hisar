import SwiftUI

struct NewSecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    @State private var secretKey: String?
    @State private var export: EmergencyKitExport?
    @State private var password = ""
    @State private var isPasswordShown = false
    @State private var isPreparing = false
    @State private var wasCancelled = false
    @State private var preparationGeneration = 0

    private var kit: EmergencyKitStatus? { model.status.emergencyKit }
    private var isAuthorized: Bool { kit?.isAuthorized == true }

    var body: some View {
        NavigationStack {
            List {
                if isAuthorized {
                    readySections
                } else {
                    Section {
                        Text("Enter your master password to show your Secret Key.")
                        if model.status.biometricEnabled && !isPasswordShown {
                            Button("Continue with \(SettingsView.biometryName)") {
                                Task { await authorize() }
                            }
                            .applyProminentGlassStyle()
                            .accessibilityIdentifier("new-secret-key-biometric")
                            Button("Use master password") { isPasswordShown = true }
                        } else {
                            CredentialField("Master password", text: $password)
                                .textContentType(.password)
                                .submitLabel(.go)
                                .onSubmit { Task { await authorize() } }
                                .accessibilityIdentifier("new-secret-key-password")
                            Button("Continue") { Task { await authorize() } }
                                .applyProminentGlassStyle()
                                .disabled(password.isEmpty)
                                .accessibilityIdentifier("new-secret-key-authorize")
                        }
                        Button("Cancel") { model.closeEmergencyKit() }
                    }
                    .disabled(model.isBusy)
                }
                if let errorMessage = model.errorMessage {
                    Section { ErrorBanner(message: errorMessage) }
                }
            }
            .applyThemedListStyle()
            .reportsActivity(to: model)
            .navigationTitle(kit?.kind == "replace-secret-key" ? "Save your new Emergency Kit" : "Save your Emergency Kit")
            .navigationBarTitleDisplayMode(.inline)
            .task(id: isAuthorized) {
                guard isAuthorized else { return }
                isPreparing = true
                secretKey = await model.revealNewAccountSecretKey()
                guard !Task.isCancelled else { return }
                await prepareKit()
            }
            .onDisappear {
                preparationGeneration += 1
                if let export { model.removeEmergencyKit(export) }
                export = nil
                secretKey = nil
                password = ""
            }
        }
    }

    @ViewBuilder
    private var readySections: some View {
        Section {
            Text(kit?.kind == "replace-secret-key"
                 ? "Your Secret Key changed, so your old Emergency Kit no longer works. Save the new one."
                 : "Hisar created a Secret Key for your account. You need it and your master password to sign in on a new device. You don't need to remember it.")
        }
        if let secretKey {
            SecretKeySection(secretKey: secretKey, valueIdentifier: "new-secret-key", copyIdentifier: "new-secret-key-copy", onCopy: model.reportActivity)
        }
        Section {
            Text("The Emergency Kit holds your Secret Key and has a space to write your master password.")
            Text("Keep it where you can get to it without this iPhone: print it, or save it to storage you can open from another device. Don't keep the only copy in Hisar.")
            if let kit, !kit.savedBy.isEmpty {
                Text(kit.deliveryMessage)
                    .accessibilityIdentifier("new-secret-key-delivery")
                Text(EmergencyKitStatus.savedAdvice)
                Button("I saved my Emergency Kit") {
                    Task { await model.finishNewSecretKey() }
                }
                .applyProminentGlassStyle()
                .disabled(model.isBusy)
                .accessibilityIdentifier("new-secret-key-done")
            }
            if let export {
                let epoch = model.emergencyKitDeliveryEpoch
                EmergencyKitShareButton(
                    url: URL(fileURLWithPath: export.emergencyKitPath),
                    label: "Save or print Emergency Kit",
                    onPresent: model.reportActivity
                ) { method in
                    wasCancelled = method == nil
                    Task { await model.recordEmergencyKitDelivery(exportID: export.exportId ?? "", method: method, epoch: epoch) }
                }
                .applyGlassButtonStyle(isProminent: kit?.savedBy.isEmpty != false)
                .controlSize(.large)
                .disabled(model.isBusy)
                .accessibilityIdentifier("new-secret-key-share-kit")
            } else if isPreparing {
                ProgressView("Preparing Emergency Kit…")
            } else {
                Button("Try again") { Task { await prepareKit() } }
                    .accessibilityIdentifier("new-secret-key-retry-kit")
            }
            if wasCancelled {
                Text("The Emergency Kit wasn't saved.")
                    .accessibilityIdentifier("new-secret-key-cancelled")
            }
        }
        Section {
            Button {
                Task { await model.postponeNewSecretKey() }
            } label: {
                Text("Save later")
                    .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                    .contentShape(Rectangle())
            }
                .disabled(model.isBusy)
                .accessibilityIdentifier("new-secret-key-later")
            Text(EmergencyKitStatus.postponementAdvice)
                .font(.footnote)
                .foregroundStyle(.themeSecondary)
        }
    }

    private func authorize() async {
        let enteredPassword = password
        password = ""
        await model.authorizeEmergencyKit(password: model.status.biometricEnabled && !isPasswordShown ? nil : enteredPassword)
    }

    private func prepareKit() async {
        preparationGeneration += 1
        let generation = preparationGeneration
        isPreparing = true
        if let export { model.removeEmergencyKit(export) }
        export = nil
        model.clearError()
        let prepared = await model.exportNewAccountEmergencyKit()
        guard !Task.isCancelled, generation == preparationGeneration else {
            if let prepared { model.removeEmergencyKit(prepared) }
            return
        }
        isPreparing = false
        export = prepared
    }
}

struct BiometricOfferView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationStack {
            List {
                Section {
                    VStack(spacing: 14) {
                        Image(systemName: SettingsView.biometrySystemImage)
                            .font(.system(size: 56))
                            .foregroundStyle(.tint)
                        Text(SettingsView.biometryConsent)
                            .multilineTextAlignment(.center)
                            .accessibilityIdentifier("biometric-offer-text")
                    }
                    .frame(maxWidth: .infinity)
                    .listRowBackground(Color.clear)
                }
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
            }
            .applyThemedListStyle()
            .reportsActivity(to: model)
            .navigationTitle("Unlock with \(SettingsView.biometryName)?")
            .safeAreaInset(edge: .bottom) {
                VStack(spacing: 8) {
                    Button {
                        Task { await model.turnOnBiometricUnlockFromOffer() }
                    } label: {
                        Group {
                            if model.isBusy {
                                ProgressView()
                            } else {
                                Text("Turn on \(SettingsView.biometryName)")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .controlSize(.large)
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("biometric-offer-turn-on")
                    Button("Not now") {
                        model.dismissNewSecretKey()
                    }
                    .controlSize(.large)
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("biometric-offer-not-now")
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 8)
            }
        }
    }
}

extension View {
    // Onboarding screens hold the user for minutes (writing a Secret Key down, printing), so their appearance and scrolling
    // count as activity for auto-lock; idle time on them still locks. No tap gesture: on the list it takes the taps of its
    // rows' buttons, such as Save later and Copy Secret Key.
    func reportsActivity(to model: AppModel) -> some View {
        onAppear { model.reportActivity() }
            .onScrollPhaseChange { _, phase in
                if phase == .interacting { model.reportActivity() }
            }
    }
}
