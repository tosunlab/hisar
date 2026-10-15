import CryptoKit
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
        XCTAssertTrue(code.count == 6 && code.allSatisfy(\.isNumber), "\(code) is not a 6-digit code", file: file, line: line)
        XCTAssertTrue([Self.computeCode(at: startedAt), Self.computeCode(at: Date())].contains(code), "\(code) is not the current code", file: file, line: line)
    }

    func testSetupKeyShowsCodeThatCopiesAndFills() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_ONE_TIME_CODE_INVITE_LINK")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: "ios-one-time-code@example.com", name: "iOS One-Time Code")

        app.buttons["vault-Personal"].tap()
        waitFor(app.buttons["add-item"])
        app.buttons["add-item"].tap()
        waitFor(app.buttons["add-login"])
        app.buttons["add-login"].tap()
        type("Example", into: app.textFields["editor-title"])
        type("code-user", into: app.textFields["editor-username"])
        type("code-password-1", into: app.secureTextFields["editor-password"])
        type("not a setup key!", into: app.textFields["editor-one-time-password"])
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts["The one-time password is not a valid secret or otpauth:// link"])
        saveScreenshot("otp-editor-invalid")
        let oneTimePasswordField = app.textFields["editor-one-time-password"]
        oneTimePasswordField.tap()
        oneTimePasswordField.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 20) + Self.setupKey)
        type("accounts.example.com", into: app.textFields["editor-website-0"])
        saveScreenshot("otp-editor")
        app.buttons["editor-save"].tap()
        waitFor(app.buttons["item-Example"])

        let startedAt = Date()
        app.buttons["item-Example"].tap()
        let codeText = app.staticTexts["one-time-code-value"]
        waitFor(codeText)
        checkCurrentCode(codeText.label.replacingOccurrences(of: " ", with: ""), startedAt: startedAt)
        let ring = app.descendants(matching: .any)["one-time-code-seconds"]
        waitFor(ring)
        let secondsLeft = Int(ring.label.components(separatedBy: " ").first ?? "") ?? -1
        XCTAssertTrue((0...30).contains(secondsLeft), ring.label)
        saveScreenshot("otp-item-detail")
        app.buttons["copy-one-time-code"].tap()
        let notice = app.staticTexts["One-time code copied. It clears in 90 seconds."]
        waitFor(notice, timeout: 5)
        saveScreenshot("otp-copied")

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
