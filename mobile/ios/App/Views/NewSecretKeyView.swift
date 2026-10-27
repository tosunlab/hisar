import SwiftUI

struct NewSecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    var secretKey: String

    private enum KitState {
        case loading
        case failed(String)
        case ready(EmergencyKitExport)
    }

    @State private var kitState = KitState.loading
    @State private var isKitShared = false
    @State private var preparationGeneration = 0

    private var export: EmergencyKitExport? {
        if case .ready(let export) = kitState { return export }
        return nil
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text("To sign in on another device you need your master password and this Secret Key.")
                        .listRowBackground(Color.clear)
                        .listRowInsets(EdgeInsets(top: 0, leading: 4, bottom: 0, trailing: 4))
                }
                SecretKeySection(secretKey: secretKey, valueIdentifier: "new-secret-key", copyIdentifier: "new-secret-key-copy")
                Section {
                    switch kitState {
                    case .ready(let export):
                        EmergencyKitShareButton(
                            url: URL(fileURLWithPath: export.emergencyKitPath),
                            label: "Save or print Emergency Kit",
                            onPresent: model.reportActivity,
                            onCompleted: { isKitShared = true }
                        )
                        .applyProminentGlassStyle()
                        .controlSize(.large)
                        .listRowBackground(Color.clear)
                        .listRowInsets(EdgeInsets())
                        .accessibilityIdentifier("new-secret-key-share-kit")
                    case .failed(let reason):
                        ErrorBanner(message: "The Emergency Kit could not be created (\(reason)). Write down the Secret Key above.")
                        Button("Try again") {
                            Task { await prepareKit() }
                        }
                        .accessibilityIdentifier("new-secret-key-retry-kit")
                    case .loading:
                        ProgressView()
                            .frame(maxWidth: .infinity)
                            .listRowBackground(Color.clear)
                    }
                } header: {
                    Text("Emergency Kit")
                } footer: {
                    Text("The Emergency Kit holds your Secret Key and has a space to write your master password. Keep it away from your devices.")
                }
                Section {
                    Label {
                        Text("A forgotten master password can't be recovered by anyone, including Hisar.")
                            .font(.subheadline.weight(.medium))
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundStyle(.themeWarning)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("master-password-warning")
                }
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
            }
            .applyThemedListStyle()
            .navigationTitle("Save your Secret Key")
            .safeAreaInset(edge: .bottom) {
                VStack(spacing: 8) {
                    Button {
                        Task { await model.finishNewSecretKey() }
                    } label: {
                        Group {
                            if model.isBusy {
                                ProgressView()
                            } else {
                                Text("I saved my Emergency Kit")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .controlSize(.large)
                    .disabled(export == nil || !isKitShared || model.isBusy)
                    .accessibilityIdentifier("new-secret-key-done")
                    Button("Finish later") {
                        Task { await model.postponeNewSecretKey() }
                    }
                    .controlSize(.large)
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("new-secret-key-later")
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 8)
            }
            .task {
                await prepareKit()
            }
            .onDisappear {
                preparationGeneration += 1
                if let export {
                    model.removeEmergencyKit(export)
                }
                kitState = .loading
            }
        }
    }

    private func prepareKit() async {
        preparationGeneration += 1
        let requestedGeneration = preparationGeneration
        kitState = .loading
        isKitShared = false
        model.clearError()
        let prepared = await model.exportNewAccountEmergencyKit()
        guard !Task.isCancelled, requestedGeneration == preparationGeneration else {
            if let prepared { model.removeEmergencyKit(prepared) }
            return
        }
        if let prepared {
            kitState = .ready(prepared)
        } else {
            kitState = .failed(model.errorMessage ?? "Try again.")
            model.clearError()
        }
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
                        Text("Unlock Hisar with \(SettingsView.biometryName) instead of typing your master password. Hisar still asks for the master password at least every 14 days.")
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
