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
        waitFor(picker)
        picker.tap()
        let option = app.buttons[role].firstMatch
        waitFor(option)
        option.tap()
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
        waitFor(app.buttons["share-vault"])
        app.buttons["share-vault"].tap()
        let personalAlert = app.alerts["Personal vaults are private"]
        waitFor(personalAlert)
        personalAlert.buttons["OK"].tap()
        goBack()

        choose("new-vault", from: "new-menu")
        type("Deploy keys", into: app.textFields["new-vault-name"])
        let createButton = app.buttons["new-vault-create"]
        XCTAssertTrue(createButton.wait(for: \.isEnabled, toEqual: true, timeout: 30), "Create waits until Where is loaded")
        createButton.tap()
        waitFor(app.buttons["add-item"], timeout: 60)
        XCTAssertTrue(app.navigationBars["Deploy keys"].exists, "the new vault opens")

        app.buttons["share-vault"].tap()
        waitFor(app.textFields["share-email"])
        waitFor(findElement("vault-member-\(email)"), timeout: 30)
        XCTAssertTrue(findElement("vault-member-\(email)").label.contains("Owner"))
        XCTAssertTrue(findElement("vault-member-\(email)").label.contains("Security code"))
        type(partnerEmail, into: app.textFields["share-email"])
        chooseRole("Reader", in: app.buttons["share-role"])
        app.buttons["share-vault-submit"].tap()
        waitFor(findText(containing: "Invitation sent to \(partnerEmail)"), timeout: 30)
        saveLightAndDarkScreenshot("sharing-share-sheet")

        type(unknownEmail, into: app.textFields["share-email"])
        app.buttons["share-vault-submit"].tap()
        waitFor(findText(containing: "Invitation sent to \(unknownEmail)"), timeout: 30)
        XCTAssertFalse(findElement("error-message").exists, "an address without an account gets the same answer")

        app.swipeUp()
        let partnerRow = findElement("vault-member-\(partnerEmail)")
        waitFor(partnerRow, timeout: 90)
        XCTAssertTrue(partnerRow.label.contains("Reader"), partnerRow.label)
        XCTAssertTrue(partnerRow.label.contains("Security code"), partnerRow.label)
        let unknownInvitation = findElement("vault-invitation-\(unknownEmail)")
        waitFor(unknownInvitation)
        XCTAssertTrue(unknownInvitation.label.contains("Waiting for them to accept"), unknownInvitation.label)
        saveLightAndDarkScreenshot("sharing-members")

        unknownInvitation.swipeLeft()
        app.buttons["Cancel"].firstMatch.tap()
        let confirmCancel = app.buttons["confirm-cancel-invitation"].firstMatch
        waitFor(confirmCancel)
        confirmCancel.tap()
        XCTAssertTrue(unknownInvitation.waitForNonExistence(timeout: 30), "the cancelled invitation disappears")

        partnerRow.tap()
        waitFor(findElement("member-security-code"))
        saveLightAndDarkScreenshot("sharing-member")
        chooseRole("Editor", in: app.buttons["member-role"])
        let memberBackButton = app.navigationBars["iOS Partner"].buttons.element(boundBy: 0)
        waitFor(memberBackButton)
        memberBackButton.tap()
        waitFor(findText(containing: "Editor"))
        XCTAssertTrue(findElement("vault-member-\(partnerEmail)").label.contains("Editor"), findElement("vault-member-\(partnerEmail)").label)

        findElement("vault-member-\(partnerEmail)").tap()
        waitFor(findElement("member-security-code"))
        saveScreenshot("sharing-member-editor")
        app.swipeUp()
        waitFor(app.buttons["remove-member"])
        app.buttons["remove-member"].tap()
        let confirmRemove = app.buttons["confirm-remove-member"].firstMatch
        waitFor(confirmRemove)
        confirmRemove.tap()
        XCTAssertTrue(findElement("vault-member-\(partnerEmail)").waitForNonExistence(timeout: 60), "the removed person leaves the members")
        XCTAssertTrue(findElement("vault-member-\(email)").exists)
        app.buttons["vault-sharing-done"].tap()

        goBack()
        openSettings("open-people")
        waitFor(app.buttons["invite-person"], timeout: 30)
        XCTAssertTrue(app.buttons["invite-person"].label.contains("Invite to Hisar"), app.buttons["invite-person"].label)
        XCTAssertTrue(findText(containing: "Creates a Hisar account: the server emails them a sign-up link.").exists)
        saveScreenshot("people-invite")
        app.buttons["invite-person"].tap()
        waitFor(app.navigationBars["Invite to Hisar"])
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
        let invitationText = findElement("banner-invitation-text")
        XCTAssertTrue(invitationText.label.contains("iOS Partner shared a vault with you as editor"), invitationText.label)
        XCTAssertTrue(invitationText.label.contains(partnerEmail), invitationText.label)
        XCTAssertTrue(app.buttons["banner-decline-invitation"].exists)
        XCTAssertFalse(app.buttons["vault-Infrastructure"].exists, "no access before accepting")
        saveLightAndDarkScreenshot("sharing-invitation-banner")

        acceptButton.tap()
        XCTAssertTrue(acceptButton.waitForNonExistence(timeout: 30), "the banner goes away once accepted")
        let sharedVault = app.buttons["vault-Infrastructure"]
        waitFor(sharedVault, timeout: 90)
        XCTAssertTrue(sharedVault.label.contains("Editor"), sharedVault.label)
        sharedVault.tap()
        waitFor(app.buttons["item-Staging database"], timeout: 30)
        XCTAssertFalse(app.buttons["share-vault"].exists, "only owners share")

        app.buttons["vault-members"].tap()
        waitFor(findElement("vault-member-\(partnerEmail)"), timeout: 30)
        XCTAssertTrue(findElement("vault-member-\(partnerEmail)").label.contains("Owner"))
        XCTAssertTrue(findElement("vault-member-\(email)").label.contains("Editor"))
        XCTAssertFalse(app.textFields["share-email"].exists, "editors cannot invite")
        app.buttons["vault-sharing-done"].tap()
    }
}
