import XCTest
@testable import Hisar

@MainActor
final class AutoFillModelTests: XCTestCase {
    private var backend: FakeBackend!
    private var allCompletedCredential: [LoginCredential] = []
    private var cancelCount = 0

    override func setUp() async throws {
        backend = FakeBackend()
        allCompletedCredential = []
        cancelCount = 0
    }

    private func createModel() -> AutoFillModel {
        AutoFillModel(
            createBackend: { self.backend },
            completeRequest: { [unowned self] in allCompletedCredential.append($0) },
            cancelRequest: { [unowned self] in cancelCount += 1 }
        )
    }

    func testSignedOutShowsMessage() async {
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        XCTAssertEqual(model.screen, .signedOut)
    }

    func testLockedThenUnlockListsMatchingLogins() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["https://github.com/login", "github.com"])
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.serviceName, "github.com")
        let isUnlocked = await model.unlock(password: FakeBackend.password)
        XCTAssertTrue(isUnlocked)
        XCTAssertEqual(model.screen, .list)
        XCTAssertEqual(model.allMatchingLogin.map(\.title), ["GitHub"], "the same login matched twice is listed once")
    }

    func testWrongPasswordStaysLocked() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        let isUnlocked = await model.unlock(password: "nope")
        XCTAssertFalse(isUnlocked)
        XCTAssertEqual(model.screen, .locked)
        XCTAssertNotNil(model.errorMessage)
    }

    func testSelectingMatchingLoginCompletesWithPassword() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["www.netflix.com"])
        XCTAssertEqual(model.screen, .list)
        guard let login = model.allMatchingLogin.first else { return XCTFail("no match") }
        await model.select(login)
        XCTAssertEqual(allCompletedCredential, [LoginCredential(username: "family@example.com", password: "netflix-password")])
        XCTAssertTrue(backend.allCallName.contains("getLoginForService"))
    }

    func testSearchFindsOtherLoginsAndSelectsThem() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        model.searchText = "bank"
        await model.loadAllOtherLogin()
        XCTAssertTrue(model.isOtherLoginLoaded)
        XCTAssertTrue(model.allVisibleMatchingLogin.isEmpty)
        XCTAssertEqual(model.allVisibleOtherLogin.map(\.title), ["Bank"])
        await model.select(model.allVisibleOtherLogin[0])
        XCTAssertEqual(allCompletedCredential, [LoginCredential(username: "alice@example.com", password: "bank-password")])
    }

    func testSearchHidesMatchingLoginsFromOtherSection() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        model.searchText = "git"
        await model.loadAllOtherLogin()
        XCTAssertEqual(model.allVisibleMatchingLogin.map(\.title), ["GitHub"])
        XCTAssertTrue(model.allVisibleOtherLogin.isEmpty)
    }

    func testQuickTypeRequestUnlocksThenCompletes() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareInterfaceToProvideCredential(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(allCompletedCredential.isEmpty)
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(allCompletedCredential, [LoginCredential(username: "alice", password: "github-password")])
    }

    func testWithoutInteractionNeedsAnUnlockedProcess() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        let lockedCredential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertNil(lockedCredential, "no backend exists yet in a fresh extension process")
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        await model.unlock(password: FakeBackend.password)
        let credential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertEqual(credential?.password, "github-password")
        let mismatchedCredential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "evil.example")
        XCTAssertNil(mismatchedCredential)
    }

    func testCancel() {
        let model = createModel()
        model.cancel()
        XCTAssertEqual(cancelCount, 1)
    }

    func testBiometricUnlock() async {
        backend.seedSignedIn()
        _ = try? backend.enableBiometricUnlock(password: FakeBackend.password)
        _ = backend.lock()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        XCTAssertTrue(model.status.biometricReady)
        await model.unlockWithBiometric()
        XCTAssertEqual(model.screen, .list)
        XCTAssertEqual(model.allMatchingLogin.count, 1)
    }
}
