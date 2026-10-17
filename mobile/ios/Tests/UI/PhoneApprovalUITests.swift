import XCTest

// Drives the approval sheet through the debug-only preview (PhoneApprovalPreviewView): the Mac side of the flow
// cannot run in the simulator, and Go tests cover it end to end.
final class PhoneApprovalUITests: HisarUITestCase {
    func testApproveSSHRequestWithFaceID() {
        launchPreview("ssh")
        waitFor(app.descendants(matching: .any)["phone-approval"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Terminal wants to push to git.tryhisar.com")
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
        XCTAssertEqual(header.label.uppercased(), "2 REQUESTS WAITING FOR YOUR APPROVAL")
        let sshSummary = app.buttons["pending-approval-preview-ssh"]
        XCTAssertTrue(sshSummary.label.contains("Alper's MacBook Pro · expires in 1:"), sshSummary.label)
        XCTAssertLessThan(sshSummary.frame.minY, app.staticTexts["Personal"].firstMatch.frame.minY, "the card is above the vaults")
        saveScreenshot("phone-approval-card")

        app.buttons["pending-approval-more"].tap()
        saveScreenshot("phone-approval-list")
        app.buttons["approval-row-preview-cli"].tap()
        app.buttons["phone-approval-deny"].tap()
        let result = app.staticTexts["phone-approval-result"]
        waitFor(result)
        XCTAssertEqual(result.label, "Denied preview-cli")
        XCTAssertFalse(app.buttons["pending-approval-more"].exists, "one request left, nothing more to show")

        app.buttons["pending-approval-approve-preview-ssh"].tap()
        XCTAssertTrue(header.waitForNonExistence(timeout: 10), "no requests, no card")
        XCTAssertEqual(result.label, "Approved preview-ssh with Face ID")
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
