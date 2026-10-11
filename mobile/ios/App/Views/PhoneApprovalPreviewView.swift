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
        let expiresAt = ISO8601DateFormatter().string(from: Date().addingTimeInterval(60))
        switch allArgument[index + 1] {
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
        ZStack {
            Theme.background.ignoresSafeArea()
            VStack(spacing: 16) {
                LogoMark(size: 64)
                if let resultMessage {
                    Text(resultMessage)
                        .font(.headline)
                        .foregroundStyle(Theme.text)
                        .accessibilityIdentifier("phone-approval-result")
                }
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
            .preferredColorScheme(.dark)
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
#endif
