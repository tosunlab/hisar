import SwiftUI

enum VaultDeletionLogic {
    static func isNameConfirmed(_ typedName: String, vaultName: String) -> Bool {
        typedName.trimmingCharacters(in: .whitespacesAndNewlines) == vaultName
    }

    static func findDefaultTargetVaultID(_ actions: VaultActions) -> String {
        actions.allTargetVault.first?.id ?? ""
    }

    // buildRequest returns nil while the choice is incomplete: no target to move to, or the vault's name not typed.
    static func buildRequest(actions: VaultActions, isDeletingItems: Bool, targetVaultID: String, typedName: String) -> VaultDeletionRequest? {
        guard actions.canDelete else { return nil }
        if actions.vault.itemCount == 0 {
            return VaultDeletionRequest()
        }
        if isDeletingItems || actions.allTargetVault.isEmpty {
            guard isNameConfirmed(typedName, vaultName: actions.vault.name) else { return nil }
            return VaultDeletionRequest(isDeletingItems: true, confirmedName: actions.vault.name)
        }
        guard actions.allTargetVault.contains(where: { $0.id == targetVaultID }) else { return nil }
        return VaultDeletionRequest(moveToVaultID: targetVaultID)
    }
}

struct RecoveryDeletionReview: Identifiable {
    let vault: VaultSummary
    let issue: ReadIssue

    var id: String { "\(issue.vaultId)/\(issue.itemId ?? "")/\(issue.revision ?? "")" }
    var isItem: Bool { issue.itemId != nil }
    var title: String { isItem ? "Delete unreadable item?" : "Delete “\(vault.name)”?" }
    var confirmation: String {
        vault.readIssues.contains { $0.kind == "vault-name" || $0.kind == "vault-key" } ? String(vault.id.prefix(8)) : vault.name
    }
    var message: String {
        if let itemID = issue.itemId {
            return "Delete unreadable item \(itemID) from “\(vault.name)” on every device. Everyone who has the vault loses this item. This cannot be undone."
        }
        let items = vault.readIssues.contains { $0.kind == "vault-key" } ? "all its items" : "its \(vault.itemCount) items, including the readable ones"
        return "Delete the vault and \(items). Everyone who has the vault loses them. This cannot be undone."
    }
    func canDelete(typed: String) -> Bool { issue.canDelete && (isItem || typed == confirmation) }
}

struct RecoveryDeletionSheet: View {
    let review: RecoveryDeletionReview

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var typed = ""

    var body: some View {
        NavigationStack {
            Form {
                if let message = model.errorMessage {
                    Section { ErrorBanner(message: message) }
                }
                Section {
                    Text(review.message)
                    Text(review.vault.name)
                        .font(.headline)
                        .fixedSize(horizontal: false, vertical: true)
                    LabeledContent("Vault ID", value: review.vault.id)
                    if !review.isItem {
                        TextField("Type \(review.confirmation) to confirm", text: $typed)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("delete-unreadable-input")
                    }
                }
                Section {
                    Button("Permanently delete", role: .destructive) {
                        Task {
                            if await model.deleteUnreadableRecord(review.issue) { dismiss() }
                        }
                    }
                    .disabled(!review.canDelete(typed: typed) || model.isBusy)
                    .accessibilityIdentifier("delete-unreadable-confirm")
                }
            }
            .navigationTitle(review.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .accessibilityIdentifier("delete-unreadable-cancel")
                }
            }
        }
        .presentationDetents([.medium, .large])
        .onAppear { model.clearError() }
    }
}

enum ExportLogic {
    static let plainWarning = "Plain JSON is not encrypted. Anyone who gets the file can read every password, one-time code secret and SSH private key in it. Delete it when you are done."

    // checkInput returns what is missing before an export can start, or nil.
    static func checkInput(format: ExportFormat, passphrase: String, repeatedPassphrase: String, minPassphraseLength: Int) -> String? {
        guard format == .encrypted else { return nil }
        if passphrase.count < minPassphraseLength {
            return "Use at least \(minPassphraseLength) characters for the export passphrase."
        }
        if passphrase != repeatedPassphrase {
            return "The passphrases don't match."
        }
        return nil
    }
}

extension VaultActions: Identifiable {
    var id: String { vault.id }
}

// VaultActionsMenu offers every action; one the user can't take explains why instead.
struct VaultActionsMenu: View {
    var actions: VaultActions?
    var rename: () -> Void
    var delete: (VaultActions) -> Void
    var leave: () -> Void
    var showBlockedReason: (String) -> Void

