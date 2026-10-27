import SwiftUI

struct PasswordGeneratorSheet: View {
    var use: (String) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var password = ""

    var body: some View {
        NavigationStack {
            PasswordGeneratorView(password: $password)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button {
                            dismiss()
                        } label: {
                            Label("Close", systemImage: "xmark")
                        }
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Use") {
                            use(password)
                            dismiss()
                        }
                        .disabled(password.isEmpty)
                        .accessibilityIdentifier("generator-use")
                    }
                }
        }
        .presentationDetents([.medium, .large])
    }
}

struct PasswordGeneratorView: View {
    @Binding var password: String

    @EnvironmentObject private var model: AppModel
    @State private var length = 24.0
    @State private var isDigitIncluded = true
    @State private var isSymbolIncluded = true

    var body: some View {
        Form {
            Section {
                Text(password.isEmpty ? " " : password)
                    .font(.title3.monospaced().weight(.medium))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                    .contentTransition(.opacity)
                    .accessibilityIdentifier("generated-password")
                ItemPasswordStrengthMeter(password: password)
                Button {
                    Task { await generate() }
                } label: {
                    Label("Regenerate", systemImage: "arrow.clockwise")
                }
                .accessibilityIdentifier("generator-regenerate")
                Button {
                    model.copyText(password, label: "Password")
                } label: {
                    Label("Copy", systemImage: "doc.on.doc")
                }
                .disabled(password.isEmpty)
                .accessibilityIdentifier("generator-copy")
            }
            Section {
                HStack(spacing: 12) {
                    Text("Length")
                        .accessibilityHidden(true)
                    Slider(value: $length, in: 8...64, step: 1) {
                        Text("Length")
                    }
                    // A slider reads its value as a percentage by default; the length is a number of characters.
                    .accessibilityValue("\(Int(length))")
                    .accessibilityIdentifier("generator-length")
                    // Two digits' width, so the slider keeps its length when the value drops below 10.
                    Text(verbatim: "00")
                        .monospacedDigit()
                        .hidden()
                        .overlay(alignment: .trailing) {
                            Text("\(Int(length))")
                                .monospacedDigit()
                        }
                        .accessibilityHidden(true)
                }
                Toggle("Digits", isOn: $isDigitIncluded)
                Toggle("Symbols", isOn: $isSymbolIncluded)
            } header: {
                Text("Options")
                    .foregroundStyle(.themeSecondary)
            }
        }
        .navigationTitle("Password generator")
        .task { await generate() }
        .onChange(of: length) { _, _ in Task { await generate() } }
        .onChange(of: isDigitIncluded) { _, _ in Task { await generate() } }
        .onChange(of: isSymbolIncluded) { _, _ in Task { await generate() } }
    }

    private func generate() async {
        if let generatedPassword = await model.generatePassword(length: Int(length), isDigitIncluded: isDigitIncluded, isSymbolIncluded: isSymbolIncluded) {
            password = generatedPassword
        }
    }
}

struct StandalonePasswordGeneratorView: View {
    @State private var password = ""

    var body: some View {
        PasswordGeneratorView(password: $password)
    }
}
