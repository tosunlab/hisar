import SwiftUI

enum SettingsDestination: Hashable {
    case account
    case secretKey
    case security
    case appearance
    case macApprovals
    case people
    case devices
    case autoFill
    case passwordGenerator
    case about
}

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isSignOutConfirmationShown = false
    @State private var isAutoFillEnabled = false

    static let biometryName = BiometricKeychain(accessGroup: nil).getBiometryName()
    static let biometrySystemImage = biometryName == "Touch ID" ? "touchid" : "faceid"

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
        NavigationStack {
            List {
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
                    NavigationLink(value: SettingsDestination.secretKey) {
                        Label("Secret Key and Emergency Kit", systemImage: "doc.text")
                    }
                    .accessibilityIdentifier("open-secret-key")
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
                        Label("People and sharing", systemImage: "person.2")
                            .badge(model.receivedInvitationCount)
                    }
                    .accessibilityIdentifier("open-people")
                    NavigationLink(value: SettingsDestination.devices) {
                        Label("Devices", systemImage: "iphone.and.arrow.forward")
                    }
                    .accessibilityIdentifier("open-devices")
                }
                Section {
                    NavigationLink(value: SettingsDestination.autoFill) {
                        LabeledContent {
                            Text(isAutoFillEnabled ? "On" : "Off")
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
                    NavigationLink(value: SettingsDestination.about) {
                        LabeledContent {
                            Text(AboutView.versionText)
                        } label: {
                            Label("About Hisar", systemImage: "info.circle")
                        }
                    }
                    .accessibilityIdentifier("open-about")
                    Button("Sign out", role: .destructive) {
                        isSignOutConfirmationShown = true
                    }
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("sign-out")
                }
            }
            .applyThemedListStyle()
            .navigationTitle("Settings")
            .navigationDestination(for: SettingsDestination.self) { destination in
                switch destination {
                case .account: AccountView()
                case .secretKey: SecretKeyView()
                case .security: SecurityView()
                case .appearance: AppearanceView()
                case .macApprovals: MacApprovalsView()
                case .people: PeopleView()
                case .devices: DevicesView()
                case .autoFill: AutoFillSettingsView(isEnabled: $isAutoFillEnabled)
                case .passwordGenerator: StandalonePasswordGeneratorView()
                case .about: AboutView()
                }
            }
            .confirmationDialog("Sign out of Hisar on this iPhone?", isPresented: $isSignOutConfirmationShown, titleVisibility: .visible) {
                Button("Sign out", role: .destructive) {
                    Task { await model.signOut() }
                }
            } message: {
                Text("You will need your Secret Key and master password to sign in again.")
            }
            .task {
                isAutoFillEnabled = await AutoFillSettings.checkIsEnabled()
                await model.refreshReceivedInvitationCount()
            }
        }
    }
}

struct AccountView: View {
    @EnvironmentObject private var model: AppModel

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
                NavigationLink(value: SettingsDestination.secretKey) {
                    Label("Secret Key and Emergency Kit", systemImage: "doc.text")
                }
            } footer: {
                Text("To sign in on a new device you need your master password and your Secret Key. The Emergency Kit has both places to keep them.")
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Account")
    }
}

