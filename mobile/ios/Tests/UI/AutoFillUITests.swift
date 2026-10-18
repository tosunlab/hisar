import XCTest

// Drives the AutoFill extension's screens through the debug-only preview in the app (AutoFillPreviewView): a
// simulator build without a development team is not offered as a password provider in Safari.
final class AutoFillUITests: HisarUITestCase {
    func testAutoFillListsMatchingLoginsAndFills() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_AUTOFILL_INVITE_LINK")

        app.launch()
        _ = signUp(inviteLink: inviteLink, email: "ios-autofill@example.com", name: "iOS AutoFill")
        addLogin(title: "Example", username: "autofill-user", password: "autofill-password-1", website: "accounts.example.com")
        goBack()
        addLogin(title: "Other site", username: "other-user", password: "other-password-1", website: "other.example.org")

        launchAutoFillAndUnlock()
        let exampleLogin = app.buttons["autofill-login-Example"]
        waitFor(exampleLogin)
        XCTAssertFalse(app.buttons["autofill-login-Other site"].exists, "a login for another site is not suggested")
        saveScreenshot("autofill-list")
        exampleLogin.tap()
        checkResult("Filled autofill-user with a password of 19 characters")

        launchAutoFillAndUnlock()
        type("other", into: app.searchFields.firstMatch)
        saveScreenshot("autofill-search")
        app.buttons["autofill-login-Other site"].tap()
        checkResult("Filled other-user with a password of 16 characters")
    }

    private func launchAutoFillAndUnlock() {
        app.terminate()
        app.launchArguments = ["-HisarUITest", "-HisarAutoFillPreview", "https://example.com/login"]
        app.launch()
        waitFor(app.secureTextFields["unlock-password"])
        saveScreenshot("autofill-lock")
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
    }

    private func checkResult(_ expectedMessage: String, file: StaticString = #filePath, line: UInt = #line) {
        let result = app.descendants(matching: .any)["autofill-result"]
        waitFor(result, file: file, line: line)
        XCTAssertEqual(result.label, expectedMessage, file: file, line: line)
    }
}
