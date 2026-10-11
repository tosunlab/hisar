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

    var id: String { requestID }

    enum CodingKeys: String, CodingKey {
        case requestID = "requestId"
        case requesterDeviceName, createdAt, expiresAt, kind, appName, purpose, host, commandLine, repositoryPath, keyName, isForwarded, forwardedFromHost, terminalName
    }

    init(requestID: String, requesterDeviceName: String, createdAt: String = "", expiresAt: String, kind: String, appName: String, purpose: String, host: String = "", commandLine: String = "", repositoryPath: String = "", keyName: String = "", isForwarded: Bool = false, forwardedFromHost: String = "", terminalName: String = "") {
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
    }

    var expiryDate: Date? { Self.parseDate(expiresAt) }

    var macName: String { requesterDeviceName.isEmpty ? "your Mac" : requesterDeviceName }

    var title: String { "Approve on \(macName)?" }

    var summary: String {
        let requester = isForwarded ? (forwardedFromHost.isEmpty ? "Another computer" : forwardedFromHost) : appName
        return "\(requester) wants to \(purpose)"
    }

    var systemImage: String { kind == Self.kindCommandLine ? "apple.terminal" : "key.horizontal" }

    var allDetailRow: [DetailRow] {
        let allCandidate = [
            DetailRow(label: "App", value: appName, isMonospaced: false),
            DetailRow(label: kind == Self.kindCommandLine ? "Program" : "Command", value: commandLine, isMonospaced: true),
            DetailRow(label: "Repository", value: repositoryPath, isMonospaced: true),
            DetailRow(label: "Server", value: host, isMonospaced: true),
            DetailRow(label: "Key", value: keyName, isMonospaced: false),
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
