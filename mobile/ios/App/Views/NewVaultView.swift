import SwiftUI

struct NewVaultView: View {
    var finish: (VaultSummary) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    /// Nil until Where is loaded, and while it could not be: the teams are unknown then, so Create waits.
    @State private var allLocation: [VaultLocation]?
    @State private var locationID = VaultLocation.sharedVault.id
    @FocusState private var isNameFocused: Bool

    private var location: VaultLocation? { OrganizationLogic.findVaultLocation(id: locationID, in: allLocation) }
    private var canCreate: Bool { !name.trimmingCharacters(in: .whitespaces).isEmpty && location != nil }

    var body: some View {
        NavigationStack {
            Form {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                Section {
                    TextField("Vault name", text: $name, prompt: Text(verbatim: "Production, Staging, Team…").foregroundStyle(.themeSecondary))
                        .focused($isNameFocused)
                        .submitLabel(.done)
                        .onSubmit(create)
                        .accessibilityIdentifier("new-vault-name")
                    if let allLocation, allLocation.count > 1 || location == nil {
                        Picker("Where", selection: $locationID) {
                            ForEach(allLocation) { location in
                                Text(location.label).tag(location.id)
                            }
                        }
                        .accessibilityIdentifier("new-vault-where")
                    }
                } footer: {
                    if let location {
                        Text(location.isSharedVault
                             ? "A shared vault starts with only you. Share it by email with Share in the vault's toolbar; your Personal vault stays private."
                             : "\(location.teamName)'s members get this vault. Manage it in Settings → Organizations.")
                    }
                }
            }
            .navigationTitle("New vault")
            .navigationBarTitleDisplayMode(.inline)
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
        .interactiveDismissDisabled(model.isBusy)
        .onAppear {
            model.clearError()
            isNameFocused = true
        }
        .task {
            await loadLocations()
        }
        .onChange(of: model.remoteChangeCount) { _, _ in
            Task { await loadLocations() }
        }
    }

    // A failed reload keeps the choices already loaded; without any, Create stays disabled and the banner says why.
    private func loadLocations() async {
        allLocation = await model.loadVaultLocations() ?? allLocation
    }

    private func create() {
        guard canCreate, !model.isBusy, let location else { return }
        Task {
            let vault = location.isSharedVault
                ? await model.createVault(name: name)
                : await model.createOrganizationVault(organizationID: location.organizationID, teamID: location.teamID, name: name)
            if let vault {
                dismiss()
                finish(vault)
            }
        }
    }
}
