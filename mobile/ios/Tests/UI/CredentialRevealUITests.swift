import XCTest

final class CredentialRevealUITests: HisarUITestCase {
    func testCredentialRevealNamesAndIndependentToggles() {
        app.launch()
        waitFor(app.buttons["sign-in-password-reveal"])
        XCTAssertEqual(app.buttons["sign-in-password-reveal"].label, "Show master password")
        app.segmentedControls["sign-in-mode"].buttons["Create account"].tap()
        scrollTo(app.secureTextFields["sign-up-password"])
        type("long master password", into: app.secureTextFields["sign-up-password"])
        app.buttons["sign-up-password-reveal"].tap()
        XCTAssertEqual(app.buttons["sign-up-password-reveal"].label, "Hide master password")
        XCTAssertEqual(app.textFields["sign-up-password"].value as? String, "long master password")
        XCTAssertTrue(app.secureTextFields["sign-up-repeated-password"].exists)
        XCTAssertEqual(app.buttons["sign-up-repeated-password-reveal"].label, "Show repeated master password")
        app.buttons["sign-up-repeated-password-reveal"].tap()
        XCTAssertTrue(app.textFields["sign-up-password"].exists)
        XCTAssertTrue(app.textFields["sign-up-repeated-password"].exists)
        app.buttons["sign-up-password-reveal"].tap()
        XCTAssertTrue(app.secureTextFields["sign-up-password"].exists)
        XCTAssertTrue(app.textFields["sign-up-repeated-password"].exists)
    }

    func testCredentialRevealConcealsOnNavigationAndInactivity() {
        app.launch()
        waitFor(app.buttons["sign-in-password-reveal"])
        type("typed credential", into: app.secureTextFields["sign-in-password"])
        app.buttons["sign-in-password-reveal"].tap()
        XCUIDevice.shared.press(.home)
        app.activate()
        waitFor(app.secureTextFields["sign-in-password"])
        XCTAssertEqual(app.buttons["sign-in-password-reveal"].label, "Show master password")
        app.buttons["sign-in-password-reveal"].tap()
        XCTAssertEqual(app.buttons["sign-in-password-reveal"].label, "Hide master password")
        selectSignInMode("Create account")
        selectSignInMode("Sign in")
        XCTAssertEqual(app.buttons["sign-in-password-reveal"].label, "Show master password")
        XCTAssertTrue(app.secureTextFields["sign-in-password"].exists)
    }

    func testCredentialRevealConcealsOnSubmission() {
        app.launch()
        waitFor(app.textFields["sign-in-email"])
        type("invalid@example.com", into: app.textFields["sign-in-email"])
        type("not-a-secret-key", into: app.textFields["sign-in-secret-key"])
        type("typed credential", into: app.secureTextFields["sign-in-password"])
        app.buttons["sign-in-password-reveal"].tap()
        scrollTo(app.buttons["sign-in-submit"])
        app.buttons["sign-in-submit"].tap()
        waitFor(app.buttons["sign-in-password-reveal"])
        XCTAssertEqual(app.buttons["sign-in-password-reveal"].label, "Show master password")
        XCTAssertTrue(app.secureTextFields["sign-in-password"].exists)
    }
}
