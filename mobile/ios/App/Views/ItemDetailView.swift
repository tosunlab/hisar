import SwiftUI

struct ItemDetailView: View {
    var vaultID: String
    var itemID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.openURL) private var openURL
    @Environment(\.dismiss) private var dismiss
    @State private var item: ItemDetail?
    @State private var isPasswordRevealed = false
    @State private var allRevealedFieldName = Set<String>()
    @State private var isEditorShown = false

    private var isWritable: Bool {
        guard let vault = model.findVault(vaultID: vaultID) else { return false }
        return vault.type == "personal" || vault.role != "reader"
    }

    var body: some View {
        Group {
            if let item {
                buildContent(item: item)
            } else if let errorMessage = model.errorMessage {
                ErrorBanner(message: errorMessage).padding(20)
            } else {
                ProgressView().tint(Theme.textMuted)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Theme.background)
        .navigationTitle(item?.title ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .applyNavigationBarTheme()
        .toolbar {
            if isWritable, item != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Edit") { isEditorShown = true }
                        .accessibilityIdentifier("edit-item")
                }
            }
        }
        .sheet(isPresented: $isEditorShown) {
            if let item {
                ItemEditorView(vaultID: vaultID, originalItem: item) { savedItem in
                    if let savedItem {
                        self.item = savedItem
                    } else {
                        dismiss()
                    }
                }
                .environmentObject(model)
            }
        }
        .task { await loadItem() }
        .onDisappear {
            item = nil
            isPasswordRevealed = false
        }
    }

    private func loadItem() async {
        model.clearError()
        item = await model.loadItem(vaultID: vaultID, itemID: itemID)
    }

    private func buildContent(item: ItemDetail) -> some View {
        List {
            Section {
                HStack(spacing: 14) {
                    ItemIcon(item: item)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(item.title)
                            .font(.title3.weight(.semibold))
                            .foregroundStyle(Theme.text)
                        Text(model.findVault(vaultID: vaultID)?.name ?? "")
                            .font(.caption)
                            .foregroundStyle(Theme.textMuted)
                    }
                }
                .listRowBackground(Color.clear)
            }
            if item.isSSHKey {
                buildSSHKeySection(item: item)
            } else {
                buildLoginSection(item: item)
            }
            if !item.allURL.isEmpty {
                Section("Websites") {
                    ForEach(Array(item.allURL.enumerated()), id: \.offset) { _, url in
                        Button {
                            if let openableURL = VaultLogic.convertToOpenableURL(url) {
                                openURL(openableURL)
                            }
                        } label: {
                            HStack {
                                Text(url)
                                    .foregroundStyle(Theme.text)
                                    .lineLimit(1)
                                Spacer()
                                Image(systemName: "arrow.up.right.square")
                                    .foregroundStyle(Theme.textMuted)
                            }
                        }
                        .listRowBackground(Theme.surface)
                        .accessibilityIdentifier("open-website")
                    }
                }
            }
            if !item.allField.isEmpty {
                Section("Fields") {
                    ForEach(Array(item.allField.enumerated()), id: \.offset) { _, field in
                        buildFieldRow(label: field.name, value: field.value, isConcealed: field.concealed, isRevealed: allRevealedFieldName.contains(field.name), toggleReveal: {
                            if allRevealedFieldName.contains(field.name) {
                                allRevealedFieldName.remove(field.name)
                            } else {
                                allRevealedFieldName.insert(field.name)
                            }
                        }, copy: { copy(fieldName: field.name, label: field.name) })
                    }
                }
            }
            if !item.notes.isEmpty {
                Section("Notes") {
                    Text(item.notes)
                        .foregroundStyle(Theme.text)
                        .textSelection(.enabled)
                        .listRowBackground(Theme.surface)
                }
            }
        }
        .listStyle(.insetGrouped)
        .applyListTheme()
        .refreshable { await loadItem() }
    }

    @ViewBuilder
    private func buildLoginSection(item: ItemDetail) -> some View {
        Section {
            if !item.username.isEmpty {
                buildFieldRow(label: "Username", value: item.username, isConcealed: false, isRevealed: true, toggleReveal: nil, copy: { copy(fieldName: "username", label: "Username") })
            }
            if !item.password.isEmpty {
                buildFieldRow(label: "Password", value: item.password, isConcealed: true, isRevealed: isPasswordRevealed, toggleReveal: { isPasswordRevealed.toggle() }, copy: { copy(fieldName: "password", label: "Password") })
            }
        }
    }

    @ViewBuilder
    private func buildSSHKeySection(item: ItemDetail) -> some View {
        Section("SSH key") {
            buildFieldRow(label: "Key type", value: item.keyType, isConcealed: false, isRevealed: true, toggleReveal: nil, copy: nil)
            buildFieldRow(label: "Fingerprint", value: item.fingerprint, isConcealed: false, isRevealed: true, toggleReveal: nil, copy: { copy(fieldName: "fingerprint", label: "Fingerprint") })
            buildFieldRow(label: "Public key", value: item.publicKey, isConcealed: false, isRevealed: true, toggleReveal: nil, copy: { copy(fieldName: "public key", label: "Public key") })
        }
    }

    private func buildFieldRow(label: String, value: String, isConcealed: Bool, isRevealed: Bool, toggleReveal: (() -> Void)?, copy: (() -> Void)?) -> some View {
        HStack(alignment: .center, spacing: 12) {
            VStack(alignment: .leading, spacing: 4) {
                Text(label)
                    .font(.caption)
                    .foregroundStyle(Theme.textMuted)
                Text(isConcealed && !isRevealed ? String(repeating: "•", count: 12) : value)
                    .font(isConcealed ? .system(.body, design: .monospaced) : .body)
                    .foregroundStyle(Theme.text)
                    .lineLimit(isConcealed ? 3 : 5)
                    .accessibilityIdentifier("\(label.lowercased())-value")
            }
            Spacer()
            if let toggleReveal {
                Button(action: toggleReveal) {
                    Image(systemName: isRevealed ? "eye.slash" : "eye")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(Theme.textSecondary)
                .accessibilityLabel(isRevealed ? "Hide \(label)" : "Reveal \(label)")
                .accessibilityIdentifier("reveal-\(label.lowercased())")
            }
            if let copy {
                Button(action: copy) {
                    Image(systemName: "doc.on.doc")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(Theme.textSecondary)
                .accessibilityLabel("Copy \(label)")
                .accessibilityIdentifier("copy-\(label.lowercased())")
            }
        }
        .padding(.vertical, 2)
        .listRowBackground(Theme.surface)
    }

    private func copy(fieldName: String, label: String) {
        Task { await model.copyField(vaultID: vaultID, itemID: itemID, fieldName: fieldName, label: label) }
    }
}
