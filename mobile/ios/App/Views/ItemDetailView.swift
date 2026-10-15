import SwiftUI

struct ItemDetailView: View {
    var vaultID: String
    var itemID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var item: ItemDetail?
    @State private var isPasswordRevealed = false
    @State private var allRevealedFieldName = Set<String>()
    @State private var isEditorShown = false
    @State private var passkeyToDelete: ItemPasskey?

    private var vault: VaultSummary? { model.findVault(vaultID: vaultID) }

    private var isWritable: Bool {
        guard let vault else { return false }
        return vault.type == "personal" || vault.role != "reader"
    }

    var body: some View {
        Group {
            if let item {
                buildContent(item: item)
            } else if let errorMessage = model.errorMessage {
                ContentUnavailableView {
                    Label("Could not open the item", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(errorMessage)
                        .accessibilityIdentifier("error-message")
                }
            } else {
                ProgressView()
            }
        }
        .navigationTitle(item?.title ?? "")
        .navigationBarTitleDisplayMode(.inline)
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
        .confirmationDialog(
            "Delete this passkey?",
            isPresented: Binding(get: { passkeyToDelete != nil }, set: { if !$0 { passkeyToDelete = nil } }),
            titleVisibility: .visible,
            presenting: passkeyToDelete
        ) { passkey in
            Button("Delete Passkey", role: .destructive) {
                Task { await deletePasskey(passkey) }
            }
            .accessibilityIdentifier("confirm-delete-passkey")
        } message: { passkey in
            Text("You can no longer sign in to \(passkey.relyingPartyID) as \(passkey.userName) with it.")
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
                    ItemIcon(item: item, size: 56)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(item.title)
                            .font(.title2.weight(.bold))
                        Text("\(vault?.name ?? "") · \(item.isSSHKey ? "SSH key" : (item.isSecureNote ? "Secure note" : (item.isCreditCard ? (item.card ?? ItemCard()).summary : "Login")))")
                            .font(.subheadline)
                            .foregroundStyle(.themeSecondary)
                    }
                }
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets(top: 0, leading: 4, bottom: 0, trailing: 4))
                if !item.allTag.isEmpty {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 8) {
                            ForEach(item.allTag, id: \.self) { tag in
                                Label(tag, systemImage: "tag")
                                    .font(.caption.weight(.medium))
                                    .padding(.horizontal, 10)
                                    .padding(.vertical, 5)
                                    .background(.fill.tertiary, in: Capsule())
                                    .accessibilityIdentifier("item-tag-\(tag)")
                            }
                        }
                        .padding(.vertical, 2)
                    }
                    .foregroundStyle(.themeSecondary)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets(top: 0, leading: 4, bottom: 0, trailing: 4))
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("item-tags")
                }
            }
            if item.isSSHKey {
                buildSSHKeySection(item: item)
            } else if item.isSecureNote {
                buildSecureNoteSection(item: item)
            } else if item.isCreditCard {
                buildCardSection(card: item.card ?? ItemCard())
            } else {
                buildLoginSection(item: item)
            }
            if !item.allField.isEmpty {
                Section("Other fields") {
                    ForEach(Array(item.allField.enumerated()), id: \.offset) { _, field in
                        SecretFieldRow(
                            label: field.name, value: field.value, isConcealed: field.concealed,
                            isRevealed: allRevealedFieldName.contains(field.name),
                            toggleReveal: { toggleReveal(fieldName: field.name) },
                            copy: { copy(fieldName: field.name, label: field.name) },
                            copyReference: { copyReference(fieldName: field.name) }
                        )
                    }
                }
            }
            if !item.allURL.isEmpty {
                Section(item.allURL.count == 1 ? "Website" : "Websites") {
                    ForEach(Array(item.allURL.enumerated()), id: \.offset) { _, url in
                        if let openableURL = VaultLogic.convertToOpenableURL(url) {
                            Link(destination: openableURL) {
                                Label(url, systemImage: "safari")
                                    .lineLimit(1)
                            }
                            .contextMenu {
                                Button {
                                    model.copyText(url, label: "Website")
                                } label: {
                                    Label("Copy", systemImage: "doc.on.doc")
                                }
                                ShareLink(item: openableURL)
                            }
                            .accessibilityIdentifier("open-website")
                        } else {
                            Text(url)
                        }
                    }
                }
            }
            if !item.allPasskey.isEmpty {
                Section("Passkeys") {
                    ForEach(item.allPasskey) { passkey in
                        buildPasskeyRow(passkey)
                    }
                }
            }
            if !item.notes.isEmpty && !item.isSecureNote {
                Section {
                    Text(item.notes)
                        .textSelection(.enabled)
                        .contextMenu {
                            Button {
                                copy(fieldName: "notes", label: "Notes")
                            } label: {
                                Label("Copy", systemImage: "doc.on.doc")
                            }
                            Button {
                                copyReference(fieldName: "notes")
                            } label: {
                                Label("Copy Secret Reference", systemImage: "terminal")
                            }
                        }
                } header: {
                    Text("Notes")
                }
            }
            if isWritable && item.canConvertToSecureNote {
                Section {
                    Button {
                        Task { await convertToSecureNote() }
                    } label: {
                        Label("Convert to secure note", systemImage: "note.text")
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("convert-to-secure-note")
                } footer: {
                    Text("This login has no username, password or website. As a secure note, it opens straight to its notes and fields.")
                }
            }
            if let referenceField = getReferenceField(item: item) {
                Section {
                    Button {
                        copyReference(fieldName: referenceField)
                    } label: {
                        Label("Copy secret reference", systemImage: "terminal")
                    }
                    .foregroundStyle(.primary)
                    .accessibilityIdentifier("copy-secret-reference")
                } footer: {
                    Text("A hisar:// reference to the \(referenceField), for hisar run and hisar read. Touch and hold a field for its own reference.")
                }
            }
        }
        .applyThemedListStyle()
        .refreshable { await loadItem() }
    }

    @ViewBuilder
    private func buildLoginSection(item: ItemDetail) -> some View {
        if !item.username.isEmpty || !item.password.isEmpty || item.hasOneTimePassword {
            Section {
                if !item.username.isEmpty {
                    SecretFieldRow(
                        label: "Username", value: item.username, isConcealed: false, isRevealed: true, toggleReveal: nil,
                        copy: { copy(fieldName: "username", label: "Username") },
                        copyReference: { copyReference(fieldName: "username") }
                    )
                }
                if !item.password.isEmpty {
                    SecretFieldRow(
                        label: "Password", value: item.password, isConcealed: true, isRevealed: isPasswordRevealed,
                        toggleReveal: { isPasswordRevealed.toggle() },
                        copy: { copy(fieldName: "password", label: "Password") },
                        copyReference: { copyReference(fieldName: "password") }
                    )
                }
                if item.hasOneTimePassword {
                    OneTimeCodeRow(vaultID: vaultID, itemID: item.id) {
                        copy(fieldName: "one-time code", label: "One-time code")
                    } copyReference: {
                        copyReference(fieldName: "one-time code")
                    }
                }
            }
        }
    }

    /// The field the "Copy secret reference" button names: the one a script most likely wants.
    private func getReferenceField(item: ItemDetail) -> String? {
        if item.isSSHKey { return "public key" }
        if item.isSecureNote { return item.notes.isEmpty ? nil : "notes" }
        if item.isCreditCard { return (item.card?.number ?? "").isEmpty ? nil : CardFieldName.number }
        return item.password.isEmpty ? nil : "password"
    }

    @ViewBuilder
    private func buildCardSection(card: ItemCard) -> some View {
        let allRow: [(label: String, value: String, isConcealed: Bool, warning: String)] = [
            (CardFieldName.cardholder, card.cardholder, false, ""),
            (CardFieldName.type, card.type, false, ""),
            (CardFieldName.number, card.number, true, card.numberWarning),
            (CardFieldName.expiry, card.expiry, false, card.expiryWarning),
            (CardFieldName.securityCode, card.securityCode, true, ""),
            (CardFieldName.pin, card.pin, true, ""),
            (CardFieldName.billingAddress, card.billingAddress, false, ""),
        ].filter { !$0.value.isEmpty }
        Section {
            if allRow.isEmpty {
                Text("This card has no details yet.")
                    .foregroundStyle(.themeSecondary)
            }
            ForEach(allRow, id: \.label) { row in
                VStack(alignment: .leading, spacing: 4) {
                    SecretFieldRow(
                        label: row.label, value: row.value, isConcealed: row.isConcealed,
                        isRevealed: !row.isConcealed || allRevealedFieldName.contains(row.label),
                        toggleReveal: row.isConcealed ? { toggleReveal(fieldName: row.label) } : nil,
                        copy: { copy(fieldName: row.label, label: row.label) },
                        copyReference: { copyReference(fieldName: row.label) }
                    )
                    if !row.warning.isEmpty {
                        Text(row.warning)
                            .font(.footnote)
                            .foregroundStyle(.orange)
                            .accessibilityIdentifier("card-warning-\(row.label)")
                    }
                }
            }
        } header: {
            Text("Card")
        }
    }

    @ViewBuilder
    private func buildSecureNoteSection(item: ItemDetail) -> some View {
        Section {
            if item.notes.isEmpty {
                Text("This note is empty.")
                    .foregroundStyle(.themeSecondary)
            } else {
                Text(item.notes)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("secure-note-notes")
                Button {
                    copy(fieldName: "notes", label: "Notes")
                } label: {
                    Label("Copy notes", systemImage: "doc.on.doc")
                }
                .accessibilityIdentifier("copy-notes")
            }
        } header: {
            Text("Note")
        }
    }

    private func convertToSecureNote() async {
        guard let item, let convertedItem = await model.convertToSecureNote(vaultID: vaultID, itemID: item.id) else { return }
        self.item = convertedItem
    }

    @ViewBuilder
    private func buildSSHKeySection(item: ItemDetail) -> some View {
        Section("SSH key") {
            LabeledContent("Key type", value: item.keyType)
            SecretFieldRow(
                label: "Fingerprint", value: item.fingerprint, isConcealed: false, isRevealed: true, toggleReveal: nil,
                copy: { copy(fieldName: "fingerprint", label: "Fingerprint") },
                copyReference: { copyReference(fieldName: "fingerprint") }
            )
            SecretFieldRow(
                label: "Public key", value: item.publicKey, isConcealed: false, isRevealed: true, toggleReveal: nil,
                copy: { copy(fieldName: "public key", label: "Public key") },
                copyReference: { copyReference(fieldName: "public key") }
            )
        }
    }

    private func buildPasskeyRow(_ passkey: ItemPasskey) -> some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text(passkey.relyingPartyID)
                Text(passkey.userName.isEmpty ? passkey.userDisplayName : passkey.userName)
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
                if let createdDate = passkey.createdDate {
                    Text("Created \(createdDate.formatted(date: .abbreviated, time: .omitted))")
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("passkey-\(passkey.relyingPartyID)")
        } icon: {
            Image(systemName: "person.badge.key")
        }
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            if isWritable {
                Button("Delete", role: .destructive) { passkeyToDelete = passkey }
            }
        }
        .overlay(alignment: .trailing) {
            if isWritable {
                Button {
                    passkeyToDelete = passkey
                } label: {
                    Image(systemName: "trash")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(.red)
                .accessibilityLabel("Delete passkey for \(passkey.relyingPartyID)")
                .accessibilityIdentifier("delete-passkey-\(passkey.relyingPartyID)")
            }
        }
    }

    private func toggleReveal(fieldName: String) {
        if allRevealedFieldName.contains(fieldName) {
            allRevealedFieldName.remove(fieldName)
        } else {
            allRevealedFieldName.insert(fieldName)
        }
    }

    private func deletePasskey(_ passkey: ItemPasskey) async {
        passkeyToDelete = nil
        if let updatedItem = await model.deletePasskey(vaultID: vaultID, itemID: itemID, credentialID: passkey.credentialID) {
            item = updatedItem
        }
    }

    private func copy(fieldName: String, label: String) {
        Task { await model.copyField(vaultID: vaultID, itemID: itemID, fieldName: fieldName, label: label) }
    }

    private func copyReference(fieldName: String) {
        Task { await model.copySecretReference(vaultID: vaultID, itemID: itemID, fieldName: fieldName) }
    }
}

