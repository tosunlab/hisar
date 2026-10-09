import SwiftUI

struct PasswordGeneratorView: View {
    var isUseShown: Bool
    var use: (String) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var length = 24.0
    @State private var isDigitIncluded = true
    @State private var isSymbolIncluded = true
    @State private var password = ""
    @State private var isCopied = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(password.isEmpty ? " " : password)
                        .font(.system(.title3, design: .monospaced).weight(.medium))
                        .foregroundStyle(Theme.text)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, minHeight: 60, alignment: .leading)
                        .accessibilityIdentifier("generated-password")
                        .listRowBackground(Theme.surface)
                    HStack(spacing: 12) {
                        Button {
                            Task { await generate() }
                        } label: {
                            Label("Regenerate", systemImage: "arrow.clockwise")
                        }
                        .buttonStyle(SecondaryButtonStyle())
                        Button {
                            _ = PasteboardClipboard().setText(password)
                            isCopied = true
                            model.showNotice("Password copied. It clears in 90 seconds.")
                        } label: {
                            Label(isCopied ? "Copied" : "Copy", systemImage: "doc.on.doc")
                        }
                        .buttonStyle(SecondaryButtonStyle())
                    }
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }
                Section("Options") {
                    VStack(alignment: .leading) {
                        Text("Length: \(Int(length))")
                            .foregroundStyle(Theme.text)
                        Slider(value: $length, in: 8...64, step: 1)
                            .accessibilityIdentifier("generator-length")
                    }
                    .listRowBackground(Theme.surface)
                    Toggle("Digits", isOn: $isDigitIncluded)
                        .tint(Theme.success)
                        .listRowBackground(Theme.surface)
                    Toggle("Symbols", isOn: $isSymbolIncluded)
                        .tint(Theme.success)
                        .listRowBackground(Theme.surface)
                }
                if isUseShown {
                    Section {
                        Button("Use this password") {
                            use(password)
                            dismiss()
                        }
                        .buttonStyle(PrimaryButtonStyle())
                        .disabled(password.isEmpty)
                        .listRowBackground(Color.clear)
                        .listRowInsets(EdgeInsets())
                        .accessibilityIdentifier("generator-use")
                    }
                }
            }
            .applyListTheme()
            .navigationTitle("Password generator")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
            }
            .task { await generate() }
            .onChange(of: length) { _, _ in Task { await generate() } }
            .onChange(of: isDigitIncluded) { _, _ in Task { await generate() } }
            .onChange(of: isSymbolIncluded) { _, _ in Task { await generate() } }
        }
        .preferredColorScheme(.dark)
    }

    private func generate() async {
        isCopied = false
        if let generatedPassword = await model.generatePassword(length: Int(length), isDigitIncluded: isDigitIncluded, isSymbolIncluded: isSymbolIncluded) {
            password = generatedPassword
        }
    }
}
