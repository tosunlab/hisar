import XCTest
@testable import Hisar

@MainActor
final class PhoneApprovalTests: XCTestCase {
    private var backend: FakeBackend!
    private let currentDate = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private func createApproval(requestID: String = "0123456789abcdef0123456789abcdef", secondsLeft: TimeInterval = 45, isForwarded: Bool = false) -> PhoneApproval {
        PhoneApproval(
            requestID: requestID, requesterDeviceName: "Alper's MacBook Pro", expiresAt: ISO8601DateFormatter().string(from: currentDate.addingTimeInterval(secondsLeft)),
            kind: PhoneApproval.kindSSH, appName: "Terminal", purpose: "push to git.tryhisar.com", host: "git.tryhisar.com:22", commandLine: "git push origin main",
            repositoryPath: "~/src/core", keyName: "hisar gitlab", isForwarded: isForwarded, forwardedFromHost: isForwarded ? "build.example.com" : ""
        )
    }

    private func createUnlockedModel(allApproval: [PhoneApproval]) async -> AppModel {
        backend.seedSignedIn()
        backend.allPendingApproval = allApproval
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore(), getCurrentDate: { [currentDate] in currentDate })
        model.start { self.backend }
        await model.refreshAll()
        try? await Task.sleep(nanoseconds: 200_000_000)
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
        XCTAssertEqual(approval.systemImage, "apple.terminal")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Program", "Terminal"])
        XCTAssertEqual(approval.allDetailRow.last?.value, "/dev/ttys003")
        XCTAssertNotNil(approval.expiryDate)
        XCTAssertFalse(approval.isExpired(at: PhoneApproval.parseDate("2027-01-15T08:00:59Z")!))
        XCTAssertTrue(approval.isExpired(at: PhoneApproval.parseDate("2027-01-15T08:01:01Z")!))
    }

    func testDescribesForwardedRequests() {
        let approval = createApproval(isForwarded: true)
        XCTAssertEqual(approval.summary, "build.example.com wants to push to git.tryhisar.com")
        XCTAssertEqual(approval.allDetailRow.map(\.label), ["App", "Command", "Repository", "Server", "Key", "Forwarded from"])
        XCTAssertEqual(createApproval().allDetailRow.map(\.label), ["App", "Command", "Repository", "Server", "Key"])
    }

    func testShowsOnlyOpenRequests() async {
        let model = await createUnlockedModel(allApproval: [createApproval(requestID: "expired", secondsLeft: -1), createApproval(requestID: "open")])
        XCTAssertEqual(model.pendingApproval?.requestID, "open")
    }

    func testApproveWithFaceID() async {
        let model = await createUnlockedModel(allApproval: [createApproval()])
        await model.approvePendingApproval()
        XCTAssertEqual(backend.allApprovalAnswer, ["approve 0123456789abcdef0123456789abcdef"])
        XCTAssertNil(model.pendingApproval)
        XCTAssertEqual(model.noticeMessage, "Approved on your Mac")
        XCTAssertNil(model.approvalErrorMessage)
    }

    func testDenyNeedsNoFaceID() async {
        backend.isBiometricSucceeding = false
        let model = await createUnlockedModel(allApproval: [createApproval()])
        await model.denyPendingApproval()
        XCTAssertEqual(backend.allApprovalAnswer, ["deny 0123456789abcdef0123456789abcdef"])
        XCTAssertNil(model.pendingApproval)
        XCTAssertEqual(model.noticeMessage, "Denied on your Mac")
    }

    func testCancelledFaceIDKeepsTheRequest() async {
        backend.isBiometricSucceeding = false
        let model = await createUnlockedModel(allApproval: [createApproval()])
        await model.approvePendingApproval()
        XCTAssertEqual(backend.allApprovalAnswer, [])
        XCTAssertNotNil(model.pendingApproval)
        XCTAssertNil(model.approvalErrorMessage)
        XCTAssertFalse(model.isApprovalPasswordRequired)
    }

    func testFallsBackToTheMasterPassword() async {
        backend.isBiometricAvailable = false
        let model = await createUnlockedModel(allApproval: [createApproval()])
        await model.approvePendingApproval()
        XCTAssertTrue(model.isApprovalPasswordRequired)
        XCTAssertNil(model.approvalErrorMessage)
        await model.approvePendingApproval(password: "wrong")
        XCTAssertEqual(model.approvalErrorMessage, "Incorrect master password.")
        XCTAssertNotNil(model.pendingApproval)
        await model.approvePendingApproval(password: FakeBackend.password)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve 0123456789abcdef0123456789abcdef"])
        XCTAssertNil(model.pendingApproval)
        XCTAssertFalse(model.isApprovalPasswordRequired)
    }

    func testRequestThatIsGoneCloses() async {
        let model = await createUnlockedModel(allApproval: [createApproval()])
        backend.allPendingApproval = []
        await model.approvePendingApproval()
        XCTAssertNil(model.pendingApproval)
        XCTAssertEqual(model.noticeMessage, "The request is no longer waiting")
    }

    func testEventsShowAndCloseRequests() async {
        let model = await createUnlockedModel(allApproval: [])
        XCTAssertNil(model.pendingApproval)
        backend.allPendingApproval = [createApproval(requestID: "first"), createApproval(requestID: "second")]
        await model.refreshPendingApproval()
        XCTAssertEqual(model.pendingApproval?.requestID, "first")
        model.handleApprovalEvent(name: PhoneApproval.eventResolved, requestID: "first")
        XCTAssertEqual(model.pendingApproval?.requestID, "second")
        model.handleApprovalEvent(name: PhoneApproval.eventAnswered, requestID: "second")
        XCTAssertNil(model.pendingApproval)
        await model.refreshPendingApproval()
        XCTAssertNil(model.pendingApproval, "a request answered elsewhere is not shown again")
    }

    func testNothingWhileLockedOrInTheBackground() async {
        let model = await createUnlockedModel(allApproval: [createApproval()])
        XCTAssertNotNil(model.pendingApproval)
        model.lock()
        XCTAssertNil(model.pendingApproval)
        await model.refreshPendingApproval()
        XCTAssertNil(model.pendingApproval)

        let backgroundModel = await createUnlockedModel(allApproval: [])
        backgroundModel.handleDidEnterBackground()
        backend.allPendingApproval = [createApproval(requestID: "while-away")]
        await backgroundModel.refreshPendingApproval()
        XCTAssertNil(backgroundModel.pendingApproval)
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
