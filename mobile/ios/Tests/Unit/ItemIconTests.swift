import SwiftUI
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
        for (category, preset) in ["login": "lock", "credit-card": "card", "server": "server", "database": "database", "ssh-key": "key",
                                   "secure-note": "lock", "identity": "lock", "api-credential": "lock", "address": "lock", "software-license": "lock"] {
            XCTAssertEqual(ChosenIcon.applyingColor("green", to: nil, category: category), ChosenIcon(preset: preset, color: "green"))
        }
        let photo = ChosenIcon(image: Data([1, 2, 3]))
        XCTAssertEqual(ChosenIcon.applyingColor("green", to: photo, category: "login"), photo)
    }

    func testIconsRoundTripThroughTheItemJSON() throws {
        let decoded = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"title":"Bank","icon":{"preset":"bank","color":"green"}}"#.utf8))
        XCTAssertEqual(decoded.icon, ChosenIcon(preset: "bank", color: "green"))
        let withImage = ItemDetail(title: "GitHub", icon: ChosenIcon(image: Data([1, 2, 3])))
        let encoded = try JSONSerialization.jsonObject(with: JSONEncoder().encode(withImage)) as? [String: Any]
        XCTAssertEqual((encoded?["icon"] as? [String: Any])?["image"] as? String, "AQID", "images cross as base64, as Go's []byte")
        XCTAssertNil(try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"title":"Old"}"#.utf8)).icon)
        let colored = ItemDetail(title: "Login", icon: ChosenIcon.applyingColor("blue", to: nil, category: "login"))
        let reopened = try JSONDecoder().decode(ItemDetail.self, from: JSONEncoder().encode(colored))
        XCTAssertEqual(reopened.icon, ChosenIcon(preset: "lock", color: "blue"))
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
        do {
            _ = try await model.prepareItemIcon(Data([1]))
            XCTFail("Unreadable photos must fail")
        } catch {
            XCTAssertEqual(describeError(error), AppModel.unreadablePhotoMessage)
        }
        XCTAssertFalse(backend.allCallName.contains("prepareItemIcon"), "nothing UIKit can't read reaches the backend")

        let photo = try XCTUnwrap(encodeTestPhoto(type: .tiff, size: CGSize(width: 100, height: 100)))
        do {
            _ = try await model.prepareItemIcon(photo)
            XCTFail("Refused photos must fail")
        } catch {
            XCTAssertEqual(describeError(error), "choose a PNG, JPEG, GIF or WebP image")
        }
        backend.preparedIcon = Data([4, 5])
        let prepared = try await model.prepareItemIcon(photo)
        XCTAssertEqual(prepared, Data([4, 5]))
        XCTAssertNil(model.errorMessage, "the picker owns preparation errors")
    }

    func testDetailHeaderUsesFullWidthAtAccessibilitySizesAndCapsItsIcon() throws {
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        for width: CGFloat in [320, 393] {
            for size: DynamicTypeSize in [.large, .xxxLarge, .accessibility5] {
                let icon = ImageRenderer(content: ItemIcon(item: ItemDetail(title: "Bank"), size: 56, maxSize: 56).environment(\.dynamicTypeSize, size))
                XCTAssertEqual(try XCTUnwrap(icon.uiImage).size, CGSize(width: 56, height: 56))
                for title in ["GitHub", "Northwind's production account and recovery credentials"] {
                    let content = ItemDetailHeader(item: ItemDetail(title: title), subtitle: "Personal · Login")
                        .environmentObject(model)
                        .environment(\.dynamicTypeSize, size)
                        .frame(width: width)
                    let image = try XCTUnwrap(ImageRenderer(content: content).uiImage)
                    XCTAssertEqual(image.size.width, width)
                    if size.isAccessibilitySize {
                        let text = VStack(alignment: .leading, spacing: 2) {
                            Text(title).font(.title2.weight(.bold))
                            Text("Personal · Login").font(.subheadline)
                        }
                        .frame(width: width, alignment: .leading)
                        .environment(\.dynamicTypeSize, size)
                        let textHeight = try XCTUnwrap(ImageRenderer(content: text).uiImage).size.height
                        XCTAssertEqual(image.size.height, 56 + 14 + textHeight, accuracy: 1)
                    }
                    let attachment = XCTAttachment(image: image)
                    attachment.name = "detail-\(width)-\(size)-\(title.prefix(8))"
                    attachment.lifetime = .keepAlways
                    add(attachment)
                }
            }
        }
    }

    func testPresetGridFitsScaledTargetsAtBothPhoneWidths() throws {
        for width: CGFloat in [320, 393] {
            for size: DynamicTypeSize in [.large, .xxxLarge, .accessibility5] {
                let tile = ItemIcon(item: ItemDetail(icon: ChosenIcon(preset: "bank")), size: 40)
                    .environment(\.dynamicTypeSize, size)
                let tileWidth = max(44, try XCTUnwrap(ImageRenderer(content: tile).uiImage).size.width + 6)
                let gridWidth = width - 32
                let columns = floor((gridWidth + 8) / (tileWidth + 8))
                let rows = ceil(CGFloat(ChosenIcon.allPreset.count) / columns)
                let grid = IconPresetGrid(item: ItemDetail(), icon: ChosenIcon(preset: "bank"), color: "", onChoose: { _ in })
                    .frame(width: gridWidth)
                    .environment(\.dynamicTypeSize, size)
                let image = try XCTUnwrap(ImageRenderer(content: grid).uiImage)
                XCTAssertEqual(image.size.width, gridWidth)
                XCTAssertEqual(image.size.height, rows * tileWidth + (rows - 1) * 8 + 8, accuracy: rows)
                XCTAssertGreaterThanOrEqual((gridWidth - (columns - 1) * 8) / columns, tileWidth)
                let attachment = XCTAttachment(image: image)
                attachment.name = "presets-\(width)-\(size)"
                attachment.lifetime = .keepAlways
                add(attachment)
            }
        }
    }

    private func waitUntil(_ condition: () -> Bool) async throws {
        for _ in 0..<200 where !condition() {
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition())
    }
}
