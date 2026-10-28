import XCTest

final class HisarUITests: HisarUITestCase {
    func testPasswordMismatchWaitsUntilConfirmationLosesFocus() {
        app.launch()
        app.segmentedControls["sign-in-mode"].buttons["Create account"].tap()
        let password = app.secureTextFields["sign-up-password"]
        let repeated = app.secureTextFields["sign-up-repeated-password"]
        type("a long master password", into: password)
        type("a long", into: repeated)
        XCTAssertFalse(app.staticTexts["The passwords do not match."].exists)
        XCTAssertFalse(app.buttons["sign-in-submit"].isEnabled)
        password.tap()
        waitFor(app.staticTexts["The passwords do not match."])
        repeated.tap()
        typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 6) + "a long master password", into: repeated, showing: "a long master password")
        XCTAssertTrue(app.staticTexts["The passwords do not match."].waitForNonExistence(timeout: 30))
    }

    func testAccountItemLockAndSignInFlow() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_INVITE_LINK")
        let email = "ios-ui@example.com"
        app.launch()

        let secretKey = signUp(inviteLink: inviteLink, email: email, name: "iOS UI")
        XCTAssertFalse(app.buttons["open-tags"].exists, "Vaults shows Tags before any item is tagged")
        saveScreenshot("vaults")

        addLogin(title: "GitHub", username: "octo-alice", password: "ui-test-password-1", website: "github.com")
        saveScreenshot("items")
        app.buttons["item-GitHub"].tap()
        let passwordNotice = app.staticTexts["Password copied. It clears in 90 seconds."]
        let passwordRow = app.buttons.matching(identifier: "copy-password").firstMatch
        waitFor(passwordRow)
        XCTAssertEqual(passwordRow.value as? String, "Hidden")
        XCTAssertEqual(passwordRow.label, "Copy password")
        XCTAssertEqual(app.buttons["reveal-password"].label, "Reveal password")
        let passwordCell = app.cells.containing(.button, identifier: "copy-password").firstMatch
        waitFor(passwordCell)
        for point in [CGVector(dx: 0.01, dy: 0.05), CGVector(dx: 0.99, dy: 0.95)] {
            passwordCell.coordinate(withNormalizedOffset: point).tap()
            waitFor(passwordNotice)
            XCTAssertEqual(passwordRow.value as? String, "Hidden")
            XCTAssertTrue(passwordNotice.waitForNonExistence(timeout: 10))
        }
        app.buttons["reveal-password"].tap()
        XCTAssertEqual(passwordRow.value as? String, "ui-test-password-1")
        XCTAssertFalse(passwordNotice.waitForExistence(timeout: 2), "revealing must not copy")
        saveScreenshot("item-detail")
        app.descendants(matching: .any)["open-website"].firstMatch.tap()
        let safari = XCUIApplication(bundleIdentifier: "com.apple.mobilesafari")
        XCTAssertTrue(safari.wait(for: .runningForeground, timeout: 10))
        app.activate()
        waitFor(passwordRow)
        XCTAssertFalse(app.staticTexts["Website copied. It clears in 90 seconds."].waitForExistence(timeout: 2), "opening a website must not copy")

        app.buttons["edit-item"].tap()
        app.buttons["editor-generate-password"].tap()
        saveScreenshot("password-generator")
        app.buttons["generator-use"].tap()
        type("dev, work", into: app.textFields["editor-new-tag"])
        for (index, name) in ["Recovery code", "Recovery code", "recovery CODE", "Password"].enumerated() {
            app.buttons["Add field"].tap()
            type(name, into: app.textFields["editor-field-\(index)-name"])
            type("row-secret-\(index)", into: app.textFields["editor-field-\(index)-value"])
            app.cells.containing(.textField, identifier: "editor-field-\(index)-name").buttons["Secret"].tap()
        }
        app.buttons["editor-save"].tap()
        // The item under the editor shows the old password until the save finishes and the editor closes.
        XCTAssertTrue(app.textFields["editor-title"].waitForNonExistence(timeout: 30), "the editor did not close after saving")
        XCTAssertNotEqual(passwordRow.value as? String, "ui-test-password-1", "the generated password replaced the old one")
        let customRows = app.buttons.matching(identifier: "copy-recovery code")
        for index in 0..<4 {
            let copy = index < 3 ? customRows.element(boundBy: index) : app.buttons.matching(identifier: "copy-password").element(boundBy: 1)
            copy.tap()
            app.buttons["edit-item"].tap()
            let title = app.textFields["editor-title"]
            title.tap()
            expectation(for: NSPredicate(format: "hasKeyboardFocus == true"), evaluatedWith: title)
            waitForExpectations(timeout: 10)
            typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: "GitHub".count), into: title, showing: "")
            title.press(forDuration: 1)
            let paste = app.menuItems["Paste"]
            waitFor(paste)
            paste.tap()
            XCTAssertEqual(title.value as? String, "row-secret-\(index)")
            app.buttons["editor-cancel"].tap()
            let discard = app.buttons["editor-discard"].firstMatch
            waitForHittable(discard)
            discard.tap()
            waitFor(passwordRow)
        }
        app.buttons.matching(identifier: "reveal-recovery code").element(boundBy: 1).tap()
        XCTAssertEqual(customRows.element(boundBy: 1).value as? String, "row-secret-1")
        XCTAssertEqual(customRows.element(boundBy: 0).value as? String, "Hidden")
        XCTAssertEqual(customRows.element(boundBy: 2).value as? String, "Hidden")

        app.buttons["copy-secret-reference"].tap()
        goBack()
        goBack()
        let tagsRow = app.buttons["open-tags"]
        waitFor(tagsRow)
        XCTAssertEqual(app.buttons.matching(identifier: "open-tags").count, 1)
        XCTAssertFalse(app.buttons["tag-dev"].exists, "Vaults lists individual tags instead of one Tags row")
        XCTAssertFalse(app.buttons["tag-work"].exists, "Vaults lists individual tags instead of one Tags row")
        tagsRow.tap()
        waitFor(app.buttons["tag-dev"])
        XCTAssertTrue(app.buttons["tag-work"].exists)
        saveScreenshot("tags")
        app.buttons["tag-work"].tap()
        waitFor(app.buttons["item-GitHub"])
        app.buttons["item-GitHub"].tap()
        waitFor(app.buttons["copy-password"])
        goBack()
        goBack()
        goBack()

        choose("new-secure-note", from: "new-menu")
        type("Recovery codes", into: app.textFields["editor-title"])
        let notesField = app.textViews["editor-notes"].exists ? app.textViews["editor-notes"] : app.textFields["editor-notes"]
        type("4821-9930", into: notesField)
        saveScreenshot("add-secure-note")
        app.buttons["editor-save"].tap()
        let notesRow = app.buttons["secure-note-notes"]
        waitFor(notesRow)
        XCTAssertEqual(notesRow.value as? String, "4821-9930")
        saveScreenshot("secure-note")
        XCTAssertEqual(app.buttons.matching(NSPredicate(format: "label == %@", "Copy notes")).count, 1)
        notesRow.press(forDuration: 1)
        let copyNotesReference = app.buttons.matching(NSPredicate(
            format: "label == %@ AND identifier != %@", "Copy secret reference", "copy-secret-reference"
        )).firstMatch
        let copyNotes = app.buttons["Copy"].firstMatch
        waitFor(copyNotes)
        waitFor(copyNotesReference)
        copyNotes.tap()
        waitFor(app.staticTexts["Notes copied. It clears in 90 seconds."])
        notesRow.press(forDuration: 1)
        copyNotesReference.tap()
        waitFor(app.staticTexts["Copied hisar://Personal/Recovery%20codes/notes."])
        goBack()

        choose("New credit card", from: "new-menu", under: "new-more")
        type("Visa", into: app.textFields["editor-title"])
        type("4111111111111111", into: app.secureTextFields["editor-card-number"], normalizedBy: CardInput.formatNumber)
        type("1229", into: app.textFields["editor-card-expiry"], normalizedBy: CardInput.formatExpiry)
        type("123", into: app.secureTextFields["editor-card-security-code"])
        saveScreenshot("add-credit-card")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Visa •••• 1111")).firstMatch)
        XCTAssertEqual(app.buttons["copy-security code"].value as? String, "Hidden", "the security code is shown before it is revealed")
        saveScreenshot("credit-card")
        goBack()

        choose("New identity", from: "new-menu", under: "new-more")
        type("Passport", into: app.textFields["editor-title"])
        type("Alice", into: app.textFields["editor-identity-first-name"])
        type("Example", into: app.textFields["editor-identity-last-name"])
        type("U1234567", into: app.secureTextFields["editor-identity-passport-number"])
        saveScreenshot("add-identity")
        app.buttons["editor-save"].tap()
        let identitySummary = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Alice Example")).firstMatch
        waitFor(identitySummary)
        let passportRow = app.buttons["copy-passport number"]
        XCTAssertEqual(passportRow.value as? String, "Hidden", "the passport number is shown before it is revealed")
        XCTAssertFalse(app.buttons["copy-driver's license number"].exists, "a document number nobody typed was saved")
        saveScreenshot("identity")
        app.buttons["reveal-passport number"].tap()
        XCTAssertEqual(passportRow.value as? String, "U1234567")
        goBack()

        choose("New API credential", from: "new-menu", under: "new-more")
        type("Billing key", into: app.textFields["editor-title"])
        type("Stripe", into: app.textFields["editor-api-credential-service"])
        type("key_1", into: app.textFields["editor-api-credential-key-id"])
        type("sk_live_from_the_phone", into: app.secureTextFields["editor-api-credential-secret"])
        type("api.stripe.com", into: app.textFields["editor-api-credential-host"])
        saveScreenshot("add-api-credential")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Personal · API credential")).firstMatch)
        XCTAssertEqual(app.buttons["copy-service"].value as? String, "Stripe")
        XCTAssertEqual(app.buttons["copy-host"].value as? String, "api.stripe.com")
        let secretRow = app.buttons["copy-secret"]
        XCTAssertEqual(secretRow.value as? String, "Hidden", "the secret is shown before it is revealed")
        saveScreenshot("api-credential")
        app.buttons["reveal-secret"].tap()
        XCTAssertEqual(secretRow.value as? String, "sk_live_from_the_phone")
        goBack()

        choose("New address", from: "new-menu", under: "new-more")
        type("Home", into: app.textFields["editor-title"])
        // A street may take several lines, so it is a text view rather than a text field.
        type("1 Main St", into: app.descendants(matching: .any)["editor-address-street"].firstMatch)
        type("Springfield", into: app.textFields["editor-address-city"])
        type("US", into: app.textFields["editor-address-country"])
        saveScreenshot("add-address")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Springfield, US")).firstMatch)
        XCTAssertEqual(app.buttons["copy-street"].value as? String, "1 Main St", "the street is not shown")
        saveScreenshot("address")
        goBack()

        choose("New software license", from: "new-menu", under: "new-more")
        type("Sketch", into: app.textFields["editor-title"])
        type("Sketch", into: app.textFields["editor-license-product"])
        type("100", into: app.textFields["editor-license-version"])
        type("LICENSE-FROM-THE-PHONE", into: app.secureTextFields["editor-license-key"])
        type("Alice Example", into: app.textFields["editor-license-licensed-to"])
        saveScreenshot("add-software-license")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Version 100 · Alice Example")).firstMatch)
        XCTAssertEqual(app.buttons["copy-license key"].value as? String, "Hidden", "the license key is shown before it is revealed")
        saveScreenshot("software-license")
        goBack()

        choose("New server", from: "new-menu", under: "new-more")
        type("Build server", into: app.textFields["editor-title"])
        type("build.example.com", into: app.textFields["editor-server-host"])
        type("2222", into: app.textFields["editor-server-port"])
        type("deploy", into: app.textFields["editor-username"])
        type("server-password-from-the-phone", into: app.secureTextFields["editor-password"])
        type("JBSWY3DPEHPK3PXP", into: app.secureTextFields["editor-one-time-password"])
        saveScreenshot("add-server")
        app.buttons["editor-save"].tap()
        waitFor(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "build.example.com:2222 · deploy")).firstMatch)
        XCTAssertEqual(app.buttons["copy-password"].value as? String, "Hidden", "the password is shown before it is revealed")
        saveScreenshot("server")
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

        choose("New database", from: "new-menu", under: "new-more")
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
        let connectionStringRow = app.buttons["copy-connection string"]
        waitFor(connectionStringRow)
        XCTAssertEqual(connectionStringRow.value as? String, "Hidden", "the connection string is shown before it is revealed")
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

        app.buttons["open-account"].tap()
        waitForHittable(app.buttons["open-secret-key"])
        app.buttons["open-secret-key"].tap()
        type(Self.masterPassword, into: app.secureTextFields["secret-key-password"])
        app.buttons["secret-key-reveal"].tap()
        let shownSecretKey = app.staticTexts["secret-key-value"]
        waitFor(shownSecretKey, timeout: 60)
        XCTAssertEqual(shownSecretKey.label, secretKey)
        saveScreenshot("secret-key")
        goBack()
        goBack()

        for _ in 0..<5 where !app.buttons["open-help"].isHittable {
            app.swipeUp()
        }
        app.buttons["open-help"].tap()
        XCTAssertFalse(app.staticTexts["App version"].exists)
        app.buttons["copy-diagnostics"].tap()
        saveScreenshot("settings-help")
        goBack()
        let about = app.buttons["open-about"]
        scrollTo(about)
        XCTAssertEqual(about.label, "About Hisar")
        about.tap()
        waitFor(app.staticTexts["Version"])
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
        XCTAssertFalse(app.staticTexts["Sign in to use Hisar on this iPhone."].exists)
        XCTAssertFalse(app.textFields["sign-in-server"].exists)
        useDifferentServerButton.tap()
        let serverField = app.textFields["sign-in-server"]
        waitFor(serverField)
        XCTAssertEqual(serverField.value as? String, "https://eu.tryhisar.com")
        XCTAssertTrue(app.staticTexts["Server"].exists)
        XCTAssertTrue(app.staticTexts["Signs in to eu.tryhisar.com."].exists)
        serverField.tap()
        typeText("/hisar", into: serverField, showing: "https://eu.tryhisar.com/hisar")
        XCTAssertTrue(app.staticTexts["Signs in to eu.tryhisar.com/hisar."].exists)
        app.buttons["sign-in-use-default-server"].tap()
        XCTAssertFalse(serverField.exists)
        XCTAssertTrue(app.staticTexts["Signs in to eu.tryhisar.com."].exists)
        useDifferentServerButton.tap()
        XCTAssertEqual(serverField.value as? String, "https://eu.tryhisar.com")
        app.segmentedControls["sign-in-mode"].buttons["Create account"].tap()
        XCTAssertFalse(app.staticTexts["Enter the invite code you received, or sign up on a server that lets anyone join."].exists)
        XCTAssertTrue(app.textFields["sign-up-invite-link"].exists)
        XCTAssertTrue(app.staticTexts["Leave it empty if the server lets anyone sign up."].exists)
        XCTAssertTrue(app.textFields["sign-up-server"].exists)
    }

    func testPrivacyCoverHidesContentInTheBackground() throws {
        app.launch()
        waitFor(app.segmentedControls["sign-in-mode"])
        for _ in 0..<2 {
            XCUIDevice.shared.press(.home)
            XCTAssertTrue(app.wait(for: .runningBackground, timeout: 10) || app.wait(for: .runningBackgroundSuspended, timeout: 10))
            app.activate()
            waitFor(app.segmentedControls["sign-in-mode"])
            XCTAssertTrue(app.otherElements["privacy-cover"].waitForNonExistence(timeout: 5), "the cover goes away once the app is active")
        }
    }
}
