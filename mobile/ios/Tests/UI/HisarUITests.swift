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

        app.buttons["copy-secret-reference"].tap()
        waitFor(app.staticTexts["Secret reference copied."], timeout: 5)

        goBack()
        goBack()
        waitFor(app.buttons["vault-Personal"])
        openTab("Search")
        let searchField = app.searchFields.firstMatch
        type("git", into: searchField)
        waitFor(app.buttons["item-GitHub"])
        saveScreenshot("search")
        let closeSearchButton = app.buttons["Close"].exists ? app.buttons["Close"] : app.buttons["Cancel"]
        closeSearchButton.firstMatch.tap()
        openTab("Vaults")
        waitFor(app.buttons["vault-Personal"])

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

        openTab("Approvals")
        waitFor(app.descendants(matching: .any)["approvals-empty"])
        saveScreenshot("approvals-empty")

        openSettings("open-security")
        waitFor(app.switches["biometric-toggle"])
        app.buttons["5 minutes"].firstMatch.tap()
        app.buttons["1 hour"].firstMatch.tap()
        saveScreenshot("security")
        goBack()
        waitFor(app.staticTexts["1 h"])

        app.buttons["open-devices"].tap()
        waitFor(app.staticTexts["This iPhone"].exists ? app.staticTexts["This iPhone"] : app.staticTexts["THIS IPHONE"])
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'Active now'")).firstMatch.exists)
        saveScreenshot("devices")
        goBack()

        app.buttons["open-secret-key"].tap()
        type(Self.masterPassword, into: app.secureTextFields["secret-key-password"])
        app.buttons["secret-key-reveal"].tap()
        let shownSecretKey = app.staticTexts["secret-key-value"]
        waitFor(shownSecretKey, timeout: 60)
        XCTAssertEqual(shownSecretKey.label, secretKey)
        XCTAssertTrue(app.descendants(matching: .any)["share-emergency-kit"].exists)
        saveScreenshot("secret-key")
        goBack()

        for _ in 0..<5 where !app.buttons["sign-out"].isHittable {
            app.swipeUp()
        }
        saveScreenshot("settings")
        app.buttons["sign-out"].tap()
        confirm("Sign out", excludingIdentifier: "sign-out")
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
