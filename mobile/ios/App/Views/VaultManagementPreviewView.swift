#if DEBUG
import SwiftUI

// -HisarVaultManagementPreview delete|leave|notice|export shows the vault and export screens with made-up data, for screenshots.
struct VaultManagementPreviewView: View {
    static let argument = "-HisarVaultManagementPreview"

    enum Mode: String {
        case delete
        case leave
        case notice
        case export
    }

    static func readMode() -> Mode? {
        let allArgument = ProcessInfo.processInfo.arguments
        guard let index = allArgument.firstIndex(of: argument), index + 1 < allArgument.count else { return nil }
        return Mode(rawValue: allArgument[index + 1])
    }

    var mode: Mode

    @State private var isSheetShown = false
    @State private var isLeaveConfirmationShown = false
    @State private var resultMessage: String?

    private static let workVault = VaultSummary(id: "work", name: "Work", type: "shared", role: "owner", itemCount: 12)
    private static let previewActions = VaultActions(
        vault: workVault, canRename: true, canDelete: true,
        allMemberLosingAccess: [
            Member(accountID: "bob", name: "Bob Jansen", email: "bob@example.com", role: Member.vaultRoleEditor),
            Member(accountID: "carol", name: "Carol de Vries", email: "carol@example.com", role: Member.vaultRoleReader),
        ],
        allTargetVault: [
            VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 24),
            VaultSummary(id: "team", name: "Team", type: "shared", role: "editor", itemCount: 7),
        ]
    )

    var body: some View {
        NavigationStack {
            List {
                if mode == .notice {
                    RemovedVaultNoticeSection(allNotice: [
                        RemovedVaultNotice(id: "infra", name: "Infra", message: "“Infra” is no longer available: it was deleted, or you no longer have access to it."),
                    ]) { _ in resultMessage = "Dismissed" }
                }
                if mode == .notice || mode == .export {
                    Section {
                        VaultRow(vault: VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 24), itemCount: 24)
                        VaultRow(vault: Self.workVault, itemCount: 12)
                    } header: {
                        Text("Vaults")
                            .foregroundStyle(.themeSecondary)
                    }
                } else {
                    Section {
                        ForEach(["AWS production", "GitHub", "Stripe"], id: \.self) { title in
                            ItemRow(item: ItemDetail(id: title, revision: "r1", category: "login", title: title, username: "deploy@example.com"))
                        }
                    }
                }
                if let resultMessage {
                    Section {
                        Text(resultMessage)
                            .accessibilityIdentifier("vault-management-result")
                    }
                }
            }
            .applyThemedListStyle()
            .navigationTitle(mode == .notice || mode == .export ? "Vaults" : "Work")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {} label: { Label("Vault actions", systemImage: "ellipsis") }
                }
            }
            .confirmationDialog("Leave “Work”?", isPresented: $isLeaveConfirmationShown, titleVisibility: .visible) {
                Button("Leave vault", role: .destructive) { resultMessage = "Left Work" }
            } message: {
                Text("You lose access to its items on all your devices. An owner can invite you again.")
            }
        }
        .sheet(isPresented: $isSheetShown) {
            NavigationStack {
                if mode == .export {
                    ExportDataForm(options: ExportOptions(isBiometricAvailable: true, minPassphraseLength: 10), isBusy: false, errorMessage: nil, result: nil) { format, _, _ in
                        resultMessage = "Exported \(format.rawValue)"
                        isSheetShown = false
                    }
                    .navigationTitle("Export data")
                    .navigationBarTitleDisplayMode(.inline)
                } else {
                    VaultDeletionForm(actions: Self.previewActions, isBusy: false, errorMessage: nil) { request in
                        resultMessage = request.moveToVaultID.isEmpty ? "Deleted with items" : "Moved to \(request.moveToVaultID)"
                        isSheetShown = false
                    }
                    .navigationTitle("Delete “Work”")
                    .navigationBarTitleDisplayMode(.inline)
                }
            }
            .presentationDetents(mode == .export ? [.large] : [.medium, .large])
        }
        .onAppear {
            switch mode {
            case .delete, .export: isSheetShown = true
            case .leave: isLeaveConfirmationShown = true
            case .notice: break
            }
        }
    }
}
#endif
