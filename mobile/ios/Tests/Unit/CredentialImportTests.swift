import AuthenticationServices
import XCTest
@testable import Hisar

@MainActor
final class CredentialImportTests: XCTestCase {
    private var backend: FakeBackend!
    private var identityStore: FakeCredentialIdentityStore!
    private var allReceivedToken: [UUID] = []
    /// What the other app exported; FakeBackend reads it as the titles of the logins to import, or as one list of titles
    /// per collection when allExportedCollectionTitle is set.
    private var allExportedTitle = ["GitHub", "Docker", "Slack", "Figma"]
    private var allExportedCollectionTitle: [[String]]?

    override func setUp() async throws {
        backend = FakeBackend()
        identityStore = FakeCredentialIdentityStore()
        allReceivedToken = []
    }

    private func createModel() async -> AppModel {
        let model = AppModel(credentialIdentityStore: identityStore, receiveExportedCredentials: { [unowned self] token in
            self.allReceivedToken.append(token)
            return try self.allExportedCollectionTitle.map { try JSONEncoder().encode($0) } ?? JSONEncoder().encode(self.allExportedTitle)
        })
        await model.start { self.backend }?.value
        return model
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        return await createModel()
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

    /// Writes a throwaway file the file picker would hand to importHisarExport; FakeBackend ignores its bytes.
    private func makeExportFile(_ name: String = "export.hisar") -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("\(UUID().uuidString)-\(name)")
        try! Data("{\"format\":\"hisar-encrypted-export\"}".utf8).write(to: url)
        return url
    }

    func testPreviewCountsOmissionsRatherThanRejectedItems() {
        let preview = ImportPreview(fileName: "Passwords", vaultID: "personal", vaultName: "Personal", itemCount: 1,
            loginCount: 1, passkeyCount: 1, allSkipped: [
                ImportSkippedEntry(title: "GitHub", reason: "Passkey extension data is not supported."),
                ImportSkippedEntry(title: "GitHub", reason: "Provider associations are not imported."),
            ])
        XCTAssertEqual(preview.itemCount, 1)
        XCTAssertEqual(preview.loginCount, 1)
        XCTAssertEqual(preview.passkeyCount, 1)
        XCTAssertEqual(ImportSkippedEntry.heading(count: preview.allSkipped.count), "Not imported · 2")
        XCTAssertEqual(ImportSkippedEntry.explanation, "Some entries below are fields or credentials from items that will still be imported.")
        XCTAssertEqual(preview.allSkipped.map(\.title), ["GitHub", "GitHub"])
        XCTAssertEqual(preview.allSkipped.last?.reason, "Provider associations are not imported.")
    }

    func testExportDatesReachTheBackendAsSecondsSince1970() throws {
        let now = Date(timeIntervalSince1970: 1_791_547_200)
        let exporterSeconds = 1_699_200_000.0
        let readAgainst2001 = Date(timeIntervalSinceReferenceDate: exporterSeconds)
        let readAgainst1970 = Date(timeIntervalSince1970: exporterSeconds)
        for created in [readAgainst2001, readAgainst1970] {
            let item = ASImportableItem(id: Data("item".utf8), created: created, lastModified: created, title: "amazon.com", credentials: [])
            let account = ASImportableAccount(id: Data("account".utf8), userName: "alper", email: "alper@example.com", collections: [], items: [item])
            let exported = ASExportedCredentialData(accounts: [account], formatVersion: .v1, exporterRelyingPartyIdentifier: "example.com", exporterDisplayName: "Passwords", timestamp: created)
            let json = try JSONSerialization.jsonObject(with: AppModel.makeCredentialExchangeEncoder(now: now).encode(exported)) as? [String: Any]
            let encodedItem = ((json?["accounts"] as? [[String: Any]])?.first?["items"] as? [[String: Any]])?.first
            XCTAssertEqual(encodedItem?["creationAt"] as? Double, exporterSeconds, "created \(created)")
            XCTAssertEqual(encodedItem?["modifiedAt"] as? Double, exporterSeconds, "modified \(created)")
            XCTAssertEqual(json?["timestamp"] as? Double, exporterSeconds, "exported \(created)")
        }
    }

    func testTokenWaitsForUnlockAndPreparesOnce() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = await createModel()
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

    func testTokenWhileSignedOutExplainsInstead() async {
        let model = await createModel()
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
        XCTAssertEqual(model.credentialImportPreview?.groups.first?.vaultID, "family")
        XCTAssertEqual(model.credentialImportPreview?.groups.first?.members.map(\.email), ["bob@example.com", "alice@example.com"])
    }

