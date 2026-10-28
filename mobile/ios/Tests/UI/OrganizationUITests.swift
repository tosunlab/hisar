import XCTest

// scripts/test-ios-ui.sh runs a partner on the command line who accepts every organization invitation, gives team members
// the keys of its vaults and writes "Seen by partner" into Runbooks once it reads it.
final class OrganizationUITests: HisarUITestCase {
    private func findElement(_ identifier: String) -> XCUIElement {
        app.descendants(matching: .any)[identifier].firstMatch
    }

    func testCreateAnOrganizationAndGiveItsTeamAVault() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_ORGANIZATION_ADMIN_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_ORGANIZATION_ADMIN_EMAIL")
        let partnerEmail = try readEnvironment("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: email, name: "iOS Organization Admin")

        openSettings("open-organizations")
        app.buttons["create-organization"].tap()
        type("Contoso", into: app.textFields["new-organization-name"])
        app.buttons["new-organization-create"].tap()
        waitFor(app.navigationBars["Contoso"], timeout: 30)

        type(partnerEmail, into: app.textFields["organization-invite-email"])
        app.buttons["organization-invite-submit"].tap()
        let partnerMember = findElement("organization-member-\(partnerEmail)")
        waitFor(partnerMember, timeout: 120)
        XCTAssertTrue(partnerMember.label.contains("Member"), partnerMember.label)

        let newTeamButton = app.buttons["new-team"]
        scrollTo(newTeamButton)
        newTeamButton.tap()
        let newTeamAlert = app.alerts["New team"]
        let teamNameField = newTeamAlert.textFields.firstMatch
        teamNameField.tap()
        typeText("Ops", into: teamNameField)
        newTeamAlert.buttons["Create"].firstMatch.tap()
        let addMemberMenu = app.buttons["team-Ops-add-member"]
        scrollTo(addMemberMenu)
        choose("team-Ops-add-\(email)", from: "team-Ops-add-member")
        waitFor(findElement("team-Ops-member-\(email)"), timeout: 30)
        scrollTo(addMemberMenu)
        choose("team-Ops-add-\(partnerEmail)", from: "team-Ops-add-member")
        waitFor(findElement("team-Ops-member-\(partnerEmail)"), timeout: 30)
        saveScreenshot("organization-screen")

        openTab("Vaults")
        choose("new-vault", from: "new-menu")
        type("Runbooks", into: app.textFields["new-vault-name"])
        let wherePicker = app.buttons["new-vault-where"]
        waitFor(wherePicker, timeout: 30)
        wherePicker.tap()
        app.buttons["Contoso › Ops"].tap()
        saveScreenshot("new-vault-where")
        app.buttons["new-vault-create"].tap()
        waitFor(app.navigationBars["Runbooks"], timeout: 60)

        // No password: iOS would offer to save it in a sheet that can come up after any wait for it.
        choose("add-login", from: "add-item")
        type("Pager", into: app.textFields["editor-title"])
        type("pager-oncall", into: app.textFields["editor-username"])
        app.buttons["editor-save"].tap()
        waitFor(app.buttons["item-Pager"], timeout: 30)

        app.buttons["share-vault"].tap()
        let opsRow = findElement("vault-team-Ops")
        waitFor(opsRow, timeout: 30)
        XCTAssertTrue(opsRow.label.contains("Owner"), opsRow.label)
        XCTAssertTrue(app.buttons["give-vault-to-team"].exists, "admins give the vault to other teams")
        saveScreenshot("organization-vault-sharing-admin")
        opsRow.tap()
        goBack()
        app.buttons["vault-sharing-done"].tap()

        // The partner reads Pager through the key this iPhone gave it, then writes into the vault.
        waitFor(app.buttons["item-Seen by partner"], timeout: 180)

        goBack()
        waitFor(app.buttons["vault-Runbooks"])
        saveScreenshot("organization-vaults-admin")

        // Settings still shows Contoso, opened after creating it.
        openTab("Settings")
        goBack()
        let contosoRow = app.buttons["organization-Contoso"]
        waitFor(contosoRow)
        XCTAssertTrue(contosoRow.label.contains("Admin · 2 members"), contosoRow.label)
        saveScreenshot("organizations-list")
    }
}
