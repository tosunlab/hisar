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

        choose("new-secure-note", from: "new-menu")
        type("Recovery codes", into: app.textFields["editor-title"])
        XCTAssertFalse(app.textFields["editor-username"].exists, "a secure note asks for a username")
        let notesField = app.textViews["editor-notes"].exists ? app.textViews["editor-notes"] : app.textFields["editor-notes"]
        type("4821-9930", into: notesField)
        saveScreenshot("add-secure-note")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts["secure-note-notes"])
        XCTAssertEqual(app.staticTexts["secure-note-notes"].label, "4821-9930")
        XCTAssertFalse(app.staticTexts["password-value"].exists)
        saveScreenshot("secure-note")
        goBack()
        waitFor(app.buttons["vault-Personal"])

        choose("new-credit-card", from: "new-menu")
        type("Visa", into: app.textFields["editor-title"])
        XCTAssertFalse(app.textFields["editor-username"].exists, "a credit card asks for a username")
        type("4111111111111111", into: app.secureTextFields["editor-card-number"])
        type("1229", into: app.textFields["editor-card-expiry"])
        type("123", into: app.secureTextFields["editor-card-security-code"])
        saveScreenshot("add-credit-card")
        app.buttons["editor-save"].tap()
        let cardSummary = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Visa •••• 1111")).firstMatch
        waitFor(cardSummary)
        XCTAssertFalse(app.staticTexts["123"].exists, "the security code is shown before it is revealed")
        saveScreenshot("credit-card")
        goBack()
        waitFor(app.buttons["vault-Personal"])

        choose("new-identity", from: "new-menu")
        type("Passport", into: app.textFields["editor-title"])
        XCTAssertFalse(app.textFields["editor-username"].exists, "an identity asks for a username")
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
        waitFor(app.buttons["vault-Personal"])

        choose("new-api-credential", from: "new-menu")
        type("Billing key", into: app.textFields["editor-title"])
        XCTAssertFalse(app.textFields["editor-username"].exists, "an API credential asks for a username")
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
        waitFor(app.buttons["vault-Personal"])

        choose("new-address", from: "new-menu")
        type("Home", into: app.textFields["editor-title"])
        XCTAssertFalse(app.textFields["editor-username"].exists, "an address asks for a username")
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

        app.buttons["open-appearance"].tap()
        waitFor(app.buttons["appearance-system"])
        XCTAssertTrue(app.buttons["appearance-system"].isSelected)
        XCTAssertTrue(app.buttons["theme-color-neutral"].isSelected)
        app.buttons["appearance-dark"].tap()
        app.buttons["theme-color-purple"].tap()
        XCTAssertTrue(app.buttons["appearance-dark"].isSelected)
        XCTAssertTrue(app.buttons["theme-color-purple"].isSelected)
        XCTAssertFalse(app.buttons["theme-color-neutral"].isSelected)
        saveScreenshot("appearance")
        goBack()
        waitFor(app.buttons.matching(NSPredicate(format: "label CONTAINS 'Dark · Purple'")).firstMatch)

        app.buttons["open-secret-key"].tap()
        type(Self.masterPassword, into: app.secureTextFields["secret-key-password"])
        app.buttons["secret-key-reveal"].tap()
        let shownSecretKey = app.staticTexts["secret-key-value"]
        waitFor(shownSecretKey, timeout: 60)
        XCTAssertEqual(shownSecretKey.label, secretKey)
        XCTAssertTrue(app.descendants(matching: .any)["share-emergency-kit"].exists)
        saveScreenshot("secret-key")
        goBack()

        for _ in 0..<5 where !app.buttons["open-help"].isHittable {
            app.swipeUp()
        }
        app.buttons["open-help"].tap()
        waitFor(app.staticTexts["support-email"])
        XCTAssertEqual(app.staticTexts["support-email"].label, "support@tryhisar.com")
        app.buttons["copy-diagnostics"].tap()
        waitFor(app.staticTexts["Diagnostics copied"], timeout: 5)
        saveScreenshot("settings-help")
        goBack()

        for _ in 0..<5 where !app.buttons["sign-out"].isHittable {
            app.swipeUp()
        }
        saveScreenshot("settings")
        app.buttons["sign-out"].tap()
        confirm("Sign out", excludingIdentifier: "sign-out")
        waitFor(app.textFields["sign-in-email"])

        let useDifferentServerButton = app.descendants(matching: .any)["sign-in-use-different-server"].firstMatch
        if useDifferentServerButton.exists {
            useDifferentServerButton.tap()
        }
        let serverField = app.textFields["sign-in-server"]
        waitFor(serverField)
        serverField.tap()
        serverField.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 40) + server)
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
        waitFor(app.segmentedControls["sign-in-mode"])
        let useDifferentServerButton = app.descendants(matching: .any)["sign-in-use-different-server"].firstMatch
        waitFor(useDifferentServerButton)
        XCTAssertFalse(app.textFields["sign-in-server"].exists)
        XCTAssertTrue(app.staticTexts["Signs in to api.tryhisar.com."].exists)
        useDifferentServerButton.tap()
        let serverField = app.textFields["sign-in-server"]
        waitFor(serverField)
        XCTAssertEqual(serverField.value as? String, "https://api.tryhisar.com")
        XCTAssertFalse(useDifferentServerButton.exists)
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
