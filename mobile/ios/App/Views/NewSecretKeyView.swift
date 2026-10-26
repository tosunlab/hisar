import SwiftUI

struct NewSecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    var secretKey: String

    @State private var isCopied = false
    @State private var export: EmergencyKitExport?
    @State private var isKitShared = false

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text("To sign in on another device you need your master password and this Secret Key.")
                        .listRowBackground(Color.clear)
                        .listRowInsets(EdgeInsets(top: 0, leading: 4, bottom: 0, trailing: 4))
                }
                Section {
                    Text(secretKey)
                        .font(.title3.monospaced().weight(.medium))
                        .textSelection(.enabled)
                        .accessibilityIdentifier("new-secret-key")
                    Button {
                        _ = PasteboardClipboard().setText(secretKey)
                        isCopied = true
                    } label: {
                        Label(isCopied ? "Copied · clears in 90 seconds" : "Copy Secret Key", systemImage: isCopied ? "checkmark" : "doc.on.doc")
                            .contentTransition(.symbolEffect(.replace))
                    }
                    .sensoryFeedback(.success, trigger: isCopied)
                    .accessibilityIdentifier("new-secret-key-copy")
                } header: {
                    Text("Secret Key")
                }
                Section {
                    if let export {
                        ShareLink(item: URL(fileURLWithPath: export.emergencyKitPath), preview: SharePreview("Hisar Emergency Kit", image: Image(systemName: "doc.text"))) {
                            Label("Save or Print Emergency Kit", systemImage: "square.and.arrow.up")
                                .frame(maxWidth: .infinity)
                        }
                        .applyProminentGlassStyle()
                        .controlSize(.large)
                        .simultaneousGesture(TapGesture().onEnded { isKitShared = true })
                        .listRowBackground(Color.clear)
                        .listRowInsets(EdgeInsets())
                        .accessibilityIdentifier("new-secret-key-share-kit")
                    } else if model.errorMessage == nil {
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
                    .disabled(export == nil || !(isKitShared || isCopied) || model.isBusy)
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
                export = await model.exportNewAccountEmergencyKit()
            }
            .onDisappear {
                if let export {
                    model.removeEmergencyKit(export)
                }
                export = nil
            }
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