    var body: some View {
        Menu {
            if let actions {
                if actions.canRename {
                    Button("Rename…", systemImage: "pencil", action: rename)
                        .accessibilityIdentifier("rename-vault")
                }
                if actions.vault.type != "personal" && !actions.canDelete && actions.canLeave {
                    Button("Leave vault…", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive, action: leave)
                        .accessibilityIdentifier("leave-vault")
                } else {
                    Button("Delete vault…", systemImage: "trash", role: .destructive) {
                        if actions.canDelete {
                            delete(actions)
                        } else {
                            showBlockedReason(actions.deleteBlockedReason)
                        }
                    }
                    .accessibilityIdentifier("delete-vault")
                    if actions.vault.type != "personal" {
                        Button("Leave vault…", systemImage: "rectangle.portrait.and.arrow.right") {
                            if actions.canLeave {
                                leave()
                            } else {
                                showBlockedReason(actions.leaveBlockedReason)
                            }
                        }
                        .accessibilityIdentifier("leave-vault")
                    }
                }
            } else {
                Text("Loading…")
            }
        } label: {
            Label("Vault actions", systemImage: "ellipsis")
        }
        .accessibilityIdentifier("vault-actions")
    }
}

struct VaultDeletionForm: View {
    var actions: VaultActions
    var isBusy: Bool
    var errorMessage: String?
    var delete: (VaultDeletionRequest) -> Void

    @State private var isDeletingItems = false
    @State private var targetVaultID = ""
    @State private var typedName = ""

    private var itemCount: Int { actions.vault.itemCount }
    private var isMovePossible: Bool { !actions.allTargetVault.isEmpty }
    private var request: VaultDeletionRequest? {
        VaultDeletionLogic.buildRequest(actions: actions, isDeletingItems: isDeletingItems, targetVaultID: targetVaultID, typedName: typedName)
    }

    var body: some View {
        Form {
            if let errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            if !actions.allMemberLosingAccess.isEmpty {
                Section {
                    ForEach(actions.allMemberLosingAccess) { member in
                        LabeledContent {
                            Text(describeVaultRole(member.role))
                        } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(member.name.isEmpty ? member.email : member.name)
                                Text(member.email)
                                    .font(.footnote)
                                    .foregroundStyle(.themeSecondary)
                            }
                        }
                        .accessibilityIdentifier("losing-access-\(member.email)")
                    }
                } header: {
                    Text("These people lose access")
                } footer: {
                    Text("Their Hisar removes the vault on its next sync and tells them it is no longer available.")
                }
            }
            if itemCount > 0 {
                Section {
                    if isMovePossible {
                        Picker("Items", selection: $isDeletingItems) {
                            Text("Move \(AppModel.describeItemCount(itemCount)) to another vault").tag(false)
                            Text("Delete the vault and its \(AppModel.describeItemCount(itemCount))").tag(true)
                        }
                        .pickerStyle(.inline)
                        .labelsHidden()
                        .accessibilityIdentifier("deletion-choice")
                    }
                    if isMovePossible && !isDeletingItems {
                        Picker("Move to", selection: $targetVaultID) {
                            ForEach(actions.allTargetVault) { target in
                                Text(target.name).tag(target.id)
                            }
                        }
                        .accessibilityIdentifier("deletion-target")
                    } else {
                        TextField("Type \(actions.vault.name) to confirm", text: $typedName)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("deletion-confirm-name")
                    }
                } header: {
                    Text("Items")
                } footer: {
                    if isMovePossible && !isDeletingItems {
                        Text("Hisar re-encrypts each item on this iPhone with the other vault's key before it moves. The server never sees them.")
                    } else {
                        Text("Deleted items can't be restored. Export your data first if you may need them.")
                    }
                }
            } else {
                Section {
                    Text("This vault is empty.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            Section {
                Button(role: .destructive) {
                    if let request { delete(request) }
                } label: {
                    Group {
                        if isBusy {
                            ProgressView()
                        } else {
                            Text("Delete vault")
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .disabled(request == nil || isBusy)
                .accessibilityIdentifier("delete-vault-confirm")
            }
        }
        .onAppear {
            if targetVaultID.isEmpty {
                targetVaultID = VaultDeletionLogic.findDefaultTargetVaultID(actions)
            }
        }
    }
}

struct VaultDeletionSheet: View {
    var actions: VaultActions
    var onDeleted: () -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            VaultDeletionForm(actions: actions, isBusy: model.isBusy, errorMessage: model.errorMessage) { request in
                Task {
                    if await model.deleteVault(vaultID: actions.vault.id, request: request) {
                        dismiss()
                        onDeleted()
                    }
                }
            }
            .navigationTitle("Delete “\(actions.vault.name)”?")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .accessibilityIdentifier("delete-vault-cancel")
                }
            }
        }
        .presentationDetents([.medium, .large])
        .onAppear { model.clearError() }
    }
}

struct RemovedVaultNoticeSection: View {
    var allNotice: [RemovedVaultNotice]
    var dismiss: (RemovedVaultNotice) -> Void

    var body: some View {
        if !allNotice.isEmpty {
            Section {
                ForEach(allNotice) { notice in
                    HStack(alignment: .top, spacing: 12) {
                        Label {
                            Text(notice.message)
                                .font(.subheadline)
                        } icon: {
                            Image(systemName: "archivebox")
                                .foregroundStyle(.themeSecondary)
                        }
                        Spacer(minLength: 0)
                        Button {
                            dismiss(notice)
                        } label: {
                            Image(systemName: "xmark")
                                .font(.footnote.weight(.semibold))
                                .foregroundStyle(.themeSecondary)
                        }
                        .buttonStyle(.borderless)
                        .accessibilityLabel("Dismiss")
                        .accessibilityIdentifier("dismiss-removed-vault-\(notice.id)")
                    }
                    .accessibilityIdentifier("removed-vault-\(notice.id)")
                }
            }
        }
    }
}

