import UIKit
import XCTest
@testable import Hisar

@MainActor
final class SettingsModelTests: XCTestCase {
    private var backend: FakeBackend!
    private var currentDate = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUp() async throws {
        backend = FakeBackend()
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore(), getCurrentDate: { [unowned self] in currentDate })
        model.start { self.backend }
        await model.refreshAll()
        return model
    }

    private func createApproval(requestID: String) -> PhoneApproval {
        PhoneApproval(
            requestID: requestID, requesterDeviceName: "MacBook Pro", expiresAt: ISO8601DateFormatter().string(from: currentDate.addingTimeInterval(60)),
            kind: PhoneApproval.kindSSH, appName: "Terminal", purpose: "push to github.com", keyName: "GitHub deploy key"
        )
    }

    func testAutoLockSettingChangesTheBackgroundLock() async {
        let model = await createUnlockedModel()
        XCTAssertEqual(model.autoLockInterval, 300, "5 minutes until the user picks another delay")
        let isThirtySecondsSet = await model.setAutoLock(seconds: 30)
        XCTAssertTrue(isThirtySecondsSet)
        XCTAssertEqual(model.status.autoLockSeconds, 30)
        model.handleDidEnterBackground()
        currentDate += 31
        model.handleWillEnterForeground()
        XCTAssertEqual(model.screen, .locked)
    }

    func testThemeStartsFromTheBackendAndIsSaved() async {
        backend.theme = VaultTheme(appearance: .dark, color: .purple)
        let model = await createUnlockedModel()
        XCTAssertEqual(model.theme, VaultTheme(appearance: .dark, color: .purple))
        await model.setTheme(VaultTheme(appearance: .light, color: .green))
        XCTAssertEqual(model.theme, VaultTheme(appearance: .light, color: .green))
        XCTAssertEqual(backend.theme, VaultTheme(appearance: .light, color: .green))
        XCTAssertTrue(backend.allCallName.contains("setTheme"))
    }

    func testThemeGoesBackWhenSavingFails() async {
        let model = await createUnlockedModel()
        backend.isThemeSaveFailing = true
        await model.setTheme(VaultTheme(appearance: .dark, color: .blue))
        XCTAssertEqual(model.theme, .standard)
        XCTAssertEqual(model.errorMessage, "Could not save hisar.json")
    }

    func testThemeDecodesAndFallsBackToTheDefaults() throws {
        let theme = try JSONDecoder().decode(VaultTheme.self, from: Data(#"{"appearance":"light","color":"blue"}"#.utf8))
        XCTAssertEqual(theme, VaultTheme(appearance: .light, color: .blue))
        let unknownTheme = try JSONDecoder().decode(VaultTheme.self, from: Data(#"{"appearance":"sepia"}"#.utf8))
        XCTAssertEqual(unknownTheme, .standard)
        XCTAssertNil(VaultTheme.standard.colorScheme)
        XCTAssertEqual(VaultTheme(appearance: .dark, color: .neutral).userInterfaceStyle, .dark)
    }

    func testEveryColourHasAPalette() {
        let allTint = ThemeColorName.allCases.map { ThemePalette.find(color: $0).tint.light }
        XCTAssertEqual(allTint, [0x1D2025, 0x2356C4, 0x6A3DCC, 0x1B7348])
        let darkTint = ThemePalette.blue.tint.uiColor.resolvedColor(with: UITraitCollection(userInterfaceStyle: .dark))
        XCTAssertEqual(darkTint, UIColor(hexadecimal: 0x7EA6F8))
    }

    func testAutoLockRefusesOtherDelays() async {
        let model = await createUnlockedModel()
        let isOddDelaySet = await model.setAutoLock(seconds: 45)
        XCTAssertFalse(isOddDelaySet)
        XCTAssertNotNil(model.errorMessage)
        XCTAssertEqual(model.status.autoLockSeconds, 300)
    }

    func testAutoLockStatusDecodesAndDescribes() throws {
        let status = try JSONDecoder().decode(VaultStatus.self, from: Data(#"{"state":"unlocked","autoLockSeconds":600}"#.utf8))
        XCTAssertEqual(status.autoLockSeconds, 600)
        let oldStatus = try JSONDecoder().decode(VaultStatus.self, from: Data(#"{"state":"locked"}"#.utf8))
        XCTAssertEqual(oldStatus.autoLockSeconds, VaultStatus.defaultAutoLockSeconds)
        XCTAssertEqual(VaultStatus.allAutoLockSeconds.map { VaultStatus.describeAutoLock(seconds: $0) }, ["30 seconds", "1 minute", "5 minutes", "10 minutes", "1 hour"])
        XCTAssertEqual(VaultStatus.describeShortAutoLock(seconds: 300), "5 min")
    }

    func testDevicesAreListedAndAnotherOneSignsOut() async {
        backend.allDevice = [
            Device(id: "this", name: "Hisar on iPhone", platform: "ios/mobile", lastSeenAt: "2027-01-15T08:00:00Z", isCurrent: true),
            Device(id: "mac", name: "MacBook Pro", platform: "darwin/desktop", lastSeenAt: "2027-01-14T08:00:00Z"),
        ]
        let model = await createUnlockedModel()
        let allDevice = await model.loadAllDevice()
        XCTAssertEqual(allDevice?.map(\.id), ["this", "mac"])
        let isMacSignedOut = await model.revokeDevice(backend.allDevice[1])
        XCTAssertTrue(isMacSignedOut)
        XCTAssertEqual(backend.allDevice.map(\.id), ["this"])
        XCTAssertEqual(model.noticeMessage, "MacBook Pro is signed out.")
        let isThisPhoneSignedOut = await model.revokeDevice(backend.allDevice[0])
        XCTAssertFalse(isThisPhoneSignedOut, "this iPhone signs out from Settings instead")
    }

    func testDeviceDescribesItsPlatform() throws {
        let device = try JSONDecoder().decode(Device.self, from: Data(#"{"id":"1","name":"build","platform":"linux/cli","lastSeenAt":"2027-01-15T08:00:00Z","isCurrent":false}"#.utf8))
        XCTAssertEqual(device.platformDescription, "Command line on Linux")
        XCTAssertEqual(device.systemImage, "apple.terminal")
        XCTAssertNotNil(device.lastSeenDate)
        XCTAssertEqual(Device(id: "2", name: "Mac", platform: "darwin/desktop").platformDescription, "macOS app")
        XCTAssertEqual(Device(id: "3", name: "iPhone", platform: "ios/mobile").systemImage, "iphone")
    }

    func testEmergencyKitNeedsTheMasterPasswordAndIsRemoved() async throws {
        let model = await createUnlockedModel()
        let wrongExport = await model.exportEmergencyKit(password: "wrong")
        XCTAssertNil(wrongExport)
        XCTAssertEqual(model.errorMessage, "Incorrect master password.")
        let possibleExport = await model.exportEmergencyKit(password: FakeBackend.password)
        let export = try XCTUnwrap(possibleExport)
        XCTAssertEqual(export.secretKey, FakeBackend.secretKey)
        XCTAssertTrue(FileManager.default.fileExists(atPath: export.emergencyKitPath))
        XCTAssertTrue(export.emergencyKitPath.hasPrefix(FileManager.default.temporaryDirectory.path), "the kit is written to the app's temporary folder")
        model.removeEmergencyKit(export)
        XCTAssertFalse(FileManager.default.fileExists(atPath: export.emergencyKitPath))
    }

    func testCopyingCountsForHapticsAndCopiesReferences() async {
        let model = await createUnlockedModel()
        await model.copyField(vaultID: "personal", itemID: "github", fieldName: "password", label: "Password")
        XCTAssertEqual(model.copyCount, 1)
        await model.copySecretReference(vaultID: "personal", itemID: "github", fieldName: "password")
        XCTAssertEqual(model.copyCount, 2)
        XCTAssertEqual(backend.allCopiedReference, ["hisar://Personal/GitHub/password"])
        XCTAssertEqual(model.noticeMessage, "Secret reference copied.")
    }

    func testTurningOffMacApprovalsHidesTheRequests() async {
        backend.seedSignedIn()
        backend.allPendingApproval = [createApproval(requestID: "first")]
        let model = await createUnlockedModel()
        await model.refreshPendingApproval()
        XCTAssertEqual(model.allPendingApproval.count, 1)
        let isTurnedOff = await model.setMacApprovalAllowed(false)
        XCTAssertTrue(isTurnedOff)
        XCTAssertFalse(model.isMacApprovalAllowed)
        XCTAssertFalse(backend.isMacApprovalAllowedValue)
        XCTAssertEqual(model.allPendingApproval, [])
        await model.refreshPendingApproval()
        XCTAssertEqual(model.allPendingApproval, [], "no polling while turned off")
        let isTurnedOn = await model.setMacApprovalAllowed(true)
        XCTAssertTrue(isTurnedOn)
        XCTAssertTrue(model.isMacApprovalAllowed)
    }

    func testApproveFromTheCardWithoutOpeningTheSheet() async {
        backend.allPendingApproval = [createApproval(requestID: "first"), createApproval(requestID: "second")]
        let model = await createUnlockedModel()
        await model.refreshPendingApproval()
        await model.approve(model.allPendingApproval[0])
        XCTAssertEqual(backend.allApprovalAnswer, ["approve first"])
        XCTAssertNil(model.presentedApproval)
        await model.deny(model.allPendingApproval[0])
        XCTAssertEqual(backend.allApprovalAnswer, ["approve first", "deny second"])
        XCTAssertEqual(model.allPendingApproval, [])
    }

    func testApproveFromTheCardOpensTheSheetForTheMasterPassword() async {
        backend.isBiometricAvailable = false
        backend.allPendingApproval = [createApproval(requestID: "first")]
        let model = await createUnlockedModel()
        await model.refreshPendingApproval()
        await model.approve(model.allPendingApproval[0])
        XCTAssertEqual(model.presentedApproval?.requestID, "first")
        XCTAssertTrue(model.isApprovalPasswordRequired)
        await model.approvePendingApproval(password: FakeBackend.password, isRemembered: false)
        XCTAssertEqual(backend.allApprovalAnswer, ["approve first"])
        XCTAssertNil(model.presentedApproval)
    }

    func testLockReturnsToTheVaultsTab() async {
        let model = await createUnlockedModel()
        model.selectedTab = .settings
        model.lock()
        XCTAssertEqual(model.selectedTab, .vaults)
    }

    func testApprovalDetailLineNamesTheKeyOrCommand() {
        XCTAssertEqual(createApproval(requestID: "first").detailLine, "with “GitHub deploy key”")
        let commandLine = PhoneApproval(requestID: "cli", requesterDeviceName: "Mac", expiresAt: "", kind: PhoneApproval.kindCommandLine, appName: "hisar", purpose: "use Hisar", commandLine: "hisar read")
        XCTAssertEqual(commandLine.detailLine, "hisar read")
    }

    func testDescribeServerShowsTheHost() {
        XCTAssertEqual(VaultLogic.describeServer("https://api.tryhisar.com"), "api.tryhisar.com")
        XCTAssertEqual(VaultLogic.describeServer("http://127.0.0.1:18080"), "127.0.0.1:18080")
    }
}
