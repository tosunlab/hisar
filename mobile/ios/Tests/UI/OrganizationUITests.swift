import XCTest

// scripts/test-ios-ui.sh runs a partner on the command line: the admin of Northwind, whose team Platform has Deploy as
// editor and Audit as reader. It adds the joining account to Platform once it joined, removes it again once it saved
// "Remove me" in Deploy, accepts every organization invitation and writes "Seen by partner" into Runbooks once it reads it.
final class OrganizationUITests: HisarUITestCase {
    private func findElement(_ identifier: String) -> XCUIElement {
        app.descendants(matching: .any)[identifier].firstMatch
    }

    private func findText(containing text: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch
    }

    func testJoinAnOrganizationAndReachItsVaultsThroughATeam() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_ORGANIZATION_JOIN_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_ORGANIZATION_JOIN_EMAIL")
        let partnerEmail = try readEnvironment("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: email, name: "iOS Organization Member")

        let acceptButton = app.buttons["banner-accept-organization-invitation"]
        waitFor(acceptButton, timeout: 30)
        let invitationText = findElement("banner-organization-invitation-text")
        XCTAssertTrue(invitationText.label.contains("iOS Partner invited you to Northwind as a member"), invitationText.label)
        XCTAssertTrue(invitationText.label.contains(partnerEmail), invitationText.label)
        XCTAssertTrue(app.buttons["banner-decline-organization-invitation"].exists)
        saveScreenshot("organization-invitation-banner")
        acceptButton.tap()
        XCTAssertTrue(acceptButton.waitForNonExistence(timeout: 30), "the banner goes away once accepted")

        let deployVault = app.buttons["vault-Deploy"]
        let auditVault = app.buttons["vault-Audit"]
        waitFor(deployVault, timeout: 120)
        waitFor(auditVault, timeout: 60)
        XCTAssertTrue(deployVault.label.contains("Organization · Editor"), deployVault.label)
        XCTAssertTrue(auditVault.label.contains("Organization · Reader"), auditVault.label)
        let northwindSection = findElement("organization-section-Northwind")
        waitFor(northwindSection)
        XCTAssertGreaterThan(northwindSection.frame.minY, app.buttons["vault-Personal"].frame.minY, "the organization's section comes after the other vaults")
        XCTAssertLessThan(northwindSection.frame.minY, min(deployVault.frame.minY, auditVault.frame.minY), "both vaults are listed under Northwind")
        saveScreenshot("organization-vaults")

        auditVault.tap()
        let auditItem = app.buttons["item-Audit log"]
        waitFor(auditItem, timeout: 30)
        XCTAssertFalse(app.buttons["add-item"].exists, "readers cannot add items")
        auditItem.tap()
        waitFor(app.navigationBars["Audit log"])
        waitFor(findElement("favorite-item"))
        XCTAssertFalse(app.buttons["edit-item"].exists, "readers cannot edit")
        goBack()
        waitFor(auditItem)
        goBack()

        deployVault.tap()
        waitFor(app.buttons["item-Release signing"], timeout: 30)
        XCTAssertFalse(app.buttons["share-vault"].exists, "only owners get Share")
        app.buttons["vault-members"].tap()
        waitFor(findText(containing: "Shared through teams in Northwind"), timeout: 30)
        let platformRow = findElement("vault-team-Platform")
        waitFor(platformRow)
        XCTAssertTrue(platformRow.label.contains("Editor"), platformRow.label)
        XCTAssertTrue(platformRow.label.contains("iOS Organization Member (you)"), platformRow.label)
        XCTAssertTrue(findElement("vault-team-Owners").label.contains("Owner"), findElement("vault-team-Owners").label)
        XCTAssertTrue(findText(containing: "Ask an admin of Northwind to change who has this vault.").exists)
        XCTAssertFalse(app.textFields["share-email"].exists, "an organization vault has no Invite by email")
        XCTAssertFalse(app.buttons["give-vault-to-team"].exists, "only admins give the vault to a team")
        saveScreenshot("organization-vault-sharing-member")
        app.buttons["vault-sharing-done"].tap()

        choose("leave-vault", from: "vault-actions")
        let blockedAlert = app.alerts["Can't do that"]
        waitFor(blockedAlert)
        XCTAssertTrue(blockedAlert.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "through your teams in its organization")).firstMatch.exists, "the alert gives the organization's reason")
        blockedAlert.buttons["OK"].tap()

        choose("add-login", from: "add-item")
        type("Remove me", into: app.textFields["editor-title"])
        type("member", into: app.textFields["editor-username"])
        app.buttons["editor-save"].tap()
        XCTAssertTrue(app.buttons["editor-save"].waitForNonExistence(timeout: 30), "the item is saved")

        // The partner sees the item, removes this account from Platform and gives both vaults a new key, which can happen
        // before the vault's list shows the item: the notice is what this account sees.
        waitFor(findText(containing: "“Deploy” is no longer available"), timeout: 180)
        waitFor(findText(containing: "“Audit” is no longer available"), timeout: 30)
        XCTAssertTrue(deployVault.waitForNonExistence(timeout: 30), "Deploy is gone with the team")
        XCTAssertFalse(auditVault.exists, "Audit is gone with the team")
        XCTAssertFalse(northwindSection.exists, "no vaults are left under Northwind")
        saveScreenshot("organization-vaults-removed")
    }

    func testCreateAnOrganizationAndGiveItsTeamAVault() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_ORGANIZATION_ADMIN_INVITE_LINK")
        let email = try readEnvironment("HISAR_UI_TEST_ORGANIZATION_ADMIN_EMAIL")
        let partnerEmail = try readEnvironment("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: email, name: "iOS Organization Admin")

        openSettings("open-organizations")
        let createButton = app.buttons["create-organization"]
        waitFor(createButton)
        createButton.tap()
        type("Contoso", into: app.textFields["new-organization-name"])
        app.buttons["new-organization-create"].tap()
        waitFor(app.navigationBars["Contoso"], timeout: 30)
        let role = findElement("organization-role")
        waitFor(role)
        XCTAssertTrue(role.label.contains("Admin"), role.label)

        type(partnerEmail, into: app.textFields["organization-invite-email"])
        app.buttons["organization-invite-submit"].tap()
        waitFor(findText(containing: "Invitation sent to \(partnerEmail)"), timeout: 30)
        XCTAssertFalse(findElement("error-message").exists)
        let partnerMember = findElement("organization-member-\(partnerEmail)")
        waitFor(partnerMember, timeout: 120)
        XCTAssertTrue(partnerMember.label.contains("Member"), partnerMember.label)
        XCTAssertTrue(partnerMember.label.contains("Security code"), partnerMember.label)
        XCTAssertTrue(findElement("organization-invitation-\(partnerEmail)").waitForNonExistence(timeout: 30), "the accepted invitation leaves Invited")

        let newTeamButton = app.buttons["new-team"]
        scrollTo(newTeamButton)
        newTeamButton.tap()
        let newTeamAlert = app.alerts["New team"]
        waitFor(newTeamAlert)
        let teamNameField = newTeamAlert.textFields.firstMatch
        teamNameField.tap()
        teamNameField.typeText("Ops")
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
        let teamChoice = app.buttons["Contoso › Ops"]
        waitFor(teamChoice)
        teamChoice.tap()
        waitFor(findText(containing: "Ops's members get this vault. Manage it in Settings → Organizations."))
        saveScreenshot("new-vault-where")
        app.buttons["new-vault-create"].tap()
        waitFor(app.buttons["add-item"], timeout: 60)
        XCTAssertTrue(app.navigationBars["Runbooks"].exists, "the new vault opens")

        // No password: iOS would offer to save it in a sheet that can come up after any wait for it.
        choose("add-login", from: "add-item")
        type("Pager", into: app.textFields["editor-title"])
        type("pager-oncall", into: app.textFields["editor-username"])
        app.buttons["editor-save"].tap()
        waitFor(app.buttons["item-Pager"], timeout: 30)

        app.buttons["share-vault"].tap()
        waitFor(findText(containing: "Shared through teams in Contoso"), timeout: 30)
        let opsRow = findElement("vault-team-Ops")
        waitFor(opsRow)
        XCTAssertTrue(opsRow.label.contains("Owner"), opsRow.label)
        XCTAssertTrue(opsRow.label.contains("iOS Partner"), opsRow.label)
        XCTAssertTrue(app.buttons["give-vault-to-team"].exists, "admins give the vault to other teams")
        XCTAssertFalse(app.textFields["share-email"].exists, "an organization vault has no Invite by email")
        XCTAssertFalse(findText(containing: "Ask an admin").exists)
        saveScreenshot("organization-vault-sharing-admin")
        app.buttons["vault-sharing-done"].tap()

        // The partner reads Pager through the key this iPhone gave it, then writes into the vault.
        waitFor(app.buttons["item-Seen by partner"], timeout: 180)

        goBack()
        let runbooks = app.buttons["vault-Runbooks"]
        waitFor(runbooks)
        XCTAssertTrue(runbooks.label.contains("Organization · Owner"), runbooks.label)
        waitFor(findElement("organization-section-Contoso"))
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
