import XCTest
@testable import Hisar

@MainActor
final class CredentialImportTests: XCTestCase {
    private var backend: FakeBackend!
    private var identityStore: FakeCredentialIdentityStore!
    private var allReceivedToken: [UUID] = []
    /// What the other app exported; FakeBackend reads it as the titles of the logins to import.
    private var allExportedTitle = ["GitHub", "Docker", "Slack", "Figma"]

    override func setUp() async throws {
        backend = FakeBackend()
        identityStore = FakeCredentialIdentityStore()
        allReceivedToken = []
    }

    private func createModel() -> AppModel {
        let model = AppModel(credentialIdentityStore: identityStore, receiveExportedCredentials: { [unowned self] token in
            self.allReceivedToken.append(token)
            return try JSONEncoder().encode(self.allExportedTitle)
        })
        model.start { self.backend }
        return model
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = createModel()
        await model.refreshAll()
        return model
    }

    private func waitUntil(_ description: String, _ condition: () -> Bool) async {
        for _ in 0..<200 where !condition() {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition(), description)
    }

    private func countCall(_ name: String) -> Int {
        backend.allCallName.filter { $0 == name }.count
    }

    private func receivePreview(_ model: AppModel) async {
        model.receiveCredentialExchange(token: UUID())
        await waitUntil("the preview arrives") { model.credentialImportPreview != nil }
    }

    func testTokenWaitsForUnlockAndPreparesOnce() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        let token = UUID()
        model.receiveCredentialExchange(token: token)
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.pendingCredentialImportToken, token)
        XCTAssertFalse(model.isCredentialImportShown)
        XCTAssertTrue(allReceivedToken.isEmpty, "nothing is fetched while locked")

        await model.unlock(password: FakeBackend.password)
        await waitUntil("the preview arrives") { model.credentialImportPreview != nil }
        XCTAssertNil(model.pendingCredentialImportToken)
        XCTAssertTrue(model.isCredentialImportShown)
        XCTAssertEqual(allReceivedToken, [token])
        XCTAssertEqual(countCall("prepareCredentialExchangeImport"), 1)
        XCTAssertEqual(model.credentialImportPreview?.vaultID, "personal", "the personal vault is chosen first")
        XCTAssertEqual(model.credentialImportPreview?.allDuplicate.map(\.title), ["GitHub"])

        model.refreshStatus()
        await model.refreshAll()
        XCTAssertEqual(countCall("prepareCredentialExchangeImport"), 1, "a later status change does not import again")
    }

    func testTokenWhileSignedOutExplainsInstead() {
        let model = createModel()
        model.receiveCredentialExchange(token: UUID())
        XCTAssertEqual(model.credentialImportMessage, AppModel.signedOutCredentialImportMessage)
        XCTAssertNil(model.pendingCredentialImportToken)
        XCTAssertFalse(model.isCredentialImportShown)
        XCTAssertTrue(allReceivedToken.isEmpty)
    }

    func testImportContinuesUntilDoneAndRefreshes() async {
        let model = await createUnlockedModel()
        await receivePreview(model)
        let replaceCountBefore = identityStore.replaceCount

        await model.startCredentialImport(isSkippingDuplicate: true)
        XCTAssertEqual(countCall("startImport"), 1)
        XCTAssertEqual(countCall("continueImport"), 2, "two batches of two save the three new logins")
        XCTAssertEqual(model.credentialImportProgress?.isDone, true)
        XCTAssertEqual(model.credentialImportProgress?.savedCount, 3)
        XCTAssertEqual(model.credentialImportProgress?.totalCount, 3)
        XCTAssertNil(model.errorMessage)
        XCTAssertFalse(model.isImportingCredentials)
        let allTitle = model.listAllItemReference(vaultID: "personal").map(\.item.title)
        XCTAssertTrue(["Docker", "Slack", "Figma"].allSatisfy(allTitle.contains), "the finished import lists the vault again")
        XCTAssertGreaterThan(identityStore.replaceCount, replaceCountBefore, "AutoFill gets the imported items")

        model.closeCredentialImport()
        XCTAssertFalse(model.isCredentialImportShown)
        await waitUntil("the finished import is dropped") { self.countCall("cancelImport") == 1 }
    }

    func testPausedImportRetries() async {
        let model = await createUnlockedModel()
        await receivePreview(model)
        backend.continueImportErrorMessage = "Hisar can't reach the server"

        await model.startCredentialImport(isSkippingDuplicate: false)
        XCTAssertEqual(model.errorMessage, "Hisar can't reach the server")
        XCTAssertEqual(model.credentialImportProgress?.isDone, false)
        XCTAssertEqual(model.credentialImportProgress?.savedCount, 0)

        await model.continueCredentialImport()
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(model.credentialImportProgress?.isDone, true)
        XCTAssertEqual(model.credentialImportProgress?.savedCount, 4)
    }

    func testChoosingAnotherVaultPreviewsAgain() async {
        let model = await createUnlockedModel()
        await receivePreview(model)
        XCTAssertEqual(model.allCredentialImportVault.map(\.id), ["personal", "family"])

        await model.previewCredentialImport(vaultID: "family")
        XCTAssertEqual(model.credentialImportPreview?.vaultID, "family")
        XCTAssertEqual(model.credentialImportPreview?.allDuplicate, [])
    }

    func testCancelDropsTheImport() async {
        let model = await createUnlockedModel()
        await receivePreview(model)

        model.closeCredentialImport()
        XCTAssertFalse(model.isCredentialImportShown)
        XCTAssertNil(model.credentialImportPreview)
        await waitUntil("the import is cancelled") { self.countCall("cancelImport") == 1 }
        XCTAssertNil(backend.allPendingImportTitle)
        XCTAssertEqual(countCall("startImport"), 0)
    }

    func testLockClosesTheImport() async {
        let model = await createUnlockedModel()
        await receivePreview(model)

        model.lock()
        XCTAssertFalse(model.isCredentialImportShown)
        XCTAssertNil(model.credentialImportPreview)
    }
}
