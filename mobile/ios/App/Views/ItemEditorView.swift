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
    @State private var isCardSecretRevealed = false
    @State private var cardDescription = CardDescription()
    @State private var isDocumentNumberRevealed = false
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
                } else if draft.isIdentity {
                    buildIdentitySections()
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
            .navigationTitle(navigationTitle)
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
        if draft.isIdentity, draft.identity == nil {
            draft.identity = ItemIdentity()
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

    private var navigationTitle: String {
        guard isNew else { return "Edit item" }
        if draft.isSecureNote { return "New secure note" }
        if draft.isCreditCard { return "New credit card" }
        if draft.isIdentity { return "New identity" }
        return "New login"
    }

    /// A binding to one of the identity's values, creating the identity on first write.
    private func bindIdentity(_ keyPath: WritableKeyPath<ItemIdentity, String>) -> Binding<String> {
        Binding(
            get: { draft.identity?[keyPath: keyPath] ?? "" },
            set: { value in
                var identity = draft.identity ?? ItemIdentity()
                identity[keyPath: keyPath] = value
                draft.identity = identity
            }
        )
    }

    /// Birth dates are stored as YYYY-MM-DD, a day without a time or time zone.
    private static let birthDateFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(identifier: "UTC")
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter
    }()

    /// A date picker for a stored YYYY-MM-DD birth date; an imported date in another form, such as 04/12/1990, stays text.
    @ViewBuilder
    private func buildBirthDateRow() -> some View {
        let birthDate = bindIdentity(\.birthDate)
        if birthDate.wrappedValue.isEmpty {
            Button {
                birthDate.wrappedValue = Self.birthDateFormatter.string(from: Date())
            } label: {
                Label("Add birth date", systemImage: "calendar.badge.plus")
            }
            .accessibilityIdentifier("editor-identity-add-birth-date")
        } else if let date = Self.birthDateFormatter.date(from: birthDate.wrappedValue) {
            HStack {
                DatePicker(
                    "Birth date",
                    selection: Binding(get: { date }, set: { birthDate.wrappedValue = Self.birthDateFormatter.string(from: $0) }),
                    in: ...Date(),
                    displayedComponents: .date
                )
                .environment(\.timeZone, TimeZone(identifier: "UTC") ?? .current)
                .accessibilityIdentifier("editor-identity-birth-date")
                Button(role: .destructive) {
                    birthDate.wrappedValue = ""
                } label: {
                    Image(systemName: "minus.circle")
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Remove birth date")
            }
        } else {
            TextField("Birth date", text: birthDate)
                .accessibilityIdentifier("editor-identity-birth-date")
        }
    }

    @ViewBuilder
    private func buildDocumentField(_ title: String, _ keyPath: WritableKeyPath<ItemIdentity, String>, identifier: String) -> some View {
        Group {
            if isDocumentNumberRevealed {
                TextField(title, text: bindIdentity(keyPath))
            } else {
                SecureField(title, text: bindIdentity(keyPath), prompt: Text(title).foregroundStyle(.themeSecondary))
            }
        }
        .font(.system(.body, design: .monospaced))
        // Without a content type, iOS takes several secure fields with a text keyboard for a sign-up form and fills them
        // with a generated strong password; a one-time code type opts out of that.
        .textContentType(.oneTimeCode)
        .textInputAutocapitalization(.characters)
        .autocorrectionDisabled()
        .accessibilityIdentifier(identifier)
    }

    @ViewBuilder
    private func buildIdentitySections() -> some View {
        Section("Name") {
            TextField("First name", text: bindIdentity(\.firstName))
                .textContentType(.givenName)
                .accessibilityIdentifier("editor-identity-first-name")
            TextField("Middle name", text: bindIdentity(\.middleName))
                .textContentType(.middleName)
                .accessibilityIdentifier("editor-identity-middle-name")
            TextField("Last name", text: bindIdentity(\.lastName))
                .textContentType(.familyName)
                .accessibilityIdentifier("editor-identity-last-name")
        }
        Section("Details") {
            buildBirthDateRow()
            TextField("Email", text: bindIdentity(\.email))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-identity-email")
            TextField("Phone", text: bindIdentity(\.phone))
                .keyboardType(.phonePad)
                .textContentType(.telephoneNumber)
                .accessibilityIdentifier("editor-identity-phone")
            TextField("Address", text: bindIdentity(\.address), axis: .vertical)
                .lineLimit(2...5)
                .textContentType(.fullStreetAddress)
                .accessibilityIdentifier("editor-identity-address")
        }
        Section("Documents") {
            buildDocumentField("Passport number", \.passportNumber, identifier: "editor-identity-passport-number")
            buildDocumentField("Driver's license number", \.driversLicenseNumber, identifier: "editor-identity-drivers-license-number")
            buildDocumentField("National ID number", \.nationalIDNumber, identifier: "editor-identity-national-id-number")
            Button {
                isDocumentNumberRevealed.toggle()
            } label: {
                Label(isDocumentNumberRevealed ? "Hide document numbers" : "Show document numbers", systemImage: isDocumentNumberRevealed ? "eye.slash" : "eye")
            }
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

    private func save() {
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
