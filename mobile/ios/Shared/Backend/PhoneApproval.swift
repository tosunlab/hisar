import Foundation

// A request the user's Mac is waiting on: approve or deny one SSH or command-line use.
struct PhoneApproval: Codable, Equatable, Identifiable {
    static let kindSSH = "ssh"
    static let kindCommandLine = "cli"
    static let eventRequested = "approval-requested"
    static let eventAnswered = "approval-answered"
    static let eventResolved = "approval-resolved"

    struct DetailRow: Equatable, Identifiable {
        var label: String
        var value: String
        var isMonospaced: Bool
        var id: String { label }
    }

    var requestID: String
    var requesterDeviceName: String
    var createdAt: String
    var expiresAt: String
    var kind: String
    var appName: String
    var purpose: String
    var host: String
    var commandLine: String
    var repositoryPath: String
    var keyName: String
    var isForwarded: Bool
    var forwardedFromHost: String
    var terminalName: String
    var secrets: [String]
    var appIcon: Data?
    var appWarning: String
    // rememberLabel is set when approving can also be remembered on the Mac; the sheet shows it as a switch.
    var rememberLabel: String
    var isRememberedByDefault: Bool

    var id: String { requestID }

    var isRememberable: Bool { !rememberLabel.isEmpty }

    // resolveRememberChoice is the switch as the user left it, or its default; a request without the switch is never remembered.
    func resolveRememberChoice(_ chosenValue: Bool?) -> Bool {
        isRememberable && (chosenValue ?? isRememberedByDefault)
    }

    enum CodingKeys: String, CodingKey {
        case requestID = "requestId"
        case requesterDeviceName, createdAt, expiresAt, kind, appName, purpose, host, commandLine, repositoryPath, keyName, isForwarded, forwardedFromHost, terminalName, secrets, appIcon, appWarning, rememberLabel, isRememberedByDefault
    }

    init(requestID: String, requesterDeviceName: String, createdAt: String = "", expiresAt: String, kind: String, appName: String, purpose: String, host: String = "", commandLine: String = "", repositoryPath: String = "", keyName: String = "", isForwarded: Bool = false, forwardedFromHost: String = "", terminalName: String = "", secrets: [String] = [], appIcon: Data? = nil, appWarning: String = "", rememberLabel: String = "", isRememberedByDefault: Bool = false) {
        self.requestID = requestID
        self.requesterDeviceName = requesterDeviceName
        self.createdAt = createdAt
        self.expiresAt = expiresAt
        self.kind = kind
        self.appName = appName
        self.purpose = purpose
        self.host = host
        self.commandLine = commandLine
        self.repositoryPath = repositoryPath
        self.keyName = keyName
        self.isForwarded = isForwarded
        self.forwardedFromHost = forwardedFromHost
        self.terminalName = terminalName
        self.secrets = secrets
        self.appIcon = appIcon
        self.appWarning = appWarning
        self.rememberLabel = rememberLabel
        self.isRememberedByDefault = isRememberedByDefault
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        requestID = try container.decode(String.self, forKey: .requestID)
        requesterDeviceName = try container.decodeIfPresent(String.self, forKey: .requesterDeviceName) ?? ""
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
        expiresAt = try container.decodeIfPresent(String.self, forKey: .expiresAt) ?? ""
        kind = try container.decodeIfPresent(String.self, forKey: .kind) ?? ""
        appName = try container.decodeIfPresent(String.self, forKey: .appName) ?? ""
        purpose = try container.decodeIfPresent(String.self, forKey: .purpose) ?? ""
        host = try container.decodeIfPresent(String.self, forKey: .host) ?? ""
        commandLine = try container.decodeIfPresent(String.self, forKey: .commandLine) ?? ""
        repositoryPath = try container.decodeIfPresent(String.self, forKey: .repositoryPath) ?? ""
        keyName = try container.decodeIfPresent(String.self, forKey: .keyName) ?? ""
        isForwarded = try container.decodeIfPresent(Bool.self, forKey: .isForwarded) ?? false
        forwardedFromHost = try container.decodeIfPresent(String.self, forKey: .forwardedFromHost) ?? ""
        terminalName = try container.decodeIfPresent(String.self, forKey: .terminalName) ?? ""
        secrets = try container.decodeIfPresent([String].self, forKey: .secrets) ?? []
        appIcon = try? container.decodeIfPresent(Data.self, forKey: .appIcon)
        appWarning = try container.decodeIfPresent(String.self, forKey: .appWarning) ?? ""
        rememberLabel = try container.decodeIfPresent(String.self, forKey: .rememberLabel) ?? ""
        isRememberedByDefault = try container.decodeIfPresent(Bool.self, forKey: .isRememberedByDefault) ?? false
    }

    var expiryDate: Date? { Self.parseDate(expiresAt) }

    var macName: String { requesterDeviceName.isEmpty ? "your Mac" : requesterDeviceName }

    var title: String { "Approve on \(macName)?" }

    var summary: String {
        let requester = isForwarded ? (forwardedFromHost.isEmpty ? "Another computer" : forwardedFromHost) : appName
        return "\(requester) wants to \(purpose)"
    }

    var bannerTitle: String {
        if kind == Self.kindCommandLine {
            return "\(macName) is asking to use Hisar from the command line"
        }
        return "\(macName) is asking to use \(keyName.isEmpty ? "your SSH key" : "“\(keyName)”")"
    }

    var detailLine: String {
        if !keyName.isEmpty {
            return "with “\(keyName)”"
        }
        if !secrets.isEmpty {
            return secrets.joined(separator: ", ")
        }
        return commandLine
    }

    func formatTimeLeft(at date: Date) -> String {
        let secondCount = max(0, Int((expiryDate ?? date).timeIntervalSince(date).rounded(.up)))
        if secondCount < 60 {
            return secondCount == 1 ? "Expires in 1 second" : "Expires in \(secondCount) seconds"
        }
        return String(format: "Expires in %d:%02d", secondCount / 60, secondCount % 60)
    }

    var systemImage: String { kind == Self.kindCommandLine ? "apple.terminal" : "key.horizontal" }

    var allDetailRow: [DetailRow] {
        let allCandidate = [
            DetailRow(label: "App", value: appName, isMonospaced: false),
            DetailRow(label: "Warning", value: appWarning, isMonospaced: false),
            DetailRow(label: kind == Self.kindCommandLine && secrets.isEmpty ? "Program" : "Command", value: commandLine, isMonospaced: true),
            DetailRow(label: "Repository", value: repositoryPath, isMonospaced: true),
            DetailRow(label: "Server", value: host, isMonospaced: true),
            DetailRow(label: "Key", value: keyName, isMonospaced: false),
            DetailRow(label: "Secrets", value: secrets.joined(separator: "\n"), isMonospaced: false),
            DetailRow(label: "Forwarded from", value: isForwarded ? forwardedFromHost : "", isMonospaced: true),
            DetailRow(label: "Terminal", value: terminalName.isEmpty ? "" : "/dev/\(terminalName)", isMonospaced: true),
        ]
        return allCandidate.filter { !$0.value.isEmpty }
    }

    func isExpired(at date: Date) -> Bool {
        guard let expiryDate else { return true }
        return expiryDate <= date
    }

    static func parseDate(_ text: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: text) {
            return date
        }
        return ISO8601DateFormatter().date(from: text)
    }
}
