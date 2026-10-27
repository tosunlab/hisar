import SwiftUI

struct ItemEditorView: View {
    var vaultID: String
    var originalItem: ItemDetail?
    /// The category of a new item; an existing item keeps its own.
    var newCategory: String = ItemDetail.categoryLogin
    /// The saved item, or nil after a deletion, and the vault it is in.
    var finish: (ItemDetail?, _ vaultID: String) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var draft = ItemDetail()
    @State private var allURLText: [String] = [""]
    /// A new item's vault: the one the editor opened from, until the user picks another.
    @State private var chosenVaultID = ""
    /// The draft as loaded, so Cancel and a swipe down know whether there is anything to discard.
    @State private var loadedItem = ItemDetail()
    @State private var isDiscardConfirmationShown = false
    @State private var isPasswordRevealed = false
    @State private var revealedCustomFields = Set<Int>()
    @State private var isGeneratorShown = false
    @State private var isIconPickerShown = false
    @State private var isDeleteConfirmationShown = false
    @State private var hasLoadedDraft = false
    @State private var newTagText = ""
    @State private var isCardSecretRevealed = false
    @State private var cardDescription = CardDescription()
    @State private var isDocumentNumberRevealed = false
    @State private var isCredentialSecretRevealed = false
    @State private var isLicenseKeyRevealed = false
    /// An identity's or card's one-line address saved before it had address fields stays on screen, even when emptied,
    /// until the item is saved.
    @State private var isOldAddressShown = false
    /// The brand of the number last checked: the type follows it until the user picks a different one.
    @State private var numberBrand = ""
    @State private var saveError: String?
    @State private var saveFailureCount = 0
    private enum SaveErrorFocus: Hashable { case summary, oneTimeCode }
    @AccessibilityFocusState private var focusedSaveError: SaveErrorFocus?

    private var oneTimeCodeError: String? {
        saveError == "The one-time code is not a valid secret or otpauth:// link" ? saveError : nil
    }

    private var isNew: Bool { originalItem == nil }

    private var hasChanges: Bool {
        hasLoadedDraft && (buildIconPreviewItem() != loadedItem || !newTagText.isEmpty)
    }

    private var allWritableVault: [VaultSummary] { model.allVault.filter(AppModel.isWritableVault) }
    private var destinationGroups: [VaultDestinationGroup] { OrganizationLogic.destinationGroups(allWritableVault, organizations: model.allOrganization) }

    private var credentialExpiryWarning: String {
        ItemAPICredential.describeEditedExpiry(draft.apiCredential?.expiry ?? "", saved: originalItem?.apiCredential)
    }

