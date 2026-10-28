import CryptoKit
import UIKit
import XCTest

final class OneTimeCodeUITests: HisarUITestCase {
    // RFC 6238's test secret "12345678901234567890", typed the way a site shows a setup key.
    private static let setupKey = "gezd gnbv gy3t qojq gezd gnbv gy3t qojq"
    private static let secret = Data("12345678901234567890".utf8)

    private static func computeCode(at date: Date) -> String {
        var counter = UInt64(date.timeIntervalSince1970 / 30).bigEndian
        let message = Data(bytes: &counter, count: MemoryLayout<UInt64>.size)
        let digest = Array(HMAC<Insecure.SHA1>.authenticationCode(for: message, using: SymmetricKey(data: secret)))
        let offset = Int(digest[19] & 0x0f)
        let binary = (UInt32(digest[offset] & 0x7f) << 24) | (UInt32(digest[offset + 1]) << 16) | (UInt32(digest[offset + 2]) << 8) | UInt32(digest[offset + 3])
        return String(format: "%06u", binary % 1_000_000)
    }

    // A code read across a 30 s boundary may be the earlier one.
    private func checkCurrentCode(_ code: String, startedAt: Date, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue([Self.computeCode(at: startedAt), Self.computeCode(at: Date())].contains(code), "\(code) is not the current code", file: file, line: line)
    }

    func testSetupKeyShowsCodeThatCopiesAndFills() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_ONE_TIME_CODE_INVITE_LINK")
        if ProcessInfo.processInfo.environment["HISAR_UI_TEST_REDUCED_MOTION"] == "1" {
            XCTAssertTrue(UIAccessibility.isReduceMotionEnabled)
        }
        app.launchArguments.removeAll { $0 == "-HisarUITest" }
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: "ios-one-time-code@example.com", name: "iOS One-Time Code")

        app.buttons["vault-Personal"].tap()
        choose("add-login", from: "add-item")
        type("Example", into: app.textFields["editor-title"])
        type("code-user", into: app.textFields["editor-username"])
        type("code-password-1", into: app.secureTextFields["editor-password"])
        type("not a setup key!", into: app.secureTextFields["editor-one-time-password"])
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts["The one-time code is not a valid secret or otpauth:// link"])
        saveScreenshot("otp-editor-invalid")
        let oneTimePasswordField = app.secureTextFields["editor-one-time-password"]
        oneTimePasswordField.tap()
        typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 20) + Self.setupKey, into: oneTimePasswordField, showing: Self.setupKey)
        type("accounts.example.com", into: app.textFields["editor-website-0"])
        saveScreenshot("otp-editor")
        app.buttons["editor-save"].tap()
        waitFor(app.buttons["item-Example"])

        let startedAt = Date()
        app.buttons["item-Example"].tap()
        let copyNotice = app.staticTexts["One-time code copied. It clears in 90 seconds."]
        let codeRow = app.buttons["copy-one-time-code"]
        waitFor(codeRow)
        let value = (codeRow.value as? String) ?? ""
        let shownCode = String(value.split(separator: ",").first ?? "")
        XCTAssertTrue(value.contains("seconds left"))
        checkCurrentCode(shownCode.replacingOccurrences(of: " ", with: ""), startedAt: startedAt)
        saveScreenshot("otp-item-detail")
        codeRow.tap()
        waitFor(copyNotice)
        XCTAssertTrue(copyNotice.waitForNonExistence(timeout: 10))
        codeRow.coordinate(withNormalizedOffset: CGVector(dx: 0.99, dy: 0.02)).tap()
        waitFor(copyNotice)
        XCTAssertTrue(copyNotice.waitForNonExistence(timeout: 10))
        let codeCell = app.cells.containing(.button, identifier: "copy-one-time-code").firstMatch
        codeCell.coordinate(withNormalizedOffset: CGVector(dx: 0.99, dy: 0.95)).tap()
        waitFor(copyNotice)
        saveScreenshot("otp-copied")

        app.buttons["edit-item"].tap()
        waitFor(app.secureTextFields["editor-one-time-password"])
        app.buttons["Reveal setup key"].tap()
        waitFor(app.textFields["editor-one-time-password"])
        XCTAssertTrue(app.secureTextFields["editor-password"].exists)
        app.buttons["Hide setup key"].tap()
        waitFor(app.secureTextFields["editor-one-time-password"])
        app.buttons["Reveal setup key"].tap()
        app.buttons["editor-cancel"].tap()
        app.buttons["edit-item"].tap()
        waitFor(app.secureTextFields["editor-one-time-password"])
        app.buttons["editor-cancel"].tap()

        goBack()
        addLogin(title: "Other site", username: "other-user", password: "other-password-1", website: "other.example.org")

        launchAutoFill(argument: "-HisarAutoFillOneTimeCodePreview")
        let codeLogin = app.buttons["autofill-login-Example"]
        waitFor(codeLogin)
        saveScreenshot("otp-autofill-list")
        let fillStartedAt = Date()
        codeLogin.tap()
        let filledCode = readResult().replacingOccurrences(of: "Filled the one-time code ", with: "")
        checkCurrentCode(filledCode, startedAt: fillStartedAt)

        launchAutoFill(argument: "-HisarAutoFillPreview")
        waitFor(app.buttons["autofill-login-Example"])
        let passwordStartedAt = Date()
        app.buttons["autofill-login-Example"].tap()
        let passwordResult = readResult()
        let prefix = "Filled code-user with a password of 15 characters and copied the one-time code "
        XCTAssertTrue(passwordResult.hasPrefix(prefix), passwordResult)
        checkCurrentCode(String(passwordResult.dropFirst(prefix.count)), startedAt: passwordStartedAt)

        launchAutoFill(argument: "-HisarAutoFillPreview")
        type("other", into: app.searchFields.firstMatch)
        app.buttons["autofill-login-Other site"].tap()
        let confirmation = app.alerts.firstMatch
        waitFor(confirmation)
        XCTAssertTrue(confirmation.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "other.example.org")).firstMatch.exists)
        confirmation.buttons["Fill once"].tap()
        XCTAssertEqual(readResult(), "Filled other-user with a password of 16 characters")
    }

    private func launchAutoFill(argument: String) {
        app.terminate()
        app.launchArguments = ["-HisarUITest", argument, "https://accounts.example.com/two-factor"]
        app.launch()
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
    }

    private func readResult() -> String {
        let result = app.descendants(matching: .any)["autofill-result"]
        waitFor(result)
        return result.label
    }
}
