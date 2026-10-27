#if DEBUG
import SwiftUI

// The approval sheet with a made-up request from a Mac, for UI tests: the simulator cannot run the Mac side.
struct PhoneApprovalPreviewView: View {
    static let argument = "-HisarApprovalPreview"
    static let passwordArgument = "-HisarApprovalPreviewPassword"
    static let previewPassword = "preview master password"
    static let touchIDArgument = "-HisarApprovalPreviewTouchID"

    static func readApproval() -> PhoneApproval? {
        let allArgument = ProcessInfo.processInfo.arguments
        guard let index = allArgument.firstIndex(of: argument), index + 1 < allArgument.count else { return nil }
        return createApproval(kind: allArgument[index + 1], secondsLeft: 60)
    }

    static func createApproval(kind: String, secondsLeft: TimeInterval) -> PhoneApproval {
        let expiresAt = ISO8601DateFormatter().string(from: Date().addingTimeInterval(secondsLeft))
        switch kind {
        case "cli":
            return PhoneApproval(
                requestID: "preview-cli", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindCommandLine,
                appName: "hisar in Terminal", purpose: "use Hisar", commandLine: "/Applications/Hisar.app/Contents/Helpers/hisar", terminalName: "ttys003",
                allowance: "Read secrets and one-time codes\nAdd, change and delete items\nManage vaults, sharing, invitations, organizations and devices\nShare links and exports still ask each time."
            )
        case "forwarded":
            return PhoneApproval(
                requestID: "preview-forwarded", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindSSH,
                appName: "Terminal", purpose: "sign in to github.com", host: "github.com:22", commandLine: "ssh -A deniz@build.tryhisar.dev",
                keyName: "GitHub SSH key", isForwarded: true, forwardedFromHost: "build.tryhisar.dev"
            )
        case "signing":
            return PhoneApproval(
                requestID: "preview-signing", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindSSH,
                appName: "Claude", purpose: "sign a commit", commandLine: "“Add phone approvals”", repositoryPath: "~/Documents/hisar/core · phase-6/phone-approvals",
                keyName: "Signing key", rememberLabel: "Also allows Claude to sign commits in ~/Documents/hisar/core for 10 minutes"
            )
        case "launcher":
            return PhoneApproval(
                requestID: "preview-launcher", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindLauncher,
                appName: "Raycast", purpose: "search Hisar and copy or paste passwords and one-time codes"
            )
        case "launcher-share":
            return PhoneApproval(
                requestID: "preview-launcher-share", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindLauncher,
                appName: "Raycast", purpose: "create a share link for “GitHub”", commandLine: "Expires in 7 days · 1 view", secrets: ["username", "password"],
                appWarning: "Anyone with the link can read these until it expires or its views run out."
            )
        case "agent":
            return PhoneApproval(
                requestID: "preview-agent", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindCommandLine,
                appName: "omp in Tern (via hisar)", purpose: "use 2 secrets", commandLine: "npm run db:migrate", repositoryPath: "~/src/acme-web",
                projectPath: "~/src/acme-web", secrets: ["Agents › Staging Postgres › connection string", "Agents › Stripe › secret key"],
                rememberLabel: "Also allows these secrets for this agent for 1 hour"
            )
        default:
            return PhoneApproval(
                requestID: "preview-ssh", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindSSH,
                appName: "Terminal", purpose: "push to git.tryhisar.com", host: "git.tryhisar.com:22", commandLine: "git push origin main",
                repositoryPath: "~/Documents/hisar/core", keyName: "hisar gitlab", rememberLabel: "Approve SSH requests from Terminal with this key for up to 1 hour. Any host. Ends when Hisar locks.", isRememberedByDefault: true
            )
        }
    }

    var approval: PhoneApproval

    private var biometryName: String {
        ProcessInfo.processInfo.arguments.contains(Self.touchIDArgument) ? "Touch ID" : "Face ID"
    }

    @State private var isSheetShown = true
    @State private var isBusy = false
    @State private var isPasswordRequired = ProcessInfo.processInfo.arguments.contains(PhoneApprovalPreviewView.passwordArgument)
    @State private var errorMessage: String?
    @State private var resultMessage: String?
    @State private var chosenRemember: Bool?

    var body: some View {
        VStack(spacing: 16) {
            LogoMark(size: 64)
            if let resultMessage {
                Text(resultMessage)
                    .font(.headline)
                    .accessibilityIdentifier("phone-approval-result")
            }
        }
        .sheet(isPresented: $isSheetShown) {
            PhoneApprovalSheet(
                approval: approval,
                isBusy: isBusy,
                isPasswordRequired: isPasswordRequired,
                errorMessage: errorMessage,
                isRemembered: Binding(get: { approval.resolveRememberChoice(chosenRemember) }, set: { chosenRemember = $0 }),
                biometryName: biometryName,
                biometrySystemImage: biometryName == "Touch ID" ? "touchid" : "faceid",
                approve: { await approve(isRemembered: $0) },
                approveWithPassword: { await approveWithPassword($0, isRemembered: $1) },
                deny: { finish("Denied \(approval.requestID)") }
            )
        }
    }

    private func approve(isRemembered: Bool) async {
        if ProcessInfo.processInfo.arguments.contains(Self.passwordArgument) {
            isPasswordRequired = true
            return
        }
        finish("Approved \(approval.requestID) with \(biometryName)\(isRemembered ? ", remembered" : "")")
    }