struct SecretFieldRow: View {
    var label: String
    var value: String
    var isConcealed: Bool
    var isRevealed: Bool
    var toggleReveal: (() -> Void)?
    var copy: () -> Void
    var copyReference: () -> Void

    private var isHidden: Bool { isConcealed && !isRevealed }

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 3) {
                Text(label)
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
                Text(isHidden ? String(repeating: "•", count: 12) : value)
                    .font(isConcealed ? .body.monospaced() : .body)
                    .lineLimit(isConcealed ? 3 : 5)
                    .textSelection(.enabled)
                    .accessibilityLabel(isHidden ? "Hidden" : value)
                    .accessibilityIdentifier("\(label.lowercased())-value")
            }
            Spacer(minLength: 0)
            if let toggleReveal {
                Button(action: toggleReveal) {
                    Image(systemName: isRevealed ? "eye.slash" : "eye")
                        .contentTransition(.symbolEffect(.replace))
                }
                .buttonStyle(.borderless)
                .accessibilityLabel(isRevealed ? "Hide \(label)" : "Reveal \(label)")
                .accessibilityIdentifier("reveal-\(label.lowercased())")
            }
            Button(action: copy) {
                Image(systemName: "doc.on.doc")
            }
            .buttonStyle(.borderless)
            .accessibilityLabel("Copy \(label)")
            .accessibilityIdentifier("copy-\(label.lowercased())")
        }
        .foregroundStyle(.primary)
        .contextMenu {
            Button(action: copy) {
                Label("Copy \(label)", systemImage: "doc.on.doc")
            }
            Button(action: copyReference) {
                Label("Copy Secret Reference", systemImage: "terminal")
            }
            if let toggleReveal {
                Button(action: toggleReveal) {
                    Label(isRevealed ? "Hide" : "Reveal", systemImage: isRevealed ? "eye.slash" : "eye")
                }
            }
        }
    }
}
