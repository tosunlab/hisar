import Foundation

// A request the user's Mac is waiting on: approve or deny one SSH or command-line use, one share link an app such as
// Raycast creates, or that app using Hisar until it locks on the Mac.
struct PhoneApproval: Codable, Equatable, Identifiable {
    static let kindSSH = "ssh"
    static let kindCommandLine = "cli"
    static let kindLauncher = "launcher"
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
    // projectPath is the folder of the coding agent's harness, such as omp's project; empty from an older Mac.
    var projectPath: String
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
    // commandLineLifetime is how long approving a command-line request also lets its terminal, app or connection use
    // Hisar, as the Mac words it ("until 10 minutes after …"); empty from an older Mac and for a request approved once.
    var commandLineLifetime: String

    var id: String { requestID }

    var isRememberable: Bool { !rememberLabel.isEmpty }

    // resolveRememberChoice is the switch as the user left it, or its default; a request without the switch is never remembered.
    func resolveRememberChoice(_ chosenValue: Bool?) -> Bool {
        isRememberable && (chosenValue ?? isRememberedByDefault)
    }

    enum CodingKeys: String, CodingKey {
        case requestID = "requestId"
        case requesterDeviceName, createdAt, expiresAt, kind, appName, purpose, host, commandLine, repositoryPath, projectPath, keyName, isForwarded, forwardedFromHost, terminalName, secrets, appIcon, appWarning, rememberLabel, isRememberedByDefault
        case commandLineLifetime = "lifetime"
    }

    init(requestID: String, requesterDeviceName: String, createdAt: String = "", expiresAt: String, kind: String, appName: String, purpose: String, host: String = "", commandLine: String = "", repositoryPath: String = "", projectPath: String = "", keyName: String = "", isForwarded: Bool = false, forwardedFromHost: String = "", terminalName: String = "", secrets: [String] = [], appIcon: Data? = nil, appWarning: String = "", rememberLabel: String = "", isRememberedByDefault: Bool = false, commandLineLifetime: String = "") {
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
        self.projectPath = projectPath
        self.keyName = keyName
        self.isForwarded = isForwarded
        self.forwardedFromHost = forwardedFromHost
        self.terminalName = terminalName
        self.secrets = secrets
        self.appIcon = appIcon
        self.appWarning = appWarning
        self.rememberLabel = rememberLabel
        self.isRememberedByDefault = isRememberedByDefault
        self.commandLineLifetime = commandLineLifetime
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
        projectPath = try container.decodeIfPresent(String.self, forKey: .projectPath) ?? ""
        keyName = try container.decodeIfPresent(String.self, forKey: .keyName) ?? ""
        isForwarded = try container.decodeIfPresent(Bool.self, forKey: .isForwarded) ?? false
        forwardedFromHost = try container.decodeIfPresent(String.self, forKey: .forwardedFromHost) ?? ""
        terminalName = try container.decodeIfPresent(String.self, forKey: .terminalName) ?? ""
        secrets = try container.decodeIfPresent([String].self, forKey: .secrets) ?? []
        appIcon = try? container.decodeIfPresent(Data.self, forKey: .appIcon)
        appWarning = try container.decodeIfPresent(String.self, forKey: .appWarning) ?? ""
        rememberLabel = try container.decodeIfPresent(String.self, forKey: .rememberLabel) ?? ""
        isRememberedByDefault = try container.decodeIfPresent(Bool.self, forKey: .isRememberedByDefault) ?? false
        commandLineLifetime = try container.decodeIfPresent(String.self, forKey: .commandLineLifetime) ?? ""
    }

    var expiryDate: Date? { Self.parseDate(expiresAt) }

    var macName: String { requesterDeviceName.isEmpty ? "your Mac" : requesterDeviceName }

    var summary: String {
        let requester = isForwarded ? (forwardedFromHost.isEmpty ? "Another computer" : forwardedFromHost) : appName
        return "\(requester) wants to \(purpose)"
    }

    var bannerTitle: String {
        switch kind {
        case Self.kindCommandLine:
            return "\(macName) is asking to use Hisar from the command line"
        case Self.kindLauncher:
            return "\(macName) is asking to let \(appName) use Hisar"
        default:
            return "\(macName) is asking to use \(keyName.isEmpty ? "your SSH key" : "“\(keyName)”")"
        }
    }

    // lifetime is set for an approval that outlasts the request: a launcher keeps its approval until Hisar locks on the Mac,
    // and a command-line approval lasts as long as commandLineLifetime says. A launcher request that names secrets is a
    // share link, which is approved once like any other request.
    var lifetime: String {
        if kind == Self.kindLauncher {
            return secrets.isEmpty ? "Until Hisar locks on \(macName)" : ""
        }
        return commandLineLifetime.prefix(1).uppercased() + commandLineLifetime.dropFirst()
    }

    var approvalNote: String {
        if kind == Self.kindLauncher && !lifetime.isEmpty {
            return "Approving lets \(appName) use Hisar until it locks on your Mac; it does not unlock Hisar there."
        }
        if !lifetime.isEmpty {
            return "Approving also lets \(appName) use Hisar without asking until then. It does not unlock Hisar on your Mac."
        }
        return "Approving lets this one request continue; it does not unlock Hisar on your Mac."
    }

    var detailLine: String {
        if !keyName.isEmpty {
            return "with “\(keyName)”"
        }
        if !secrets.isEmpty {
            return secrets.joined(separator: ", ")
        }
        if !lifetime.isEmpty {
            return lifetime
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

    var systemImage: String {
        switch kind {
        case Self.kindCommandLine:
            return "apple.terminal"
        case Self.kindLauncher:
            return "magnifyingglass"
        default:
            return "key.horizontal"
        }
    }

    var allDetailRow: [DetailRow] {
        let appRow = DetailRow(label: "App", value: appName, isMonospaced: false)
        let secretRow = DetailRow(label: "Secrets", value: secrets.joined(separator: "\n"), isMonospaced: false)
        // The sheet shows appWarning as a notice of its own, above these rows.
        let allCandidate = kind == Self.kindLauncher ? [
            appRow,
            secretRow,
            DetailRow(label: "Link", value: commandLine, isMonospaced: false),
            DetailRow(label: "Lasts", value: lifetime, isMonospaced: false),
        ] : [
            appRow,
            DetailRow(label: "Project", value: projectPath, isMonospaced: true),
            DetailRow(label: kind == Self.kindCommandLine && secrets.isEmpty ? "Program" : "Command", value: commandLine, isMonospaced: true),
            DetailRow(label: "Repository", value: isRepositoryTheProject ? "" : repositoryPath, isMonospaced: true),
            DetailRow(label: "Server", value: host, isMonospaced: true),
            DetailRow(label: "Key", value: keyName, isMonospaced: false),
            secretRow,
            DetailRow(label: "Forwarded from", value: isForwarded ? forwardedFromHost : "", isMonospaced: true),
            DetailRow(label: "Terminal", value: terminalName.isEmpty ? "" : "/dev/\(terminalName)", isMonospaced: true),
            DetailRow(label: "Lasts", value: lifetime, isMonospaced: false),
        ]
        return allCandidate.filter { !$0.value.isEmpty }
    }

    // A coding agent usually runs in its project's root, where the repository row would only repeat the project.
    private var isRepositoryTheProject: Bool {
        func trimSlash(_ path: String) -> Substring { path.hasSuffix("/") ? path.dropLast() : Substring(path) }
        return !projectPath.isEmpty && trimSlash(repositoryPath) == trimSlash(projectPath)
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
