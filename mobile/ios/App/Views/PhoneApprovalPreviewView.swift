#if DEBUG
import SwiftUI

// The approval sheet with a made-up request from a Mac, for UI tests: the simulator cannot run the Mac side.
struct PhoneApprovalPreviewView: View {
    static let argument = "-HisarApprovalPreview"
    static let passwordArgument = "-HisarApprovalPreviewPassword"
    static let previewPassword = "preview master password"

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
                appName: "hisar in Terminal", purpose: "use Hisar", commandLine: "/Applications/Hisar.app/Contents/Helpers/hisar", terminalName: "ttys003"
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
                keyName: "Signing key"
            )
        default:
            return PhoneApproval(
                requestID: "preview-ssh", requesterDeviceName: "Alper's MacBook Pro", expiresAt: expiresAt, kind: PhoneApproval.kindSSH,
                appName: "Terminal", purpose: "push to git.tryhisar.com", host: "git.tryhisar.com:22", commandLine: "git push origin main",
                repositoryPath: "~/Documents/hisar/core", keyName: "hisar gitlab"
            )
        }
    }

    var approval: PhoneApproval

    @State private var isSheetShown = true
    @State private var isBusy = false
    @State private var isPasswordRequired = false
    @State private var errorMessage: String?
    @State private var resultMessage: String?

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
                approve: approve,
                approveWithPassword: approveWithPassword,
                deny: { finish("Denied \(approval.requestID)") }
            )
        }
    }

    private func approve() async {
        if ProcessInfo.processInfo.arguments.contains(Self.passwordArgument) {
            isPasswordRequired = true
            return
        }
        finish("Approved \(approval.requestID) with Face ID")
    }

    private func approveWithPassword(_ password: String) async {
        guard password == Self.previewPassword else {
            errorMessage = "Incorrect master password."
            return
        }
        finish("Approved \(approval.requestID) with the master password")
    }

    private func finish(_ message: String) {
        resultMessage = message
        isSheetShown = false
    }
}

// The Vaults card and the Approvals tab with a made-up SSH and command-line request from a Mac.
struct PendingApprovalPreviewView: View {
    static let argument = "-HisarApprovalBannerPreview"

    static func isRequested() -> Bool {
        ProcessInfo.processInfo.arguments.contains(argument)
    }

    @State private var allApproval = [
        PhoneApprovalPreviewView.createApproval(kind: "ssh", secondsLeft: 115),
        PhoneApprovalPreviewView.createApproval(kind: "cli", secondsLeft: 70),
    ]
    @State private var presentedApproval: PhoneApproval?
    @State private var resultMessage: String?
    @State private var selectedTab = AppModel.Tab.vaults

    var body: some View {
        TabView(selection: $selectedTab) {
            Tab("Vaults", systemImage: "lock.rectangle.stack", value: AppModel.Tab.vaults) {
                NavigationStack {
                    List {
                        PendingApprovalCardSection(
                            allApproval: allApproval,
                            isBusy: false,
                            review: { presentedApproval = $0 },
                            approve: { finish($0, message: "Approved \($0.requestID) with Face ID") },
                            deny: { finish($0, message: "Denied \($0.requestID)") },
                            showAll: { selectedTab = .approvals }
                        )
                        Section("Vaults") {
                            VaultRow(vault: VaultSummary(id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 24), itemCount: 24)
                            VaultRow(vault: VaultSummary(id: "work", name: "Work", type: "shared", role: "editor", itemCount: 41), itemCount: 41)
                        }
                        resultSection
                    }
                    .listStyle(.insetGrouped)
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
                        review: { presentedApproval = $0 },
                        deny: { finish($0, message: "Denied \($0.requestID)") },
                        allowMacApproval: {}
                    )
                    .navigationTitle("Approvals")
                }
            }
            .badge(allApproval.count)
            Tab("Settings", systemImage: "gearshape", value: AppModel.Tab.settings) {
                Text("Settings")
            }
            Tab(value: AppModel.Tab.search, role: .search) {
                Text("Search")
            }
        }
        .sheet(item: $presentedApproval) { approval in
            PhoneApprovalSheet(
                approval: approval,
                isBusy: false,
                isPasswordRequired: false,
                errorMessage: nil,
                approve: { finish(approval, message: "Approved \(approval.requestID) with Face ID") },
                approveWithPassword: { _ in },
                deny: { finish(approval, message: "Denied \(approval.requestID)") }
            )
        }
    }

    @ViewBuilder
    private var resultSection: some View {
        if let resultMessage {
            Section {
                Text(resultMessage)
                    .foregroundStyle(.secondary)
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