    private func approveWithPassword(_ password: String, isRemembered: Bool) async {
        guard password == Self.previewPassword else {
            errorMessage = "Incorrect master password."
            return
        }
        finish("Approved \(approval.requestID) with the master password\(isRemembered ? ", remembered" : "")")
    }

    private func finish(_ message: String) {
        resultMessage = message
        isSheetShown = false
    }
}

// The Vaults card and the Approvals tab with made-up SSH, command-line and launcher requests from a Mac.
struct PendingApprovalPreviewView: View {
    static let argument = "-HisarApprovalBannerPreview"
    // -HisarThemePreview <appearance> <color> <vaults|settings> themes the preview and picks its tab, for screenshots.
    static let themeArgument = "-HisarThemePreview"

    static func isRequested() -> Bool {
        ProcessInfo.processInfo.arguments.contains(argument)
    }

    static func readThemeArgument() -> (theme: VaultTheme, tab: AppModel.Tab) {
        let allArgument = ProcessInfo.processInfo.arguments
        guard let index = allArgument.firstIndex(of: themeArgument), index + 3 < allArgument.count else { return (.standard, .vaults) }
        let theme = VaultTheme(
            appearance: ThemeAppearanceName(rawValue: allArgument[index + 1]) ?? .system,
            color: ThemeColorName(rawValue: allArgument[index + 2]) ?? .neutral
        )
        return (theme, allArgument[index + 3] == "settings" ? .settings : .vaults)
    }

    @State private var allApproval = [
        PhoneApprovalPreviewView.createApproval(kind: "ssh", secondsLeft: 115),
        PhoneApprovalPreviewView.createApproval(kind: "cli", secondsLeft: 70),
        PhoneApprovalPreviewView.createApproval(kind: "launcher", secondsLeft: 90),
    ]
    @State private var presentedApproval: PhoneApproval?
    @State private var resultMessage: String?
    @State private var selectedTab = Self.readThemeArgument().tab
    @State private var theme = Self.readThemeArgument().theme
    @State private var rememberChoices: [String: Bool] = [:]

    var body: some View {
        TabView(selection: $selectedTab) {
            Tab("Vaults", systemImage: "lock.rectangle.stack", value: AppModel.Tab.vaults) {
                NavigationStack {
                    List {
                        PendingApprovalCardSection(
                            allApproval: allApproval,
                            isBusy: false,
                            rememberChoice: rememberChoice,
                            review: { presentedApproval = $0 },
                            approve: { finish($0, message: "Approved \($0.requestID) with Face ID\($1 ? ", remembered" : "")") },
                            deny: { finish($0, message: "Denied \($0.requestID)") },
                            showAll: { selectedTab = .approvals }
                        )
                        Section {
                            VaultRow(vault: VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 24), itemCount: 24)
                            VaultRow(vault: VaultSummary(id: "work", name: "Work", type: "shared", role: "editor", itemCount: 41), itemCount: 41)
                        } header: {
                            Text("Vaults")
                                .foregroundStyle(.themeSecondary)
                        }
                        resultSection
                    }
                    .applyThemedListStyle()
                    .navigationTitle("Vaults")
                    .toolbar {
                        ToolbarItem(placement: .topBarTrailing) {
                            Button {} label: { Label("Lock", systemImage: "lock") }
                        }
                    }
                }
            }
            Tab("Approvals", systemImage: "laptopcomputer.and.iphone", value: AppModel.Tab.approvals) {
                NavigationStack {
                    ApprovalList(
                        allApproval: allApproval,
                        isMacApprovalAllowed: true,
                        listState: .loaded,
                        retry: {},
                        review: { presentedApproval = $0 },
                        deny: { finish($0, message: "Denied \($0.requestID)") },
                        allowMacApproval: {}
                    )
                    .navigationTitle("Approvals")
                }
            }
            .badge(allApproval.count)
            Tab("Settings", systemImage: "gearshape", value: AppModel.Tab.settings) {
                NavigationStack {
                    ThemePickerList(theme: theme) { theme = $0 }
                }
            }
            Tab(value: AppModel.Tab.search, role: .search) {
                Text("Search")
            }
        }
        .applyTheme(theme)
        .sheet(item: $presentedApproval) { approval in
            PhoneApprovalSheet(
                approval: approval,
                isBusy: false,
                isPasswordRequired: false,
                errorMessage: nil,
                isRemembered: rememberChoice(approval),
                approve: { finish(approval, message: "Approved \(approval.requestID) with Face ID\($0 ? ", remembered" : "")") },
                approveWithPassword: { _, _ in },
                deny: { finish(approval, message: "Denied \(approval.requestID)") }
            )
        }
    }

    private func rememberChoice(_ approval: PhoneApproval) -> Binding<Bool> {
        Binding(get: { approval.resolveRememberChoice(rememberChoices[approval.requestID]) }, set: { rememberChoices[approval.requestID] = $0 })
    }

    @ViewBuilder
    private var resultSection: some View {
        if let resultMessage {
            Section {
                Text(resultMessage)
                    .foregroundStyle(.themeSecondary)
                    .accessibilityIdentifier("phone-approval-result")
            }
        }
    }

    private func finish(_ approval: PhoneApproval, message: String) {
        allApproval.removeAll { $0.requestID == approval.requestID }
        resultMessage = message
        presentedApproval = nil
        selectedTab = .vaults
    }
}
#endif
