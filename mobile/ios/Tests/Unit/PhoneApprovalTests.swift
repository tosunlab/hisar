import Combine
import UIKit
import XCTest
@testable import Hisar

@MainActor
final class PhoneApprovalTests: XCTestCase {
    private var backend: FakeBackend!
    private var currentDate = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private func createApproval(requestID: String = "0123456789abcdef0123456789abcdef", secondsLeft: TimeInterval = 45, kind: String = PhoneApproval.kindSSH, isForwarded: Bool = false) -> PhoneApproval {
        PhoneApproval(
            requestID: requestID, requesterDeviceName: "Alper's MacBook Pro", expiresAt: ISO8601DateFormatter().string(from: currentDate.addingTimeInterval(secondsLeft)),
            kind: kind, appName: "Terminal", purpose: "push to git.tryhisar.com", host: "git.tryhisar.com:22", commandLine: "git push origin main",
            repositoryPath: "~/src/core", keyName: "hisar gitlab", isForwarded: isForwarded, forwardedFromHost: isForwarded ? "build.example.com" : ""
        )
    }

    // createUnlockedModel is a launch with the app in the foreground: unlocking lists the open requests at once.
    private func createUnlockedModel(allApproval: [PhoneApproval]) async -> AppModel {
        backend.seedSignedIn()
        backend.allPendingApproval = allApproval
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore(), getCurrentDate: { [unowned self] in currentDate })
        await model.start { self.backend }?.value
        await model.refreshPendingApproval()
        return model
    }

    private func createReviewingModel(allApproval: [PhoneApproval]) async -> AppModel {
        let model = await createUnlockedModel(allApproval: allApproval)
        if let first = model.allPendingApproval.first {
            model.reviewApproval(first)
        }
        return model
    }

    func testDecodesTheBackendsRequest() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","createdAt":"2027-01-15T08:00:00.123Z","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"cli","appName":"hisar in Terminal","purpose":"use Hisar","host":"","commandLine":"/Applications/Hisar.app/Contents/Helpers/hisar","repositoryPath":"",
          "keyName":"","isForwarded":false,"forwardedFromHost":"","terminalName":"ttys003"}]
        """
        let allApproval = try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8))
        XCTAssertEqual(allApproval.count, 1)
        let approval = allApproval[0]
        XCTAssertEqual(approval.title, "Approve on MacBook Pro?")
        XCTAssertEqual(approval.summary, "hisar in Terminal wants to use Hisar")
        XCTAssertEqual(approval.bannerTitle, "MacBook Pro is asking to use Hisar from the command line")
        XCTAssertEqual(approval.systemImage, "apple.terminal")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Program", "Terminal"])
        XCTAssertEqual(approval.allDetailRow.last?.value, "/dev/ttys003")
        XCTAssertNotNil(approval.expiryDate)
        XCTAssertFalse(approval.isExpired(at: PhoneApproval.parseDate("2027-01-15T08:00:59Z")!))
        XCTAssertTrue(approval.isExpired(at: PhoneApproval.parseDate("2027-01-15T08:01:01Z")!))
    }

    func testDescribesARunRequestWithItsSecrets() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"cli","appName":"hisar in Terminal","purpose":"run a command with 5 secrets","commandLine":"npm test","terminalName":"ttys003",
          "secrets":["Personal › GitHub › password","Work › AWS › access key","and 3 more"]}]
        """
        let approval = try XCTUnwrap(try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8)).first)
        XCTAssertEqual(approval.summary, "hisar in Terminal wants to run a command with 5 secrets")
        XCTAssertEqual(approval.bannerTitle, "MacBook Pro is asking to use Hisar from the command line")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Command", "Secrets", "Terminal"])
        XCTAssertEqual(approval.allDetailRow[2].value, "Personal › GitHub › password\nWork › AWS › access key\nand 3 more")
    }

    func testCodingAgentRequestShowsItsProject() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"cli","appName":"omp in Tern (via hisar)","purpose":"use 1 secret","commandLine":"npm run db:migrate","repositoryPath":"~/src/acme-web/packages/db",
          "projectPath":"~/src/acme-web","secrets":["Agents › Stripe › secret key"]}]
        """
        let approval = try XCTUnwrap(try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8)).first)
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Project", "Command", "Repository", "Secrets"])
        XCTAssertEqual(approval.allDetailRow[1], PhoneApproval.DetailRow(label: "Project", value: "~/src/acme-web", isMonospaced: true))
        XCTAssertEqual(createApproval().projectPath, "", "a request from an older Mac has no project")
    }

    func testCodingAgentRequestOmitsTheRepositoryThatIsItsProject() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"cli","appName":"omp in Tern (via hisar)","purpose":"use 1 secret","commandLine":"npm run db:migrate","repositoryPath":"~/src/acme-web/",
          "projectPath":"~/src/acme-web","secrets":["Agents › Stripe › secret key"]}]
        """
        let approval = try XCTUnwrap(try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8)).first)
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Project", "Command", "Secrets"])
        XCTAssertEqual(approval.allDetailRow[1].value, "~/src/acme-web")
    }

    func testLauncherRequestNamesTheAppAndHowLongItLasts() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","createdAt":"2027-01-15T08:00:00.123Z","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"launcher","appName":"Raycast","purpose":"search Hisar and copy or paste passwords and one-time codes","host":"","commandLine":"",
          "repositoryPath":"","keyName":"","isForwarded":false,"forwardedFromHost":"","terminalName":"",
          "appIcon":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}]
        """
        let approval = try XCTUnwrap(try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8)).first)
        XCTAssertEqual(approval.title, "Approve on MacBook Pro?")
        XCTAssertEqual(approval.summary, "Raycast wants to search Hisar and copy or paste passwords and one-time codes")
        XCTAssertEqual(approval.bannerTitle, "MacBook Pro is asking to let Raycast use Hisar")
        XCTAssertEqual(approval.systemImage, "magnifyingglass")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Lasts"])
        XCTAssertEqual(approval.allDetailRow.map(\.value), ["Raycast", "Until Hisar locks on MacBook Pro"])
        XCTAssertEqual(approval.detailLine, "Until Hisar locks on MacBook Pro", "the card says how long approving lasts, as there is no command")
        XCTAssertEqual(approval.approvalNote, "Approving lets Raycast use Hisar until it locks on your Mac; it does not unlock Hisar there.")
        XCTAssertFalse(approval.isRememberable)
    }

    func testLauncherShareLinkRequestIsApprovedOnce() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","createdAt":"2027-01-15T08:00:00.123Z","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"launcher","appName":"Raycast","purpose":"create a share link for “GitHub”","host":"","commandLine":"Expires in 7 days · 1 view",
          "repositoryPath":"","keyName":"","isForwarded":false,"forwardedFromHost":"","terminalName":"","secrets":["username","password"],
          "appIcon":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
          "appWarning":"Anyone with the link can read these until it expires or its views run out."}]
        """
        let approval = try XCTUnwrap(try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8)).first)
        XCTAssertEqual(approval.summary, "Raycast wants to create a share link for “GitHub”")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Secrets", "Link", "Warning"], "a share link lasts for this one request")
        XCTAssertEqual(approval.allDetailRow.map(\.value), [
            "Raycast", "username\npassword", "Expires in 7 days · 1 view", "Anyone with the link can read these until it expires or its views run out.",
        ])
        XCTAssertEqual(approval.approvalNote, "Approving lets this one request continue; it does not unlock Hisar on your Mac.")
        XCTAssertEqual(approval.detailLine, "username, password", "the card names what the link shares")
        XCTAssertFalse(approval.isRememberable)
    }

    func testDecodesTheAppIconAndSignatureWarning() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"ssh","appName":"iTerm2","purpose":"sign a commit","commandLine":"“Fix the login redirect”",
          "appIcon":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
          "appWarning":"iTerm2 is not signed, so macOS cannot confirm who made it."}]
        """
        let approval = try XCTUnwrap(try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8)).first)
        XCTAssertEqual(approval.summary, "iTerm2 wants to sign a commit")
        XCTAssertNotNil(approval.appIcon.flatMap(UIImage.init(data:)))
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Warning", "Command"])
        XCTAssertNil(createApproval().appIcon)
    }

    func testDescribesRequestsForTheBanner() {
        let approval = createApproval(secondsLeft: 115, isForwarded: true)
        XCTAssertEqual(approval.summary, "build.example.com wants to push to git.tryhisar.com")
        XCTAssertEqual(approval.bannerTitle, "Alper's MacBook Pro is asking to use “hisar gitlab”")
        XCTAssertEqual(approval.formatTimeLeft(at: currentDate), "Expires in 1:55")
        XCTAssertEqual(approval.formatTimeLeft(at: currentDate.addingTimeInterval(114)), "Expires in 1 second")
        XCTAssertEqual(approval.formatTimeLeft(at: currentDate.addingTimeInterval(80)), "Expires in 35 seconds")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Command", "Repository", "Server", "Key", "Forwarded from"])
        XCTAssertEqual(createApproval().allDetailRow.map(\.label), ["App", "Command", "Repository", "Server", "Key"])
    }

    func testUnlockListsWaitingRequestsWithoutOpeningTheSheet() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "expired", secondsLeft: -1), createApproval(requestID: "first"), createApproval(requestID: "second")])
        XCTAssertEqual(model.allPendingApproval.map(\.requestID), ["first", "second"])
        XCTAssertNil(model.presentedApproval)
        model.reviewApproval(model.allPendingApproval[1])
        XCTAssertEqual(model.presentedApproval?.requestID, "second")
        model.closeApprovalSheet()
        XCTAssertNil(model.presentedApproval)
        XCTAssertEqual(model.allPendingApproval.count, 2, "closing the sheet keeps the request in the list")
    }

    func testARequestArrivingInTheForegroundOpensTheSheet() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "listed-at-unlock")])
        backend.allPendingApproval.append(createApproval(requestID: "new"))
        await model.refreshPendingApproval(isOpeningNewRequest: true)
        XCTAssertEqual(model.presentedApproval?.requestID, "new")
        XCTAssertEqual(model.allPendingApproval.count, 2)
    }

    private func createRememberingApproval(requestID: String = "0123456789abcdef0123456789abcdef", isRememberedByDefault: Bool = true) -> PhoneApproval {
        var approval = createApproval(requestID: requestID)
        approval.rememberLabel = "Also allows Terminal for 1 hour"
        approval.isRememberedByDefault = isRememberedByDefault
        return approval
    }

    func testDecodesTheRememberSwitch() throws {
        let json = """
        [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"ssh","appName":"iTerm2","purpose":"sign a commit","repositoryPath":"~/src/core",
          "rememberLabel":"Also allows iTerm2 to sign commits in ~/src/core for 10 minutes"},
         {"requestId":"fedcba9876543210fedcba9876543210","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com","rememberLabel":"Also allows Terminal for 1 hour","isRememberedByDefault":true},
         {"requestId":"00112233445566778899aabbccddeeff","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
          "kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com"}]
        """
        let allApproval = try JSONDecoder().decode([PhoneApproval].self, from: Data(json.utf8))
        XCTAssertEqual(allApproval.map(\.isRememberable), [true, true, false])
        XCTAssertEqual(allApproval[0].rememberLabel, "Also allows iTerm2 to sign commits in ~/src/core for 10 minutes")
        XCTAssertFalse(allApproval[0].resolveRememberChoice(nil), "commit signing starts off")
        XCTAssertTrue(allApproval[0].resolveRememberChoice(true))
        XCTAssertTrue(allApproval[1].resolveRememberChoice(nil), "the switch starts as the Mac's checkbox does")
        XCTAssertFalse(allApproval[1].resolveRememberChoice(false))
        XCTAssertFalse(allApproval[2].resolveRememberChoice(true), "a request without the switch is never remembered")
    }

    func testCardApprovesWithTheRequestsDefault() async {
        let model = await createUnlockedModel(allApproval: [
            createRememberingApproval(requestID: "ssh"),
            createRememberingApproval(requestID: "signing", isRememberedByDefault: false),
            createApproval(requestID: "once"),
        ])
        await model.approve(model.allPendingApproval[0])
        await model.approve(model.allPendingApproval[0])
        await model.approve(model.allPendingApproval[0])
        XCTAssertEqual(backend.allApprovalAnswer, ["approve remembered ssh", "approve signing", "approve once"])
    }

    func testCardFooterNamesWhatApproveSends() {
        XCTAssertEqual(createRememberingApproval(requestID: "ssh").cardApprovalNote, "Also allows Terminal for 1 hour. Tap the request to approve it once. It never unlocks Hisar on your Mac.")
        XCTAssertEqual(createRememberingApproval(requestID: "signing", isRememberedByDefault: false).cardApprovalNote, "Approving answers this one request. It never unlocks Hisar on your Mac.")
        XCTAssertEqual(createApproval(requestID: "once").cardApprovalNote, "Approving answers this one request. It never unlocks Hisar on your Mac.")
    }

    func testSheetSwitchIsSentWithTheApproval() async {
        let model = await createReviewingModel(allApproval: [createRememberingApproval(requestID: "remembered"), createRememberingApproval(requestID: "once")])
        await model.approvePendingApproval(isRemembered: true)
        model.reviewApproval(model.allPendingApproval[0])
        await model.approvePendingApproval(isRemembered: false)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve remembered remembered", "approve once"])
    }

    func testSheetSwitchIsSentWithThePasswordApproval() async {
        backend.isBiometricAvailable = false
        let model = await createReviewingModel(allApproval: [createRememberingApproval()])
        await model.approvePendingApproval(isRemembered: true)
        XCTAssertTrue(model.isApprovalPasswordRequired)
        await model.approvePendingApproval(password: FakeBackend.password, isRemembered: true)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve remembered 0123456789abcdef0123456789abcdef"])
    }

    func testOldRequestsApproveOnce() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "old")])
        model.reviewApproval(model.allPendingApproval[0])
        await model.approvePendingApproval(isRemembered: true)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve old"], "a request without the switch is never remembered")
    }

    func testApproveWithFaceID() async {
        let model = await createReviewingModel(allApproval: [createApproval()])
        await model.approvePendingApproval(isRemembered: false)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve 0123456789abcdef0123456789abcdef"])
        XCTAssertNil(model.presentedApproval)
        XCTAssertTrue(model.allPendingApproval.isEmpty)
        XCTAssertEqual(model.noticeMessage, "Approved on your Mac")
        XCTAssertNil(model.approvalErrorMessage)
    }

    func testDenyNeedsNoFaceID() async {
        backend.isBiometricSucceeding = false
        let model = await createReviewingModel(allApproval: [createApproval()])
        await model.denyPendingApproval()
        XCTAssertEqual(backend.allApprovalAnswer, ["deny 0123456789abcdef0123456789abcdef"])
        XCTAssertNil(model.presentedApproval)
        XCTAssertEqual(model.noticeMessage, "Denied on your Mac")
    }

    func testCancelledFaceIDKeepsTheRequest() async {
        backend.isBiometricSucceeding = false
        let model = await createReviewingModel(allApproval: [createApproval()])
        await model.approvePendingApproval(isRemembered: false)
        XCTAssertEqual(backend.allApprovalAnswer, [])
        XCTAssertNotNil(model.presentedApproval)
        XCTAssertNil(model.approvalErrorMessage)
        XCTAssertFalse(model.isApprovalPasswordRequired)
    }

    func testFallsBackToTheMasterPassword() async {
        backend.isBiometricAvailable = false
        let model = await createReviewingModel(allApproval: [createApproval()])
        await model.approvePendingApproval(isRemembered: false)
        XCTAssertTrue(model.isApprovalPasswordRequired)
        XCTAssertNil(model.approvalErrorMessage)
        await model.approvePendingApproval(password: "wrong", isRemembered: false)
        XCTAssertEqual(model.approvalErrorMessage, "Incorrect master password.")
        XCTAssertNotNil(model.presentedApproval)
        await model.approvePendingApproval(password: FakeBackend.password, isRemembered: false)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve 0123456789abcdef0123456789abcdef"])
        XCTAssertNil(model.presentedApproval)
        XCTAssertFalse(model.isApprovalPasswordRequired)
    }

    func testRequestThatIsGoneCloses() async {
        let model = await createReviewingModel(allApproval: [createApproval()])
        backend.allPendingApproval = []
        await model.approvePendingApproval(isRemembered: false)
        XCTAssertNil(model.presentedApproval)
        XCTAssertTrue(model.allPendingApproval.isEmpty)
        XCTAssertEqual(model.noticeMessage, "The request is no longer waiting")
    }

    func testAnsweredWithdrawnAndExpiredRequestsLeaveTheList() async {
        let model = await createReviewingModel(allApproval: [createApproval(requestID: "first"), createApproval(requestID: "second", secondsLeft: 90), createApproval(requestID: "third", secondsLeft: 20)])
        XCTAssertEqual(model.presentedApproval?.requestID, "first")
        model.handleApprovalEvent(name: PhoneApproval.eventResolved, requestID: "first")
        XCTAssertNil(model.presentedApproval, "the Mac withdrew the request under review")
        XCTAssertEqual(model.allPendingApproval.map(\.requestID), ["second", "third"])
        model.handleApprovalEvent(name: PhoneApproval.eventAnswered, requestID: "second")
        XCTAssertEqual(model.allPendingApproval.map(\.requestID), ["third"])
        currentDate = currentDate.addingTimeInterval(21)
        model.removeAllExpiredApproval()
        XCTAssertTrue(model.allPendingApproval.isEmpty)
        await model.refreshPendingApproval()
        XCTAssertTrue(model.allPendingApproval.isEmpty, "finished requests are not shown again")
    }

    func testReturningToTheForegroundReconnectsAndLists() async {
        let model = await createUnlockedModel(allApproval: [])
        model.handleDidEnterBackground()
        backend.allPendingApproval = [createApproval(requestID: "posted-while-away")]
        await model.refreshPendingApproval()
        XCTAssertTrue(model.allPendingApproval.isEmpty, "nothing is listed in the background")
        model.handleApprovalEvent(name: PhoneApproval.eventRequested, requestID: "posted-while-away")
        model.handleWillEnterForeground()
        XCTAssertTrue(backend.allCallName.contains("reconnectEvents"))
        let deadline = Date().addingTimeInterval(5)
        while model.allPendingApproval.isEmpty && Date() < deadline {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertEqual(model.allPendingApproval.map(\.requestID), ["posted-while-away"])
        XCTAssertNil(model.presentedApproval, "a request found on returning is listed, not pushed into a sheet")
    }

    func testPollingUnchangedRequestsRedrawsNothing() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "first")])
        var changeCount = 0
        let subscription = model.objectWillChange.sink { changeCount += 1 }
        await model.refreshPendingApproval()
        await model.refreshPendingApproval()
        XCTAssertEqual(changeCount, 0, "a poll that finds the same requests redraws the app")
        backend.allPendingApproval.append(createApproval(requestID: "second", secondsLeft: 90))
        await model.refreshPendingApproval()
        XCTAssertEqual(model.allPendingApproval.map(\.requestID), ["first", "second"])
        XCTAssertGreaterThan(changeCount, 0, "a new request is not shown")
        subscription.cancel()
    }

    // Starts a poll that has read the open requests and waits for the returned closure before it returns them, as a slow
    // poll on a phone.
    private func startHeldPoll(of model: AppModel) async -> (poll: Task<Void, Never>, finish: () -> Void) {
        let isListing = DispatchSemaphore(value: 0)
        let mayFinish = DispatchSemaphore(value: 0)
        backend.onListAllPendingApproval = {
            isListing.signal()
            mayFinish.wait()
        }
        let poll = Task { await model.refreshPendingApproval() }
        while isListing.wait(timeout: .now()) == .timedOut {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        backend.onListAllPendingApproval = nil
        return (poll, { mayFinish.signal() })
    }

    func testALateOlderListingKeepsANewerRequest() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "first")])
        let (olderPoll, finishOlderPoll) = await startHeldPoll(of: model)
        backend.withApprovalLock { backend.allPendingApproval.append(createApproval(requestID: "second", secondsLeft: 90)) }
        await model.refreshPendingApproval(isOpeningNewRequest: true)
        XCTAssertEqual(model.presentedApproval?.requestID, "second")
        finishOlderPoll()
        await olderPoll.value
        XCTAssertEqual(model.allPendingApproval.map(\.requestID), ["first", "second"], "a poll that started earlier took the new request off the list")
        XCTAssertEqual(model.presentedApproval?.requestID, "second", "a poll that started earlier closed the new request's sheet")
    }

    func testTurningMacApprovalsOffDuringAListingKeepsTheListEmpty() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "first")])
        backend.withApprovalLock { backend.allPendingApproval.append(createApproval(requestID: "second", secondsLeft: 90)) }
        let (poll, finishPoll) = await startHeldPoll(of: model)
        await model.setMacApprovalAllowed(false)
        XCTAssertTrue(model.allPendingApproval.isEmpty)
        finishPoll()
        await poll.value
        XCTAssertTrue(model.allPendingApproval.isEmpty, "a listing that started before approvals were turned off listed a request")
    }

    func testNothingWhileLocked() async {
        let model = await createReviewingModel(allApproval: [createApproval()])
        XCTAssertNotNil(model.presentedApproval)
        model.lock()
        XCTAssertNil(model.presentedApproval)
        XCTAssertTrue(model.allPendingApproval.isEmpty)
        await model.refreshPendingApproval()
        XCTAssertTrue(model.allPendingApproval.isEmpty)
    }

    func testClassifiesApprovalErrors() {
        XCTAssertEqual(AppModel.classifyApprovalError("This request is no longer waiting for an answer"), .gone)
        XCTAssertEqual(AppModel.classifyApprovalError("Face ID was cancelled."), .cancelled)
        XCTAssertEqual(AppModel.classifyApprovalError("Enter your master password."), .needsPassword)
        XCTAssertEqual(AppModel.classifyApprovalError("Face ID is locked. Unlock with your master password."), .needsPassword)
        XCTAssertEqual(AppModel.classifyApprovalError("Incorrect master password."), .other)
        XCTAssertEqual(AppModel.classifyApprovalError("Cannot reach the Hisar server"), .other)
    }
}
