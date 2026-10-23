import XCTest

final class HisarUITests: HisarUITestCase {
    func testAccountItemLockAndSignInFlow() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_INVITE_LINK")
        let email = "ios-ui@example.com"
        app.launch()

        let secretKey = signUp(inviteLink: inviteLink, email: email, name: "iOS UI")
        saveScreenshot("vaults")

        addLogin(title: "GitHub", username: "octo-alice", password: "ui-test-password-1", website: "github.com")
        saveScreenshot("items")
        app.buttons["item-GitHub"].tap()
        waitFor(app.staticTexts["password-value"])
        XCTAssertNotEqual(app.staticTexts["password-value"].label, "ui-test-password-1")
        app.buttons["reveal-password"].tap()
        XCTAssertEqual(app.staticTexts["password-value"].label, "ui-test-password-1")
        saveScreenshot("item-detail")
        app.buttons["copy-password"].tap()

        app.buttons["edit-item"].tap()
        app.buttons["editor-generate-password"].tap()
        saveScreenshot("password-generator")
        app.buttons["generator-use"].tap()
        app.buttons["editor-save"].tap()
        // The item under the editor shows the old password until the save finishes and the editor closes.
        XCTAssertTrue(app.textFields["editor-title"].waitForNonExistence(timeout: 30), "the editor did not close after saving")
        XCTAssertNotEqual(app.staticTexts["password-value"].label, "ui-test-password-1", "the generated password replaced the old one")

        app.buttons["copy-secret-reference"].tap()
        goBack()
        goBack()

