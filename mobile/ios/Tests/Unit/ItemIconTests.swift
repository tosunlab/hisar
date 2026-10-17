import UIKit
import XCTest
@testable import Hisar

@MainActor
final class ItemIconTests: XCTestCase {
    private var backend: FakeBackend!

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore(), getCurrentDate: { Date(timeIntervalSince1970: 1_800_000_000) })
        model.start { self.backend }
        await model.refreshAll()
        return model
    }

    private func countCalls(_ name: String) -> Int {
        backend.allCallName.filter { $0 == name }.count
    }

    func testEveryPresetHasASymbolAndEveryColorAHue() {
        XCTAssertEqual(ChosenIcon.allPreset.count, 30)
        for preset in ChosenIcon.allPreset {
            let symbol = ChosenIcon(preset: preset).symbol
            XCTAssertNotNil(symbol.flatMap { UIImage(systemName: $0) }, "preset \(preset) has no SF Symbol")
        }
        XCTAssertNil(ChosenIcon(preset: "rocket").symbol, "a preset from a newer app draws the category symbol")
        XCTAssertNil(ChosenIcon(preset: "bank", image: Data([1])).symbol, "an image wins over its preset")
        for color in ChosenIcon.allColor {
            XCTAssertNotNil(ItemIcon.getColor(color), "color \(color)")
        }
        XCTAssertNil(ItemIcon.getColor(""))
    }

    func testIconsRoundTripThroughTheItemJSON() throws {
        let decoded = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"title":"Bank","icon":{"preset":"bank","color":"green"}}"#.utf8))
        XCTAssertEqual(decoded.icon, ChosenIcon(preset: "bank", color: "green"))
        let withImage = ItemDetail(title: "GitHub", icon: ChosenIcon(image: Data([1, 2, 3])))
        let encoded = try JSONSerialization.jsonObject(with: JSONEncoder().encode(withImage)) as? [String: Any]
        XCTAssertEqual((encoded?["icon"] as? [String: Any])?["image"] as? String, "AQID", "images cross as base64, as Go's []byte")
        XCTAssertNil(try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"title":"Old"}"#.utf8)).icon)
    }

    func testOnlyLoginsWithAnHTTPSWebsiteAndNoIconShowAWebsiteIcon() {
        XCTAssertEqual(ItemDetail(allURL: ["github.com", " https://github.com/login "]).websiteIconURL, "https://github.com/login")
        XCTAssertNil(ItemDetail(allURL: ["http://github.com"]).websiteIconURL)
        XCTAssertNil(ItemDetail(allURL: ["https://github.com"], icon: ChosenIcon(preset: "code")).websiteIconURL)
        XCTAssertNil(ItemDetail(category: ItemDetail.categoryServer, allURL: ["https://admin.example.com"]).websiteIconURL)
    }

    func testWebsiteIconsLoadOncePerDomainAndFollowTheSetting() async throws {
        backend.websiteIconByURL = ["https://github.com/login": Data([9]), "https://github.com/": Data([9])]
        let model = await createUnlockedModel()
        model.loadWebsiteIcon(url: "https://github.com/login")
        model.loadWebsiteIcon(url: "https://github.com/")
        try await waitUntil { model.getWebsiteIcon(url: "https://github.com/") != nil }
        XCTAssertEqual(model.getWebsiteIcon(url: "https://GitHub.com/settings"), Data([9]))
        XCTAssertEqual(countCalls("getWebsiteIcon"), 1)

        model.lock()
        XCTAssertNil(model.getWebsiteIcon(url: "https://github.com/"), "locking forgets the icons")

        backend.seedSignedIn()
        await model.refreshAll()
        await model.setWebsiteIconsEnabled(false)
        XCTAssertFalse(model.isWebsiteIconsEnabled)
        XCTAssertFalse(backend.isWebsiteIconsEnabledValue)
        model.loadWebsiteIcon(url: "https://github.com/")
        XCTAssertEqual(countCalls("getWebsiteIcon"), 1, "nothing is asked with website icons off")
    }

    func testPreparingAPhotoShowsWhyItWasRefused() async {
        let model = await createUnlockedModel()
        let refused = await model.prepareItemIcon(Data([1]))
        XCTAssertNil(refused)
        XCTAssertEqual(model.errorMessage, "choose a PNG, JPEG, GIF or WebP image")
        backend.preparedIcon = Data([4, 5])
        let prepared = await model.prepareItemIcon(Data([1]))
        XCTAssertEqual(prepared, Data([4, 5]))
    }

    private func waitUntil(_ condition: () -> Bool) async throws {
        for _ in 0..<200 where !condition() {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition())
    }
}
