import SwiftUI
import UniformTypeIdentifiers

enum SettingsDestination: Hashable {
    case account
    case secretKey
    case security
    case appearance
    case macApprovals
    case notifications
    case people
    case organizations
    case devices
    case autoFill
    case passwordGenerator
    case help
    case about
    case changeMasterPassword
    case replaceSecretKey
    case deleteAccount
}

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var selection: SettingsDestination?
    /// The screens pushed below the selected section, of several value types (organizations and their members push their own).
    @State private var detailPath = NavigationPath()

    static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()
    static let biometrySystemImage = biometryName == "Touch ID" ? "touchid" : "faceid"
    static let biometryConsent: String = {
        let scope = biometryName == "Touch ID"
            ? "Any fingerprint set up on this iPhone can unlock Hisar."
            : "Anyone enrolled in Face ID on this iPhone, including an alternate appearance, can unlock Hisar."
        return "Unlock Hisar with \(biometryName) instead of typing your master password. \(scope) Hisar still asks for your master password at least every 14 days. Changing \(biometryName) turns off \(biometryName) unlock until you turn it on again with your master password. You can change this later in Settings, Security."
    }()

    private var initials: String {
        let source = model.status.name.isEmpty ? model.status.email : model.status.name
        let allWord = source.split(separator: " ").prefix(2)
        return allWord.compactMap { $0.first.map { String($0).uppercased() } }.joined()
    }

    private var securitySummary: String {
        let autoLock = VaultStatus.describeShortAutoLock(seconds: model.status.autoLockSeconds)
        return model.status.biometricEnabled ? "\(Self.biometryName) · \(autoLock)" : autoLock
    }

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                Section {
                    NavigationLink(value: SettingsDestination.account) {
                        HStack(spacing: 14) {
                            Text(initials)
                                .font(.headline)
                                .foregroundStyle(.themeSecondary)
                                .frame(width: 48, height: 48)
                                .background(.fill.tertiary, in: .circle)
                                .accessibilityHidden(true)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(model.status.name.isEmpty ? "Account" : model.status.name)
                                    .font(.headline)
                                Text("\(model.status.email) · \(VaultLogic.describeServer(model.status.server))")
                                    .font(.footnote)
                                    .foregroundStyle(.themeSecondary)
                                    .lineLimit(1)
                            }
                        }
                        .padding(.vertical, 4)
                    }
                    .accessibilityIdentifier("open-account")
                }
                Section {
                    NavigationLink(value: SettingsDestination.security) {
                        LabeledContent {
                            Text(securitySummary)
                        } label: {
                            Label("Security", systemImage: "lock.shield")
                        }
                    }
                    .accessibilityIdentifier("open-security")
                    NavigationLink(value: SettingsDestination.macApprovals) {
                        LabeledContent {
                            Text(model.isMacApprovalAllowed ? "On" : "Off")
                        } label: {
                            Label("Mac approvals", systemImage: "laptopcomputer.and.iphone")
                        }
                    }
                    .accessibilityIdentifier("open-mac-approvals")
                    NavigationLink(value: SettingsDestination.notifications) {
                        NotificationSettingsLabel(notifications: model.notifications)
                    }
                    .accessibilityIdentifier("open-notifications")
                }
                Section {
                    NavigationLink(value: SettingsDestination.appearance) {
                        LabeledContent {
                            Text("\(model.theme.appearance.label) · \(model.theme.color.label)")
                        } label: {
                            Label("Appearance", systemImage: "paintpalette")
                        }
                    }
                    .accessibilityIdentifier("open-appearance")
                }
                Section {
                    NavigationLink(value: SettingsDestination.people) {
                        Label("People", systemImage: "person.2")
                            .badge(model.peopleInvitationCount)
                    }
                    .accessibilityIdentifier("open-people")
                    NavigationLink(value: SettingsDestination.organizations) {
                        Label("Organizations", systemImage: "building.2")
                            .badge(model.organizationInvitationCount)
                    }
                    .accessibilityIdentifier("open-organizations")
                    NavigationLink(value: SettingsDestination.devices) {
                        Label("Devices", systemImage: "iphone.and.arrow.forward")
                    }
                    .accessibilityIdentifier("open-devices")
                }
                Section {
                    NavigationLink(value: SettingsDestination.autoFill) {
                        LabeledContent {
                            Text(model.isAutoFillEnabled ? "On" : "Off")
                        } label: {
                            Label("AutoFill", systemImage: "key.viewfinder")
                        }
                    }
                    .accessibilityIdentifier("open-autofill")
                    NavigationLink(value: SettingsDestination.passwordGenerator) {
                        Label("Password generator", systemImage: "wand.and.stars")
                    }
                    .accessibilityIdentifier("open-password-generator")
                }
                Section {
                    NavigationLink(value: SettingsDestination.help) {
                        Label("Help", systemImage: "questionmark.circle")
                    }
                    .accessibilityIdentifier("open-help")
                    NavigationLink(value: SettingsDestination.about) {
                        Label("About Hisar", systemImage: "info.circle")
                    }
                    .accessibilityIdentifier("open-about")
                }
            }
            .applyThemedListStyle()
            .navigationTitle("Settings")
            .task {
                await model.refreshExternalSetupStatus()
                await model.refreshReceivedInvitationCount()
            }
            .onReceive(model.$requestedSettingsDestination) { destination in
                guard let destination else { return }
                model.requestedSettingsDestination = nil
                // The new Secret Key is shown only once, so its screen stays until Done. The section opens over whatever
                // was pushed below the one selected before, also when it is the same section.
                if !model.isNewSecretKeyShown {
                    detailPath = NavigationPath()
                    selection = destination
                }
            }
        } detail: {
            // Beside the list on the open iPhone Duo, pushed over it on any other iPhone. Screens below a section (Account →
            // Change master password) push within the detail column.
            NavigationStack(path: $detailPath) {
                Group {
                    if let selection {
                        buildDestination(selection)
                    } else {
                        ContentUnavailableView("Settings", systemImage: "gearshape", description: Text("Choose a section."))
                    }
                }
                .navigationDestination(for: SettingsDestination.self) { destination in
                    buildDestination(destination)
                }
            }
        }
        .navigationSplitViewStyle(.balanced)
        .onChange(of: selection) { _, _ in detailPath = NavigationPath() }
    }

    @ViewBuilder
    private func buildDestination(_ destination: SettingsDestination) -> some View {
        switch destination {
        case .account: AccountView()
        case .secretKey: SecretKeyView()
        case .security: SecurityView()
        case .appearance: AppearanceView()
        case .macApprovals: MacApprovalsView()
        case .notifications: NotificationSettingsView(notifications: model.notifications)
        case .people: PeopleView()
        case .organizations: OrganizationsView()
        case .devices: DevicesView()
        case .autoFill: AutoFillSettingsView()
        case .passwordGenerator: StandalonePasswordGeneratorView()
        case .help: HelpView()
        case .about: AboutView()
        case .changeMasterPassword: ChangeMasterPasswordView()
        case .replaceSecretKey: ReplaceSecretKeyView()
        case .deleteAccount: DeleteAccountView()
        }
    }
}

