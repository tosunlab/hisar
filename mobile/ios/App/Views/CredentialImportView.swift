import SwiftUI

/// CredentialImportView is the sheet for items another password manager exported to Hisar through Credential Exchange:
/// what arrived, the vault they go to, then the import's progress.
struct CredentialImportView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isSkippingDuplicate = true

    private var preview: ImportPreview? { model.credentialImportPreview }
    private var progress: ImportProgress? { model.credentialImportProgress }
    private var isDone: Bool { progress?.isDone == true }

    var body: some View {
        NavigationStack {
            Form {
                if let progress {
                    progressSections(progress)
                } else if let preview {
                    previewSections(preview)
                } else if let errorMessage = model.errorMessage, !model.isReceivingCredentialImport {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                } else {
                    Section {
                        HStack(spacing: 12) {
                            ProgressView()
                            Text("Receiving items from the other app…")
                                .foregroundStyle(.themeSecondary)
                        }
                        .accessibilityIdentifier("credential-import-receiving")
                    }
                }
            }
            .navigationTitle(preview.map { "Import from \($0.fileName)" } ?? "Import")
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
        .interactiveDismissDisabled(model.isImportingCredentials)
        .accessibilityIdentifier("credential-import")
    }

    @ViewBuilder
    private func previewSections(_ preview: ImportPreview) -> some View {
        Section {
            Picker("Vault", selection: Binding(get: { preview.vaultID }, set: { vaultID in
                Task { await model.previewCredentialImport(vaultID: vaultID) }
            })) {
                ForEach(model.allCredentialImportVault) { vault in
                    Text(vault.name).tag(vault.id)
                }
            }
            .disabled(model.isBusy)
            .accessibilityIdentifier("credential-import-vault")
        } footer: {
            Text("\(AppModel.describeItemCount(preview.itemCount)) from \(preview.fileName).")
        }
        let allCountRow = preview.allCountRow
        if !allCountRow.isEmpty {
            Section("Items") {
                ForEach(allCountRow) { row in
                    LabeledContent(row.label, value: "\(row.count)")
                }
            }
            .accessibilityIdentifier("credential-import-counts")
        }
        if !preview.allDuplicate.isEmpty {
            Section {
                Toggle("Skip items already in \(preview.vaultName)", isOn: $isSkippingDuplicate)
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
            } footer: {
                if preview.duplicatePasskeyCount > 0 {
                    Text(preview.duplicatePasskeyCount == 1
                        ? "The passkey of a skipped login is still added to the login \(preview.vaultName) has."
                        : "Passkeys of skipped logins are still added to the logins \(preview.vaultName) has.")
                }
            }
            .accessibilityIdentifier("credential-import-duplicates")
        }
        if !preview.allSkipped.isEmpty {
            skippedSection(preview.allSkipped, header: preview.allSkipped.count == 1 ? "1 item can't be imported" : "\(preview.allSkipped.count) items can't be imported")
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
                        Text("Import into \(preview.vaultName)")
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
            Section {
                Label {
                    Text("Imported \(AppModel.describeItemCount(progress.savedCount)) into \(progress.vaultName).")
                } icon: {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundStyle(.tint)
                }
                .accessibilityIdentifier("credential-import-finished")
            }
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
                }
            }
        }
        if !progress.allFailed.isEmpty {
            skippedSection(progress.allFailed, header: progress.allFailed.count == 1 ? "1 item couldn't be saved" : "\(progress.allFailed.count) items couldn't be saved")
                .accessibilityIdentifier("credential-import-failed")
        }
    }

    private func skippedSection(_ allEntry: [ImportSkippedEntry], header: String) -> some View {
        Section(header) {
            ForEach(Array(allEntry.enumerated()), id: \.offset) { _, entry in
                VStack(alignment: .leading, spacing: 2) {
                    Text(entry.title.isEmpty ? "Untitled" : entry.title)
                    Text(entry.reason)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                }
            }
        }
    }
}
