import XCTest

// Other tests and earlier runs share the server, so every email here is unique to this run and every check is scoped to it.
final class SharingUITests: HisarUITestCase {
    func testCreateVaultShareByEmailAndInvite() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_SHARING_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_SHARING_EMAIL")
        let partnerEmail = "partner-\(UUID().uuidString.prefix(8).lowercased())@example.com"
        let invitedEmail = "invited-\(UUID().uuidString.prefix(8).lowercased())@example.com"
        let mentionsPartner = NSPredicate(format: "label CONTAINS %@", partnerEmail)
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: email, name: "iOS Sharing")

        app.buttons["new-menu"].tap()
        waitFor(app.buttons["new-vault"])
        app.buttons["new-vault"].tap()
        type("Streaming", into: app.textFields["new-vault-name"])
        saveScreenshot("new-vault")
        app.buttons["new-vault-create"].tap()
        waitFor(app.buttons["add-item"], timeout: 60)
        XCTAssertTrue(app.navigationBars["Streaming"].exists, "the new vault opens")

        app.buttons["vault-members"].tap()
        waitFor(app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", email)).firstMatch, timeout: 30)
        XCTAssertTrue(app.staticTexts["Owner"].exists)
        saveScreenshot("vault-members")
        app.buttons["share-vault"].tap()
        type(partnerEmail, into: app.textFields["share-email"])
        app.buttons["share-vault-submit"].tap()
        waitFor(app.staticTexts.matching(mentionsPartner).firstMatch, timeout: 30)
        saveScreenshot("vault-invitation")
        app.buttons["vault-members-done"].tap()

        goBack()
        waitFor(app.buttons["vault-Streaming"])
        XCTAssertTrue(app.buttons["vault-Personal"].exists)

        openSettings("open-people")
        waitFor(app.buttons["invite-person"], timeout: 30)
        waitFor(app.staticTexts.matching(mentionsPartner).firstMatch, timeout: 30)
        XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'Admin'")).firstMatch.exists, "People shows no account roles")
        saveScreenshot("people")
        app.buttons["invite-person"].tap()
        type(invitedEmail, into: app.textFields["invite-email"])
        app.buttons["invite-submit"].tap()
        let sentOrNotConfigured = NSPredicate(format: "label CONTAINS 'Invitation sent' OR label CONTAINS 'cannot send email'")
        waitFor(app.staticTexts.matching(sentOrNotConfigured).firstMatch, timeout: 30)
    }
}