struct SecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    @State private var password = ""
    @State private var export: EmergencyKitExport?
    @FocusState private var isPasswordFocused: Bool

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
                } footer: {
                    Text("Hisar keeps it on this iPhone; nobody else has a copy. Anyone with it and your master password can open your vaults.")
                }
                Section {
                    ShareLink(item: URL(fileURLWithPath: export.emergencyKitPath), preview: SharePreview("Hisar Emergency Kit", image: Image(systemName: "doc.text"))) {
                        Label("Save or Print Emergency Kit", systemImage: "square.and.arrow.up")
                    }
                    .accessibilityIdentifier("share-emergency-kit")
                } header: {
                    Text("Emergency Kit")
                } footer: {
                    Text("A PDF with your server, email and Secret Key, and a space to write your master password. Print it or save it somewhere safe, away from your devices.")
                }
            } else {
                Section {
                    SecureField("Master password", text: $password)
                        .textContentType(.password)
                        .submitLabel(.go)
                        .focused($isPasswordFocused)
                        .onSubmit(reveal)
                        .accessibilityIdentifier("secret-key-password")
                } footer: {
                    Text("Enter your master password to see your Secret Key and save your Emergency Kit.")
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
                     ? "Turning on \(SettingsView.biometryName) needs your master password. Hisar still asks for the master password at least every 14 days."
                     : "\(SettingsView.biometryName) is not set up on this iPhone.")
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
            } footer: {
                Text("Hisar locks after this long without activity. Copied values are cleared after 90 seconds.")
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
        .alert("Turn on \(SettingsView.biometryName)", isPresented: $isBiometricPasswordPromptShown) {
            SecureField("Master password", text: $biometricPassword)
                .accessibilityIdentifier("biometric-password")
            Button("Cancel", role: .cancel) { biometricPassword = "" }
            Button("Turn on") {
                let password = biometricPassword
                biometricPassword = ""
                Task { await model.setBiometricUnlock(isEnabled: true, password: password) }
            }
        } message: {
            Text("Enter your master password to unlock with \(SettingsView.biometryName).")
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
        ThemePickerList(theme: model.theme, errorMessage: model.errorMessage) { theme in
            Task { await model.setTheme(theme) }
        }
        .onAppear { model.clearError() }
    }
}

struct ThemePickerList: View {
    var theme: VaultTheme
    var errorMessage: String?
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

    var body: some View {
        List {
            Section {
                Toggle(isOn: Binding(get: { model.isMacApprovalAllowed }, set: { isAllowed in
                    Task { await model.setMacApprovalAllowed(isAllowed) }
                })) {
                    Label("Allow approving Mac requests", systemImage: "laptopcomputer.and.iphone")
                }
                .accessibilityIdentifier("mac-approval-toggle")
            } footer: {
                Text("When Hisar on your Mac asks to use an SSH key or the command-line tool, you can approve or deny the request here, while Hisar is open and unlocked on this iPhone. Turned off, your Mac decides on its own.")
            }
            Section {
                Label("A card at the top of Vaults, with Deny and Approve", systemImage: "rectangle.stack")
                Label("Every waiting request in the Approvals tab, with a countdown", systemImage: "list.bullet.rectangle")
                Label("Approve asks for \(SettingsView.biometryName) or your master password", systemImage: SettingsView.biometrySystemImage)
            } header: {
                Text("How requests appear")
            } footer: {
                Text("Approving answers one request and never unlocks Hisar on your Mac. The Mac sends requests only while “Allow approvals from my iPhone” is on in its SSH agent settings.")
            }
        }
        .applyThemedListStyle()
        .navigationTitle("Mac approvals")
    }
}

struct AutoFillSettingsView: View {
    @Binding var isEnabled: Bool
    @EnvironmentObject private var model: AppModel

    private static let isAutoFillIncluded = HisarConfiguration.current.isAppGroupAvailable

    var body: some View {
        List {
            Section {
                if !Self.isAutoFillIncluded {
                    Label("Not available in this build", systemImage: "key.viewfinder")
                        .foregroundStyle(.themeSecondary)
                } else if isEnabled {
                    Label("Hisar fills passwords, passkeys and one-time codes in apps and Safari", systemImage: "checkmark.circle")
                } else {
                    Button {
                        Task {
                            isEnabled = await AutoFillSettings.requestToTurnOn()
                            if isEnabled {
                                await model.refreshAll()
                            }
                        }
                    } label: {
                        Label("Turn on Hisar AutoFill", systemImage: "key.viewfinder")
                    }
                    .accessibilityIdentifier("turn-on-autofill")
                }
            } footer: {
                Text(Self.isAutoFillIncluded
                     ? "You can also turn it on in the Settings app: General, AutoFill & Passwords."
                     : "This build is signed with a free Apple ID, which cannot include the AutoFill extension.")
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
        }
        .applyThemedListStyle()
        .navigationTitle("AutoFill")
        .task { isEnabled = await AutoFillSettings.checkIsEnabled() }
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
                Link(destination: URL(string: "https://github.com/tryhisar/hisar")!) {
                    Label("Source code", systemImage: "chevron.left.forwardslash.chevron.right")
                }
            }
        }
        .applyThemedListStyle()
        .navigationTitle("About Hisar")
        .navigationBarTitleDisplayMode(.inline)
    }
}
