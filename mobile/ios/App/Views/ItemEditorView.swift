import SwiftUI

struct ItemEditorView: View {
    var vaultID: String
    var originalItem: ItemDetail?
    /// The category of a new item; an existing item keeps its own.
    var newCategory: String = ItemDetail.categoryLogin
    var finish: (ItemDetail?) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var draft = ItemDetail()
    @State private var allURLText: [String] = [""]
    @State private var isPasswordRevealed = false
    @State private var isGeneratorShown = false
    @State private var isDeleteConfirmationShown = false
    @State private var hasLoadedDraft = false
    @State private var newTagText = ""
    @State private var isCardSecretRevealed = false
    @State private var cardDescription = CardDescription()
    /// The brand of the number last checked: the type follows it until the user picks a different one.
    @State private var numberBrand = ""

    private var isNew: Bool { originalItem == nil }

    var body: some View {
        NavigationStack {
            Form {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                Section {
                    TextField("Title", text: $draft.title)
                        .accessibilityIdentifier("editor-title")
                }
                if draft.isSecureNote {
                    Section("Note") {
                        TextField("Recovery codes, door codes, account details…", text: $draft.notes, axis: .vertical)
                            .lineLimit(8...30)
                            .accessibilityIdentifier("editor-notes")
                    }
                    buildFieldSection()
                } else if draft.isCreditCard {
                    buildCardSections()
                    buildFieldSection()
                } else if !draft.isSSHKey {
                    Section("Login") {
                        TextField("Username or email", text: $draft.username)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-username")
                        HStack {
                            Group {
                                if isPasswordRevealed {
                                    TextField("Password", text: $draft.password)
                                        .font(.system(.body, design: .monospaced))
                                } else {
                                    SecureField("Password", text: $draft.password, prompt: Text("Password").foregroundStyle(.themeSecondary))
                                }
                            }
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-password")
                            Button {
                                isPasswordRevealed.toggle()
                            } label: {
                                Image(systemName: isPasswordRevealed ? "eye.slash" : "eye")
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel(isPasswordRevealed ? "Hide password" : "Reveal password")
                            Button {
                                isGeneratorShown = true
                            } label: {
                                Image(systemName: "wand.and.stars")
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel("Generate password")
                            .accessibilityIdentifier("editor-generate-password")
                        }
                    }
                    Section {
                        TextField("Setup key or otpauth:// link", text: $draft.oneTimePassword)
                            .font(.system(.body, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-one-time-password")
                    } header: {
                        Text("One-time password")
                    } footer: {
                        Text("The setup key or link from the site's two-factor setup. Hisar then shows the current code.")
                    }
                    Section("Websites") {
                        ForEach(allURLText.indices, id: \.self) { index in
                            TextField("Website", text: $allURLText[index], prompt: Text(verbatim: "https://example.com"))
                                .keyboardType(.URL)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .accessibilityIdentifier("editor-website-\(index)")
                        }
                        .onDelete { allURLText.remove(atOffsets: $0) }
                        Button {
                            allURLText.append("")
                        } label: {
                            Label("Add website", systemImage: "plus")
                        }
                    }
                    buildFieldSection()
                }
                if !draft.isSecureNote {
                    Section("Notes") {
                        TextField("Notes", text: $draft.notes, axis: .vertical)
                            .lineLimit(3...10)
                            .accessibilityIdentifier("editor-notes")
                    }
                }
                buildTagSection()
                if !isNew {
                    Section {
                        Button("Delete item", role: .destructive) {
                            isDeleteConfirmationShown = true
                        }
                        .frame(maxWidth: .infinity)
                        .accessibilityIdentifier("editor-delete")
                    }
                }
            }
            .navigationTitle(isNew ? (draft.isSecureNote ? "New secure note" : (draft.isCreditCard ? "New credit card" : "New login")) : "Edit item")
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
                        Button("Save", action: save)
                            .disabled(draft.title.trimmingCharacters(in: .whitespaces).isEmpty)
                            .accessibilityIdentifier("editor-save")
                    }
                }
            }
            .confirmationDialog("Delete “\(draft.title)”?", isPresented: $isDeleteConfirmationShown, titleVisibility: .visible) {
                Button("Delete", role: .destructive, action: delete)
            } message: {
                Text("It is deleted on every device that can see this vault.")
            }
            .sheet(isPresented: $isGeneratorShown) {
                PasswordGeneratorSheet { password in
                    draft.password = password
                    isPasswordRevealed = true
                }
                .environmentObject(model)
            }
        }
        .interactiveDismissDisabled(model.isBusy)
        .onAppear(perform: loadDraft)
    }

    private func loadDraft() {
        guard !hasLoadedDraft else { return }
        hasLoadedDraft = true
        model.clearError()
        if let originalItem {
            draft = originalItem
            allURLText = originalItem.allURL.isEmpty ? [""] : originalItem.allURL
        } else {
            draft.category = newCategory
        }
        if draft.isCreditCard {
            let card = draft.card ?? ItemCard()
            draft.card = card
            cardDescription = CardDescription(brand: card.brand, last4: card.last4, numberWarning: card.numberWarning, expiryWarning: card.expiryWarning)
            numberBrand = card.brand
        }
    }

    /// The types the picker offers: the known brands, plus a type saved from elsewhere that is not one of them.
    private var allCardType: [String] {
        let cardType = draft.card?.type ?? ""
        return cardType.isEmpty || ItemCard.allBrand.contains(cardType) ? ItemCard.allBrand : ItemCard.allBrand + [cardType]
    }

    /// A type the user chose that the number does not match; the card still saves.
    private var cardTypeMismatch: String {
        let cardType = draft.card?.type ?? ""
        let brand = cardDescription.brand
        return !cardType.isEmpty && !brand.isEmpty && cardType != brand ? "The number looks like a \(brand) card" : ""
    }

    /// A binding to one of the card's values, creating the card on first write. The number and expiry are formatted as they
    /// are typed, the way Hisar saves them.
    private func bindCard(_ keyPath: WritableKeyPath<ItemCard, String>) -> Binding<String> {
        Binding(
            get: { draft.card?[keyPath: keyPath] ?? "" },
            set: { value in
                var card = draft.card ?? ItemCard()
                switch keyPath {
                case \ItemCard.number: card.number = ItemCard.formatNumberInput(value)
                case \ItemCard.expiry: card.expiry = ItemCard.formatExpiryInput(value)
                default: card[keyPath: keyPath] = value
                }
                draft.card = card
            }
        )
    }

    @ViewBuilder
    private func buildCardField(_ title: String, _ keyPath: WritableKeyPath<ItemCard, String>, identifier: String, isSecret: Bool) -> some View {
        Group {
            if isSecret && !isCardSecretRevealed {
                SecureField(title, text: bindCard(keyPath), prompt: Text(title).foregroundStyle(.themeSecondary))
            } else {
                TextField(title, text: bindCard(keyPath))
            }
        }
        .font(isSecret ? .system(.body, design: .monospaced) : .body)
        .keyboardType(isSecret ? .numberPad : .default)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .accessibilityIdentifier(identifier)
    }

    @ViewBuilder
    private func buildCardSections() -> some View {
        Section {
            TextField("Name on the card", text: bindCard(\.cardholder))
                .textContentType(.name)
                .accessibilityIdentifier("editor-cardholder")
            buildCardField("Card number", \.number, identifier: "editor-card-number", isSecret: true)
            Picker("Type", selection: bindCard(\.type)) {
                Text("Not set").tag("")
                ForEach(allCardType, id: \.self) { cardType in
                    Text(cardType).tag(cardType)
                }
            }
            .accessibilityIdentifier("editor-card-type")
            TextField("Expiry (MM/YY)", text: bindCard(\.expiry))
                .keyboardType(.numberPad)
                .accessibilityIdentifier("editor-card-expiry")
            buildCardField("Security code", \.securityCode, identifier: "editor-card-security-code", isSecret: true)
            buildCardField("PIN", \.pin, identifier: "editor-card-pin", isSecret: true)
            Button {
                isCardSecretRevealed.toggle()
            } label: {
                Label(isCardSecretRevealed ? "Hide number and codes" : "Show number and codes", systemImage: isCardSecretRevealed ? "eye.slash" : "eye")
            }
        } header: {
            Text("Card")
        } footer: {
            let allWarning = [cardDescription.numberWarning, cardTypeMismatch, cardDescription.expiryWarning].filter { !$0.isEmpty }
            if !allWarning.isEmpty {
                Text(allWarning.joined(separator: "\n"))
                    .foregroundStyle(.orange)
                    .accessibilityIdentifier("editor-card-warning")
            }
        }
        .task(id: "\(draft.card?.number ?? "")|\(draft.card?.expiry ?? "")") {
            try? await Task.sleep(for: .milliseconds(250))
            guard !Task.isCancelled, let description = await model.describeCard(number: draft.card?.number ?? "", expiry: draft.card?.expiry ?? "") else { return }
            cardDescription = description
            if var card = draft.card, card.type.isEmpty || card.type == numberBrand {
                card.type = description.brand
                draft.card = card
            }
            numberBrand = description.brand
        }
        Section("Billing address") {
            TextField("Billing address", text: bindCard(\.billingAddress), axis: .vertical)
                .lineLimit(2...5)
                .textContentType(.fullStreetAddress)
                .accessibilityIdentifier("editor-card-billing-address")
        }
    }

    private func buildFieldSection() -> some View {
        Section("Other fields") {
            ForEach(draft.allField.indices, id: \.self) { index in
                VStack(alignment: .leading, spacing: 8) {
                    TextField("Name", text: $draft.allField[index].name)
                        .font(.caption)
                    if draft.allField[index].concealed {
                        SecureField("Value", text: $draft.allField[index].value)
                    } else {
                        TextField("Value", text: $draft.allField[index].value)
                    }
                    Toggle("Secret", isOn: $draft.allField[index].concealed)
                        .font(.caption)
                        .foregroundStyle(.themeSecondary)
                }
            }
            .onDelete { draft.allField.remove(atOffsets: $0) }
            Button {
                draft.allField.append(ItemField(name: "", value: "", concealed: false))
            } label: {
                Label("Add field", systemImage: "plus")
            }
        }
    }

    /// One row per tag, removed by swiping, and a field that adds what is typed on return; commas separate several tags.
    private func buildTagSection() -> some View {
        Section {
            ForEach(draft.allTag, id: \.self) { tag in
                Label(tag, systemImage: "tag")
                    .accessibilityIdentifier("editor-tag-\(tag)")
            }
            .onDelete { draft.allTag.remove(atOffsets: $0) }
            TextField("Add a tag", text: $newTagText)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.done)
                .onSubmit(addTags)
                .accessibilityIdentifier("editor-new-tag")
        } header: {
            Text("Tags")
        } footer: {
            if draft.allTag.isEmpty {
                Text("Tags group items across vaults; the Vaults screen lists them.")
            }
        }
    }

    private func addTags() {
        for tag in ItemDetail.splitTags(newTagText) where !draft.allTag.contains(tag) {
            draft.allTag.append(tag)
        }
        newTagText = ""
    }

    private func save() {
        addTags()
        var item = draft
        item.allURL = allURLText.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        Task {
            if let savedItem = await model.saveItem(vaultID: vaultID, item: item) {
                finish(savedItem)
                dismiss()
            }
        }
    }

    private func delete() {
        guard let originalItem else { return }
        Task {
            if await model.deleteItem(vaultID: vaultID, itemID: originalItem.id) {
                dismiss()
                finish(nil)
            }
        }
    }
}