    func testEachCollectionGoesToTheVaultChosenForIt() async {
        allExportedCollectionTitle = [["Docker", "Slack"], ["Figma"]]
        let model = await createUnlockedModel()
        await receivePreview(model)
        XCTAssertEqual(model.credentialImportPreview?.groups.map(\.vaultID), ["personal", "personal"], "every collection starts in the personal vault")

        await model.previewCredentialImport(vaultID: "family")
        XCTAssertEqual(model.credentialImportPreview?.groups.map(\.vaultID), ["personal", "personal"], "one vault is never chosen for every collection at once")

        await model.mapCredentialImportGroup(groupID: "group-1", vaultID: "family")
        XCTAssertEqual(model.credentialImportPreview?.groups.map(\.vaultID), ["personal", "family"])
        XCTAssertEqual(model.credentialImportPreview?.groups.last?.members.map(\.email), ["bob@example.com", "alice@example.com"], "the shared vault's members are shown")
        XCTAssertEqual(model.credentialImportPreview?.vaultID, "")

        await model.startCredentialImport(isSkippingDuplicate: true)
        XCTAssertEqual(model.credentialImportProgress?.isDone, true)
        XCTAssertEqual(model.credentialImportProgress?.savedCount, 3)
        XCTAssertEqual(backend.allItemByVaultID["family"]?.map(\.title).filter { ["Docker", "Slack", "Figma"].contains($0) }, ["Figma"])
        XCTAssertEqual(Set(backend.allItemByVaultID["personal"]?.map(\.title) ?? []).intersection(["Docker", "Slack", "Figma"]), ["Docker", "Slack"])
    }

    func testCancelDropsTheImport() async {
        let model = await createUnlockedModel()
        await receivePreview(model)

        model.closeCredentialImport()
        XCTAssertFalse(model.isCredentialImportShown)
        XCTAssertNil(model.credentialImportPreview)
        await waitUntil("the import is cancelled") { self.countCall("cancelImport") == 1 }
        XCTAssertNil(backend.allPendingImportGroup)
        XCTAssertEqual(countCall("startImport"), 0)
    }

    func testLockClosesTheImport() async {
        let model = await createUnlockedModel()
        await receivePreview(model)

        model.lock()
        XCTAssertFalse(model.isCredentialImportShown)
        XCTAssertNil(model.credentialImportPreview)
    }

    func testImportingAnEncryptedHisarExportAsksForItsPassphrase() async {
        let model = await createUnlockedModel()
        await model.importHisarExport(from: makeExportFile())

        XCTAssertTrue(model.isCredentialImportShown)
        XCTAssertTrue(model.isHisarExportImport)
        XCTAssertEqual(model.credentialImportPreview?.isPassphraseRequired, true)
        XCTAssertNil(model.credentialImportProgress)
        XCTAssertEqual(countCall("prepareHisarExportImport"), 1)
        XCTAssertEqual(countCall("openImportFile"), 0)
    }

    func testAWrongExportPassphraseKeepsTheExportWaiting() async {
        let model = await createUnlockedModel()
        await model.importHisarExport(from: makeExportFile())

        await model.openCredentialImportFile(passphrase: "not the one")
        XCTAssertEqual(model.errorMessage, "that passphrase doesn't open this export")
        XCTAssertEqual(model.credentialImportPreview?.isPassphraseRequired, true, "the export still waits for the right passphrase")
        XCTAssertNil(model.credentialImportProgress)

        await model.openCredentialImportFile(passphrase: backend.hisarExportPassphrase)
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(model.credentialImportPreview?.isPassphraseRequired, false)
        XCTAssertEqual(model.credentialImportPreview?.itemCount, 3)
    }

    func testAPlainHisarExportPreviewsAtOnce() async {
        backend.hisarExportPreview = FakeBackend.plainHisarExportPreview()
        let model = await createUnlockedModel()
        await model.importHisarExport(from: makeExportFile("export.json"))

        XCTAssertTrue(model.isCredentialImportShown)
        XCTAssertEqual(model.credentialImportPreview?.isPassphraseRequired, false)
        XCTAssertEqual(model.credentialImportPreview?.itemCount, 3)
        XCTAssertEqual(model.credentialImportPreview?.groups.map(\.name), ["Personal"])
        XCTAssertEqual(countCall("openImportFile"), 0)
    }
}
