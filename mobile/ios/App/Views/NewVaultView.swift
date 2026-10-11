import SwiftUI

struct NewVaultView: View {
    var finish: (VaultSummary) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @FocusState private var isNameFocused: Bool

    private var canCreate: Bool { !name.trimmingCharacters(in: .whitespaces).isEmpty }

    var body: some View {
        NavigationStack {
            Form {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                            .listRowInsets(EdgeInsets())
                            .listRowBackground(Color.clear)
                    }
                }
                Section {
                    TextField("Vault name", text: $name, prompt: Text(verbatim: "Streaming, Kids, Travel…").foregroundStyle(Theme.textMuted))
                        .focused($isNameFocused)
                        .submitLabel(.done)
                        .onSubmit(create)
                        .accessibilityIdentifier("new-vault-name")
                        .listRowBackground(Theme.surface)
                } footer: {
                    Text("A shared vault starts with only you. Invite people by email from the vault's Members screen; your Personal vault stays private.")
                }
            }
            .applyListTheme()
            .navigationTitle("New vault")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        model.clearError()
                        dismiss()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.isBusy {
                        ProgressView()
                    } else {
                        Button("Create", action: create)
                            .disabled(!canCreate)
                            .accessibilityIdentifier("new-vault-create")
                    }
                }
            }
        }
        .preferredColorScheme(.dark)
        .interactiveDismissDisabled(model.isBusy)
        .onAppear {
            model.clearError()
            isNameFocused = true
        }
    }

    private func create() {
        guard canCreate, !model.isBusy else { return }
        Task {
            if let vault = await model.createVault(name: name) {
                dismiss()
                finish(vault)
            }
        }
    }
}
