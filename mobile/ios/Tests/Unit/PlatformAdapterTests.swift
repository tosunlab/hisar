import UIKit
import XCTest
@testable import Hisar

final class KeychainSecretKeyStoreTests: XCTestCase {
    private let store = KeychainSecretKeyStore(service: "com.tryhisar.hisar.tests.secret-key", accessGroup: HisarConfiguration.current.keychainAccessGroup)

    override func tearDown() {
        store.deleteAllSecretKey()
    }

    func testStoreLoadDelete() throws {
        try store.storeSecretKey("ACCOUNT1", secretKey: "H1-ACCOUNT1-AAAAA")
        try store.storeSecretKey("ACCOUNT1", secretKey: "H1-ACCOUNT1-BBBBB")
        var error: NSError?
        XCTAssertEqual(store.loadSecretKey("ACCOUNT1", error: &error), "H1-ACCOUNT1-BBBBB")
        XCTAssertNil(error)
        try store.deleteSecretKey("ACCOUNT1")
        XCTAssertEqual(store.loadSecretKey("ACCOUNT1", error: &error), "")
        XCTAssertNotNil(error, "a missing Secret Key is an error for Go")
        XCTAssertNoThrow(try store.deleteSecretKey("ACCOUNT1"), "deleting twice is fine")
    }

    func testSharedAccessGroupIsUsedInTheSimulator() throws {
        XCTAssertNotNil(HisarConfiguration.current.keychainAccessGroup)
        XCTAssertTrue(HisarConfiguration.current.keychainAccessGroup?.hasSuffix("com.tryhisar.hisar.shared") ?? false)
        try store.storeSecretKey("ACCOUNT2", secretKey: "H1-ACCOUNT2")
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "com.tryhisar.hisar.tests.secret-key",
            kSecAttrAccount as String: "ACCOUNT2",
            kSecReturnAttributes as String: true,
        ]
        var result: AnyObject?
        XCTAssertEqual(SecItemCopyMatching(query as CFDictionary, &result), errSecSuccess)
        let attributes = result as? [String: Any]
        XCTAssertEqual(attributes?[kSecAttrAccessGroup as String] as? String, HisarConfiguration.current.keychainAccessGroup)
        XCTAssertEqual(attributes?[kSecAttrAccessible as String] as? String, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly as String)
    }
}

final class HisarConfigurationTests: XCTestCase {
    func testAppGroupContainerHoldsTheState() throws {
        let configuration = HisarConfiguration.current
        XCTAssertEqual(configuration.appGroupIdentifier, "group.com.tryhisar.hisar")
        let stateDirectory = try configuration.prepareStateDirectory()
        XCTAssertTrue(stateDirectory.path.hasSuffix("Library/Application Support/Hisar"))
        if let container = configuration.getAppGroupContainer() {
            XCTAssertTrue(stateDirectory.path.hasPrefix(container.path))
        }
    }

    func testPersonalTeamFallsBackToApplicationSupport() throws {
        let configuration = HisarConfiguration(appGroupIdentifier: nil, keychainAccessGroup: nil)
        XCTAssertFalse(configuration.isAppGroupAvailable)
        let stateDirectory = try configuration.prepareStateDirectory()
        let applicationSupport = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: false)
        XCTAssertEqual(stateDirectory.standardizedFileURL, applicationSupport.appendingPathComponent("Hisar", isDirectory: true).standardizedFileURL)
        XCTAssertEqual(try stateDirectory.resourceValues(forKeys: [.isExcludedFromBackupKey]).isExcludedFromBackup, true)
    }
}

@MainActor
final class PasteboardClipboardTests: XCTestCase {
    func testCopiedTextIsReportedUntilReplaced() {
        let pasteboard = UIPasteboard.withUniqueName()
        defer { UIPasteboard.remove(withName: pasteboard.name) }
        let clipboard = PasteboardClipboard(pasteboard: pasteboard)
        XCTAssertEqual(clipboard.getText(), "")
        XCTAssertTrue(clipboard.setText("s3cret"))
        XCTAssertEqual(pasteboard.string, "s3cret")
        XCTAssertEqual(clipboard.getText(), "s3cret")
        pasteboard.string = "something else"
        XCTAssertEqual(clipboard.getText(), "", "Hisar never reads text another app copied")
    }

    func testEmptyTextClearsThePasteboard() {
        let pasteboard = UIPasteboard.withUniqueName()
        defer { UIPasteboard.remove(withName: pasteboard.name) }
        let clipboard = PasteboardClipboard(pasteboard: pasteboard)
        _ = clipboard.setText("s3cret")
        _ = clipboard.setText("")
        XCTAssertFalse(pasteboard.hasStrings)
    }

    func testSetTextFromABackgroundThread() async {
        let pasteboard = UIPasteboard.withUniqueName()
        defer { UIPasteboard.remove(withName: pasteboard.name) }
        let clipboard = PasteboardClipboard(pasteboard: pasteboard)
        let isSet = try? await runInBackground { clipboard.setText("from Go") }
        XCTAssertEqual(isSet, true)
        XCTAssertEqual(pasteboard.string, "from Go")
    }
}

final class BiometricKeychainTests: XCTestCase {
    func testDeleteMissingKeyIsFine() {
        let biometric = BiometricKeychain(service: "com.tryhisar.hisar.tests.biometric", accessGroup: HisarConfiguration.current.keychainAccessGroup)
        XCTAssertNoThrow(try biometric.deleteKey("NOBODY"))
        XCTAssertThrowsError(try biometric.loadKey("NOBODY"))
        XCTAssertFalse(biometric.getBiometryName().isEmpty)
    }
}

final class HisarKitBackendTests: XCTestCase {
    func testGoBackendStartsSignedOutAndGeneratesPasswords() throws {
        let stateDirectory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: stateDirectory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: stateDirectory) }
        let backend = try HisarKitBackend(stateDirectory: stateDirectory, secretKeyStore: KeychainSecretKeyStore(service: "com.tryhisar.hisar.tests.go", accessGroup: nil), biometric: nil, clipboard: nil, eventListener: nil)
        XCTAssertEqual(backend.getStatus().state, VaultStatus.stateSignedOut)
        XCTAssertEqual(try backend.generatePassword(length: 30, isDigitIncluded: true, isSymbolIncluded: false).count, 30)
        XCTAssertThrowsError(try backend.listAllVault()) { error in
            XCTAssertFalse(describeError(error).isEmpty)
            XCTAssertEqual(describeError(error).first?.isUppercase, true)
        }
        XCTAssertThrowsError(try backend.signIn(server: "http://127.0.0.1:1", email: "a@example.com", secretKey: "H1-NOT-VALID", password: "x"))
    }
}
