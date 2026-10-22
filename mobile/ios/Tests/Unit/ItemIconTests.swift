import UIKit
import UniformTypeIdentifiers
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
        await model.start { self.backend }?.value
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

    /// encodeTestPhoto draws a photo-sized image and encodes it as the given type, as a photo library might hold it.
    private func encodeTestPhoto(type: UTType, size: CGSize = CGSize(width: 4032, height: 3024), isOpaque: Bool = true) -> Data? {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = isOpaque
        let image = UIGraphicsImageRenderer(size: size, format: format).image { context in
            UIColor.systemTeal.setFill()
            context.fill(CGRect(x: 0, y: 0, width: size.width / 2, height: size.height))
        }
        guard let cgImage = image.cgImage else { return nil }
        let data = NSMutableData()
        guard let destination = CGImageDestinationCreateWithData(data, type.identifier as CFString, 1, nil) else { return nil }
        CGImageDestinationAddImage(destination, cgImage, nil)
        return CGImageDestinationFinalize(destination) ? data as Data : nil
    }

    func testPhotosInFormatsTheBackendDoesNotReadBecomeJPEGOrPNG() throws {
        let tiff = try XCTUnwrap(encodeTestPhoto(type: .tiff))
        let converted = try XCTUnwrap(AppModel.convertPhotoForIcon(tiff))
        XCTAssertEqual(Array(converted.prefix(3)), [0xFF, 0xD8, 0xFF], "an opaque photo becomes a JPEG")
        let image = try XCTUnwrap(UIImage(data: converted))
        XCTAssertEqual(image.size, CGSize(width: 512, height: 384))

        let transparent = try XCTUnwrap(encodeTestPhoto(type: .png, size: CGSize(width: 64, height: 64), isOpaque: false))
        let png = try XCTUnwrap(AppModel.convertPhotoForIcon(transparent))
        XCTAssertEqual(Array(png.prefix(4)), [0x89, 0x50, 0x4E, 0x47], "transparency keeps a PNG")
        XCTAssertEqual(UIImage(data: png)?.size, CGSize(width: 64, height: 64), "small images are not scaled up")

        // Camera photos are HEIC; the simulator encodes HEIC where the Mac's hardware does.
        if let heic = encodeTestPhoto(type: .heic) {
            XCTAssertNotNil(AppModel.convertPhotoForIcon(heic).flatMap { UIImage(data: $0) })
        }
        XCTAssertNil(AppModel.convertPhotoForIcon(Data("not a photo".utf8)))
    }

    func testPreparingAPhotoShowsWhyItWasRefused() async throws {
        let model = await createUnlockedModel()
        let unreadable = await model.prepareItemIcon(Data([1]))
        XCTAssertNil(unreadable)
        XCTAssertEqual(model.errorMessage, AppModel.unreadablePhotoMessage)
        XCTAssertFalse(backend.allCallName.contains("prepareItemIcon"), "nothing UIKit can't read reaches the backend")

        let photo = try XCTUnwrap(encodeTestPhoto(type: .tiff, size: CGSize(width: 100, height: 100)))
        let refused = await model.prepareItemIcon(photo)
        XCTAssertNil(refused)
        XCTAssertEqual(model.errorMessage, "choose a PNG, JPEG, GIF or WebP image")
        backend.preparedIcon = Data([4, 5])
        let prepared = await model.prepareItemIcon(photo)
        XCTAssertEqual(prepared, Data([4, 5]))
    }

    private func waitUntil(_ condition: () -> Bool) async throws {
        for _ in 0..<200 where !condition() {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition())
    }
}
