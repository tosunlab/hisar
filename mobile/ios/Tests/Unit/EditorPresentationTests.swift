import SwiftUI
import UIKit
import XCTest
@testable import Hisar

@MainActor
final class EditorPresentationTests: XCTestCase {
    func testPersistentEditorLabelsKeepTheirSpaceWithPopulatedAndEmptyValues() throws {
        for size: DynamicTypeSize in [.large, .accessibility5] {
            for (label, value) in [("First name", "Alex"), ("City", "Alexandria"), ("Key ID", "review_key_2026"), ("Version", "2026.10"), ("Host", "server.example.com"), ("Database", "production")] {
                for text in ["", value] {
                    let content = Text(text).font(.body).editorFieldLabel(label)
                        .frame(width: 280)
                        .environment(\.dynamicTypeSize, size)
                    let image = try XCTUnwrap(ImageRenderer(content: content).uiImage)
                    let labelImage = try XCTUnwrap(ImageRenderer(content: Text(label).font(.footnote).frame(width: 280, alignment: .leading).environment(\.dynamicTypeSize, size)).uiImage)
                    let valueImage = try XCTUnwrap(ImageRenderer(content: Text(text).font(.body).frame(width: 280, alignment: .leading).environment(\.dynamicTypeSize, size)).uiImage)
                    XCTAssertEqual(image.size.height, labelImage.size.height + 4 + valueImage.size.height, accuracy: 1)
                }
            }
        }
    }

    func testPopulatedEditorsRenderLabeledIdentityAddressCredentialLicenseServerAndDatabaseFields() async throws {
        let backend = FakeBackend()
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await model.start { backend }?.value
        var identity = ItemIdentity()
        identity.firstName = "Alex"
        identity.middleName = "Taylor"
        identity.lastName = "Morgan"
        var credential = ItemAPICredential()
        credential.service = "Cloudflare"
        credential.keyID = "review_key_2026"
        credential.host = "api.cloudflare.com"
        credential.secret = "test-only-secret"
        let items = [
            ItemDetail(category: ItemDetail.categoryIdentity, title: "Identity", identity: identity),
            ItemDetail(category: ItemDetail.categoryAddress, title: "Address", address: ItemAddress(street: "12 Example Street", city: "Alexandria", region: "Virginia", postalCode: "22301", country: "United States")),
            ItemDetail(category: ItemDetail.categoryAPICredential, title: "Credential", apiCredential: credential),
            ItemDetail(category: ItemDetail.categorySoftwareLicense, title: "License", softwareLicense: ItemSoftwareLicense(product: "Editor", version: "2026.10", licenseKey: "TEST-KEY", licensedTo: "Alex Morgan", email: "alex@example.com")),
            ItemDetail(category: ItemDetail.categoryServer, title: "Server", server: ItemServer(host: "server.example.com", port: "22")),
            ItemDetail(category: ItemDetail.categoryDatabase, title: "Database", database: ItemDatabase(engine: "postgres", host: "db.example.com", port: "5432", name: "production", options: "sslmode=require")),
        ]
        let scene = try XCTUnwrap(UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first)
        for size: DynamicTypeSize in [.large, .accessibility5] {
            for item in items {
                let root = ItemEditorView(vaultID: "personal", originalItem: item) { _, _ in }
                    .environmentObject(model)
                    .environment(\.dynamicTypeSize, size)
                    .applyTheme(VaultTheme(appearance: .light, color: .neutral))
                let host = UIHostingController(rootView: root)
                let window = UIWindow(windowScene: scene)
                window.frame = CGRect(x: 0, y: 0, width: 393, height: 852)
                window.rootViewController = host
                window.makeKeyAndVisible()
                host.view.layoutIfNeeded()
                try await Task.sleep(nanoseconds: 150_000_000)
                let image = UIGraphicsImageRenderer(bounds: window.bounds).image { _ in
                    XCTAssertTrue(window.drawHierarchy(in: window.bounds, afterScreenUpdates: true))
                }
                let attachment = XCTAttachment(image: image)
                attachment.name = "populated-editor-\(item.category)-\(size)"
                attachment.lifetime = .keepAlways
                add(attachment)
                window.isHidden = true
            }
        }
    }
}
