import XCTest

// scripts/test-ios-ui.sh runs a partner on the command line who accepts every share invitation and completes every share it sent.
final class SharingUITests: HisarUITestCase {
    override func tearDown() {
        XCUIDevice.shared.appearance = .light
        super.tearDown()
    }

    private func findElement(_ identifier: String) -> XCUIElement {
        app.descendants(matching: .any)[identifier].firstMatch
    }

    private func findText(containing text: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch
    }

    private func saveLightAndDarkScreenshot(_ name: String) {
        saveScreenshot("\(name)-light")
        XCUIDevice.shared.appearance = .dark
        Thread.sleep(forTimeInterval: 1.5)
        saveScreenshot("\(name)-dark")
        XCUIDevice.shared.appearance = .light
        Thread.sleep(forTimeInterval: 1.5)
    }

    private func chooseRole(_ role: String, in picker: XCUIElement) {
        picker.tap()
        app.buttons[role].firstMatch.tap()
    }

    func testShareVaultFromTheToolbarAndManageItsMembers() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_SHARING_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_SHARING_EMAIL")
        let partnerEmail = try readEnvironment("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        let unknownEmail = "nobody-\(UUID().uuidString.prefix(8).lowercased())@example.com"
        let invitedEmail = "invited-\(UUID().uuidString.prefix(8).lowercased())@example.com"
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: email, name: "iOS Sharing")

        app.buttons["vault-Personal"].tap()
        app.buttons["share-vault"].tap()
        app.alerts["Personal vaults are private"].buttons["OK"].tap()
        goBack()

        choose("new-vault", from: "new-menu")
        type("Deploy keys", into: app.textFields["new-vault-name"])
        let createButton = app.buttons["new-vault-create"]
        XCTAssertTrue(createButton.wait(for: \.isEnabled, toEqual: true, timeout: 30), "Create waits until Where is loaded")
        createButton.tap()
        waitFor(app.navigationBars["Deploy keys"], timeout: 60)

        app.buttons["share-vault"].tap()
        type(partnerEmail, into: app.textFields["share-email"])
        chooseRole("Reader", in: app.buttons["share-role"])
        XCTAssertEqual(app.buttons["share-vault-submit"].label, "Send invitation")
        app.buttons["share-vault-submit"].tap()
        waitFor(findText(containing: "Invitation sent to \(partnerEmail)"), timeout: 30)
        saveLightAndDarkScreenshot("sharing-share-sheet")

        type(unknownEmail, into: app.textFields["share-email"])
        app.buttons["share-vault-submit"].tap()
        waitFor(findText(containing: "Invitation sent to \(unknownEmail)"), timeout: 30)

        let reviewRecipient = app.buttons["review-share-\(partnerEmail)"]
        waitFor(reviewRecipient, timeout: 90)
        reviewRecipient.tap()
        app.buttons["confirm-share-recipient"].firstMatch.tap()
        app.swipeUp()
        let partnerRow = findElement("vault-member-\(partnerEmail)")
        waitFor(partnerRow, timeout: 90)
        XCTAssertTrue(partnerRow.label.contains("Reader"), partnerRow.label)
        let unknownInvitation = findElement("vault-invitation-\(unknownEmail)")
        saveLightAndDarkScreenshot("sharing-members")

        revealSwipeActions(of: unknownInvitation, .left)
        let cancelAction = app.buttons["Cancel"].firstMatch
        waitForHittable(cancelAction)
        cancelAction.tap()
        XCTAssertEqual(app.buttons["confirm-cancel-invitation"].firstMatch.label, "Cancel invitation")
        XCTAssertTrue(app.buttons["Keep invitation"].exists)
        app.buttons["confirm-cancel-invitation"].firstMatch.tap()
        XCTAssertTrue(unknownInvitation.waitForNonExistence(timeout: 30), "the cancelled invitation disappears")

        partnerRow.tap()
        saveLightAndDarkScreenshot("sharing-member")
        chooseRole("Editor", in: app.buttons["member-role"])
        app.navigationBars["iOS Partner"].buttons.element(boundBy: 0).tap()
        waitFor(findText(containing: "Editor"))

        findElement("vault-member-\(partnerEmail)").tap()
        saveScreenshot("sharing-member-editor")
        app.swipeUp()
        XCTAssertEqual(app.buttons["remove-member"].label, "Remove from vault")
        app.buttons["remove-member"].tap()
        app.buttons["confirm-remove-member"].firstMatch.tap()
        XCTAssertTrue(findElement("vault-member-\(partnerEmail)").waitForNonExistence(timeout: 60), "the removed person leaves the members")
        app.buttons["vault-sharing-done"].tap()

        goBack()
        openSettings("open-people")
        saveScreenshot("people-invite")
        app.buttons["invite-person"].tap()
        type(invitedEmail, into: app.textFields["invite-email"])
        app.buttons["invite-submit"].tap()
        let sentOrNotConfigured = NSPredicate(format: "label CONTAINS 'Invitation sent' OR label CONTAINS 'cannot send email'")
        waitFor(app.staticTexts.matching(sentOrNotConfigured).firstMatch, timeout: 30)
    }

    func testAcceptAnInvitationFromTheBannerAboveTheVaults() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_BANNER_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_BANNER_EMAIL")
        let partnerEmail = try readEnvironment("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: email, name: "iOS Banner")

        let acceptButton = app.buttons["banner-accept-invitation"]
        waitFor(acceptButton, timeout: 30)
        XCTAssertFalse(app.buttons["vault-Infrastructure"].exists, "no access before accepting")
        saveLightAndDarkScreenshot("sharing-invitation-banner")

        acceptButton.tap()
        let sharedVault = app.buttons["vault-Infrastructure"]
        waitFor(sharedVault, timeout: 90)
        XCTAssertTrue(sharedVault.label.contains("Editor"), sharedVault.label)
        sharedVault.tap()
        waitFor(app.buttons["item-Staging database"], timeout: 30)
        XCTAssertFalse(app.buttons["share-vault"].exists, "only owners share")

        app.buttons["vault-members"].tap()
        waitFor(findElement("vault-member-\(partnerEmail)"), timeout: 30)
        XCTAssertFalse(app.textFields["share-email"].exists, "editors cannot invite")
        app.buttons["vault-sharing-done"].tap()
    }
}