struct ExportDataForm: View {
    var options: ExportOptions
    var isBusy: Bool
    var errorMessage: String?
    var result: ExportResult?
    var export: (ExportFormat, String, String) -> Void

    @State private var format = ExportFormat.encrypted
    @State private var passphrase = ""
    @State private var repeatedPassphrase = ""
    @State private var password = ""

    private var inputProblem: String? {
        ExportLogic.checkInput(format: format, passphrase: passphrase, repeatedPassphrase: repeatedPassphrase, minPassphraseLength: options.minPassphraseLength)
    }
    private var isPasswordNeeded: Bool { !options.isBiometricAvailable }
    private var canExport: Bool { inputProblem == nil && !isBusy && (!isPasswordNeeded || !password.isEmpty) }

    var body: some View {
        Form {
            if let result {
                Section {
                    if let warning = result.warning, !warning.isEmpty {
                        Text(warning).foregroundStyle(.themeWarning).accessibilityIdentifier("export-provenance")
                    }
                    ShareLink(item: URL(fileURLWithPath: result.path), preview: SharePreview(URL(fileURLWithPath: result.path).lastPathComponent, image: Image(systemName: "doc"))) {
                        Label("Save or share export", systemImage: "square.and.arrow.up")
                    }
                    .accessibilityIdentifier("share-export")
                } header: {
                    Text("Export ready")
                } footer: {
                    Text("\(result.vaultCount == 1 ? "1 vault" : "\(result.vaultCount) vaults"), \(AppModel.describeItemCount(result.itemCount)). The file is removed from this iPhone when you close this screen.")
                }
            } else {
                Section {
                    Picker("Format", selection: $format) {
                        Text("Encrypted export (recommended)").tag(ExportFormat.encrypted)
                        Text("Plain JSON").tag(ExportFormat.plain)
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                    .accessibilityIdentifier("export-format")
                } header: {
                    Text("Format")
                } footer: {
                    Text("Every vault you can open, with logins, notes, one-time code secrets, SSH keys and other fields. hisar import reads both formats.")
                    Text("Offline exports are cached copies. Changes since completed syncs may be absent. The result and file report known omissions.")
                }
                if format == .encrypted {
                    Section {
                        SecureField("Export passphrase", text: $passphrase)
                            .textContentType(.newPassword)
                            .accessibilityIdentifier("export-passphrase")
                        SecureField("Repeat export passphrase", text: $repeatedPassphrase)
                            .textContentType(.newPassword)
                            .accessibilityIdentifier("export-passphrase-repeat")
                    } footer: {
                        Text("At least \(options.minPassphraseLength) characters. You need it to import the file; Hisar can't recover it.")
                    }
                } else {
                    Section {
                        Label {
                            Text(ExportLogic.plainWarning)
                                .font(.subheadline)
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundStyle(.themeWarning)
                        }
                        .accessibilityIdentifier("export-plain-warning")
                    }
                }
                if isPasswordNeeded {
                    Section {
                        SecureField("Master password", text: $password)
                            .textContentType(.password)
                            .accessibilityIdentifier("export-password")
                    }
                }
                if let errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                } else if !passphrase.isEmpty || !repeatedPassphrase.isEmpty, let inputProblem {
                    Section {
                        Text(inputProblem)
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                    }
                }
                Section {
                    Button {
                        export(format, passphrase, password)
                    } label: {
                        Group {
                            if isBusy {
                                ProgressView()
                            } else if isPasswordNeeded {
                                Text("Export")
                            } else {
                                Label("Export with \(SettingsView.biometryName)", systemImage: SettingsView.biometrySystemImage)
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .controlSize(.large)
                    .disabled(!canExport)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                    .accessibilityIdentifier("export-start")
                }
            }
        }
    }
}

struct ExportDataView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var options = ExportOptions(isBiometricAvailable: true, minPassphraseLength: 10)
    @State private var result: ExportResult?

    var body: some View {
        NavigationStack {
            ExportDataForm(options: options, isBusy: model.isBusy, errorMessage: model.errorMessage, result: result) { format, passphrase, password in
                Task {
                    result = await model.exportAllData(format: format, passphrase: passphrase, password: password)
                }
            }
            .navigationTitle("Export data")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(result == nil ? "Cancel" : "Done") { dismiss() }
                        .accessibilityIdentifier("export-done")
                }
            }
        }
        .task {
            model.clearError()
            if let loaded = await model.loadExportOptions() {
                options = loaded
            }
        }
        .onDisappear {
            if let result {
                model.removeExport(result)
            }
            result = nil
        }
    }
}
