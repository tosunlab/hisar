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
        case importReport
        case tags
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

    private static var importReportPreview: ImportPreview {
        var preview = ImportPreview(fileName: "Passwords", vaultID: "personal", vaultName: "Personal")
        preview.notice = "This is a cached partial export."
        preview.exportedFrom = "owner@example.com"
        preview.exportedAt = "2026-10-20T12:00:00Z"
        preview.allDuplicate = [ImportDuplicate(title: "GitHub", username: "alice", detail: "alice · github.com · different password")]
        return preview
    }

    private static var importReportProgress: ImportProgress {
        var progress = ImportProgress(fileName: "Passwords", vaultID: "personal", vaultName: "Personal", savedCount: 2, totalCount: 3, isDone: true,
            allFailed: [ImportSkippedEntry(title: "Figma", reason: "Save refused.")])
        progress.leftBehindCount = 3
        progress.allSkipped = [
            ImportSkippedEntry(title: "GitHub", reason: "Unsupported extension data."),
            ImportSkippedEntry(title: "GitHub", reason: "Different recovery code remains only in the source."),
        ]
        progress.allDuplicateOutcomes = [
            ImportSkippedEntry(title: "GitHub", reason: "Added 1 new passkey to the existing login in Hisar."),
            ImportSkippedEntry(title: "Slack", reason: "Identical duplicate already present in Hisar."),
            ImportSkippedEntry(title: "Docker", reason: "Included the duplicate as another item."),
        ]
        return progress
    }

    var body: some View {
        NavigationStack {
            if mode == .tags {
                List {
                    TagRows(allTag: [
                        TagSummary(name: "finance", itemCount: 2),
                        TagSummary(name: "personal", itemCount: 1),
                        TagSummary(name: "design-review", itemCount: 3),
                    ])
                }
                .applyThemedListStyle()
                .navigationTitle("Tags")
                .navigationDestination(for: TagDestination.self) { ItemTagListView(tag: $0.tag) }
            } else {
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
        }
        .sheet(isPresented: $isSheetShown) {
            NavigationStack {
                if mode == .importReport {
                    Form { ImportCompletionReport(progress: Self.importReportProgress, preview: Self.importReportPreview) }
                        .navigationTitle("Import from Passwords")
                        .toolbar {
                            ToolbarItem(placement: .confirmationAction) {
                                Button("Done") { isSheetShown = false }
                                    .accessibilityIdentifier("credential-import-done")
                            }
                        }
                } else if mode == .export {
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
                    .navigationTitle("Delete “Work”?")
                    .navigationBarTitleDisplayMode(.inline)
                }
            }
            .presentationDetents([.large])
        }
        .onAppear {
            switch mode {
            case .delete, .export, .importReport: isSheetShown = true
            case .leave: isLeaveConfirmationShown = true
            case .notice: break
            case .tags: break
            }
        }
    }
}
#endif
