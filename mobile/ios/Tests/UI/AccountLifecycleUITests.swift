import XCTest

final class AccountLifecycleUITests: HisarUITestCase {
    static let newMasterPassword = "ui test new master password"

    func testChangePasswordReplaceSecretKeyAndDeleteAccount() throws {
        guard let inviteLink = ProcessInfo.processInfo.environment["HISAR_UI_TEST_LIFECYCLE_INVITE_LINK"], !inviteLink.isEmpty,
              let email = ProcessInfo.processInfo.environment["HISAR_UI_TEST_LIFECYCLE_EMAIL"], !email.isEmpty else {
            throw XCTSkip("HISAR_UI_TEST_LIFECYCLE_INVITE_LINK and HISAR_UI_TEST_LIFECYCLE_EMAIL not set")
        }
        let screenshotSuffix = ProcessInfo.processInfo.environment["HISAR_UI_TEST_SCREENSHOT_SUFFIX"] ?? ""
        app.launch()
        waitFor(app.segmentedControls["sign-in-mode"])
        XCTAssertFalse(app.staticTexts["Sign in to use Hisar on this iPhone."].exists)
        app.segmentedControls["sign-in-mode"].buttons["Create account"].tap()
        XCTAssertFalse(app.staticTexts["Enter the invite code you received, or paste the invite link."].exists)
        signUp(inviteLink: inviteLink, email: email, name: "Lifecycle")
        dismissSavePasswordPrompt()

        openSettings("open-mac-approvals")
        XCTAssertTrue(app.staticTexts["When Hisar on your Mac asks to use an SSH key or the command-line tool, you can approve or deny it here while Hisar is open and unlocked on this iPhone."].exists)
        XCTAssertFalse(app.staticTexts["How requests appear"].exists)
        goBack()

        openSettings("open-security")
        app.buttons["open-change-master-password"].tap()
        let passwordWarning = app.staticTexts["Your other devices will be signed out. Sign in on them with the new master password."]
        XCTAssertFalse(passwordWarning.exists)
        type(Self.masterPassword, into: app.secureTextFields["change-password-current"])
        type(Self.newMasterPassword, into: app.secureTextFields["change-password-new"])
        type(Self.newMasterPassword, into: app.secureTextFields["change-password-repeated"])
        saveScreenshot("account-life-change-master-password\(screenshotSuffix)")
        app.buttons["change-password-submit"].tap()
        waitFor(passwordWarning)
        tapConfirmation(identifier: "change-password-confirm")
        dismissSavePasswordPrompt()
        waitFor(app.buttons["open-replace-secret-key"], timeout: 60)

        app.buttons["open-replace-secret-key"].tap()
        let secretKeyWarning = app.staticTexts["Your other devices will be signed out. Sign in on them with your master password and the new Secret Key from the new Emergency Kit. Your old Emergency Kit stops working."]
        XCTAssertFalse(secretKeyWarning.exists)
        type(Self.newMasterPassword, into: app.secureTextFields["replace-secret-key-password"])
        saveScreenshot("account-life-replace-secret-key\(screenshotSuffix)")
        app.buttons["replace-secret-key-submit"].tap()
        waitFor(secretKeyWarning)
        tapConfirmation(identifier: "replace-secret-key-confirm")
        let newSecretKey = finishPendingRecovery(email: email, password: Self.newMasterPassword, restartAfterPreparingKit: true)
        XCTAssertTrue(newSecretKey.hasPrefix("H1-"), newSecretKey)
        saveScreenshot("account-life-new-secret-key\(screenshotSuffix)")

        if app.secureTextFields["unlock-password"].waitForExistence(timeout: 2) {
            type(Self.newMasterPassword, into: app.secureTextFields["unlock-password"])
            app.buttons["unlock-submit"].tap()
        }
        waitFor(app.buttons["vault-Personal"], timeout: 60)
        openSettings("open-account")
        app.buttons["open-delete-account"].tap()
        saveScreenshot("account-life-delete-account-preview\(screenshotSuffix)")
        type(Self.newMasterPassword, into: app.secureTextFields["delete-account-password"])
        type(email, into: app.textFields["delete-account-email"], untilEnabled: app.buttons["delete-account-submit"])
        saveScreenshot("account-life-delete-account\(screenshotSuffix)")
        app.buttons["delete-account-submit"].tap()
        tapConfirmation(identifier: "delete-account-confirm")
        waitFor(app.segmentedControls["sign-in-mode"], timeout: 60)
        saveScreenshot("account-life-deleted\(screenshotSuffix)")
    }

    private func tapConfirmation(identifier: String, file: StaticString = #filePath, line: UInt = #line) {
        let button = app.buttons[identifier].firstMatch
        waitForHittable(button, file: file, line: line)
        button.tap()
    }
}