    var body: some View {
        NavigationStack {
            ScrollViewReader { proxy in
            Form {
                if let errorMessage = saveError ?? model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                            .id("editor-save-error")
                            .accessibilityFocused($focusedSaveError, equals: .summary)
                    }
                }
                Section {
                    HStack(spacing: 12) {
                        Button {
                            isIconPickerShown = true
                        } label: {
                            AppItemIcon(item: buildIconPreviewItem(), size: 40)
                                .overlay(alignment: .bottomTrailing) {
                                    Image(systemName: "pencil.circle.fill")
                                        .font(.caption)
                                        .foregroundStyle(.themeSecondary, .background)
                                        .offset(x: 4, y: 4)
                                }
                                .iconButtonHitArea()
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("Change icon")
                        .accessibilityIdentifier("editor-icon")
                        TextField("Title", text: $draft.title)
                            .accessibilityIdentifier("editor-title")
                            .editorFieldLabel("Title")
                    }
                    if isNew {
                        Picker("Vault", selection: $chosenVaultID) {
                            ForEach(destinationGroups) { group in
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
                        .accessibilityIdentifier("editor-vault")
                        if let destination = destinationGroups.flatMap(\.destinations).first(where: { $0.id == chosenVaultID }) {
                            Text(destination.context)
                                .font(.footnote)
                                .foregroundStyle(.themeSecondary)
                                .accessibilityIdentifier("editor-vault-context")
                        }
                    }
                }
                if draft.isSecureNote {
                    Section {
                        TextField("Recovery codes, door codes, account details…", text: $draft.notes, axis: .vertical)
                            .lineLimit(8...30)
                            .accessibilityIdentifier("editor-notes")
                            .editorFieldLabel("Note")
                    } header: {
                        Text("Note")
                            .foregroundStyle(.themeSecondary)
                    }
                    buildFieldSection()
                } else if draft.isCreditCard {
                    buildCardSections()
                    buildFieldSection()
                } else if draft.isIdentity {
                    buildIdentitySections()
                    buildFieldSection()
                } else if draft.isAPICredential {
                    buildAPICredentialSections()
                    buildFieldSection()
                } else if draft.isAddress {
                    buildAddressSection("Address", bindAddress)
                    buildFieldSection()
                } else if draft.isSoftwareLicense {
                    buildSoftwareLicenseSection()
                    buildFieldSection()
                } else if !draft.isSSHKey {
                    if draft.isServer {
                        buildServerSection()
                    }
                    if draft.isDatabase {
                        buildDatabaseSection()
                    }
                    Section {
                        TextField("Username or email", text: $draft.username)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            // Without content types iOS takes the username and password for a sign-in and, seconds after the
                            // editor closes, offers to save them in Apple's Passwords. A one-time code type on both opts out.
                            .textContentType(.oneTimeCode)
                            .accessibilityIdentifier("editor-username")
                            .editorFieldLabel("Username or email")
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
                            .textContentType(.oneTimeCode)
                            .accessibilityIdentifier("editor-password")
                            .editorFieldLabel("Password")
                            Button {
                                isPasswordRevealed.toggle()
                            } label: {
                                Image(systemName: isPasswordRevealed ? "eye.slash" : "eye")
                                    .iconButtonHitArea()
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel(isPasswordRevealed ? "Hide password" : "Reveal password")
                            Button {
                                isGeneratorShown = true
                            } label: {
                                Image(systemName: "wand.and.stars")
                                    .iconButtonHitArea()
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel("Generate password")
                            .accessibilityIdentifier("editor-generate-password")
                        }
                        ItemPasswordStrengthMeter(password: draft.password)
                    } header: {
                        Text("Login")
                            .foregroundStyle(.themeSecondary)
                    }
                    if !draft.isDatabase {
                        Section {
                            TextField("Setup key or otpauth:// link", text: $draft.oneTimePassword)
                                .font(.system(.body, design: .monospaced))
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .accessibilityIdentifier("editor-one-time-password")
                                .accessibilityHint(oneTimeCodeError ?? "")
                                .editorFieldLabel("Setup key or otpauth:// link")
                            if let oneTimeCodeError {
                                Text(oneTimeCodeError)
                                    .foregroundStyle(.red)
                                    .id("editor-one-time-code-error")
                                    .accessibilityIdentifier("editor-one-time-code-error")
                                    .accessibilityFocused($focusedSaveError, equals: .oneTimeCode)
                            }
                        } header: {
                            Text("One-time code")
                                .foregroundStyle(.themeSecondary)
                        }
                        Section {
                            ForEach(allURLText.indices, id: \.self) { index in
                                TextField(draft.isServer ? "Admin console URL" : "Website", text: $allURLText[index], prompt: Text(verbatim: draft.isServer ? "https://console.example.com" : "https://example.com"))
                                    .keyboardType(.URL)
                                    .textInputAutocapitalization(.never)
                                    .autocorrectionDisabled()
                                    .accessibilityIdentifier("editor-website-\(index)")
                                    .editorFieldLabel(draft.isServer ? "Admin console URL" : "Website")
                            }
                            .onDelete { allURLText.remove(atOffsets: $0) }
                            Button {
                                allURLText.append("")
                            } label: {
                                Label("Add website", systemImage: "plus")
                            }
                        } header: {
                            Text(draft.isServer ? "Admin console" : "Websites")
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                    buildFieldSection()
                }
                if !draft.isSecureNote {
                    Section {
                        TextField("Notes", text: $draft.notes, axis: .vertical)
                            .lineLimit(3...10)
                            .accessibilityIdentifier("editor-notes")
                            .editorFieldLabel("Notes")
                    } header: {
                        Text("Notes")
                            .foregroundStyle(.themeSecondary)
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
            .navigationTitle(navigationTitle)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: cancel)
                        .accessibilityIdentifier("editor-cancel")
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
                Button("Delete item", role: .destructive, action: delete)
            } message: {
                Text(VaultSummary.itemDeletionMessage(model.findVault(vaultID: vaultID)))
            }
            .confirmationDialog("Discard changes?", isPresented: $isDiscardConfirmationShown, titleVisibility: .visible) {
                Button("Discard", role: .destructive, action: close)
                    .accessibilityIdentifier("editor-discard")
                Button("Keep editing", role: .cancel) {}
                    .accessibilityIdentifier("editor-keep-editing")
            }
            .background(SheetDismissAttemptObserver { isDiscardConfirmationShown = true })
            .sheet(isPresented: $isGeneratorShown) {
                PasswordGeneratorSheet { password in
                    draft.password = password
                    isPasswordRevealed = true
                }
                .environmentObject(model)
            }
            // Outside the Form: a sheet attached to a row inside it is rebuilt whenever the model or the draft changes, which
            // resets the picker's color and drops a tap in flight.
            .sheet(isPresented: $isIconPickerShown) {
                IconPickerView(icon: $draft.icon, item: buildIconPreviewItem())
            }
            .onChange(of: saveFailureCount) { _, _ in
                withAnimation { proxy.scrollTo(oneTimeCodeError == nil ? "editor-save-error" : "editor-one-time-code-error", anchor: .center) }
                focusedSaveError = oneTimeCodeError == nil ? .summary : .oneTimeCode
            }
            }
        }
        .interactiveDismissDisabled(model.isBusy || hasChanges)
        .onAppear(perform: loadDraft)
    }

    private func cancel() {
        if hasChanges {
            isDiscardConfirmationShown = true
        } else {
            close()
        }
    }

    private func close() {
        model.clearError()
        dismiss()
    }

    private func loadDraft() {
        guard !hasLoadedDraft else { return }
        hasLoadedDraft = true
        model.clearError()
        chosenVaultID = vaultID
        if let originalItem {
            draft = originalItem
            allURLText = originalItem.allURL.isEmpty ? [""] : originalItem.allURL
        } else {
            draft.category = newCategory
        }
        if draft.isIdentity, draft.identity == nil {
            draft.identity = ItemIdentity()
        }
        if draft.isAPICredential, draft.apiCredential == nil {
            draft.apiCredential = ItemAPICredential()
        }
        if draft.isAddress, draft.address == nil {
            draft.address = ItemAddress()
        }
        if draft.isSoftwareLicense, draft.softwareLicense == nil {
            draft.softwareLicense = ItemSoftwareLicense()
        }
        if draft.isServer, draft.server == nil {
            draft.server = ItemServer()
        }
        if draft.isDatabase, draft.database == nil {
            draft.database = ItemDatabase(engine: "postgresql", engineName: "PostgreSQL")
        }
        isOldAddressShown = !(draft.identity?.address ?? "").isEmpty || !(draft.card?.billingAddress ?? "").isEmpty
        if draft.isCreditCard {
            let card = draft.card ?? ItemCard()
            draft.card = card
            cardDescription = CardDescription(brand: card.brand, last4: card.last4, numberWarning: card.numberWarning, expiryWarning: card.expiryWarning)
            numberBrand = card.brand
        }
        loadedItem = buildIconPreviewItem()
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
                case \ItemCard.number: card.number = CardInput.formatNumber(value)
                case \ItemCard.expiry: card.expiry = CardInput.formatExpiry(value)
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
        .editorFieldLabel(title)
    }

    @ViewBuilder
    private func buildCardSections() -> some View {
        Section {
            TextField("Name on the card", text: bindCard(\.cardholder))
                .textContentType(.name)
                .accessibilityIdentifier("editor-cardholder")
                .editorFieldLabel("Name on the card")
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
                .editorFieldLabel("Expiry (MM/YY)")
            buildCardField("Security code", \.securityCode, identifier: "editor-card-security-code", isSecret: true)
            buildCardField("PIN", \.pin, identifier: "editor-card-pin", isSecret: true)
            Button {
                isCardSecretRevealed.toggle()
            } label: {
                Label(isCardSecretRevealed ? "Hide number and codes" : "Show number and codes", systemImage: isCardSecretRevealed ? "eye.slash" : "eye")
            }
        } header: {
            Text("Card")
                .foregroundStyle(.themeSecondary)
        } footer: {
            let allWarning = [cardDescription.numberWarning, cardTypeMismatch, cardDescription.expiryWarning].filter { !$0.isEmpty }
            if !allWarning.isEmpty {
                Text(allWarning.joined(separator: "\n"))
                    .foregroundStyle(.themeWarning)
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
        buildAddressSection("Billing address", { bindCard((\ItemCard.postalAddress).appending(path: $0)) }, oldText: bindCard(\.billingAddress))
    }

    private var navigationTitle: String {
        guard isNew else { return "Edit item" }
        if draft.isSecureNote { return "New secure note" }
        if draft.isCreditCard { return "New credit card" }
        if draft.isIdentity { return "New identity" }
        if draft.isAPICredential { return "New API credential" }
        if draft.isAddress { return "New address" }
        if draft.isSoftwareLicense { return "New software license" }
        if draft.isServer { return "New server" }
        if draft.isDatabase { return "New database" }
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

    /// A date picker for a stored YYYY-MM-DD date, such as a birth date or an expiry; an imported date in another form, such
    /// as 04/12/1990, or a day that does not exist, such as 2023-02-31, stays text. A birth date cannot be in the future.
    @ViewBuilder
    private func buildDateRow(_ title: String, _ value: Binding<String>, identifier: String, isFutureAllowed: Bool) -> some View {
        if value.wrappedValue.isEmpty {
            Button {
                value.wrappedValue = CalendarDate.formatToday()
            } label: {
                Label("Add \(title.lowercased())", systemImage: "calendar.badge.plus")
            }
            .accessibilityIdentifier("\(identifier)-add")
        } else if let date = CalendarDate.parse(value.wrappedValue) {
            HStack {
                let selection = Binding(get: { date }, set: { value.wrappedValue = CalendarDate.format($0) })
                Group {
                    if isFutureAllowed {
                        DatePicker(title, selection: selection, displayedComponents: .date)
                    } else {
                        DatePicker(title, selection: selection, in: ...CalendarDate.getToday(), displayedComponents: .date)
                    }
                }
                .environment(\.timeZone, TimeZone(identifier: "UTC") ?? .current)
                .accessibilityIdentifier(identifier)
                Button(role: .destructive) {
                    value.wrappedValue = ""
                } label: {
                    Image(systemName: "minus.circle")
                        .iconButtonHitArea()
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Remove \(title.lowercased())")
            }
        } else {
            TextField(title, text: value)
                .accessibilityIdentifier(identifier)
                .editorFieldLabel(title)
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
        .editorFieldLabel(title)
    }

    @ViewBuilder
    private func buildIdentitySections() -> some View {
        Section {
            TextField("First name", text: bindIdentity(\.firstName))
                .textContentType(.givenName)
                .accessibilityIdentifier("editor-identity-first-name")
                .editorFieldLabel("First name")
            TextField("Middle name", text: bindIdentity(\.middleName))
                .textContentType(.middleName)
                .accessibilityIdentifier("editor-identity-middle-name")
                .editorFieldLabel("Middle name")
            TextField("Last name", text: bindIdentity(\.lastName))
                .textContentType(.familyName)
                .accessibilityIdentifier("editor-identity-last-name")
                .editorFieldLabel("Last name")
        } header: {
            Text("Name")
                .foregroundStyle(.themeSecondary)
        }
        Section {
            buildDateRow("Birth date", bindIdentity(\.birthDate), identifier: "editor-identity-birth-date", isFutureAllowed: false)
            TextField("Email", text: bindIdentity(\.email))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-identity-email")
                .editorFieldLabel("Email")
            TextField("Phone", text: bindIdentity(\.phone))
                .keyboardType(.phonePad)
                .textContentType(.telephoneNumber)
                .accessibilityIdentifier("editor-identity-phone")
                .editorFieldLabel("Phone")
        } header: {
            Text("Details")
                .foregroundStyle(.themeSecondary)
        }
        buildAddressSection("Address", { bindIdentity((\ItemIdentity.postalAddress).appending(path: $0)) }, oldText: bindIdentity(\.address))
        Section {
            buildDocumentField("Passport number", \.passportNumber, identifier: "editor-identity-passport-number")
            buildDocumentField("Driver's license number", \.driversLicenseNumber, identifier: "editor-identity-drivers-license-number")
            buildDocumentField("National ID number", \.nationalIDNumber, identifier: "editor-identity-national-id-number")
            Button {
                isDocumentNumberRevealed.toggle()
            } label: {
                Label(isDocumentNumberRevealed ? "Hide document numbers" : "Show document numbers", systemImage: isDocumentNumberRevealed ? "eye.slash" : "eye")
            }
        } header: {
            Text("Documents")
                .foregroundStyle(.themeSecondary)
        }
    }

    /// A binding to one of the address item's values, creating the address on first write.
    private func bindAddress(_ keyPath: WritableKeyPath<ItemAddress, String>) -> Binding<String> {
        Binding(
            get: { draft.address?[keyPath: keyPath] ?? "" },
            set: { value in
                var address = draft.address ?? ItemAddress()
                address[keyPath: keyPath] = value
                draft.address = address
            }
        )
    }

    /// The address fields of an address item, an identity or a card; bind gives each value's binding. oldText is the
    /// one-line address an identity or card saved before it had these fields, shown only when the item has one, with a
    /// button that moves it into the street.
    @ViewBuilder
    private func buildAddressSection(_ title: String, _ bind: @escaping (WritableKeyPath<ItemAddress, String>) -> Binding<String>, oldText: Binding<String>? = nil) -> some View {
        Section {
            TextField("Street", text: bind(\.street), axis: .vertical)
                .lineLimit(1...4)
                .textContentType(.fullStreetAddress)
                .accessibilityIdentifier("editor-address-street")
                .editorFieldLabel("Street")
            TextField("City", text: bind(\.city))
                .textContentType(.addressCity)
                .accessibilityIdentifier("editor-address-city")
                .editorFieldLabel("City")
            TextField("Region", text: bind(\.region), prompt: Text("State or province").foregroundStyle(.themeSecondary))
                .textContentType(.addressState)
                .accessibilityIdentifier("editor-address-region")
                .editorFieldLabel("Region")
            TextField("Postal code", text: bind(\.postalCode))
                .textContentType(.postalCode)
                .textInputAutocapitalization(.characters)
                .accessibilityIdentifier("editor-address-postal-code")
                .editorFieldLabel("Postal code")
            TextField("Country", text: bind(\.country))
                .textContentType(.countryName)
                .accessibilityIdentifier("editor-address-country")
                .editorFieldLabel("Country")
            if let oldText, isOldAddressShown {
                VStack(alignment: .leading, spacing: 6) {
                    Text("\(title) (old format)")
                        .font(.caption)
                        .foregroundStyle(.themeSecondary)
                    TextField("\(title) (old format)", text: oldText, axis: .vertical)
                        .lineLimit(2...5)
                        .accessibilityIdentifier("editor-old-address")
                    Button {
                        let street = bind(\.street)
                        street.wrappedValue = [street.wrappedValue, oldText.wrappedValue].filter { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }.joined(separator: "\n")
                        oldText.wrappedValue = ""
                        isOldAddressShown = false
                    } label: {
                        Label("Use as street", systemImage: "arrow.up.doc")
                    }
                    .buttonStyle(.borderless)
                    .font(.caption)
                    .accessibilityIdentifier("editor-use-as-street")
                }
            }
        } header: {
            Text(title)
                .foregroundStyle(.themeSecondary)
        }
    }

    /// A binding to one of the API credential's values, creating the credential on first write.
    private func bindAPICredential(_ keyPath: WritableKeyPath<ItemAPICredential, String>) -> Binding<String> {
        Binding(
            get: { draft.apiCredential?[keyPath: keyPath] ?? "" },
            set: { value in
                var credential = draft.apiCredential ?? ItemAPICredential()
                credential[keyPath: keyPath] = value
                draft.apiCredential = credential
            }
        )
    }

    @ViewBuilder
    private func buildAPICredentialSections() -> some View {
        Section {
            TextField("Service", text: bindAPICredential(\.service))
                .accessibilityIdentifier("editor-api-credential-service")
                .editorFieldLabel("Service")
            TextField("Key ID", text: bindAPICredential(\.keyID))
                .font(.system(.body, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-api-credential-key-id")
                .editorFieldLabel("Key ID")
            HStack {
                Group {
                    if isCredentialSecretRevealed {
                        TextField("Secret", text: bindAPICredential(\.secret))
                    } else {
                        SecureField("Secret", text: bindAPICredential(\.secret), prompt: Text("Secret").foregroundStyle(.themeSecondary))
                    }
                }
                .font(.system(.body, design: .monospaced))
                // As for document numbers: without a content type iOS may fill a secure field with a strong password.
                .textContentType(.oneTimeCode)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-api-credential-secret")
                .editorFieldLabel("Secret")
                Button {
                    isCredentialSecretRevealed.toggle()
                } label: {
                    Image(systemName: isCredentialSecretRevealed ? "eye.slash" : "eye")
                        .iconButtonHitArea()
                }
                .buttonStyle(.borderless)
                .foregroundStyle(.themeSecondary)
                .accessibilityLabel(isCredentialSecretRevealed ? "Hide secret" : "Reveal secret")
            }
            TextField("Host", text: bindAPICredential(\.host), prompt: Text(verbatim: "api.example.com"))
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-api-credential-host")
                .editorFieldLabel("Host")
            buildDateRow("Expiry", bindAPICredential(\.expiry), identifier: "editor-api-credential-expiry", isFutureAllowed: true)
        } header: {
            Text("Credential")
                .foregroundStyle(.themeSecondary)
        } footer: {
            if !credentialExpiryWarning.isEmpty {
                Text(credentialExpiryWarning)
                    .foregroundStyle(.themeWarning)
                    .accessibilityIdentifier("editor-api-credential-warning")
            }
        }
    }

    /// A binding to one of the server's values, creating the server on first write.
    private func bindServer(_ keyPath: WritableKeyPath<ItemServer, String>) -> Binding<String> {
        Binding(
            get: { draft.server?[keyPath: keyPath] ?? "" },
            set: { value in
                var server = draft.server ?? ItemServer()
                server[keyPath: keyPath] = value
                draft.server = server
            }
        )
    }

    /// The SSH keys a server may use: its vault's, so everyone who sees the server can use the key.
    private var allVaultSSHKey: [ItemDetail] {
        model.listAllItemReference(vaultID: chosenVaultID).map(\.item).filter(\.isSSHKey)
    }

    @ViewBuilder
    private func buildServerSection() -> some View {
        let sshKeyID = draft.server?.sshKeyID ?? ""
        let portWarning = ItemServer.describePort(draft.server?.port ?? "")
        Section {
            TextField("Host", text: bindServer(\.host), prompt: Text(verbatim: "server.example.com"))
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-server-host")
                .editorFieldLabel("Host")
            TextField("Port", text: bindServer(\.port), prompt: Text(verbatim: "22"))
                .keyboardType(.numberPad)
                .accessibilityIdentifier("editor-server-port")
                .editorFieldLabel("Port")
            Picker("SSH key", selection: bindServer(\.sshKeyID)) {
                Text("None").tag("")
                if !sshKeyID.isEmpty && !allVaultSSHKey.contains(where: { $0.id == sshKeyID }) {
                    Text("Missing SSH key").tag(sshKeyID)
                }
                ForEach(allVaultSSHKey) { key in
                    Text(key.title).tag(key.id)
                }
            }
            .accessibilityIdentifier("editor-server-ssh-key")
        } header: {
            Text("Server")
                .foregroundStyle(.themeSecondary)
        } footer: {
            if !portWarning.isEmpty {
                Text(portWarning)
                    .foregroundStyle(.themeWarning)
                    .accessibilityIdentifier("editor-server-port-warning")
            }
        }
    }

    /// A binding to one of the database's values, creating the database on first write.
    private func bindDatabase(_ keyPath: WritableKeyPath<ItemDatabase, String>) -> Binding<String> {
        Binding(
            get: { draft.database?[keyPath: keyPath] ?? "" },
            set: { value in
                var database = draft.database ?? ItemDatabase()
                database[keyPath: keyPath] = value
                draft.database = database
            }
        )
    }

    /// The engine picker's choice: a known engine's ID, or "other" for any other engine, whose text is edited below it.
    private var databaseEngineChoice: Binding<String> {
        Binding(
            get: {
                let engine = draft.database?.engine ?? ""
                return DatabaseEngine.isKnown(engine) ? engine : "other"
            },
            set: { choice in
                let engine = draft.database?.engine ?? ""
                if choice != "other" {
                    bindDatabase(\.engine).wrappedValue = choice
                } else if DatabaseEngine.isKnown(engine) {
                    bindDatabase(\.engine).wrappedValue = ""
                }
            }
        )
    }

    @ViewBuilder
    private func buildDatabaseSection() -> some View {
        let isOtherEngine = !DatabaseEngine.isKnown(draft.database?.engine ?? "")
        let portWarning = ItemServer.describePort(draft.database?.port ?? "")
        Section {
            Picker("Engine", selection: databaseEngineChoice) {
                ForEach(DatabaseEngine.allKnown, id: \.id) { engine in
                    Text(engine.name).tag(engine.id)
                }
                Text("Other").tag("other")
            }
            .accessibilityIdentifier("editor-database-engine")
            if isOtherEngine {
                TextField("Other engine", text: bindDatabase(\.engine), prompt: Text(verbatim: "DB2"))
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("editor-database-other-engine")
                    .editorFieldLabel("Other engine")
            }
            TextField("Host", text: bindDatabase(\.host), prompt: Text(verbatim: "db.example.com"))
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-database-host")
                .editorFieldLabel("Host")
            TextField("Port", text: bindDatabase(\.port), prompt: Text(verbatim: "5432"))
                .keyboardType(.numberPad)
                .accessibilityIdentifier("editor-database-port")
                .editorFieldLabel("Port")
            TextField("Database", text: bindDatabase(\.name), prompt: Text(verbatim: "app"))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-database-name")
                .editorFieldLabel("Database")
            TextField("Options", text: bindDatabase(\.options), prompt: Text(verbatim: "sslmode=require"))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-database-options")
                .editorFieldLabel("Options")
        } header: {
            Text("Database")
                .foregroundStyle(.themeSecondary)
        } footer: {
            if !portWarning.isEmpty {
                Text(portWarning)
                    .foregroundStyle(.themeWarning)
                    .accessibilityIdentifier("editor-database-port-warning")
            }
            if isOtherEngine {
                Text("Hisar builds connection strings only for the engines listed.")
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    /// A binding to one of the software license's values, creating the license on first write.
    private func bindSoftwareLicense(_ keyPath: WritableKeyPath<ItemSoftwareLicense, String>) -> Binding<String> {
        Binding(
            get: { draft.softwareLicense?[keyPath: keyPath] ?? "" },
            set: { value in
                var license = draft.softwareLicense ?? ItemSoftwareLicense()
                license[keyPath: keyPath] = value
                draft.softwareLicense = license
            }
        )
    }

    @ViewBuilder
    private func buildSoftwareLicenseSection() -> some View {
        Section {
            TextField("Product", text: bindSoftwareLicense(\.product))
                .accessibilityIdentifier("editor-license-product")
                .editorFieldLabel("Product")
            TextField("Version", text: bindSoftwareLicense(\.version))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-version")
                .editorFieldLabel("Version")
            HStack {
                Group {
                    if isLicenseKeyRevealed {
                        TextField("License key", text: bindSoftwareLicense(\.licenseKey))
                    } else {
                        SecureField("License key", text: bindSoftwareLicense(\.licenseKey), prompt: Text("License key").foregroundStyle(.themeSecondary))
                    }
                }
                .font(.system(.body, design: .monospaced))
                // As for an API credential's secret: without a content type iOS may fill a secure field with a strong password.
                .textContentType(.oneTimeCode)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-key")
                .editorFieldLabel("License key")
                Button {
                    isLicenseKeyRevealed.toggle()
                } label: {
                    Image(systemName: isLicenseKeyRevealed ? "eye.slash" : "eye")
                        .iconButtonHitArea()
                }
                .buttonStyle(.borderless)
                .foregroundStyle(.themeSecondary)
                .accessibilityLabel(isLicenseKeyRevealed ? "Hide license key" : "Reveal license key")
            }
            TextField("Licensed to", text: bindSoftwareLicense(\.licensedTo))
                .textContentType(.name)
                .accessibilityIdentifier("editor-license-licensed-to")
                .editorFieldLabel("Licensed to")
            TextField("Email", text: bindSoftwareLicense(\.email))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-email")
                .editorFieldLabel("Email")
            buildDateRow("Purchase date", bindSoftwareLicense(\.purchaseDate), identifier: "editor-license-purchase-date", isFutureAllowed: false)
            TextField("Download URL", text: bindSoftwareLicense(\.downloadURL), prompt: Text(verbatim: "https://example.com/download"))
                .keyboardType(.URL)
                .textContentType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-download-url")
                .editorFieldLabel("Download URL")
        } header: {
            Text("License")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func buildFieldSection() -> some View {
        Section {
            ForEach(draft.allField.indices, id: \.self) { index in
                VStack(alignment: .leading, spacing: 8) {
                    TextField("Name", text: $draft.allField[index].name)
                        .accessibilityIdentifier("editor-field-\(index)-name")
                        .editorFieldLabel("Name")
                    HStack {
                        if draft.allField[index].concealed && !revealedCustomFields.contains(index) {
                            SecureField("Value", text: $draft.allField[index].value)
                                .accessibilityLabel(draft.allField[index].name.isEmpty ? "Value" : draft.allField[index].name)
                                .accessibilityIdentifier("editor-field-\(index)-value")
                                .editorFieldLabel("Value")
                        } else {
                            TextField("Value", text: $draft.allField[index].value)
                                .accessibilityLabel(draft.allField[index].name.isEmpty ? "Value" : draft.allField[index].name)
                                .accessibilityIdentifier("editor-field-\(index)-value")
                                .editorFieldLabel("Value")
                        }
                        if draft.allField[index].concealed {
                            Button {
                                if revealedCustomFields.contains(index) { revealedCustomFields.remove(index) }
                                else { revealedCustomFields.insert(index) }
                            } label: {
                                Image(systemName: revealedCustomFields.contains(index) ? "eye.slash" : "eye")
                                    .iconButtonHitArea()
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel("\(revealedCustomFields.contains(index) ? "Hide" : "Show") \(draft.allField[index].name.isEmpty ? "value" : draft.allField[index].name)")
                            .accessibilityIdentifier("editor-field-\(index)-reveal")
                        }
                    }
                    Toggle("Secret", isOn: $draft.allField[index].concealed)
                        .accessibilityLabel("Secret \(draft.allField[index].name)")
                        .accessibilityIdentifier("editor-field-\(index)-secret")
                }
            }
            .onDelete { offsets in
                draft.allField.remove(atOffsets: offsets)
                revealedCustomFields = Set(revealedCustomFields.filter { !offsets.contains($0) }.map { row in row - offsets.filter { $0 < row }.count })
            }
            Button {
                draft.allField.append(ItemField(name: "", value: "", concealed: false))
            } label: {
                Label("Add field", systemImage: "plus")
            }
        } header: {
            Text("Other fields")
                .foregroundStyle(.themeSecondary)
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
                .editorFieldLabel("Add a tag")
        } header: {
            Text("Tags")
                .foregroundStyle(.themeSecondary)
        } footer: {
            if draft.allTag.isEmpty {
                Text("Tags group items across vaults; open Tags on the Vaults screen to browse them.")
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    private func addTags() {
        for tag in ItemDetail.splitTags(newTagText) where !draft.allTag.contains(tag) {
            draft.allTag.append(tag)
        }
        newTagText = ""
    }

    /// The draft with the websites as typed, for the icon's preview.
    private func buildIconPreviewItem() -> ItemDetail {
        var item = draft
        item.allURL = allURLText.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        return item
    }

    private func save() {
        addTags()
        var item = draft
        item.allURL = allURLText.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        let vaultID = chosenVaultID
        Task {
            if let savedItem = await model.saveItem(vaultID: vaultID, item: item) {
                loadedItem = buildIconPreviewItem()
                finish(savedItem, vaultID)
                dismiss()
            }
            else {
                saveError = model.errorMessage
                saveFailureCount += 1
            }
        }
    }

    private func delete() {
        guard let originalItem else { return }
        Task {
            if await model.deleteItem(vaultID: vaultID, itemID: originalItem.id) {
                loadedItem = buildIconPreviewItem()
                dismiss()
                finish(nil, vaultID)
            }
        }
    }
}

struct EditorFieldLabel: ViewModifier {
    var title: String

    func body(content: Content) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(.footnote)
                .foregroundStyle(.themeSecondary)
                .accessibilityHidden(true)
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

extension View {
    func editorFieldLabel(_ title: String) -> some View {
        modifier(EditorFieldLabel(title: title))
    }
}

/// Calls back when a swipe down is blocked by interactiveDismissDisabled, which SwiftUI itself does not report: the sheet's
/// presentation controller gets a delegate that answers that one question and forwards the rest to SwiftUI's.
private struct SheetDismissAttemptObserver: UIViewControllerRepresentable {
    var onAttempt: () -> Void

    func makeUIViewController(context: Context) -> ObserverController {
        ObserverController()
    }

    func updateUIViewController(_ controller: ObserverController, context: Context) {
        controller.onAttempt = onAttempt
        controller.install()
    }

    final class ObserverController: UIViewController, UIAdaptivePresentationControllerDelegate {
        var onAttempt: () -> Void = {}
        private weak var originalDelegate: UIAdaptivePresentationControllerDelegate?

        override func viewDidAppear(_ animated: Bool) {
            super.viewDidAppear(animated)
            install()
        }

        func install() {
            var sheet: UIViewController = self
            while let parent = sheet.parent {
                sheet = parent
            }
            guard let presentation = sheet.presentationController, presentation.delegate !== self else { return }
            originalDelegate = presentation.delegate
            presentation.delegate = self
        }

        func presentationControllerDidAttemptToDismiss(_ presentationController: UIPresentationController) {
            onAttempt()
        }

        override func responds(to aSelector: Selector!) -> Bool {
            super.responds(to: aSelector) || originalDelegate?.responds(to: aSelector) == true
        }

        override func forwardingTarget(for aSelector: Selector!) -> Any? {
            originalDelegate?.responds(to: aSelector) == true ? originalDelegate : super.forwardingTarget(for: aSelector)
        }
    }
}
