import XCTest

final class OnboardingUITests: HisarUITestCase {
    private func postponeAccount(_ prefix: String) throws {
        let invite = try readEnvironment("HISAR_UI_TEST_\(prefix)_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_\(prefix)_EMAIL")
        app.launch()
        beginSignUp(inviteLink: invite, email: email, name: "Onboarding")
        dismissSavePasswordPrompt()
        scrollTo(app.buttons["new-secret-key-later"])
        waitForHittable(app.buttons["new-secret-key-later"])
        app.buttons["new-secret-key-later"].tap()
        let notNow = app.buttons["biometric-offer-not-now"]
        if notNow.waitForExistence(timeout: 3) { notNow.tap() }
        waitForHittable(app.buttons["vault-Personal"])
    }

    private func relaunchAndUnlock(largeText: Bool = false) {
        app.terminate()
        app.launchArguments = ["-HisarUITest"]
        if largeText { app.launchArguments += ["-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL"] }
        app.launch()
        waitFor(app.secureTextFields["unlock-password"])
        XCTAssertFalse(app.navigationBars["Finish account setup"].exists)
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        scrollTo(app.buttons["unlock-submit"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.buttons["emergency-kit-banner-save"])
    }

    func testPostponedKitNeedsOnlyTheUnlockPassword() throws {
        try postponeAccount("ONBOARDING_POSTPONED")
        relaunchAndUnlock()
        XCTAssertFalse(app.staticTexts["new-secret-key"].exists)
        scrollTo(app.buttons["emergency-kit-banner-save"])
        app.buttons["emergency-kit-banner-save"].tap()
        waitFor(app.staticTexts["new-secret-key"], timeout: 60)
        XCTAssertFalse(app.secureTextFields["new-secret-key-password"].exists)
        XCTAssertFalse(app.secureTextFields["pending-recovery-password"].exists)
        saveScreenshot("onboarding-postponed-authorized")
        handOffEmergencyKit(shareIdentifier: "new-secret-key-share-kit")
        scrollTo(app.buttons["new-secret-key-done"])
        app.buttons["new-secret-key-done"].tap()
        let notNow = app.buttons["biometric-offer-not-now"]
        if notNow.waitForExistence(timeout: 3) { notNow.tap() }
        waitForHittable(app.buttons["vault-Personal"])
        XCTAssertFalse(app.buttons["emergency-kit-banner-save"].exists)
        saveScreenshot("onboarding-postponed-finished")
    }

    func testAutoLockDuringKitSaveResumesAfterUnlock() throws {
        try postponeAccount("ONBOARDING_AUTOLOCK")
        openSettings("open-security")
        app.buttons["5 minutes"].firstMatch.tap()
        app.buttons["30 seconds"].firstMatch.tap()
        openTab("Vaults")
        scrollTo(app.buttons["emergency-kit-banner-save"])
        app.buttons["emergency-kit-banner-save"].tap()
        waitFor(app.staticTexts["new-secret-key"], timeout: 60)
        scrollTo(app.buttons["new-secret-key-share-kit"])
        app.buttons["new-secret-key-share-kit"].tap()
        waitFor(app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Save to Files")).firstMatch)
        waitFor(app.secureTextFields["unlock-password"], timeout: 65)
        XCTAssertFalse(app.navigationBars["Finish account setup"].exists)
        let interrupted = app.staticTexts["Hisar locked while you were saving your Emergency Kit. Unlock to continue."]
        waitFor(interrupted)
        saveScreenshot("onboarding-autolock-interrupted")
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.staticTexts["new-secret-key"], timeout: 60)
        XCTAssertFalse(app.secureTextFields["new-secret-key-password"].exists)
        XCTAssertFalse(app.buttons["new-secret-key-done"].exists)
        saveScreenshot("onboarding-autolock-resumed")
        scrollTo(app.buttons["new-secret-key-later"])
        app.buttons["new-secret-key-later"].tap()
    }

    func testEmergencyKitBannerDoesNotCoverControls() throws {
        try postponeAccount("EMERGENCY_KIT_BANNER")
        checkBannerLayout("default")
        relaunchAndUnlock(largeText: true)
        checkBannerLayout("largest-text")
    }

    private func checkBannerLayout(_ name: String) {
        let save = app.buttons["emergency-kit-banner-save"]
        let notNow = app.buttons["emergency-kit-banner-not-now"]
        let list = app.collectionViews.firstMatch
        for button in [save, notNow] {
            scrollTo(button)
            waitForHittable(button)
            XCTAssertGreaterThanOrEqual(button.frame.height, 44)
            for control in [app.buttons["lock-now"], app.buttons["new-menu"]] + app.tabBars.buttons.allElementsBoundByIndex {
                XCTAssertTrue(control.isHittable, "The reminder covers \(control.identifier)")
            }
        }
        saveScreenshot("onboarding-banner-\(name)")
        XCTAssertTrue(list.otherElements["emergency-kit-banner"].exists, "The reminder must be part of the Vaults list")
        let personal = app.buttons["vault-Personal"]
        let offset = notNow.frame.minY - personal.frame.minY
        list.swipeUp()
        XCTAssertEqual(notNow.frame.minY - personal.frame.minY, offset, accuracy: 1, "The reminder must scroll with Vaults")
        scrollTo(app.buttons["vault-Personal"])
        waitForHittable(app.buttons["vault-Personal"])
    }

    func testInviteOnlyServerAsksForInvitationFirst() throws {
        let server = try readEnvironment("HISAR_UI_TEST_SERVER")
        app.launch()
        waitFor(app.buttons["welcome-create-account"])
        XCTAssertFalse(app.textFields["sign-in-secret-key"].exists)
        saveScreenshot("onboarding-welcome")
        selectSignInMode("Create account")
        app.buttons["sign-up-use-different-server"].tap()
        replaceText(server, in: app.textFields["sign-up-server"])
        waitFor(app.textFields["sign-up-invite-link"])
        XCTAssertFalse(app.textFields["sign-up-name"].exists)
        XCTAssertFalse(app.textFields["sign-up-email"].exists)
        XCTAssertFalse(app.secureTextFields["sign-up-password"].exists)
        XCTAssertTrue(app.textFields["sign-up-invite-link"].label.contains("required"))
        saveScreenshot("onboarding-invitation-first")
    }

    func testOpenServerCreatesAccountWithoutInvitation() throws {
        let server = try readEnvironment("HISAR_UI_TEST_OPEN_SERVER")
        let email = try readEnvironment("HISAR_UI_TEST_OPEN_EMAIL")
        app.launch()
        selectSignInMode("Create account")
        app.buttons["sign-up-use-different-server"].tap()
        replaceText(server, in: app.textFields["sign-up-server"])
        waitFor(app.textFields["sign-up-email"])
        XCTAssertFalse(app.textFields["sign-up-invite-link"].exists)
        saveScreenshot("onboarding-open-server")
        type(email, into: app.textFields["sign-up-email"])
        type("Open", into: app.textFields["sign-up-name"])
        type(Self.masterPassword, into: app.secureTextFields["sign-up-password"])
        type(Self.masterPassword, into: app.secureTextFields["sign-up-repeated-password"])
        XCTAssertFalse(app.textFields["sign-up-invite-link"].exists)
        scrollTo(app.buttons["sign-in-submit"])
        app.buttons["sign-in-submit"].tap()
        let key = app.staticTexts["new-secret-key"]
        waitFor(key, timeout: 60)
        XCTAssertTrue(key.label.hasPrefix("H1-"), key.label)
        saveScreenshot("onboarding-open-server-kit")
    }

    func testLostKeyHelpPreservesCredentials() {
        app.launch()
        selectSignInMode("Sign in")
        type("ada@example.com", into: app.textFields["sign-in-email"])
        type("H1-KEPT", into: app.textFields["sign-in-secret-key"])
        type("typed master password", into: app.secureTextFields["sign-in-password"])
        scrollTo(app.buttons["secret-key-note"])
        app.buttons["secret-key-note"].tap()
        waitFor(app.buttons["secret-key-help-done"])
        saveScreenshot("onboarding-lost-key-help")
        app.buttons["secret-key-help-done"].tap()
        scrollTo(app.textFields["sign-in-email"])
        XCTAssertEqual(app.textFields["sign-in-email"].value as? String, "ada@example.com")
        XCTAssertEqual(app.textFields["sign-in-secret-key"].value as? String, "H1-KEPT")
        scrollTo(app.buttons["sign-in-password-reveal"])
        app.buttons["sign-in-password-reveal"].tap()
        XCTAssertEqual(app.textFields["sign-in-password"].value as? String, "typed master password")
    }
}
