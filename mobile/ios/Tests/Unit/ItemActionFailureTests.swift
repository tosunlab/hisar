import XCTest
@testable import Hisar

@MainActor
final class ItemActionFailureTests: XCTestCase {
    private class Backend: FakeBackend {
        var failure = ""
        override func copyField(vaultID: String, itemID: String, fieldName: String) throws {
            if failure == "copy" { throw BackendError(message: "The clipboard is unavailable.") }
            try super.copyField(vaultID: vaultID, itemID: itemID, fieldName: fieldName)
        }
        override func convertToSecureNote(vaultID: String, itemID: String) throws -> ItemDetail {
            if failure == "convert" { throw BackendError(message: "The server is offline.") }
            return try super.convertToSecureNote(vaultID: vaultID, itemID: itemID)
        }
    }
    private func unlocked(_ backend: Backend) async -> AppModel {
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await model.start { backend }?.value
        return model
    }

    func testLoadedItemCopyFailurePreservesItemAndReportsError() async {
        let backend = Backend()
        let model = await unlocked(backend)
        let item = await model.loadItem(vaultID: "personal", itemID: "github")
        backend.failure = "copy"
        var presentedError: String?
        await model.copyField(vaultID: "personal", itemID: "github", fieldName: "password", label: "Password", onFailure: { presentedError = $0 })
        XCTAssertEqual(presentedError, "The clipboard is unavailable.")
        XCTAssertTrue(backend.allCopiedField.isEmpty)
        XCTAssertEqual(model.copyCount, 0)
        XCTAssertNil(model.noticeMessage)
        let stillLoaded = await model.loadItem(vaultID: "personal", itemID: "github")
        XCTAssertEqual(stillLoaded?.id, item?.id)
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(presentedError, "The clipboard is unavailable.", "a background success cannot clear the item-owned alert")
    }

    func testLoadedItemConversionFailurePreservesItemAndReportsError() async {
        let backend = Backend()
        let model = await unlocked(backend)
        let item = await model.loadItem(vaultID: "personal", itemID: "github")
        backend.failure = "convert"
        var presentedError: String?
        let result = await model.convertToSecureNote(vaultID: "personal", itemID: "github", onFailure: { presentedError = $0 })
        XCTAssertNil(result)
        XCTAssertEqual(presentedError, "The server is offline.")
        XCTAssertEqual(model.allItemByVaultID["personal"]?.first { $0.id == "github" }?.category, item?.category)
        XCTAssertNil(model.noticeMessage)
    }

    func testLoadedItemPasskeyDeletionFailurePreservesItemAndReportsError() async {
        let backend = Backend()
        let model = await unlocked(backend)
        let item = await model.loadItem(vaultID: "personal", itemID: "github")
        backend.deletePasskeyErrorMessage = "The server is offline."
        var presentedError: String?
        let result = await model.deletePasskey(vaultID: "personal", itemID: "github", credentialID: FakeBackend.gitHubPasskey.credentialID, onFailure: { presentedError = $0 })
        XCTAssertNil(result)
        XCTAssertEqual(presentedError, "The server is offline.")
        XCTAssertEqual(model.allItemByVaultID["personal"]?.first { $0.id == "github" }?.allPasskey, item?.allPasskey)
        XCTAssertEqual(backend.allPasskey.count, 1)
        XCTAssertNil(model.noticeMessage)
    }
}