struct AccountView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isExportShown = false
    @State private var isHisarImportShown = false
    @State private var isSignOutConfirmationShown = false

    private static let hisarExportTypes: [UTType] = [.json, UTType(filenameExtension: "hisar") ?? .data]

    var body: some View {
        List {
            Section {
                LabeledContent("Name", value: model.status.name)
                LabeledContent("Email", value: model.status.email)
                    .textSelection(.enabled)
                LabeledContent("Server", value: model.status.server)
                    .textSelection(.enabled)
            }
            Section {
                NavigationLink(value: SettingsDestination.changeMasterPassword) {
                    Label("Change master password…", systemImage: "key")
                }
                .accessibilityIdentifier("open-change-master-password")
                NavigationLink(value: SettingsDestination.replaceSecretKey) {
                    Label("Replace Secret Key…", systemImage: "doc.badge.arrow.up")
                }
                .accessibilityIdentifier("open-replace-secret-key")
            } footer: {
                Text("Both sign out your other devices. They need a connection to the Hisar server.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                NavigationLink(value: SettingsDestination.secretKey) {
                    Label("Secret Key and Emergency Kit", systemImage: "doc.text")
                }
                .accessibilityIdentifier("open-secret-key")
            } footer: {
                Text("To sign in on a new device you need your master password and your Secret Key. The Emergency Kit holds your Secret Key and has a space to write your master password.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                Button {
                    isExportShown = true
                } label: {
                    Label("Export data…", systemImage: "square.and.arrow.up.on.square")
                }
                .accessibilityIdentifier("open-export")
                Button {
                    isHisarImportShown = true
                } label: {
                    Label("Import a Hisar export…", systemImage: "square.and.arrow.down.on.square")
                }
                .accessibilityIdentifier("open-hisar-export-import")
                DisclosureGroup {
                    Text("Start the export in the other password manager and choose Hisar. Hisar will open so you can review the items and choose their vaults.")
                        .accessibilityIdentifier("other-app-import-guidance")
                    Text("For example, in 1Password, open Settings → Export → Hisar. Keep the source app until you have reviewed the imported items in Hisar.")
                        .accessibilityIdentifier("other-app-import-example")
                } label: {
                    Label("Import from another app", systemImage: "arrow.down.app")
                }
                .accessibilityIdentifier("other-app-import-disclosure")
            } footer: {
                Text("A copy of every vault you can open, encrypted with a passphrase you choose, or as plain JSON. You can import a Hisar export back into your vaults.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                NavigationLink(value: SettingsDestination.deleteAccount) {
                    Label("Delete account…", systemImage: "person.crop.circle.badge.xmark")
                        .foregroundStyle(.red)
                }
                .disabled(model.status.isOffline)
                .accessibilityIdentifier("open-delete-account")
            } footer: {
                Text("Deletes your account and the vaults only you own from the Hisar server. \(AccountLifecycleLogic.exportAdvice)")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                Button("Sign out", role: .destructive) {
                    isSignOutConfirmationShown = true
                }
                .frame(maxWidth: .infinity)
                .accessibilityIdentifier("sign-out")
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Account")
        .confirmationDialog(AccountLifecycleLogic.signOutTitle, isPresented: $isSignOutConfirmationShown, titleVisibility: .visible) {
            Button("Sign out", role: .destructive) {
                Task { await model.signOut() }
            }
        } message: {
            Text(AccountLifecycleLogic.signOutMessage)
        }
        .sheet(isPresented: $isExportShown) {
            ExportDataView()
                .environmentObject(model)
        }
        .fileImporter(isPresented: $isHisarImportShown, allowedContentTypes: Self.hisarExportTypes) { result in
            if case .success(let url) = result {
                Task { await model.importHisarExport(from: url) }
            }
        }
    }
}

struct SecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    @State private var password = ""
    @State private var export: EmergencyKitExport?
    @State private var isPasswordFocused = false

    var body: some View {
        List {
            if let export {
                Section {
                    Text(export.secretKey)
                        .font(.body.monospaced())
                        .textSelection(.enabled)
                        .accessibilityIdentifier("secret-key-value")
                    Button {
                        model.copyText(export.secretKey, label: "Secret Key")
                    } label: {
                        Label("Copy Secret Key", systemImage: "doc.on.doc")
                    }
                    .accessibilityIdentifier("copy-secret-key")
                } header: {
                    Text("Secret Key")
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text("Hisar keeps a copy on this iPhone. The Hisar server does not have it. Anyone with your Secret Key and master password can open your vaults.")
                        .foregroundStyle(.themeSecondary)
                }
                Section {
                    ShareLink(item: URL(fileURLWithPath: export.emergencyKitPath), preview: SharePreview("Hisar Emergency Kit", image: Image(systemName: "doc.text"))) {
                        Label("Save or print Emergency Kit", systemImage: "square.and.arrow.up")
                    }
                    .accessibilityIdentifier("share-emergency-kit")
                } header: {
                    Text("Emergency Kit")
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text("A PDF with your server, email and Secret Key, and a space to write your master password. Print it or save it somewhere safe, away from your devices.")
                        .foregroundStyle(.themeSecondary)
                }
            } else {
                Section {
                    CredentialField("Master password", text: $password)
                        .textContentType(.password)
                        .submitLabel(.go)
                        .focused($isPasswordFocused)
                        .onSubmit(reveal)
                        .accessibilityIdentifier("secret-key-password")
                        .concealWhen(model.isBusy)
                } footer: {
                    Text("Enter your master password to see your Secret Key and save your Emergency Kit.")
                        .foregroundStyle(.themeSecondary)
                }
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                Section {
                    Button(action: reveal) {
                        Group {
                            if model.isBusy {
                                ProgressView()
                            } else {
                                Text("Show Secret Key")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .controlSize(.large)
                    .disabled(password.isEmpty || model.isBusy)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                    .accessibilityIdentifier("secret-key-reveal")
                }
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Secret Key")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            model.clearError()
            isPasswordFocused = true
        }
        .onDisappear {
            if let export {
                model.removeEmergencyKit(export)
            }
            export = nil
            password = ""
        }
    }

    private func reveal() {
        guard !password.isEmpty, !model.isBusy else { return }
        let enteredPassword = password
        Task {
            if let result = await model.exportEmergencyKit(password: enteredPassword) {
                password = ""
                export = result
            }
        }
    }
}

struct SecurityView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isBiometricPasswordPromptShown = false
    @State private var biometricPassword = ""

    var body: some View {
        List {
            Section {
                Toggle(isOn: Binding(get: { model.status.biometricEnabled }, set: setBiometricUnlock)) {
                    Label("Unlock with \(SettingsView.biometryName)", systemImage: SettingsView.biometrySystemImage)
                }
                .disabled(!model.status.biometricAvailable || model.isBusy)
                .accessibilityIdentifier("biometric-toggle")
            } footer: {
                Text(model.status.biometricAvailable
                     ? SettingsView.biometryConsent
                     : "\(SettingsView.biometryName) is not set up on this iPhone.")
                    .foregroundStyle(.themeSecondary)
            }
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            Section {
                Picker("Auto-lock after", selection: Binding(get: { model.status.autoLockSeconds }, set: { seconds in
                    Task { await model.setAutoLock(seconds: seconds) }
                })) {
                    ForEach(VaultStatus.allAutoLockSeconds, id: \.self) { seconds in
                        Text(VaultStatus.describeAutoLock(seconds: seconds))
                            .tag(seconds)
                            .accessibilityIdentifier("auto-lock-\(seconds)")
                    }
                }
                .pickerStyle(.inline)
                .labelsHidden()
                .accessibilityIdentifier("auto-lock-picker")
            } header: {
                Text("Auto-lock after")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                Text("Hisar locks after this long without activity. Copied values are cleared after 90 seconds.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                Button {
                    model.lock()
                } label: {
                    Label("Lock Hisar now", systemImage: "lock")
                }
                .accessibilityIdentifier("settings-lock-now")
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Security")
        .onAppear { model.clearError() }
        .sheet(isPresented: $isBiometricPasswordPromptShown, onDismiss: { biometricPassword = "" }) {
            NavigationStack {
                Form {
                    Section {
                        CredentialField("Master password", text: $biometricPassword)
                            .textContentType(.password)
                            .accessibilityIdentifier("biometric-password")
                            .concealWhen(model.isBusy)
                    } footer: {
                        Text(SettingsView.biometryConsent + "\n\nEnter your master password to continue.")
                            .foregroundStyle(.themeSecondary)
                    }
                }
                .navigationTitle("Turn on \(SettingsView.biometryName)")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Cancel", role: .cancel) {
                            biometricPassword = ""
                            isBiometricPasswordPromptShown = false
                        }
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Turn on") {
                            let password = biometricPassword
                            biometricPassword = ""
                            isBiometricPasswordPromptShown = false
                            Task { await model.setBiometricUnlock(isEnabled: true, password: password) }
                        }
                        .disabled(biometricPassword.isEmpty || model.isBusy)
                    }
                }
            }
            .presentationDetents([.medium])
        }
    }

    private func setBiometricUnlock(_ isEnabled: Bool) {
        if isEnabled {
            isBiometricPasswordPromptShown = true
        } else {
            Task { await model.setBiometricUnlock(isEnabled: false, password: "") }
        }
    }
}

extension ThemeAppearanceName {
    var label: String {
        switch self {
        case .system: "System"
        case .light: "Light"
        case .dark: "Dark"
        }
    }
}

extension ThemeColorName {
    var label: String {
        switch self {
        case .neutral: "Neutral"
        case .blue: "Blue"
        case .purple: "Purple"
        case .green: "Green"
        }
    }
}

struct AppearanceView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ThemePickerList(
            theme: model.theme,
            errorMessage: model.errorMessage,
            isWebsiteIconsEnabled: Binding(get: { model.isWebsiteIconsEnabled }, set: { isEnabled in
                Task { await model.setWebsiteIconsEnabled(isEnabled) }
            })
        ) { theme in
            Task { await model.setTheme(theme) }
        }
        .onAppear { model.clearError() }
    }
}

