import XCTest

final class HisarUITests: HisarUITestCase {
    func testAccountItemLockAndSignInFlow() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_INVITE_LINK")
        let email = "ios-ui@example.com"
        app.launch()

        let secretKey = signUp(inviteLink: inviteLink, email: email, name: "iOS UI")
        saveScreenshot("vaults")

        addLogin(title: "GitHub", username: "octo-alice", password: "ui-test-password-1", website: "github.com", oneTimePassword: "JBSWY3DPEHPK3PXP")
        saveScreenshot("items")
        app.buttons["item-GitHub"].tap()
        let passwordNotice = app.staticTexts["Password copied. It clears in 90 seconds."]
        let passwordRow = app.buttons.matching(identifier: "copy-password").firstMatch
        waitFor(passwordRow)
        XCTAssertEqual(passwordRow.value as? String, "Hidden")
        passwordRow.tap()
        waitFor(passwordNotice)
        XCTAssertEqual(passwordRow.value as? String, "Hidden", "copying must not reveal")
        XCTAssertTrue(passwordNotice.waitForNonExistence(timeout: 10))
        app.buttons["reveal-password"].tap()
        XCTAssertEqual(passwordRow.value as? String, "ui-test-password-1")
        XCTAssertFalse(passwordNotice.waitForExistence(timeout: 2), "revealing must not copy")
        saveScreenshot("item-detail")

        app.buttons["edit-item"].tap()
        waitFor(app.secureTextFields["editor-one-time-password"], timeout: 10)
        XCTAssertFalse(app.textFields["editor-one-time-password"].exists, "the editor shows the setup key before it is revealed")
        app.buttons["editor-cancel"].tap()
        waitFor(passwordRow)
        goBack()
        goBack()

        choose("New credit card", from: "new-menu", under: "new-more")
        type("Visa", into: app.textFields["editor-title"])
        type("4111111111111111", into: app.secureTextFields["editor-card-number"], normalizedBy: CardInput.formatNumber)
        type("1229", into: app.textFields["editor-card-expiry"], normalizedBy: CardInput.formatExpiry)
        type("123", into: app.secureTextFields["editor-card-security-code"])
        saveScreenshot("add-credit-card")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Visa •••• 1111")).firstMatch)
        let cardNumberRow = app.buttons["copy-card number"]
        waitFor(cardNumberRow)
        XCTAssertEqual(cardNumberRow.value as? String, "Hidden", "the card number is shown before it is revealed")
        XCTAssertEqual(app.buttons["copy-security code"].value as? String, "Hidden", "the security code is shown before it is revealed")
        app.buttons["reveal-card number"].tap()
        XCTAssertEqual(cardNumberRow.value as? String, "4111 1111 1111 1111")
        saveScreenshot("credit-card")
        goBack()

        app.buttons["lock-now"].tap()
        type("wrong password", into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.otherElements["error-message"].exists ? app.otherElements["error-message"] : app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] 'incorrect'")).firstMatch)
        saveScreenshot("lock")
        app.secureTextFields["unlock-password"].tap()
        typeText(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.buttons["vault-Personal"])

        openTab("Settings")
        for _ in 0..<5 where !app.buttons["sign-out"].isHittable {
            app.swipeUp()
        }
        saveScreenshot("settings")
        app.buttons["sign-out"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@ AND label CONTAINS %@", "Secret Key", "master password")).firstMatch.waitForExistence(timeout: 5))
        confirm("Sign out", excludingIdentifier: "sign-out")
        waitFor(app.textFields["sign-in-email"])

        // Signing out keeps the server, behind the disclosure: signing back in needs no server typed.
        XCTAssertFalse(app.textFields["sign-in-server"].exists)
        type(email, into: app.textFields["sign-in-email"])
        type(secretKey, into: app.textFields["sign-in-secret-key"])
        type(Self.masterPassword, into: app.secureTextFields["sign-in-password"])
        app.buttons["sign-in-submit"].tap()
        waitFor(app.buttons["vault-Personal"], timeout: 60)
        dismissSavePasswordPrompt()
        saveScreenshot("signed-in-again")
        app.buttons["vault-Personal"].tap()
        waitFor(app.buttons["item-GitHub"])
        waitFor(app.buttons["item-Visa"])
    }

    func testPrivacyCoverHidesContentInTheBackground() throws {
        app.launch()
        waitFor(app.buttons["welcome-create-account"])
        for _ in 0..<2 {
            XCUIDevice.shared.press(.home)
            XCTAssertTrue(app.wait(for: .runningBackground, timeout: 10) || app.wait(for: .runningBackgroundSuspended, timeout: 10))
            app.activate()
            waitFor(app.buttons["welcome-create-account"])
            XCTAssertTrue(app.otherElements["privacy-cover"].waitForNonExistence(timeout: 5), "the cover goes away once the app is active")
        }
    }
}
