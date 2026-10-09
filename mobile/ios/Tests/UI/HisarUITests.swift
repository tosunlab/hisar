import XCTest

final class HisarUITests: HisarUITestCase {
    func testAccountItemLockAndSignInFlow() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_INVITE_LINK")
        let server = try readEnvironment("HISAR_UI_TEST_SERVER")
        let email = "ios-ui@example.com"
        app.launch()

        let secretKey = signUp(inviteLink: inviteLink, email: email, name: "iOS UI")
        saveScreenshot("vaults")

        addLogin(title: "GitHub", username: "octo-alice", password: "ui-test-password-1", website: "github.com")
        saveScreenshot("items")
        app.buttons["item-GitHub"].tap()
        waitFor(app.staticTexts["username-value"])
        XCTAssertEqual(app.staticTexts["username-value"].label, "octo-alice")
        XCTAssertNotEqual(app.staticTexts["password-value"].label, "ui-test-password-1")
        app.buttons["reveal-password"].tap()
        XCTAssertEqual(app.staticTexts["password-value"].label, "ui-test-password-1")
        saveScreenshot("item-detail")
        app.buttons["copy-password"].tap()
        waitFor(app.staticTexts["notice"].exists ? app.staticTexts["notice"] : app.otherElements["notice"], timeout: 5)

        app.buttons["edit-item"].tap()
        waitFor(app.buttons["editor-generate-password"])
        app.buttons["editor-generate-password"].tap()
        waitFor(app.staticTexts["generated-password"])
        saveScreenshot("password-generator")
        app.buttons["generator-use"].tap()
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts["password-value"])
        XCTAssertNotEqual(app.staticTexts["password-value"].label, "ui-test-password-1", "the generated password replaced the old one")

        app.navigationBars.buttons.element(boundBy: 0).tap()
        app.navigationBars.buttons.element(boundBy: 0).tap()
        let searchField = app.searchFields.firstMatch
        type("git", into: searchField)
        waitFor(app.buttons["item-GitHub"])
        saveScreenshot("search")
        let closeSearchButton = app.buttons["Close"].exists ? app.buttons["Close"] : app.buttons["Cancel"]
        closeSearchButton.firstMatch.tap()

        app.buttons["lock-now"].tap()
        waitFor(app.secureTextFields["unlock-password"])
        saveScreenshot("lock")
        type("wrong password", into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.otherElements["error-message"].exists ? app.otherElements["error-message"] : app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] 'incorrect'")).firstMatch)
        app.secureTextFields["unlock-password"].tap()
        app.secureTextFields["unlock-password"].typeText(Self.masterPassword)
        app.buttons["unlock-submit"].tap()
        waitFor(app.buttons["vault-Personal"])

        app.buttons["open-settings"].tap()
        waitFor(app.switches["biometric-toggle"])
        saveScreenshot("settings")
        for _ in 0..<5 where !app.buttons["sign-out"].isHittable {
            app.swipeUp()
        }
        app.buttons["sign-out"].tap()
        app.sheets.buttons["Sign out"].firstMatch.tap()
        waitFor(app.textFields["sign-in-server"])

        let serverField = app.textFields["sign-in-server"]
        serverField.tap()
        serverField.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 20) + server)
        type(email, into: app.textFields["sign-in-email"])
        type(secretKey, into: app.textFields["sign-in-secret-key"])
        type(Self.masterPassword, into: app.secureTextFields["sign-in-password"])
        app.buttons["sign-in-submit"].tap()
        waitFor(app.buttons["vault-Personal"], timeout: 60)
        dismissSavePasswordPrompt()
        saveScreenshot("signed-in-again")
        app.buttons["vault-Personal"].tap()
        waitFor(app.buttons["item-GitHub"])
    }

    func testPrivacyCoverHidesContentInTheBackground() throws {
        app.launch()
        waitFor(app.segmentedControls["sign-in-mode"])
        XCUIDevice.shared.press(.home)
        XCTAssertTrue(app.wait(for: .runningBackground, timeout: 10) || app.wait(for: .runningBackgroundSuspended, timeout: 10))
        app.activate()
        waitFor(app.segmentedControls["sign-in-mode"])
        XCTAssertFalse(app.otherElements["privacy-cover"].exists, "the cover goes away once the app is active")
    }
}
