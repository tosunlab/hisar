import XCTest

final class SharingUITests: HisarUITestCase {
    func testCreateVaultShareByEmailAndInvite() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_SHARING_INVITE_LINK")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: "ios-sharing@example.com", name: "iOS Sharing")

        app.buttons["new-vault"].tap()
        type("Streaming", into: app.textFields["new-vault-name"])
        saveScreenshot("new-vault")
        app.buttons["new-vault-create"].tap()
        waitFor(app.buttons["add-item"], timeout: 60)
        XCTAssertTrue(app.navigationBars["Streaming"].exists, "the new vault opens")

        app.buttons["vault-members"].tap()
        waitFor(app.otherElements["vault-member-ios-sharing@example.com"].exists ? app.otherElements["vault-member-ios-sharing@example.com"] : app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] 'iOS Sharing (you)'")).firstMatch, timeout: 30)
        XCTAssertTrue(app.staticTexts["Owner"].exists)
        saveScreenshot("vault-members")
        app.buttons["share-vault"].tap()
        type("partner@example.com", into: app.textFields["share-email"])
        app.buttons["share-vault-submit"].tap()
        waitFor(app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'partner@example.com'")).firstMatch, timeout: 30)
        saveScreenshot("vault-invitation")
        app.buttons["vault-members-done"].tap()

        app.navigationBars.buttons.element(boundBy: 0).tap()
        waitFor(app.buttons["vault-Streaming"])
        XCTAssertTrue(app.buttons["vault-Personal"].exists)

        app.buttons["open-settings"].tap()
        waitFor(app.buttons["open-people"])
        app.buttons["open-people"].tap()
        waitFor(app.buttons["invite-person"], timeout: 30)
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'partner@example.com'")).firstMatch.exists, "the pending invitation is listed")
        saveScreenshot("people")
        app.buttons["invite-person"].tap()
        type("partner@example.com", into: app.textFields["invite-email"])
        app.buttons["invite-submit"].tap()
        let sentOrNotConfigured = NSPredicate(format: "label CONTAINS 'Invitation sent' OR label CONTAINS 'cannot send email'")
        waitFor(app.staticTexts.matching(sentOrNotConfigured).firstMatch, timeout: 30)
    }
}
