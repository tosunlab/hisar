import Foundation
import UserNotifications

/// NotificationScheduling is the part of UNUserNotificationCenter Hisar uses, so tests can stand in for it.
protocol NotificationScheduling: Sendable {
    func requestAuthorization() async throws -> Bool
    func getAuthorization() async -> NotificationAuthorization
    func add(_ request: UNNotificationRequest) async throws
    func removeNotifications(withIdentifiers allIdentifier: [String])
}

enum NotificationAuthorization: Equatable {
    case notDetermined
    case denied
    case allowed
}

struct SystemNotificationCenter: NotificationScheduling {
    func requestAuthorization() async throws -> Bool {
        try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])
    }

    func getAuthorization() async -> NotificationAuthorization {
        switch await UNUserNotificationCenter.current().notificationSettings().authorizationStatus {
        case .notDetermined: .notDetermined
        case .denied: .denied
        default: .allowed
        }
    }

    func add(_ request: UNNotificationRequest) async throws {
        try await UNUserNotificationCenter.current().add(request)
    }

    func removeNotifications(withIdentifiers allIdentifier: [String]) {
        UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: allIdentifier)
        UNUserNotificationCenter.current().removeDeliveredNotifications(withIdentifiers: allIdentifier)
    }
}

/// PushNotificationsModel holds Settings → Notifications and what a notification does: asking for permission, giving
/// Hisar the APNs token, the kinds turned on, the screen a tapped notification opens, and the local reminder of an
/// approval left waiting.
@MainActor
final class PushNotificationsModel: ObservableObject {
    @Published private(set) var settings = PushSettings.unavailable
    @Published private(set) var authorization = NotificationAuthorization.notDetermined
    @Published private(set) var errorMessage: String?
    /// openedKind is the kind of the notification the user tapped, until the app has shown its screen.
    @Published var openedKind: PushKind?

    static let privacyExplanation = "Approval requests and security alerts don't name items or people. Sharing invitations show who invited you and, for an organization, its name, even while Hisar is locked. To keep them off the lock screen, set Show Previews for Hisar to When Unlocked or Never in iOS Settings. Hisar's push relay and Apple see only the kind of notification; its details are encrypted for this phone."

    static let approvalReminderIdentifier = "approval-reminder"
    /// The reminder waits a moment, so switching apps briefly does not raise it.
    static let approvalReminderDelay: TimeInterval = 5

    private let center: NotificationScheduling
    private let registerForRemoteNotifications: @MainActor () -> Void
    private let getBackend: @MainActor () -> VaultBackend?
    private let pushPlatform: String
    /// providePayloadKey returns this device's Web Push subscription key, created and kept in the shared keychain, or
    /// nil when the keychain is unavailable; the app sends it before the token so the server can encrypt push details.
    private let providePayloadKey: @Sendable () -> (p256dh: String, auth: String)?

    init(
        center: NotificationScheduling, registerForRemoteNotifications: @escaping @MainActor () -> Void,
        getBackend: @escaping @MainActor () -> VaultBackend?, pushPlatform: String = PushNotificationsModel.currentPushPlatform,
        providePayloadKey: @escaping @Sendable () -> (p256dh: String, auth: String)? = PushNotificationsModel.loadPayloadKey
    ) {
        self.center = center
        self.registerForRemoteNotifications = registerForRemoteNotifications
        self.getBackend = getBackend
        self.pushPlatform = pushPlatform
        self.providePayloadKey = providePayloadKey
    }

    /// loadPayloadKey reads or creates the device's push subscription key in the shared keychain.
    nonisolated static func loadPayloadKey() -> (p256dh: String, auth: String)? {
        let accessGroup = HisarConfiguration.current.keychainAccessGroup
        guard let key = try? PushPayloadKeyStore(accessGroup: accessGroup).loadOrCreate() else { return nil }
        return (Base64URL.encode(key.publicKeyData), Base64URL.encode(key.auth))
    }

    /// A development build gets its token from APNs's development environment.
    nonisolated static var currentPushPlatform: String {
        #if DEBUG
        "apns-sandbox"
        #else
        "apns"
        #endif
    }

    /// startAtLaunch asks APNs for this launch's token when the user allowed notifications before.
    func startAtLaunch() async {
        authorization = await center.getAuthorization()
        if authorization == .allowed { registerForRemoteNotifications() }
        await refresh()
    }

    func refresh() async {
        authorization = await center.getAuthorization()
        guard let backend = getBackend() else { return }
        if let settings = try? await runInBackground({ try backend.getPushSettings() }) {
            self.settings = settings
        }
    }

    /// turnOn asks iOS for permission, the first time with its own prompt, and then for the APNs token.
    func turnOn() async {
        do {
            if try await center.requestAuthorization() {
                registerForRemoteNotifications()
            }
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
        await refresh()
    }

    func receiveDeviceToken(_ token: Data) async {
        guard let backend = getBackend() else { return }
        let hexToken = token.map { String(format: "%02x", $0) }.joined()
        let platform = pushPlatform
        let payloadKey = providePayloadKey()
        // The payload key goes first, so the server can encrypt a push's details to it; a failure never blocks the token.
        if let payloadKey {
            try? await runInBackground { try backend.setPushPayloadKey(p256dh: payloadKey.p256dh, auth: payloadKey.auth) }
        }
        do {
            try await runInBackground { try backend.setPushToken(platform: platform, token: hexToken) }
            errorMessage = nil
        } catch {
            errorMessage = (error as? BackendError)?.message ?? error.localizedDescription
        }
        await refresh()
    }

    func setEnabled(_ kind: PushKind, isEnabled: Bool) async {
        guard let backend = getBackend() else { return }
        do {
            try await runInBackground { try backend.setPushKindEnabled(kind, isEnabled: isEnabled) }
            errorMessage = nil
        } catch {
            errorMessage = (error as? BackendError)?.message ?? error.localizedDescription
        }
        await refresh()
    }

    /// open records the notification the user tapped; userInfo["kind"] is all a Hisar push carries.
    func open(userInfo: [AnyHashable: Any]) {
        if let text = userInfo["kind"] as? String, let kind = PushKind(rawValue: text) {
            openedKind = kind
        }
    }

    /// remindOfWaitingApproval leaves a reminder when Hisar goes to the background with an approval still waiting, and
    /// removes it otherwise. It says no more than a push would.
    func remindOfWaitingApproval(isWaiting: Bool) async {
        center.removeNotifications(withIdentifiers: [Self.approvalReminderIdentifier])
        guard isWaiting, settings.isEnabled(.approval), await center.getAuthorization() == .allowed else { return }
        let content = UNMutableNotificationContent()
        content.title = "Hisar"
        content.body = String(localized: "push.approval.waiting")
        content.sound = .default
        content.userInfo = ["kind": PushKind.approval.rawValue]
        content.threadIdentifier = PushKind.approval.rawValue
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: Self.approvalReminderDelay, repeats: false)
        try? await center.add(UNNotificationRequest(identifier: Self.approvalReminderIdentifier, content: content, trigger: trigger))
    }

    func clearApprovalReminder() {
        center.removeNotifications(withIdentifiers: [Self.approvalReminderIdentifier])
    }
}
