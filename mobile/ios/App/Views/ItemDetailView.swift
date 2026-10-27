import SwiftUI

struct ItemDetailView: View {
    @State var vaultID: String
    var itemID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var item: ItemDetail?
    @State private var isPasswordRevealed = false
    @State private var allRevealedFieldName = Set<String>()
    @State private var isEditorShown = false
    @State private var passkeyToDelete: ItemPasskey?
    @State private var actionErrorMessage: String?
    @State private var isMoveShown = false

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
            if let item {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await toggleFavorite(item) }
                    } label: {
                        Label(item.isFavorite ? "Remove from Favorites" : "Add to Favorites", systemImage: item.isFavorite ? "star.fill" : "star")
                    }
                    .accessibilityIdentifier("favorite-item")
                    .accessibilityValue(item.isFavorite ? "Favorite" : "")
                }
            }
            if isWritable, item != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Edit") { isEditorShown = true }
                        .accessibilityIdentifier("edit-item")
                }
            }
            if item != nil, !model.moveDestinations(vaultID: vaultID).isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Move to vault…") { model.clearError(); isMoveShown = true }
                        .accessibilityIdentifier("move-item")
                }
            }
        }
        .sheet(isPresented: $isEditorShown) {
            if let item {
                ItemEditorView(vaultID: vaultID, originalItem: item) { savedItem, _ in
                    if let savedItem {
                        self.item = savedItem
                    } else {
                        dismiss()
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isMoveShown) {
            if let item {
                MoveItemSheet(sourceVaultID: vaultID, item: item) { moved, destinationID in
                    self.vaultID = destinationID
                    self.item = moved
                    isMoveShown = false
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
            Button("Delete passkey", role: .destructive) {
                Task { await deletePasskey(passkey) }
            }
            .accessibilityIdentifier("confirm-delete-passkey")
        } message: { passkey in
            Text(passkey.deletionMessage(vault: vault))
        }
        .alert("Could not complete the action", isPresented: Binding(
            get: { actionErrorMessage != nil },
            set: { if !$0 { actionErrorMessage = nil } }
        )) {
            Button("OK") { actionErrorMessage = nil }
        } message: {
            Text(actionErrorMessage ?? "")
        }
        .task { await loadItem() }
        .onDisappear {
            item = nil
            isPasswordRevealed = false
        }
    }

    private func toggleFavorite(_ shownItem: ItemDetail) async {
        if await model.setItemFavorite(vaultID: vaultID, itemID: shownItem.id, isFavorite: !shownItem.isFavorite, onFailure: { actionErrorMessage = $0 }) {
            item?.isFavorite = !shownItem.isFavorite
        }
    }

    private func loadItem() async {
        model.clearError()
        item = await model.loadItem(vaultID: vaultID, itemID: itemID)
    }

    private func buildContent(item: ItemDetail) -> some View {
        List {
            Section {
                ItemDetailHeader(item: item, subtitle: "\(vault?.name ?? "") · \(describeCategory(item: item))")
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
            } else if item.isIdentity {
                buildIdentitySection(identity: item.identity ?? ItemIdentity())
            } else if item.isAPICredential {
                buildAPICredentialSection(credential: item.apiCredential ?? ItemAPICredential())
            } else if item.isAddress {
                buildNamedSection(title: "Address", emptyText: "This address has no details yet.", allRow: buildAddressRows(item.address ?? ItemAddress()))
            } else if item.isSoftwareLicense {
                buildSoftwareLicenseSection(license: item.softwareLicense ?? ItemSoftwareLicense())
            } else {
                if item.isServer {
                    buildServerSection(server: item.server ?? ItemServer())
                }
                if item.isDatabase {
                    buildDatabaseSection(database: item.database ?? ItemDatabase())
                }
                buildLoginSection(item: item)
                if let connectionString = item.database?.connectionString, item.isDatabase, !connectionString.isEmpty {
                    buildConnectionStringSection(connectionString: connectionString)
                }
            }
            if !item.allField.isEmpty {
                Section {
                    ForEach(Array(item.allField.enumerated()), id: \.offset) { _, field in
                        SecretFieldRow(
                            label: field.name, isCustomField: true, value: field.value, isConcealed: field.concealed,
                            isRevealed: allRevealedFieldName.contains(field.name),
                            toggleReveal: { toggleReveal(fieldName: field.name) },
                            copy: { copy(fieldName: field.name, label: field.name) },
                            copyReference: { copyReference(fieldName: field.name) }
                        )
                    }
                } header: {
                    Text("Other fields")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if !item.allURL.isEmpty {
                Section {
                    ForEach(Array(item.allURL.enumerated()), id: \.offset) { _, url in
                        HStack(spacing: 0) {
                            CopyFieldButton(label: "website", value: url, copy: { model.copyText(url, label: "Website") }) {
                                Text(url)
                                    .lineLimit(1)
                            }
                            if let openableURL = VaultLogic.convertToOpenableURL(url) {
                                Link(destination: openableURL) {
                                    Image(systemName: "safari")
                                        .iconButtonHitArea()
                                        .padding(.trailing, 20)
                                        .frame(maxHeight: .infinity)
                                        .contentShape(.rect)
                                }
                                .buttonStyle(.borderless)
                                .accessibilityLabel("Open website")
                                .accessibilityIdentifier("open-website")
                            }
                        }
                        .fixedSize(horizontal: false, vertical: true)
                        .listRowInsets(EdgeInsets())
                        .contextMenu {
                            Button {
                                model.copyText(url, label: "Website")
                            } label: {
                                Label("Copy", systemImage: "doc.on.doc")
                            }
                            if let openableURL = VaultLogic.convertToOpenableURL(url) {
                                ShareLink(item: openableURL)
                            }
                        }
                    }
                } header: {
                    Text(item.isServer ? "Admin console" : item.allURL.count == 1 ? "Website" : "Websites")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if !item.allAndroidApp.isEmpty {
                Section {
                    ForEach(item.allAndroidApp) { androidApp in
                        buildAndroidAppRow(androidApp)
                    }
                } header: {
                    Text(item.allAndroidApp.count == 1 ? "Android app" : "Android apps")
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text("Linked on an Android phone. AutoFill there offers this login in these apps.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if !item.allPasskey.isEmpty {
                Section {
                    ForEach(item.allPasskey) { passkey in
                        buildPasskeyRow(passkey)
                    }
                } header: {
                    Text("Passkeys")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if !item.notes.isEmpty && !item.isSecureNote {
                Section {
                    CopyFieldButton(label: "notes", value: item.notes, copy: { copy(fieldName: "notes", label: "Notes") }) {
                        Text(item.notes)
                    }
                    .contextMenu {
                        Button {
                            copy(fieldName: "notes", label: "Notes")
                        } label: {
                            Label("Copy", systemImage: "doc.on.doc")
                        }
                        Button {
                            copyReference(fieldName: "notes")
                        } label: {
                            Label("Copy secret reference", systemImage: "terminal")
                        }
                    }
                } header: {
                    Text("Notes")
                        .foregroundStyle(.themeSecondary)
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
                        .foregroundStyle(.themeSecondary)
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
                    Text("For hisar run and hisar read on your Mac.")
                        .foregroundStyle(.themeSecondary)
                }
            }
        }
        .applyThemedListStyle()
        .refreshable { await loadItem() }
    }

    /// A server's host and port with copy and copy reference, and its SSH key, which opens the key item.
    @ViewBuilder
    private func buildServerSection(server: ItemServer) -> some View {
        if !server.host.isEmpty || !server.port.isEmpty || !server.sshKeyID.isEmpty {
            Section {
                if !server.host.isEmpty {
                    SecretFieldRow(
                        label: ServerFieldName.host, value: server.host, isConcealed: false, isRevealed: true, toggleReveal: nil,
                        copy: { copy(fieldName: ServerFieldName.host, label: ServerFieldName.host) },
                        copyReference: { copyReference(fieldName: ServerFieldName.host) }
                    )
                }
                if !server.port.isEmpty {
                    SecretFieldRow(
                        label: ServerFieldName.port, value: server.port, isConcealed: false, isRevealed: true, toggleReveal: nil,
                        copy: { copy(fieldName: ServerFieldName.port, label: ServerFieldName.port) },
                        copyReference: { copyReference(fieldName: ServerFieldName.port) },
                        warning: server.portWarning
                    )
                }
                if !server.sshKeyID.isEmpty {
                    if server.sshKeyTitle.isEmpty {
                        LabeledContent(ServerFieldName.sshKey, value: "Missing SSH key")
                            .accessibilityIdentifier("server-ssh-key")
                    } else {
                        NavigationLink(value: ItemDestination(vaultID: vaultID, itemID: server.sshKeyID)) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(ServerFieldName.sshKey)
                                    .font(.caption)
                                    .foregroundStyle(.themeSecondary)
                                Label(server.sshKeyTitle, systemImage: "key.horizontal")
                                Text(server.sshKeyFingerprint)
                                    .font(.caption.monospaced())
                                    .foregroundStyle(.themeSecondary)
                                    .lineLimit(1)
                            }
                        }
                        .accessibilityIdentifier("server-ssh-key")
                    }
                }
            } header: {
                Text("Server")
                    .foregroundStyle(.themeSecondary)
            }
        }
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
                    ItemPasswordStrengthMeter(password: item.password)
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
        if item.isIdentity { return nil }
        if item.isAPICredential { return (item.apiCredential?.secret ?? "").isEmpty ? nil : APICredentialFieldName.secret }
        if item.isAddress { return nil }
        if item.isSoftwareLicense { return (item.softwareLicense?.licenseKey ?? "").isEmpty ? nil : SoftwareLicenseFieldName.licenseKey }
        return item.password.isEmpty ? nil : "password"
    }

    /// A database's engine, by its name, host, port, name and options, each with copy and copy reference.
    private func buildDatabaseSection(database: ItemDatabase) -> some View {
        buildNamedSection(title: "Database", emptyText: "This database has no details yet.", allRow: [
            (DatabaseFieldName.engine, database.engineName, false, ""),
            (DatabaseFieldName.host, database.host, false, ""),
            (DatabaseFieldName.port, database.port, false, database.portWarning),
            (DatabaseFieldName.name, database.name, false, ""),
            (DatabaseFieldName.options, database.options, false, ""),
        ])
    }

    /// The connection string the backend builds from the database's values and password, hidden until revealed.
    private func buildConnectionStringSection(connectionString: String) -> some View {
        Section {
            SecretFieldRow(
                label: "Connection string", value: connectionString, isConcealed: true,
                isRevealed: allRevealedFieldName.contains(DatabaseFieldName.connectionString),
                toggleReveal: { toggleReveal(fieldName: DatabaseFieldName.connectionString) },
                copy: { copy(fieldName: DatabaseFieldName.connectionString, label: "Connection string") },
                copyReference: { copyReference(fieldName: DatabaseFieldName.connectionString) }
            )
        } footer: {
            Text("Built from the fields above, so it always has the current password.")
                .foregroundStyle(.themeSecondary)
        }
    }

    /// The line under the title: the category, or a card's, identity's, address's, software license's, server's or
    /// database's summary. An API credential's summary repeats its Service and Host rows, so it shows the category.
    private func describeCategory(item: ItemDetail) -> String {
        if item.isSSHKey { return "SSH key" }
        if item.isSecureNote { return "Secure note" }
        if item.isCreditCard { return (item.card ?? ItemCard()).summary }
        if item.isIdentity { return (item.identity ?? ItemIdentity()).summary }
        if item.isAPICredential { return "API credential" }
        if item.isAddress { return (item.address ?? ItemAddress()).summary }
        if item.isSoftwareLicense { return (item.softwareLicense ?? ItemSoftwareLicense()).summary }
        if item.isServer { return (item.server ?? ItemServer()).summary(username: item.username) }
        if item.isDatabase { return (item.database ?? ItemDatabase()).summary }
        return "Login"
    }

    private typealias NamedRow = (label: String, value: String, isConcealed: Bool, warning: String)

    /// An address's values as rows, as an address item, an identity and a card keep them.
    private func buildAddressRows(_ address: ItemAddress) -> [NamedRow] {
        address.allNamedValue.map { ($0.name, $0.value, false, "") }
    }

    private func buildCardSection(card: ItemCard) -> some View {
        let allRow: [NamedRow] = [
            (CardFieldName.cardholder, card.cardholder, false, ""),
            (CardFieldName.type, card.type, false, ""),
            (CardFieldName.number, card.number, true, card.numberWarning),
            (CardFieldName.expiry, card.expiry, false, card.expiryWarning),
            (CardFieldName.securityCode, card.securityCode, true, ""),
            (CardFieldName.pin, card.pin, true, ""),
        ] + buildAddressRows(card.postalAddress) + [
            (CardFieldName.billingAddress, card.billingAddress, false, ""),
        ]
        return buildNamedSection(title: "Card", emptyText: "This card has no details yet.", allRow: allRow)
    }

    private func buildIdentitySection(identity: ItemIdentity) -> some View {
        let allRow: [NamedRow] = [
            (IdentityFieldName.firstName, identity.firstName, false, ""),
            (IdentityFieldName.middleName, identity.middleName, false, ""),
            (IdentityFieldName.lastName, identity.lastName, false, ""),
            (IdentityFieldName.birthDate, identity.birthDate, false, ""),
            (IdentityFieldName.email, identity.email, false, ""),
            (IdentityFieldName.phone, identity.phone, false, ""),
        ] + buildAddressRows(identity.postalAddress) + [
            (IdentityFieldName.address, identity.address, false, ""),
            (IdentityFieldName.passportNumber, identity.passportNumber, true, ""),
            (IdentityFieldName.driversLicenseNumber, identity.driversLicenseNumber, true, ""),
            (IdentityFieldName.nationalIDNumber, identity.nationalIDNumber, true, ""),
        ]
        return buildNamedSection(title: "Identity", emptyText: "This identity has no details yet.", allRow: allRow)
    }

    private func buildAPICredentialSection(credential: ItemAPICredential) -> some View {
        let allRow: [NamedRow] = [
            (APICredentialFieldName.service, credential.service, false, ""),
            (APICredentialFieldName.keyID, credential.keyID, false, ""),
            (APICredentialFieldName.secret, credential.secret, true, ""),
            (APICredentialFieldName.host, credential.host, false, ""),
            (APICredentialFieldName.expiry, credential.expiry, false, credential.expiryWarning),
        ]
        return buildNamedSection(title: "Credential", emptyText: "This API credential has no details yet.", allRow: allRow)
    }

    /// A software license's values with the license key masked until revealed, and a link that opens an http or https
    /// download URL in the browser.
    @ViewBuilder
    private func buildSoftwareLicenseSection(license: ItemSoftwareLicense) -> some View {
        let allRow: [NamedRow] = [
            (SoftwareLicenseFieldName.product, license.product, false, ""),
            (SoftwareLicenseFieldName.version, license.version, false, ""),
            (SoftwareLicenseFieldName.licenseKey, license.licenseKey, true, ""),
            (SoftwareLicenseFieldName.licensedTo, license.licensedTo, false, ""),
            (SoftwareLicenseFieldName.email, license.email, false, ""),
            (SoftwareLicenseFieldName.purchaseDate, license.purchaseDate, false, ""),
            (SoftwareLicenseFieldName.downloadURL, license.downloadURL, false, ""),
        ]
        buildNamedSection(title: "License", emptyText: "This software license has no details yet.", allRow: allRow)
        if let downloadURL = VaultLogic.convertToOpenableURL(license.downloadURL) {
            Section {
                Link(destination: downloadURL) {
                    Label("Open download page", systemImage: "safari")
                }
                .accessibilityIdentifier("open-download-url")
            }
        }
    }

    /// The values of a type kept in named fields, a card's, an identity's, an API credential's, an address's or a software
    /// license's, with copy and copy reference; secret ones are masked until revealed.
    @ViewBuilder
    private func buildNamedSection(title: String, emptyText: String, allRow: [NamedRow]) -> some View {
        let allShownRow = allRow.filter { !$0.value.isEmpty }
        Section {
            if allShownRow.isEmpty {
                Text(emptyText)
                    .foregroundStyle(.themeSecondary)
            }
            ForEach(allShownRow, id: \.label) { row in
                SecretFieldRow(
                    label: row.label, value: row.value, isConcealed: row.isConcealed,
                    isRevealed: !row.isConcealed || allRevealedFieldName.contains(row.label),
                    toggleReveal: row.isConcealed ? { toggleReveal(fieldName: row.label) } : nil,
                    copy: { copy(fieldName: row.label, label: row.label) },
                    copyReference: { copyReference(fieldName: row.label) },
                    warning: row.warning
                )
            }
        } header: {
            Text(title)
                .foregroundStyle(.themeSecondary)
        }
    }

    @ViewBuilder
    private func buildSecureNoteSection(item: ItemDetail) -> some View {
        Section {
            if item.notes.isEmpty {
                Text("This note is empty.")
                    .foregroundStyle(.themeSecondary)
            } else {
                CopyFieldButton(label: "notes", value: item.notes, identifier: "secure-note-notes", copy: { copy(fieldName: "notes", label: "Notes") }) {
                    Text(item.notes)
                }
                .contextMenu {
                    Button {
                        copy(fieldName: "notes", label: "Notes")
                    } label: {
                        Label("Copy", systemImage: "doc.on.doc")
                    }
                    Button {
                        copyReference(fieldName: "notes")
                    } label: {
                        Label("Copy secret reference", systemImage: "terminal")
                    }
                }
            }
        } header: {
            Text("Note")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func convertToSecureNote() async {
        guard let item, let convertedItem = await model.convertToSecureNote(vaultID: vaultID, itemID: item.id, onFailure: { actionErrorMessage = $0 }) else { return }
        self.item = convertedItem
    }

    @ViewBuilder
    private func buildSSHKeySection(item: ItemDetail) -> some View {
        Section {
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
        } header: {
            Text("SSH key")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func buildAndroidAppRow(_ androidApp: ItemAndroidApp) -> some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text(androidApp.packageName)
                    .textSelection(.enabled)
                Text("Certificate SHA-256 \(androidApp.certificateSHA256)")
                    .font(.caption.monospaced())
                    .foregroundStyle(.themeSecondary)
                    .textSelection(.enabled)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("android-app-\(androidApp.packageName)")
        } icon: {
            Image(systemName: "apps.iphone")
        }
    }

    private func buildPasskeyRow(_ passkey: ItemPasskey) -> some View {
        HStack(spacing: 12) {
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
            Spacer(minLength: 0)
            if isWritable {
                Button {
                    passkeyToDelete = passkey
                } label: {
                    Image(systemName: "trash")
                        .iconButtonHitArea()
                }
                .buttonStyle(.borderless)
                .foregroundStyle(.red)
                .accessibilityLabel("Delete passkey for \(passkey.relyingPartyID)")
                .accessibilityIdentifier("delete-passkey-\(passkey.relyingPartyID)")
            }
        }
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            if isWritable {
                Button("Delete", role: .destructive) { passkeyToDelete = passkey }
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
        if let updatedItem = await model.deletePasskey(vaultID: vaultID, itemID: itemID, credentialID: passkey.credentialID, onFailure: { actionErrorMessage = $0 }) {
            item = updatedItem
        }
    }

    private func copy(fieldName: String, label: String) {
        Task {
            await model.copyField(vaultID: vaultID, itemID: itemID, fieldName: fieldName, label: label, onFailure: { actionErrorMessage = "Could not copy \(label). \($0)" })
        }
    }

    private func copyReference(fieldName: String) {
        Task {
            await model.copySecretReference(vaultID: vaultID, itemID: itemID, fieldName: fieldName, onFailure: { actionErrorMessage = "Could not copy the secret reference. \($0)" })
        }
    }
}

struct SecretFieldRow: View {
    var label: String
    var isCustomField = false
    var value: String
    var isConcealed: Bool
    var isRevealed: Bool
    var toggleReveal: (() -> Void)?
    var copy: () -> Void
    var copyReference: () -> Void
    var warning = ""

    private var isHidden: Bool { isConcealed && !isRevealed }

    private var actionLabel: String {
        let firstWord = label.prefix { !$0.isWhitespace }
        if isCustomField || (firstWord.count > 1 && firstWord == firstWord.uppercased()) { return label }
        return label.prefix(1).lowercased() + label.dropFirst()
    }

    var body: some View {
        HStack(spacing: 0) {
            CopyFieldButton(label: actionLabel, value: isHidden ? "Hidden" : value, copy: copy) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(label)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                    Text(isHidden ? String(repeating: "•", count: 12) : value)
                        .font(isConcealed ? .body.monospaced() : .body)
                        .lineLimit(isConcealed ? 3 : 5)
                    if !warning.isEmpty {
                        Text(warning)
                            .font(.footnote)
                            .foregroundStyle(.themeWarning)
                            .accessibilityIdentifier("card-warning-\(label)")
                    }
                }
            }
            if let toggleReveal {
                Button(action: toggleReveal) {
                    Image(systemName: isRevealed ? "eye.slash" : "eye")
                        .contentTransition(.symbolEffect(.replace))
                        .iconButtonHitArea()
                        .frame(maxHeight: .infinity)
                        .contentShape(.rect)
                }
                .buttonStyle(.borderless)
                .accessibilityLabel(isRevealed ? "Hide \(actionLabel)" : "Reveal \(actionLabel)")
                .accessibilityIdentifier("reveal-\(label.lowercased())")
            }
            CopyIconButton(copy: copy)
        }
        .fixedSize(horizontal: false, vertical: true)
        .listRowInsets(EdgeInsets())
        .foregroundStyle(.primary)
        .contextMenu {
            Button(action: copy) {
                Label("Copy", systemImage: "doc.on.doc")
            }
            Button(action: copyReference) {
                Label("Copy secret reference", systemImage: "terminal")
            }
            if let toggleReveal {
                Button(action: toggleReveal) {
                    Label(isRevealed ? "Hide" : "Reveal", systemImage: isRevealed ? "eye.slash" : "eye")
                }
            }
        }
    }
}

/// A field row's label and value, which copy the field on a tap. Assistive technology reads them as the button
/// "Copy <label>" with the value, or "Hidden" for a secret that is not revealed.
struct CopyFieldButton<Content: View>: View {
    var label: String
    var value: String
    var identifier: String?
    var copy: () -> Void
    @ViewBuilder var content: Content

    var body: some View {
        Button(action: copy) {
            content
                .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                .padding(.horizontal, 20)
                .padding(.vertical, 8)
                .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .listRowInsets(EdgeInsets())
        .accessibilityLabel("Copy \(label)")
        .accessibilityValue(value)
        .accessibilityIdentifier(identifier ?? "copy-\(label.lowercased())")
    }
}

/// The copy icon at the end of a field row. The row's CopyFieldButton already copies for assistive technology.
struct CopyIconButton: View {
    var copy: () -> Void

    var body: some View {
        Button(action: copy) {
            Image(systemName: "doc.on.doc")
                .iconButtonHitArea()
                .padding(.trailing, 20)
                .frame(maxHeight: .infinity)
                .contentShape(.rect)
        }
        .buttonStyle(.borderless)
        .accessibilityHidden(true)
    }
}

extension View {
    /// An icon-only button's 44-point hit area, on its label: a borderless button takes taps only on its label.
    func iconButtonHitArea() -> some View {
        frame(minWidth: 44, minHeight: 44).contentShape(.rect)
    }
}

private struct MoveItemSheet: View {
    var sourceVaultID: String
    var item: ItemDetail
    var finish: (ItemDetail, String) -> Void
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var targetVaultID = ""

    private var groups: [VaultDestinationGroup] {
        OrganizationLogic.destinationGroups(model.moveDestinations(vaultID: sourceVaultID), organizations: model.allOrganization)
    }
    private var selected: VaultDestination? { groups.flatMap(\.destinations).first { $0.id == targetVaultID } }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Move “\(item.title)” from \(model.findVault(vaultID: sourceVaultID)?.name ?? "this vault").")
                }
                if let error = model.errorMessage {
                    Section { ErrorBanner(message: error) }
                }
                Picker("Move to", selection: $targetVaultID) {
                    Text("Choose a vault").tag("")
                    ForEach(groups) { group in
                        Section {
                            ForEach(group.destinations) { destination in
                                Text(destination.label).tag(destination.id)
                            }
                        } header: {
                            Text(group.name)
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                }
                .accessibilityIdentifier("move-item-target")
                if let selected {
                    Section {
                        Text(selected.context)
                        Text("Everyone with access to the destination gains access to this item. People with source access but no destination access lose it on their next sync. Copies they retained independently cannot be revoked.")
                        Text("hisar:// secret references that use the source vault's name need the new vault name.")
                        if item.isServer, item.server?.sshKeyID.isEmpty == false {
                            Text("The linked SSH key must exist in the destination vault or be selected again after moving.")
                        }
                    } header: {
                        Text("Who has access")
                            .foregroundStyle(.themeSecondary)
                    }
                    .accessibilityIdentifier("move-item-context")
                }
            }
            .navigationTitle("Move to vault")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(model.isBusy)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Move") {
                        Task {
                            if let moved = await model.moveItem(vaultID: sourceVaultID, itemID: item.id, targetVaultID: targetVaultID) {
                                finish(moved, targetVaultID)
                            }
                        }
                    }
                    .disabled(selected == nil || model.isBusy)
                    .accessibilityIdentifier("move-item-confirm")
                }
            }
        }
        .interactiveDismissDisabled(model.isBusy)
struct ItemDetailHeader: View {
    var item: ItemDetail
    var subtitle: String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout = dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 14))
            : AnyLayout(HStackLayout(spacing: 14))
        layout {
            AppItemIcon(item: item, size: 56, maxSize: 56)
                .dynamicTypeSize(.large)
            VStack(alignment: .leading, spacing: 2) {
                Text(item.title)
                    .font(.title2.weight(.bold))
                    .accessibilityIdentifier("item-detail-title")
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(.themeSecondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
