import XCTest

// Drives the approval sheet through the debug-only preview (PhoneApprovalPreviewView): the Mac side of the flow
// cannot run in the simulator, and Go tests cover it end to end.
final class PhoneApprovalUITests: HisarUITestCase {
    func testApproveSSHRequestWithFaceID() {
        launchPreview("ssh")
        waitFor(app.descendants(matching: .any)["phone-approval"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Terminal wants to push to git.tryhisar.com")
        XCTAssertEqual(app.buttons["phone-approval-approve"].label, "Approve with Face ID")
        let rememberSwitch = app.switches["phone-approval-remember"]
        XCTAssertEqual(rememberSwitch.value as? String, "1", "the switch starts as the Mac's checkbox does")
        saveScreenshot("phone-approval-ssh")
        rememberSwitch.switches.firstMatch.tap()
        app.buttons["phone-approval-approve"].tap()
        checkResult("Approved preview-ssh with Face ID")
    }

    func testRememberedSSHApproval() {
        launchPreview("ssh")
        waitFor(app.switches["phone-approval-remember"])
        app.buttons["phone-approval-approve"].tap()
        checkResult("Approved preview-ssh with Face ID, remembered")
    }

    func testDenyCommandLineRequest() {
        launchPreview("cli")
        waitFor(app.staticTexts["phone-approval-title"])
        saveScreenshot("phone-approval-cli")
        app.buttons["phone-approval-deny"].tap()
        checkResult("Denied preview-cli")
    }

    func testApproveLauncherRequest() {
        launchPreview("launcher")
        waitFor(app.staticTexts["phone-approval-title"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Raycast wants to search Hisar and copy or paste passwords and one-time codes")
        XCTAssertEqual(app.staticTexts["phone-approval-row-App"].label, "App, Raycast")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Lasts"].label, "Lasts, Until Hisar locks on Alper's MacBook Pro")
        XCTAssertFalse(app.staticTexts["phone-approval-row-Program"].exists)
        XCTAssertFalse(app.staticTexts["phone-approval-row-Terminal"].exists)
        XCTAssertFalse(app.switches["phone-approval-remember"].exists, "a launcher's approval always lasts until Hisar locks")
        let note = app.descendants(matching: .any)["phone-approval-note"].label
        XCTAssertEqual(note, "Approving lets this one request continue; it does not unlock Hisar on your Mac.")
        saveScreenshot("phone-approval-launcher")
        app.buttons["phone-approval-approve"].tap()
        checkResult("Approved preview-launcher with Face ID")
    }

    func testApproveLauncherShareRequest() {
        launchPreview("launcher-share")
        waitFor(app.staticTexts["phone-approval-title"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Raycast wants to create a share link for “GitHub”")
        let secrets = app.staticTexts["phone-approval-row-Secrets"].label
        XCTAssertTrue(secrets.contains("username") && secrets.contains("password"), secrets)
        XCTAssertEqual(app.staticTexts["phone-approval-row-Link"].label, "Link, Expires in 7 days · 1 view")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Warning"].label, "Warning, Anyone with the link can read these until it expires or its views run out.")
        XCTAssertFalse(app.staticTexts["phone-approval-row-Lasts"].exists, "a share link is approved once")
        let note = app.descendants(matching: .any)["phone-approval-note"].label
        XCTAssertEqual(note, "Approving lets this one request continue; it does not unlock Hisar on your Mac.")
        saveScreenshot("phone-approval-launcher-share")
        app.buttons["phone-approval-approve"].tap()
        checkResult("Approved preview-launcher-share with Face ID")
    }

    func testCodingAgentRequestShowsProject() {
        launchPreview("agent")
        waitFor(app.staticTexts["phone-approval-title"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "omp in Tern (via hisar) wants to use 2 secrets")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Project"].label, "Project, ~/src/acme-web")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Command"].label, "Command, npm run db:migrate")
        XCTAssertFalse(app.staticTexts["phone-approval-row-Repository"].exists, "the agent runs in its project's root")
        let secrets = app.staticTexts["phone-approval-row-Secrets"].label
        XCTAssertTrue(secrets.contains("Agents › Staging Postgres › connection string") && secrets.contains("Agents › Stripe › secret key"), secrets)
        saveScreenshot("phone-approval-agent")
        app.buttons["phone-approval-deny"].tap()
        checkResult("Denied preview-agent")
    }

    func testForwardedRequestWarns() {
        launchPreview("forwarded")
        let banner = app.descendants(matching: .any)["phone-approval-forwarded"]
        waitFor(banner)
        XCTAssertTrue(banner.label.contains("build.tryhisar.dev"))
        saveScreenshot("phone-approval-forwarded")
        app.buttons["phone-approval-deny"].tap()
        checkResult("Denied preview-forwarded")
    }

    func testApproveWithMasterPasswordWhenFaceIDIsUnavailable() {
        app.launchArguments = ["-HisarUITest", "-HisarApprovalPreview", "signing", "-HisarApprovalPreviewPassword"]
        app.launch()
        waitFor(app.buttons["phone-approval-approve"])
        XCTAssertEqual(app.switches["phone-approval-remember"].value as? String, "0", "commit signing is never remembered by default")
        saveScreenshot("phone-approval-signing")
        app.buttons["phone-approval-approve"].tap()
        let passwordField = app.secureTextFields["phone-approval-password"]
        type("not the password", into: passwordField)
        XCTAssertTrue(app.staticTexts["Face ID is not available, so Hisar checks your master password instead."].exists)
        app.buttons["phone-approval-approve-password"].tap()
        waitFor(app.descendants(matching: .any)["error-message"])
        saveScreenshot("phone-approval-password")
        type("preview master password", into: passwordField)
        app.buttons["phone-approval-approve-password"].tap()
        checkResult("Approved preview-signing with the master password")
    }

    func testWaitingRequestsAreACardOnVaultsAndListedInApprovals() {
        app.launchArguments = ["-HisarUITest", "-HisarApprovalBannerPreview"]
        app.launch()
        let header = app.staticTexts["pending-approval-header"]
        waitFor(header)
        XCTAssertEqual(header.label.uppercased(), "3 REQUESTS WAITING FOR YOUR APPROVAL")
        let sshSummary = app.buttons["pending-approval-preview-ssh"]
        XCTAssertTrue(sshSummary.label.contains("Alper's MacBook Pro · expires in 1:"), sshSummary.label)
        XCTAssertLessThan(sshSummary.frame.minY, app.staticTexts["Personal"].firstMatch.frame.minY, "the card is above the vaults")
        saveScreenshot("phone-approval-card")

        app.buttons["pending-approval-more"].tap()
        let launcherRow = app.buttons["approval-row-preview-launcher"]
        waitFor(launcherRow)
        XCTAssertTrue(launcherRow.label.contains("Until Hisar locks on Alper's MacBook Pro"), launcherRow.label)
        saveScreenshot("phone-approval-list")
        app.buttons["approval-row-preview-cli"].tap()
        app.buttons["phone-approval-deny"].tap()
        let result = app.staticTexts["phone-approval-result"]
        waitFor(result)
        XCTAssertEqual(result.label, "Denied preview-cli")

        app.buttons["pending-approval-approve-preview-ssh"].tap()
        let launcherSummary = app.buttons["pending-approval-preview-launcher"]
        waitFor(launcherSummary)
        XCTAssertEqual(result.label, "Approved preview-ssh with Face ID")
        XCTAssertFalse(app.buttons["pending-approval-more"].exists, "one request left, nothing more to show")
        saveScreenshot("phone-approval-launcher-card")

        app.buttons["pending-approval-approve-preview-launcher"].tap()
        XCTAssertTrue(header.waitForNonExistence(timeout: 10), "no requests, no card")
        XCTAssertEqual(result.label, "Approved preview-launcher with Face ID")
    }

    private func launchPreview(_ kind: String) {
        app.launchArguments = ["-HisarUITest", "-HisarApprovalPreview", kind]
        app.launch()
    }

    private func checkResult(_ expectedMessage: String, file: StaticString = #filePath, line: UInt = #line) {
        let result = app.staticTexts["phone-approval-result"]
        waitFor(result, file: file, line: line)
        XCTAssertEqual(result.label, expectedMessage, file: file, line: line)
    }
}
