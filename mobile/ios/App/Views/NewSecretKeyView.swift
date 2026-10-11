import SwiftUI

struct NewSecretKeyView: View {
    var secretKey: String
    var email: String
    var server: String
    var finish: () -> Void

    @State private var isCopied = false

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text("Your account is ready. To sign in on another device you need two secrets: your master password and this Secret Key. Hisar keeps the Secret Key on this iPhone; nobody else has a copy.")
                        .listRowBackground(Color.clear)
                        .listRowInsets(EdgeInsets(top: 0, leading: 4, bottom: 0, trailing: 4))
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
                Section {
                    Text(secretKey)
                        .font(.title3.monospaced().weight(.medium))
                        .textSelection(.enabled)
                        .accessibilityIdentifier("new-secret-key")
                    Button {
                        _ = PasteboardClipboard().setText(secretKey)
                        isCopied = true
                    } label: {
                        Label(isCopied ? "Copied" : "Copy Secret Key", systemImage: isCopied ? "checkmark" : "doc.on.doc")
                            .contentTransition(.symbolEffect(.replace))
                    }
                    .sensoryFeedback(.success, trigger: isCopied)
                } header: {
                    Text("Secret Key")
                }
                Section {
                    LabeledContent("Server", value: server)
                    LabeledContent("Email", value: email)
                } header: {
                    Text("Make an Emergency Kit")
                } footer: {
                    Text("The Emergency Kit is a page with your server, email and Secret Key, and a space to write your master password. Save or print it from Settings, Secret Key and Emergency Kit, or write these down now and keep them somewhere safe, away from your devices.")
                }
            }
            .applyThemedListStyle()
            .navigationTitle("Save your Secret Key")
            .safeAreaInset(edge: .bottom) {
                Button(action: finish) {
                    Text("I saved my Secret Key")
                        .frame(maxWidth: .infinity)
                }
                .applyProminentGlassStyle()
                .controlSize(.large)
                .padding(.horizontal, 20)
                .padding(.bottom, 8)
                .accessibilityIdentifier("new-secret-key-done")
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
