import XCTest
@testable import Hisar

@MainActor
final class FavoritesTests: XCTestCase {
    private var backend: FakeBackend!

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await model.start { self.backend }?.value
        return model
    }

    func testNoFavoritesUntilAnItemIsStarred() async {
        let model = await createUnlockedModel()
        XCTAssertTrue(model.listAllFavoriteReference().isEmpty)
        XCTAssertTrue(backend.allCallName.contains("listAllFavoriteItem"), "a refresh reads the favorites again")
    }

    func testStarringListsFavoritesOfEveryVaultSortedWithTheirVault() async {
        let model = await createUnlockedModel()
        let isNetflixStarred = await model.setItemFavorite(vaultID: "family", itemID: "netflix", isFavorite: true)
        let isGitHubStarred = await model.setItemFavorite(vaultID: "personal", itemID: "github", isFavorite: true)
        XCTAssertTrue(isNetflixStarred && isGitHubStarred)
        XCTAssertEqual(model.listAllFavoriteReference().map(\.item.title), ["GitHub", "Netflix"])
        XCTAssertEqual(model.listAllFavoriteReference().map(\.vaultName), ["Personal", "Family"])
        XCTAssertEqual(model.noticeMessage, "Added to Favorites.")
        XCTAssertEqual(backend.allFavoriteRecordID, ["family/netflix", "personal/github"])
        XCTAssertEqual(model.listAllItemReference(vaultID: "personal").map(\.item.isFavorite), [false, true])
    }

    func testUnstarringRemovesTheFavorite() async {
        backend.allFavoriteRecordID = ["personal/bank"]
        let model = await createUnlockedModel()
        XCTAssertEqual(model.listAllFavoriteReference().map(\.item.title), ["Bank"])
        await model.setItemFavorite(vaultID: "personal", itemID: "bank", isFavorite: false)
        XCTAssertTrue(model.listAllFavoriteReference().isEmpty)
        XCTAssertEqual(model.noticeMessage, "Removed from Favorites.")
    }

    func testAFailedStarKeepsTheListAndShowsTheError() async {
        let model = await createUnlockedModel()
        backend.favoriteErrorMessage = "cannot reach the Hisar server; changes can be saved once you are back online"
        var presentedError: String?
        let isStarred = await model.setItemFavorite(vaultID: "personal", itemID: "github", isFavorite: true, onFailure: { presentedError = $0 })
        XCTAssertFalse(isStarred)
        XCTAssertTrue(model.listAllFavoriteReference().isEmpty)
        XCTAssertEqual(model.errorMessage, "cannot reach the Hisar server; changes can be saved once you are back online")
        XCTAssertEqual(presentedError, model.errorMessage)
        XCTAssertNil(model.noticeMessage)
        _ = await model.loadItem(vaultID: "personal", itemID: "github")
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(presentedError, "cannot reach the Hisar server; changes can be saved once you are back online")
    }

    func testADeletedFavoriteLeavesTheList() async {
        backend.allFavoriteRecordID = ["personal/bank", "personal/github"]
        let model = await createUnlockedModel()
        await model.deleteItem(vaultID: "personal", itemID: "bank")
        XCTAssertEqual(model.listAllFavoriteReference().map(\.item.title), ["GitHub"])
    }

    func testAStarFromAnotherDeviceShowsAfterTheRemoteChange() async {
        let model = await createUnlockedModel()
        backend.allFavoriteRecordID = ["family/netflix"]
        await model.refreshAll()
        XCTAssertEqual(model.listAllFavoriteReference().map(\.item.title), ["Netflix"])
    }

    func testLockingForgetsFavorites() async {
        backend.allFavoriteRecordID = ["personal/github"]
        let model = await createUnlockedModel()
        model.lock()
        XCTAssertTrue(model.listAllFavoriteReference().isEmpty)
    }

    func testTheItemDetailCarriesTheStar() async {
        backend.allFavoriteRecordID = ["personal/github"]
        let model = await createUnlockedModel()
        let item = await model.loadItem(vaultID: "personal", itemID: "github")
        XCTAssertEqual(item?.isFavorite, true)
    }

    func testFavoriteItemDecodesTheBackendJSON() throws {
        let json = #"[{"vaultId":"personal","vaultName":"Personal","vaultType":"personal","item":{"id":"github","title":"GitHub","isFavorite":true,"urls":null}}]"#
        let allFavorite = try JSONDecoder().decode([FavoriteItem].self, from: Data(json.utf8))
        XCTAssertEqual(allFavorite.first?.vaultID, "personal")
        XCTAssertEqual(allFavorite.first?.item.isFavorite, true)
        XCTAssertEqual(allFavorite.first?.id, "personal/github")
        let older = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"id":"bank","title":"Bank"}"#.utf8))
        XCTAssertFalse(older.isFavorite, "an item from an older backend is not a favorite")
    }
}
