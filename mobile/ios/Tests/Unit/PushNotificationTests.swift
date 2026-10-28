import UserNotifications
import XCTest
@testable import Hisar

final class FakeNotificationCenter: NotificationScheduling, @unchecked Sendable {
    private let lock = NSLock()
    private var authorization: NotificationAuthorization
    private var isGrantedOnRequest: Bool
    private(set) var requestCount = 0
    private(set) var allAdded: [UNNotificationRequest] = []
    private(set) var allRemovedIdentifier: [String] = []

    init(authorization: NotificationAuthorization = .notDetermined, isGrantedOnRequest: Bool = true) {
        self.authorization = authorization
        self.isGrantedOnRequest = isGrantedOnRequest
    }

    func requestAuthorization() async throws -> Bool {
        lock.withLock {
            requestCount += 1
            authorization = isGrantedOnRequest ? .allowed : .denied
            return isGrantedOnRequest
        }
    }

    func getAuthorization() async -> NotificationAuthorization {
        lock.withLock { authorization }
    }

    func add(_ request: UNNotificationRequest) async throws {
        lock.withLock { allAdded.append(request) }
    }

    func removeNotifications(withIdentifiers allIdentifier: [String]) {
        lock.withLock {
            allRemovedIdentifier.append(contentsOf: allIdentifier)
            allAdded.removeAll { allIdentifier.contains($0.identifier) }
        }
    }
}

@MainActor
final class PushNotificationTests: XCTestCase {
    private var backend: FakeBackend!
    private var center: FakeNotificationCenter!
    private var registerCount = 0
    private let currentDate = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUp() async throws {
        backend = FakeBackend()
        registerCount = 0
    }

    private func createUnlockedModel(authorization: NotificationAuthorization, allApproval: [PhoneApproval] = []) async -> AppModel {
        center = FakeNotificationCenter(authorization: authorization)
        backend.seedSignedIn()
        backend.allPendingApproval = allApproval
        let model = AppModel(
            credentialIdentityStore: FakeCredentialIdentityStore(), getCurrentDate: { [unowned self] in currentDate },
            notificationCenter: center, registerForRemoteNotifications: { [unowned self] in registerCount += 1 }
        )
        await model.start { self.backend }?.value
        await model.refreshPendingApproval()
        return model
    }

    func testNotificationPrivacyExplanation() {
        XCTAssertEqual(PushNotificationsModel.privacyExplanation, "Approval requests and security alerts don't name items or people. Sharing invitations show who invited you and, for an organization, its name, even while Hisar is locked. To keep them off the lock screen, set Show Previews for Hisar to When Unlocked or Never in iOS Settings. Hisar's push relay and Apple see only the kind of notification; its details are encrypted for this phone.")
    }

    func testTurningNotificationsOnAsksOnceThenRegistersTheTokenAsHex() async {
        let model = await createUnlockedModel(authorization: .notDetermined)
        XCTAssertEqual(registerCount, 0, "Hisar never asks for a token before the user turns notifications on")
        await model.notifications.turnOn()
        XCTAssertEqual(center.requestCount, 1)
        XCTAssertEqual(registerCount, 1)
        XCTAssertEqual(model.notifications.authorization, .allowed)

        await model.notifications.receiveDeviceToken(Data([0x0a, 0xff, 0x01]))
        XCTAssertEqual(backend.allPushToken, ["\(PushNotificationsModel.currentPushPlatform):0aff01"])
        XCTAssertTrue(model.notifications.settings.isAvailable)
    }

    func testALaunchAfterNotificationsWereAllowedAsksForThisLaunchsToken() async {
        _ = await createUnlockedModel(authorization: .allowed)
        XCTAssertEqual(registerCount, 1)
        XCTAssertEqual(center.requestCount, 0, "permission is never asked again")
    }

    func testTurningAKindOffReachesTheBackend() async {
        let model = await createUnlockedModel(authorization: .allowed)
        await model.notifications.setEnabled(.sharing, isEnabled: false)
        XCTAssertFalse(model.notifications.settings.isEnabled(.sharing))
        XCTAssertTrue(model.notifications.settings.isEnabled(.approval))
        XCTAssertEqual(backend.pushSettings.enabledKinds.sorted(), ["approval", "security"])
    }

    func testATappedApprovalNotificationOpensTheApprovalsTab() async {
        let model = await createUnlockedModel(authorization: .allowed)
        model.selectedTab = .vaults
        model.notifications.open(userInfo: ["kind": "approval"])
        XCTAssertEqual(model.notifications.openedKind, .approval)
        model.showNotification(.approval)
        XCTAssertEqual(model.selectedTab, .approvals)
        XCTAssertNil(model.notifications.openedKind)
        model.notifications.open(userInfo: ["kind": "marketing"])
        XCTAssertNil(model.notifications.openedKind, "an unknown kind opens nothing")
    }

    func testATappedSecurityAlertOpensDevices() async {
        let model = await createUnlockedModel(authorization: .allowed)
        model.selectedTab = .vaults
        model.notifications.open(userInfo: ["kind": "security"])
        model.showNotification(.security)
        XCTAssertEqual(model.selectedTab, .settings)
        XCTAssertEqual(model.requestedSettingsDestination, .devices, "Devices shows the device that just signed in")
        model.requestedSettingsDestination = nil
        model.notifications.open(userInfo: ["kind": "sharing"])
        model.showNotification(.sharing)
        XCTAssertNil(model.requestedSettingsDestination, "an invitation opens Settings itself")
    }

    func testLeavingWithAnApprovalWaitingLeavesAReminderThatSaysNoMore() async throws {
        let approval = PhoneApproval(
            requestID: "0123456789abcdef0123456789abcdef", requesterDeviceName: "MacBook Pro", expiresAt: ISO8601DateFormatter().string(from: currentDate.addingTimeInterval(60)),
            kind: PhoneApproval.kindSSH, appName: "Terminal", purpose: "push to git.tryhisar.com", host: "git.tryhisar.com:22", commandLine: "git push",
            repositoryPath: "~/src/core", keyName: "work", isForwarded: false, forwardedFromHost: ""
        )
        let model = await createUnlockedModel(authorization: .allowed, allApproval: [approval])
        XCTAssertEqual(model.allPendingApproval.count, 1)
        await model.notifications.remindOfWaitingApproval(isWaiting: !model.allPendingApproval.isEmpty)
        let reminder = try XCTUnwrap(center.allAdded.first)
        XCTAssertEqual(reminder.identifier, PushNotificationsModel.approvalReminderIdentifier)
        XCTAssertFalse(reminder.content.body.contains("git"), "the reminder names nothing from the request")
        XCTAssertFalse(reminder.content.body.contains("MacBook"))
        model.handleWillEnterForeground()
        XCTAssertTrue(center.allAdded.isEmpty)
    }

    func testNoReminderWithoutAWaitingApprovalOrPermission() async {
        let model = await createUnlockedModel(authorization: .allowed)
        await model.notifications.remindOfWaitingApproval(isWaiting: false)
        XCTAssertTrue(center.allAdded.isEmpty)
        let deniedModel = await createUnlockedModel(authorization: .denied)
        await deniedModel.notifications.remindOfWaitingApproval(isWaiting: true)
        XCTAssertTrue(center.allAdded.isEmpty)
    }
}
