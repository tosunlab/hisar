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
        signUp(inviteLink: inviteLink, email: email, name: "Lifecycle")
        dismissSavePasswordPrompt()

        openSettings("open-mac-approvals")
        XCTAssertTrue(app.staticTexts["When Hisar on your Mac asks to use an SSH key or the command-line tool, you can approve or deny it here while Hisar is open and unlocked on this iPhone."].exists)
        XCTAssertFalse(app.staticTexts["How requests appear"].exists)
        goBack()

        openSettings("open-security")
        app.buttons["open-change-master-password"].tap()
        type(Self.masterPassword, into: app.secureTextFields["change-password-current"])
        type(Self.newMasterPassword, into: app.secureTextFields["change-password-new"])
        type(Self.newMasterPassword, into: app.secureTextFields["change-password-repeated"])
        saveScreenshot("account-life-change-master-password\(screenshotSuffix)")
        app.buttons["change-password-submit"].tap()
        tapConfirmation(identifier: "change-password-confirm")
        dismissSavePasswordPrompt()
        waitFor(app.buttons["open-replace-secret-key"], timeout: 60)

        app.buttons["open-replace-secret-key"].tap()
        type(Self.newMasterPassword, into: app.secureTextFields["replace-secret-key-password"])
        saveScreenshot("account-life-replace-secret-key\(screenshotSuffix)")
        app.buttons["replace-secret-key-submit"].tap()
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
