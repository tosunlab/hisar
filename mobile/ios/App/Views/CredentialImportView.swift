import SwiftUI

/// CredentialImportView is the sheet for items another password manager exported to Hisar through Credential Exchange:
/// what arrived, the vault they go to, then the import's progress.
struct CredentialImportView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isSkippingDuplicate = true
    @State private var exportPassphrase = ""

    private var preview: ImportPreview? { model.credentialImportPreview }
    private var progress: ImportProgress? { model.credentialImportProgress }
    private var isDone: Bool { progress?.isDone == true }
    private var allDestinationGroup: [ImportVaultGroup] {
        OrganizationLogic.importVaultGroups(model.allCredentialImportVault, allOrganization: model.allOrganization)
    }

    private func destinationLabel(_ vaultID: String, fallback: String) -> String {
        allDestinationGroup.flatMap(\.allChoice).first { $0.id == vaultID }?.label ?? fallback
    }

    private var destinationChoices: some View {
        ForEach(allDestinationGroup) { group in
            Section {
                ForEach(group.allChoice) { choice in
                    Text(choice.label).tag(choice.id)
                }
            } header: {
                Text(group.name)
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    var body: some View {
        NavigationStack {
            Form {
                if model.isHisarExportImport, let fileName = preview?.fileName ?? progress?.fileName {
                    Section { ImportFileNameRow(fileName: fileName) }
                }
                if let progress {
                    progressSections(progress)
                } else if let preview {
                    if preview.isPassphraseRequired {
                        passphraseSection(preview)
                    } else {
                        previewSections(preview)
                    }
                } else if let errorMessage = model.errorMessage, !model.isReceivingCredentialImport {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                } else {
                    Section {
                        HStack(spacing: 12) {
                            ProgressView()
                            Text(model.isHisarExportImport ? "Reading the export…" : "Receiving items from the other app…")
                                .foregroundStyle(.themeSecondary)
                        }
                        .accessibilityIdentifier("credential-import-receiving")
                    }
                }
            }
            .navigationTitle(model.isHisarExportImport ? "Import Hisar export" : preview.map { "Import from \($0.fileName)" } ?? "Import")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if isDone {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Done") { model.closeCredentialImport() }
                            .accessibilityIdentifier("credential-import-done")
                    }
                } else {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(preview == nil && !model.isReceivingCredentialImport ? "Close" : "Cancel") { model.closeCredentialImport() }
                            .accessibilityIdentifier("credential-import-cancel")
                    }
                }
            }
        }
        .interactiveDismissDisabled(model.isImportingCredentials || isDone)
        .accessibilityIdentifier("credential-import")
    }

    @ViewBuilder
    private func passphraseSection(_ preview: ImportPreview) -> some View {
        Section {
            CredentialField("Export passphrase", text: $exportPassphrase, secretName: "export passphrase")
                .textContentType(.password)
                .accessibilityIdentifier("credential-import-passphrase")
                .concealWhen(model.isBusy)
            if let errorMessage = model.errorMessage {
                ErrorBanner(message: errorMessage)
            }
            Button {
                // The field is emptied after every try, so a wrong passphrase is not kept or added to.
                let entered = exportPassphrase
                exportPassphrase = ""
                Task { await model.openCredentialImportFile(passphrase: entered) }
            } label: {
                Group {
                    if model.isBusy {
                        ProgressView()
                    } else {
                        Text("Open")
                    }
                }
                .frame(maxWidth: .infinity)
            }
            .applyProminentGlassStyle()
            .controlSize(.large)
            .disabled(model.isBusy || exportPassphrase.isEmpty)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
            .accessibilityIdentifier("credential-import-open")
        } header: {
            Text("Enter the export passphrase")
                .foregroundStyle(.themeSecondary)
        } footer: {
            Text("This export is encrypted. Enter the passphrase it was exported with.")
                .foregroundStyle(.themeSecondary)
        }
    }

    @ViewBuilder
    private func previewSections(_ preview: ImportPreview) -> some View {
        let isGrouped = preview.groups.count > 1
        if preview.exportedDescription != nil || !preview.notice.isEmpty {
            Section {
                if let exportedDescription = preview.exportedDescription {
                    Text(exportedDescription)
                        .foregroundStyle(.themeSecondary)
                        .accessibilityIdentifier("credential-import-exported-from")
                }
                if !preview.notice.isEmpty {
                    Label(preview.notice, systemImage: "exclamationmark.triangle")
                        .accessibilityIdentifier("credential-import-notice")
                }
            }
        }
        Section {
            if isGrouped {
                Text(preview.isHisarExport
                    ? "This export has \(preview.groups.count) vaults. Each goes to the vault it came from, or your Personal vault, unless you choose another."
                    : "The other app kept these items in \(preview.groups.count) groups. Each goes to your Personal vault unless you choose another vault for it.")
            } else {
                Picker("Vault", selection: Binding(get: { preview.vaultID }, set: { vaultID in
                    Task { await model.previewCredentialImport(vaultID: vaultID) }
                })) {
                    destinationChoices
                }
                .disabled(model.isBusy)
                .accessibilityIdentifier("credential-import-vault")
            }
        } footer: {
            Text("\(AppModel.describeItemCount(preview.itemCount)) from \(preview.fileName).")
                .foregroundStyle(.themeSecondary)
        }
        ForEach(Array(preview.groups.enumerated()), id: \.element.id) { index, group in
            Section {
                Text("\(group.name) to \(destinationLabel(group.vaultID, fallback: group.vaultName))")
                if isGrouped {
                    Picker("Vault for \(AppModel.describeItemCount(group.itemCount))", selection: Binding(get: { group.vaultID }, set: { vaultID in
                        Task { await model.mapCredentialImportGroup(groupID: group.id, vaultID: vaultID) }
                    })) {
                        destinationChoices
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("credential-import-group-vault-\(index)")
                }
                Text(group.isPrivate ? "Private or unspecified source" : "Shared source")
                    .font(.footnote)
                ForEach(group.members) { member in
                    Text("\(member.name.isEmpty ? member.accountID : member.name) <\(member.email)> · \(member.role)\(member.isCurrentUser ? " (you)" : "")")
                }
                Text("Imported items are readable by the destination's current members. Access can change later.")
                    .font(.footnote)
            } header: {
                Text("Source and destination access")
                    .foregroundStyle(.themeSecondary)
            }
            .accessibilityIdentifier("credential-import-access")
        }
        let allCountRow = preview.allCountRow
        if !allCountRow.isEmpty {
            Section {
                ForEach(allCountRow) { row in
                    LabeledContent(row.label, value: "\(row.count)")
                }
            } header: {
                Text("Items")
                    .foregroundStyle(.themeSecondary)
            }
            .accessibilityIdentifier("credential-import-counts")
        }
        if !preview.allDuplicate.isEmpty {
            Section {
                Toggle("Skip items already in \(destinationLabel(preview.vaultID, fallback: preview.vaultName))", isOn: $isSkippingDuplicate)
                    .accessibilityIdentifier("credential-import-skip-duplicates")
                ForEach(Array(preview.allDuplicate.enumerated()), id: \.offset) { _, duplicate in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(duplicate.title)
                        let detail = duplicate.detail.isEmpty ? duplicate.username : duplicate.detail
                        if !detail.isEmpty {
                            Text(detail)
                                .font(.footnote)
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                }
            } header: {
                Text(preview.allDuplicate.count == 1 ? "1 item is already there" : "\(preview.allDuplicate.count) items are already there")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                if preview.duplicatePasskeyCount > 0 {
                    Text(preview.duplicatePasskeyCount == 1
                        ? "The passkey of a skipped login is still added to the login \(preview.vaultName) has."
                        : "Passkeys of skipped logins are still added to the logins \(preview.vaultName) has.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            .accessibilityIdentifier("credential-import-duplicates")
        }
        if !preview.allSkipped.isEmpty {
            ImportOmissionSection(entries: preview.allSkipped, header: ImportSkippedEntry.heading(count: preview.allSkipped.count), footer: ImportSkippedEntry.explanation)
                .accessibilityIdentifier("credential-import-skipped")
        }
        if let errorMessage = model.errorMessage {
            Section {
                ErrorBanner(message: errorMessage)
            }
        }
        Section {
            Button {
                Task { await model.startCredentialImport(isSkippingDuplicate: isSkippingDuplicate) }
            } label: {
                Group {
                    if model.isImportingCredentials {
                        ProgressView()
                    } else {
                        Text("Import into \(destinationLabel(preview.vaultID, fallback: preview.vaultName))")
                    }
                }
                .frame(maxWidth: .infinity)
            }
            .applyProminentGlassStyle()
            .controlSize(.large)
            .disabled(model.isBusy || model.isImportingCredentials || preview.itemCount == 0)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
            .accessibilityIdentifier("credential-import-start")
        }
    }

    @ViewBuilder
    private func progressSections(_ progress: ImportProgress) -> some View {
        if progress.isDone {
            ImportCompletionReport(progress: progress, preview: preview)
        } else {
            Section {
                ProgressView(value: Double(progress.savedCount + progress.allFailed.count), total: Double(max(progress.totalCount, 1))) {
                    Text("Importing into \(progress.vaultName)")
                } currentValueLabel: {
                    Text("\(progress.savedCount) of \(progress.totalCount) saved")
                }
                .accessibilityIdentifier("credential-import-progress")
            }
            if let errorMessage = model.errorMessage, !model.isImportingCredentials {
                Section {
                    ErrorBanner(message: errorMessage)
                    Button("Retry") {
                        Task { await model.continueCredentialImport() }
                    }
                    .accessibilityIdentifier("credential-import-retry")
                } footer: {
                    Text("The import is paused. Items saved so far stay in \(progress.vaultName).")
                        .foregroundStyle(.themeSecondary)
                }
            }
        }
        if !progress.isDone && !progress.allFailed.isEmpty {
            ImportOmissionSection(entries: progress.allFailed, header: progress.allFailed.count == 1 ? "1 item couldn't be saved" : "\(progress.allFailed.count) items couldn't be saved")
                .accessibilityIdentifier("credential-import-failed")
        }
    }

}

struct ImportCompletionReport: View {
    var progress: ImportProgress
    var preview: ImportPreview?

    var body: some View {
        Section {
            Label("Imported \(AppModel.describeItemCount(progress.savedCount)) into \(progress.vaultName).", systemImage: "checkmark.circle.fill")
                .accessibilityIdentifier("credential-import-finished")
        }
        if let preview, preview.exportedDescription != nil || !preview.notice.isEmpty {
            Section {
                if let origin = preview.exportedDescription { Text(origin) }
                if !preview.notice.isEmpty { Text(preview.notice).foregroundStyle(.themeWarning) }
            }
            .accessibilityIdentifier("credential-import-result-provenance")
        }
        let entries = progress.allSkipped.isEmpty ? preview?.allSkipped ?? [] : progress.allSkipped
        if !entries.isEmpty {
            ImportOmissionSection(entries: entries, header: ImportSkippedEntry.heading(count: entries.count),
                footer: "Some entries below are fields or credentials from items that were imported.")
                .accessibilityIdentifier("credential-import-result-skipped")
        }
        if let preview, !preview.allDuplicate.isEmpty {
            Section {
                ForEach(preview.allDuplicate.indices, id: \.self) { index in
                    let entry = preview.allDuplicate[index]
                    VStack(alignment: .leading, spacing: 2) {
                        Text(entry.title)
                        Text(entry.detail.isEmpty ? entry.username : entry.detail)
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                    }
                }
            } header: {
                Text("Reviewed duplicate matches")
                    .foregroundStyle(.themeSecondary)
            }
            .accessibilityIdentifier("credential-import-result-reviewed-duplicates")
        }
        if !progress.allDuplicateOutcomes.isEmpty {
            ImportOmissionSection(entries: progress.allDuplicateOutcomes, header: "Duplicate outcomes · \(progress.allDuplicateOutcomes.count)")
                .accessibilityIdentifier("credential-import-result-duplicates")
        }
        if !progress.allFailed.isEmpty {
            ImportOmissionSection(entries: progress.allFailed, header: "Could not be saved · \(progress.allFailed.count)")
                .accessibilityIdentifier("credential-import-failed")
        }
        Section {
            Text("Keep the other app or source file until you have checked these entries.")
            if progress.leftBehindCount > 0 {
                Text("\(progress.leftBehindCount) omission or save-failure entries need review.")
            }
        }
        .accessibilityIdentifier("credential-import-keep-source")
    }
}

private struct ImportOmissionSection: View {
    var entries: [ImportSkippedEntry]
    var header: String
    var footer = ""

    var body: some View {
        Section {
            ForEach(entries.indices, id: \.self) { index in
                let entry = entries[index]
                VStack(alignment: .leading, spacing: 2) {
                    Text(entry.title.isEmpty ? "Untitled" : entry.title)
                    Text(entry.reason)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                }
            }
        } header: {
            Text(header)
                .foregroundStyle(.themeSecondary)
        } footer: {
            if !footer.isEmpty { Text(footer) }
        }
    }
}

struct ImportFileNameRow: View {
    var fileName: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("File name")
                .font(.footnote)
                .foregroundStyle(.themeSecondary)
            Text(fileName)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 8)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("credential-import-filename")
    }
}
