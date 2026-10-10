import XCTest

final class SharingUITests: HisarUITestCase {
    func testCreateVaultSeeMembersAndInvite() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_SHARING_INVITE_LINK")
        let server = try readEnvironment("HISAR_UI_TEST_SERVER")
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
        waitFor(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Everyone in your family already has access'")).firstMatch, timeout: 30)
        app.buttons["Cancel"].tap()
        app.buttons["vault-members-done"].tap()

        app.navigationBars.buttons.element(boundBy: 0).tap()
        waitFor(app.buttons["vault-Streaming"])
        XCTAssertTrue(app.buttons["vault-Personal"].exists)

        app.buttons["open-settings"].tap()
        waitFor(app.buttons["open-family"])
        app.buttons["open-family"].tap()
        waitFor(app.buttons["invite-member"], timeout: 30)
        saveScreenshot("family")
        app.buttons["invite-member"].tap()
        type("partner@example.com", into: app.textFields["invite-email"])
        app.segmentedControls["invite-role"].buttons["Admin"].tap()
        app.buttons["invite-submit"].tap()
        let inviteLinkText = app.staticTexts["invite-link"]
        waitFor(inviteLinkText, timeout: 30)
        XCTAssertTrue(inviteLinkText.label.hasPrefix("\(server)/invite/"), inviteLinkText.label)
        saveScreenshot("invite-link")
        app.buttons["invite-copy"].tap()
        waitFor(app.buttons["Copied"], timeout: 5)
        app.buttons["invite-done"].tap()
        waitFor(app.buttons["invite-member"])
    }
}
