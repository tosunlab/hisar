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
    @State private var isDocumentNumberRevealed = false
    @State private var isCredentialSecretRevealed = false
    @State private var isLicenseKeyRevealed = false
    /// An identity's or card's one-line address saved before it had address fields stays on screen, even when emptied,
    /// until the item is saved.
    @State private var isOldAddressShown = false
    /// The brand of the number last checked: the type follows it until the user picks a different one.
    @State private var numberBrand = ""

    private var isNew: Bool { originalItem == nil }

    private var credentialExpiryWarning: String {
        ItemAPICredential.describeEditedExpiry(draft.apiCredential?.expiry ?? "", saved: originalItem?.apiCredential)
    }

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
                    Section(draft.isServer ? "Admin console" : "Websites") {
                        ForEach(allURLText.indices, id: \.self) { index in
                            TextField(draft.isServer ? "Admin console URL" : "Website", text: $allURLText[index], prompt: Text(verbatim: draft.isServer ? "https://console.example.com" : "https://example.com"))
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
        isOldAddressShown = !(draft.identity?.address ?? "").isEmpty || !(draft.card?.billingAddress ?? "").isEmpty
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
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Remove \(title.lowercased())")
            }
        } else {
            TextField(title, text: value)
                .accessibilityIdentifier(identifier)
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
            buildDateRow("Birth date", bindIdentity(\.birthDate), identifier: "editor-identity-birth-date", isFutureAllowed: false)
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
        }
        buildAddressSection("Address", { bindIdentity((\ItemIdentity.postalAddress).appending(path: $0)) }, oldText: bindIdentity(\.address))
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
        Section(title) {
            TextField("Street", text: bind(\.street), axis: .vertical)
                .lineLimit(1...4)
                .textContentType(.fullStreetAddress)
                .accessibilityIdentifier("editor-address-street")
            TextField("City", text: bind(\.city))
                .textContentType(.addressCity)
                .accessibilityIdentifier("editor-address-city")
            TextField("Region", text: bind(\.region), prompt: Text("State or province").foregroundStyle(.themeSecondary))
                .textContentType(.addressState)
                .accessibilityIdentifier("editor-address-region")
            TextField("Postal code", text: bind(\.postalCode))
                .textContentType(.postalCode)
                .textInputAutocapitalization(.characters)
                .accessibilityIdentifier("editor-address-postal-code")
            TextField("Country", text: bind(\.country))
                .textContentType(.countryName)
                .accessibilityIdentifier("editor-address-country")
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
            TextField("Key ID", text: bindAPICredential(\.keyID))
                .font(.system(.body, design: .monospaced))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-api-credential-key-id")
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
                Button {
                    isCredentialSecretRevealed.toggle()
                } label: {
                    Image(systemName: isCredentialSecretRevealed ? "eye.slash" : "eye")
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
            buildDateRow("Expiry", bindAPICredential(\.expiry), identifier: "editor-api-credential-expiry", isFutureAllowed: true)
        } header: {
            Text("Credential")
        } footer: {
            if !credentialExpiryWarning.isEmpty {
                Text(credentialExpiryWarning)
                    .foregroundStyle(.orange)
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
        model.listAllItemReference(vaultID: vaultID).map(\.item).filter(\.isSSHKey)
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
            TextField("Port", text: bindServer(\.port), prompt: Text(verbatim: "22"))
                .keyboardType(.numberPad)
                .accessibilityIdentifier("editor-server-port")
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
        } footer: {
            if !portWarning.isEmpty {
                Text(portWarning)
                    .foregroundStyle(.orange)
                    .accessibilityIdentifier("editor-server-port-warning")
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
        Section("License") {
            TextField("Product", text: bindSoftwareLicense(\.product))
                .accessibilityIdentifier("editor-license-product")
            TextField("Version", text: bindSoftwareLicense(\.version))
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-version")
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
                Button {
                    isLicenseKeyRevealed.toggle()
                } label: {
                    Image(systemName: isLicenseKeyRevealed ? "eye.slash" : "eye")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(.themeSecondary)
                .accessibilityLabel(isLicenseKeyRevealed ? "Hide license key" : "Reveal license key")
            }
            TextField("Licensed to", text: bindSoftwareLicense(\.licensedTo))
                .textContentType(.name)
                .accessibilityIdentifier("editor-license-licensed-to")
            TextField("Email", text: bindSoftwareLicense(\.email))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-email")
            buildDateRow("Purchase date", bindSoftwareLicense(\.purchaseDate), identifier: "editor-license-purchase-date", isFutureAllowed: false)
            TextField("Download URL", text: bindSoftwareLicense(\.downloadURL), prompt: Text(verbatim: "https://example.com/download"))
                .keyboardType(.URL)
                .textContentType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .accessibilityIdentifier("editor-license-download-url")
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