        choose("new-secure-note", from: "new-menu")
        type("Recovery codes", into: app.textFields["editor-title"])
        let notesField = app.textViews["editor-notes"].exists ? app.textViews["editor-notes"] : app.textFields["editor-notes"]
        type("4821-9930", into: notesField)
        saveScreenshot("add-secure-note")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts["secure-note-notes"])
        XCTAssertEqual(app.staticTexts["secure-note-notes"].label, "4821-9930")
        saveScreenshot("secure-note")
        goBack()

        choose("new-credit-card", from: "new-menu")
        type("Visa", into: app.textFields["editor-title"])
        type("4111111111111111", into: app.secureTextFields["editor-card-number"], normalizedBy: CardInput.formatNumber)
        type("1229", into: app.textFields["editor-card-expiry"], normalizedBy: CardInput.formatExpiry)
        type("123", into: app.secureTextFields["editor-card-security-code"])
        saveScreenshot("add-credit-card")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Visa •••• 1111")).firstMatch)
        XCTAssertFalse(app.staticTexts["123"].exists, "the security code is shown before it is revealed")
        saveScreenshot("credit-card")
        goBack()

        choose("new-identity", from: "new-menu")
        type("Passport", into: app.textFields["editor-title"])
        type("Alice", into: app.textFields["editor-identity-first-name"])
        type("Example", into: app.textFields["editor-identity-last-name"])
        type("U1234567", into: app.secureTextFields["editor-identity-passport-number"])
        saveScreenshot("add-identity")
        app.buttons["editor-save"].tap()
        let identitySummary = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Alice Example")).firstMatch
        waitFor(identitySummary)
        XCTAssertFalse(app.staticTexts["U1234567"].exists, "the passport number is shown before it is revealed")
        XCTAssertFalse(app.staticTexts["Driver's license number"].exists, "a document number nobody typed was saved")
        saveScreenshot("identity")
        app.buttons["reveal-passport number"].tap()
        waitFor(app.staticTexts["U1234567"], timeout: 5)
        goBack()

        choose("new-api-credential", from: "new-menu")
        type("Billing key", into: app.textFields["editor-title"])
        type("Stripe", into: app.textFields["editor-api-credential-service"])
        type("key_1", into: app.textFields["editor-api-credential-key-id"])
        type("sk_live_from_the_phone", into: app.secureTextFields["editor-api-credential-secret"])
        type("api.stripe.com", into: app.textFields["editor-api-credential-host"])
        saveScreenshot("add-api-credential")
        app.buttons["editor-save"].tap()
        let credentialSummary = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Stripe · api.stripe.com")).firstMatch
        waitFor(credentialSummary)
        XCTAssertFalse(app.staticTexts["sk_live_from_the_phone"].exists, "the secret is shown before it is revealed")
        saveScreenshot("api-credential")
        app.buttons["reveal-secret"].tap()
        waitFor(app.staticTexts["sk_live_from_the_phone"], timeout: 5)
        goBack()

        choose("new-address", from: "new-menu")
        type("Home", into: app.textFields["editor-title"])
        // A street may take several lines, so it is a text view rather than a text field.
        type("1 Main St", into: app.descendants(matching: .any)["editor-address-street"].firstMatch)
        type("Springfield", into: app.textFields["editor-address-city"])
        type("US", into: app.textFields["editor-address-country"])
        saveScreenshot("add-address")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Springfield, US")).firstMatch)
        XCTAssertTrue(app.staticTexts["1 Main St"].exists, "the street is not shown")
        saveScreenshot("address")
        goBack()

        choose("new-software-license", from: "new-menu")
        type("Sketch", into: app.textFields["editor-title"])
        type("Sketch", into: app.textFields["editor-license-product"])
        type("100", into: app.textFields["editor-license-version"])
        type("LICENSE-FROM-THE-PHONE", into: app.secureTextFields["editor-license-key"])
        type("Alice Example", into: app.textFields["editor-license-licensed-to"])
        saveScreenshot("add-software-license")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Version 100 · Alice Example")).firstMatch)
        XCTAssertFalse(app.staticTexts["LICENSE-FROM-THE-PHONE"].exists, "the license key is shown before it is revealed")
        saveScreenshot("software-license")
        goBack()

        choose("new-server", from: "new-menu")
        type("Build server", into: app.textFields["editor-title"])
        type("build.example.com", into: app.textFields["editor-server-host"])
        type("2222", into: app.textFields["editor-server-port"])
        type("deploy", into: app.textFields["editor-username"])
        type("server-password-from-the-phone", into: app.secureTextFields["editor-password"])
        saveScreenshot("add-server")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "build.example.com:2222 · deploy")).firstMatch)
        XCTAssertFalse(app.staticTexts["server-password-from-the-phone"].exists, "the password is shown before it is revealed")
        saveScreenshot("server")
        goBack()

        choose("new-database", from: "new-menu")
        type("Prod db", into: app.textFields["editor-title"])
        // The placeholders are db.example.com, 5432 and app, which the fields' values could not be told apart from.
        type("pg.example.com", into: app.textFields["editor-database-host"])
        type("6432", into: app.textFields["editor-database-port"])
        type("billing", into: app.textFields["editor-database-name"])
        type("app", into: app.textFields["editor-username"])
        type("database-password-from-the-phone", into: app.secureTextFields["editor-password"])
        XCTAssertFalse(app.textFields["editor-one-time-password"].exists, "a database asks for a one-time password")
        saveScreenshot("add-database")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "PostgreSQL · pg.example.com:6432/billing")).firstMatch)
        waitFor(app.descendants(matching: .any)["database-connection-string"].firstMatch)
        XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "database-password-from-the-phone")).firstMatch.exists, "the connection string is shown before it is revealed")
        saveScreenshot("database")
        goBack()
        openTab("Search")
        type("git", into: app.searchFields.firstMatch)
        waitFor(app.buttons["item-GitHub"])
        saveScreenshot("search")
        let closeSearchButton = app.buttons["Close"].exists ? app.buttons["Close"] : app.buttons["Cancel"]
        closeSearchButton.firstMatch.tap()
        openTab("Vaults")

        app.buttons["lock-now"].tap()
        type("wrong password", into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.otherElements["error-message"].exists ? app.otherElements["error-message"] : app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] 'incorrect'")).firstMatch)
        saveScreenshot("lock")
        app.secureTextFields["unlock-password"].tap()
        typeText(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.buttons["vault-Personal"])

        openTab("Approvals")
        saveScreenshot("approvals-empty")

        openSettings("open-security")
        app.buttons["5 minutes"].firstMatch.tap()
        app.buttons["1 hour"].firstMatch.tap()
        saveScreenshot("security")
        goBack()

        app.buttons["open-devices"].tap()
        // iOS shows the section header as "This iPhone" or "THIS IPHONE"; the screen may still be opening, so both are awaited.
        waitFor(app.staticTexts.matching(NSPredicate(format: "label ==[c] %@", "This iPhone")).firstMatch)
        saveScreenshot("devices")
        goBack()

        app.buttons["open-appearance"].tap()
        app.buttons["appearance-dark"].tap()
        app.buttons["theme-color-purple"].tap()
        saveScreenshot("appearance")
        goBack()

        app.buttons["open-secret-key"].tap()
        type(Self.masterPassword, into: app.secureTextFields["secret-key-password"])
        app.buttons["secret-key-reveal"].tap()
        let shownSecretKey = app.staticTexts["secret-key-value"]
        waitFor(shownSecretKey, timeout: 60)
        XCTAssertEqual(shownSecretKey.label, secretKey)
        saveScreenshot("secret-key")
        goBack()

        for _ in 0..<5 where !app.buttons["open-help"].isHittable {
            app.swipeUp()
        }
        app.buttons["open-help"].tap()
        app.buttons["copy-diagnostics"].tap()
        saveScreenshot("settings-help")
        goBack()

        for _ in 0..<5 where !app.buttons["sign-out"].isHittable {
            app.swipeUp()
        }
        saveScreenshot("settings")
        app.buttons["sign-out"].tap()
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
        openTab("Settings")
        waitFor(app.buttons.matching(NSPredicate(format: "label CONTAINS 'Dark · Purple'")).firstMatch)
        openTab("Vaults")
        app.buttons["vault-Personal"].tap()
        waitFor(app.buttons["item-GitHub"])
    }

    func testSignInHidesTheServerUntilAskedFor() throws {
        app.launch()
        let useDifferentServerButton = app.descendants(matching: .any)["sign-in-use-different-server"].firstMatch
        waitFor(useDifferentServerButton)
        XCTAssertFalse(app.textFields["sign-in-server"].exists)
        useDifferentServerButton.tap()
        let serverField = app.textFields["sign-in-server"]
        waitFor(serverField)
        XCTAssertEqual(serverField.value as? String, "https://api.tryhisar.com")
    }

    func testPrivacyCoverHidesContentInTheBackground() throws {
        app.launch()
        waitFor(app.segmentedControls["sign-in-mode"])
        XCUIDevice.shared.press(.home)
        XCTAssertTrue(app.wait(for: .runningBackground, timeout: 10) || app.wait(for: .runningBackgroundSuspended, timeout: 10))
        app.activate()
        waitFor(app.segmentedControls["sign-in-mode"])
        XCTAssertTrue(app.otherElements["privacy-cover"].waitForNonExistence(timeout: 5), "the cover goes away once the app is active")
    }
}