struct ThemePickerList: View {
    var theme: VaultTheme
    var errorMessage: String?
    /// The website icons switch; nil leaves it out (previews).
    var isWebsiteIconsEnabled: Binding<Bool>?
    var choose: (VaultTheme) -> Void

    var body: some View {
        List {
            Section {
                ForEach(ThemeAppearanceName.allCases, id: \.self) { appearance in
                    ThemeChoiceRow(title: appearance.label, subtitle: appearance == .system ? "Light or Dark, as set on this iPhone" : nil, isChosen: theme.appearance == appearance) {
                        AppearanceSwatch(appearance: appearance)
                    } choose: {
                        choose(VaultTheme(appearance: appearance, color: theme.color))
                    }
                    .accessibilityIdentifier("appearance-\(appearance.rawValue)")
                }
            } header: {
                Text("Appearance")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                ForEach(ThemeColorName.allCases, id: \.self) { color in
                    ThemeChoiceRow(title: color.label, subtitle: nil, isChosen: theme.color == color) {
                        ColorSwatch(palette: ThemePalette.find(color: color))
                    } choose: {
                        choose(VaultTheme(appearance: theme.appearance, color: color))
                    }
                    .accessibilityIdentifier("theme-color-\(color.rawValue)")
                }
            } header: {
                Text("Color")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                Text("Blue, Purple and Green tint the background and the controls. Hisar on your Mac has its own theme setting.")
                    .foregroundStyle(.themeSecondary)
            }
            if let isWebsiteIconsEnabled {
                Section {
                    Toggle("Show website icons", isOn: isWebsiteIconsEnabled)
                        .accessibilityIdentifier("website-icons-switch")
                } footer: {
                    Text("Logins without an icon of their own show their website's. This iPhone asks each website for it directly, never a third party.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if let errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Appearance")
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct ThemeChoiceRow<Swatch: View>: View {
    var title: String
    var subtitle: String?
    var isChosen: Bool
    @ViewBuilder var swatch: () -> Swatch
    var choose: () -> Void

    var body: some View {
        Button(action: choose) {
            HStack(spacing: 14) {
                swatch()
                    .frame(width: 30, height: 30)
                    .clipShape(.rect(cornerRadius: 8))
                    .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(.separator))
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .foregroundStyle(.primary)
                    if let subtitle {
                        Text(subtitle)
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                    }
                }
                Spacer()
                if isChosen {
                    Image(systemName: "checkmark")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(.tint)
                }
            }
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(isChosen ? .isSelected : [])
    }
}

struct AppearanceSwatch: View {
    var appearance: ThemeAppearanceName
    private let neutral = ThemePalette.neutral

    var body: some View {
        HStack(spacing: 0) {
            if appearance != .dark {
                half(background: neutral.groupedBackground.light, dot: neutral.tint.light)
            }
            if appearance != .light {
                half(background: neutral.groupedBackground.dark, dot: neutral.tint.dark)
            }
        }
    }

    private func half(background: UInt32, dot: UInt32) -> some View {
        ZStack(alignment: .bottom) {
            Color(hexadecimal: background)
            Circle()
                .fill(Color(hexadecimal: dot))
                .frame(width: 7, height: 7)
                .padding(.bottom, 5)
        }
    }
}

struct ColorSwatch: View {
    var palette: ThemePalette

    var body: some View {
        ZStack(alignment: .bottom) {
            palette.groupedBackground.color
            Capsule()
                .fill(palette.tint.color)
                .frame(width: 14, height: 7)
                .padding(.bottom, 5)
        }
    }
}

struct MacApprovalsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isSaving = false

    var body: some View {
        List {
            Section {
                Toggle(isOn: Binding(get: { model.isMacApprovalAllowed }, set: { isAllowed in
                    Task {
                        isSaving = true
                        await model.setMacApprovalAllowed(isAllowed)
                        isSaving = false
                    }
                })) {
                    Label("Approve requests from my Mac", systemImage: "laptopcomputer.and.iphone")
                }
                .accessibilityIdentifier("mac-approval-toggle")
                .disabled(isSaving)
                if isSaving { ProgressView("Saving…") }
                if let message = model.macApprovalErrorMessage {
                    ErrorBanner(message: message)
                        .accessibilityIdentifier("mac-approval-error")
                }
            } footer: {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Requests to use SSH keys, the command-line tool, coding agents and apps on your Mac appear here while Hisar is open and unlocked on both devices.")
                    Text("On your Mac, turn on Settings → SSH & Git → Approve requests on my smart device.")
                }
                .foregroundStyle(.themeSecondary)
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Mac approvals")
        .onAppear { model.macApprovalErrorMessage = nil }
    }
}

struct NotificationSettingsView: View {
    @ObservedObject var notifications: PushNotificationsModel
    @Environment(\.openURL) private var openURL

    static func describe(_ kind: PushKind) -> (title: String, detail: String, systemImage: String) {
        switch kind {
        case .approval: ("Approval requests", "Your Mac asks this iPhone to approve an SSH key, the command-line tool, a coding agent or an app.", "laptopcomputer.and.iphone")
        case .security: ("Security alerts", "A new device signed in, or your master password or Secret Key changed.", "lock.shield")
        case .sharing: ("Sharing", "Someone invited you to a vault or an organization.", "person.2")
        }
    }

    var body: some View {
        List {
            Section {
                switch notifications.authorization {
                case .notDetermined:
                    Button {
                        Task { await notifications.turnOn() }
                    } label: {
                        Label("Turn on notifications", systemImage: "bell")
                    }
                    .accessibilityIdentifier("notifications-turn-on")
                case .denied:
                    Button {
                        if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
                    } label: {
                        Label("Allow notifications in iOS Settings", systemImage: "gear")
                    }
                    .accessibilityIdentifier("notifications-open-settings")
                case .allowed:
                    ForEach(PushKind.allCases, id: \.self) { kind in
                        let description = Self.describe(kind)
                        Toggle(isOn: Binding(get: { notifications.settings.isEnabled(kind) }, set: { isEnabled in
                            Task { await notifications.setEnabled(kind, isEnabled: isEnabled) }
                        })) {
                            Label {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(description.title)
                                    Text(description.detail)
                                        .font(.footnote)
                                        .foregroundStyle(.themeSecondary)
                                }
                            } icon: {
                                Image(systemName: description.systemImage)
                            }
                        }
                        .disabled(!notifications.settings.isServerSending)
                        .accessibilityIdentifier("notifications-\(kind.rawValue)-toggle")
                    }
                }
            } footer: {
                Text(footer)
                    .foregroundStyle(.themeSecondary)
            }
            if let errorMessage = notifications.errorMessage ?? notifications.settings.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Notifications")
        .task { await notifications.refresh() }
    }

    private var footer: String {
        if notifications.authorization == .allowed && !notifications.settings.isServerSending {
            return "The Hisar server does not send notifications. Its admin can turn them on in the dashboard's settings."
        }
        return PushNotificationsModel.privacyExplanation
    }
}

private struct NotificationSettingsLabel: View {
    @ObservedObject var notifications: PushNotificationsModel

    var body: some View {
        LabeledContent {
            Text(notifications.authorization == .allowed ? "On" : "Off")
        } label: {
            Label("Notifications", systemImage: "bell.badge")
        }
    }
}

struct AutoFillSettingsView: View {
    @EnvironmentObject private var model: AppModel

    private static let isAutoFillIncluded = HisarConfiguration.current.isAppGroupAvailable

    var body: some View {
        List {
            Section {
                if !Self.isAutoFillIncluded {
                    Label("Not available in this build", systemImage: "key.viewfinder")
                        .foregroundStyle(.themeSecondary)
                } else if model.isAutoFillEnabled {
                    Label("Hisar fills passwords, passkeys and one-time codes in apps and Safari", systemImage: "checkmark.circle")
                } else {
                    Button {
                        Task {
                            _ = await AutoFillSettings.requestToTurnOn()
                            await model.refreshExternalSetupStatus()
                        }
                    } label: {
                        Label("Turn on Hisar AutoFill", systemImage: "key.viewfinder")
                    }
                    .accessibilityIdentifier("turn-on-autofill")
                }
            } footer: {
                if !Self.isAutoFillIncluded {
                    Text("This build is signed with a free Apple ID, which cannot include the AutoFill extension.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if Self.isAutoFillIncluded {
                Section {
                    Button {
                        Task { await AutoFillSettings.openSettings() }
                    } label: {
                        Label("Open AutoFill & Passwords", systemImage: "gear")
                    }
                }
            }
            Section {
                Toggle("Only fill on the exact host", isOn: Binding(
                    get: { model.isExactHostMatch },
                    set: { isExact in Task { await model.setExactHostMatch(isExact) } }
                ))
                .accessibilityIdentifier("exact-host-match")
            } footer: {
                Text("Off: a login for google.com also fills on accounts.google.com. On: only on the host saved in the item.")
                    .foregroundStyle(.themeSecondary)
            }
        }
        .applyThemedListStyle()
        .navigationTitle("AutoFill")
        .task { await model.refreshExternalSetupStatus() }
    }
}

struct AboutView: View {
    static var versionText: String {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?"
        let build = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "?"
        return "\(version) (\(build))"
    }

    var body: some View {
        List {
            Section {
                VStack(spacing: 10) {
                    LogoMark(size: 64)
                    Text("Hisar")
                        .font(.title2.weight(.bold))
                    Text("Passwords, SSH keys and secrets for developers, end-to-end encrypted.")
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                        .multilineTextAlignment(.center)
                }
                .frame(maxWidth: .infinity)
                .listRowBackground(Color.clear)
            }
            Section {
                LabeledContent("Version", value: Self.versionText)
            }
        }
        .applyThemedListStyle()
        .navigationTitle("About Hisar")
        .navigationBarTitleDisplayMode(.inline)
    }
}

@MainActor
final class SupportContact: ObservableObject {
    @Published private(set) var isMailUnavailable = false

    func openMessage(using openURL: (URL, @escaping (Bool) -> Void) -> Void) {
        let url = URL(string: "mailto:\(HelpView.supportEmail)")!
        openURL(url) { [weak self] accepted in
            self?.isMailUnavailable = !accepted
        }
    }
}

struct HelpView: View {
    static let supportEmail = "support@tryhisar.com"

    @EnvironmentObject private var model: AppModel
    @Environment(\.openURL) private var openURL
    @StateObject private var contact = SupportContact()
    @State private var currentDeviceID: String?

    static func buildDiagnostics(appVersion: String, systemVersion: String, deviceModel: String, server: String, deviceID: String?) -> String {
        var allLine = [
            "Hisar \(appVersion)",
            "iOS \(systemVersion) on \(deviceModel)",
            "Server \(server.isEmpty ? "not signed in" : server)",
        ]
        if let deviceID, !deviceID.isEmpty {
            allLine.append("Device ID \(deviceID)")
        }
        return allLine.joined(separator: "\n")
    }

    private var diagnostics: String {
        Self.buildDiagnostics(
            appVersion: AboutView.versionText, systemVersion: UIDevice.current.systemVersion, deviceModel: UIDevice.current.model,
            server: model.status.server, deviceID: currentDeviceID
        )
    }

    var body: some View {
        List {
            Section {
                Button {
                    contact.openMessage { url, completion in
                        openURL(url, completion: completion)
                    }
                } label: {
                    Label("Contact support", systemImage: "envelope")
                }
                .accessibilityIdentifier("contact-support")
                if contact.isMailUnavailable {
                    Text("No mail app is available. Copy the address and write to us from your mail app or website.")
                        .foregroundStyle(.themeSecondary)
                        .accessibilityIdentifier("support-mail-unavailable")
                }
                Text(verbatim: Self.supportEmail)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("support-email")
                Button {
                    model.copyPlainText(Self.supportEmail, label: "Support address")
                } label: {
                    Label("Copy address", systemImage: "doc.on.doc")
                }
                .accessibilityIdentifier("copy-support-email")
            } header: {
                Text("Contact support")
                    .foregroundStyle(.themeSecondary)
            } footer: {
                Text("Write to us about a problem or an idea. Diagnostics are optional. Copy them below if you want to include them in your message.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                Button {
                    model.copyPlainText(diagnostics, label: "Diagnostics")
                } label: {
                    Label("Copy diagnostics", systemImage: "stethoscope")
                }
                .accessibilityIdentifier("copy-diagnostics")
            } footer: {
                Text("The app version and build, iOS version and device model, server and this device's ID. Never your items, keys or master password.")
                    .foregroundStyle(.themeSecondary)
            }
            Section {
                LabeledContent("Server", value: VaultLogic.describeServer(model.status.server))
                    .textSelection(.enabled)
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Help")
        .task {
            currentDeviceID = await model.loadAllDevice()?.first(where: \.isCurrent)?.id
        }
    }
}
