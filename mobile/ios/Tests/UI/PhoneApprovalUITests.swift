import XCTest

// Drives the approval sheet through the debug-only preview (PhoneApprovalPreviewView): the Mac side of the flow
// cannot run in the simulator, and Go tests cover it end to end.
final class PhoneApprovalUITests: HisarUITestCase {
    func testApproveSSHRequestWithFaceID() {
        launchPreview("ssh")
        let sheet = app.descendants(matching: .any)["phone-approval"]
        waitFor(sheet)
        XCTAssertEqual(app.staticTexts["phone-approval-title"].label, "Approve on Alper's MacBook Pro?")
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Terminal wants to push to git.tryhisar.com")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Command"].label, "Command, git push origin main")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Repository"].label, "Repository, ~/Documents/hisar/core")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Server"].label, "Server, git.tryhisar.com:22")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Key"].label, "Key, hisar gitlab")
        XCTAssertTrue(app.descendants(matching: .any)["phone-approval-encryption-note"].label.contains("End-to-end encrypted"))
        XCTAssertFalse(app.descendants(matching: .any)["phone-approval-forwarded"].exists)
        XCTAssertTrue(app.buttons["phone-approval-approve"].label.contains("Approve with Face ID"))
        saveScreenshot("phone-approval-ssh")
        app.buttons["phone-approval-approve"].tap()
        checkResult("Approved preview-ssh with Face ID")
    }

    func testDenyCommandLineRequest() {
        launchPreview("cli")
        waitFor(app.staticTexts["phone-approval-title"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "hisar in Terminal wants to use Hisar")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Terminal"].label, "Terminal, /dev/ttys003")
        XCTAssertFalse(app.staticTexts["phone-approval-row-Key"].exists)
        saveScreenshot("phone-approval-cli")
        app.buttons["phone-approval-deny"].tap()
        checkResult("Denied preview-cli")
    }

    func testForwardedRequestWarns() {
        launchPreview("forwarded")
        let banner = app.descendants(matching: .any)["phone-approval-forwarded"]
        waitFor(banner)
        XCTAssertTrue(banner.label.contains("build.tryhisar.dev"))
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "build.tryhisar.dev wants to sign in to github.com")
        XCTAssertEqual(app.staticTexts["phone-approval-row-Forwarded from"].label, "Forwarded from, build.tryhisar.dev")
        saveScreenshot("phone-approval-forwarded")
        app.buttons["phone-approval-deny"].tap()
        checkResult("Denied preview-forwarded")
    }

    func testApproveWithMasterPasswordWhenFaceIDIsUnavailable() {
        app.launchArguments = ["-HisarUITest", "-HisarApprovalPreview", "signing", "-HisarApprovalPreviewPassword"]
        app.launch()
        waitFor(app.buttons["phone-approval-approve"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Claude wants to sign a commit")
        saveScreenshot("phone-approval-signing")
        app.buttons["phone-approval-approve"].tap()
        let passwordField = app.secureTextFields["phone-approval-password"]
        waitFor(passwordField)
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
        XCTAssertTrue(sshSummary.exists)
        XCTAssertTrue(sshSummary.label.contains("Alper's MacBook Pro · expires in 1:"), sshSummary.label)
        XCTAssertTrue(sshSummary.label.contains("Terminal wants to push to git.tryhisar.com"), sshSummary.label)
        XCTAssertTrue(sshSummary.label.contains("with “hisar gitlab”"), sshSummary.label)
        XCTAssertFalse(app.buttons["pending-approval-preview-cli"].exists, "the card shows the first request only")
        XCTAssertTrue(app.buttons["pending-approval-deny-preview-ssh"].exists)
        XCTAssertTrue(app.buttons["pending-approval-approve-preview-ssh"].exists)
        XCTAssertLessThan(sshSummary.frame.minY, app.staticTexts["Personal"].firstMatch.frame.minY, "the card is above the vaults")
        saveScreenshot("phone-approval-card")

        app.buttons["pending-approval-more"].tap()
        let commandLineRow = app.buttons["approval-row-preview-cli"]
        waitFor(commandLineRow)
        XCTAssertTrue(app.buttons["approval-row-preview-ssh"].exists)
        XCTAssertTrue(commandLineRow.label.contains("hisar in Terminal wants to use Hisar"), commandLineRow.label)
        saveScreenshot("phone-approval-list")
        commandLineRow.tap()
        waitFor(app.staticTexts["phone-approval-title"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "hisar in Terminal wants to use Hisar")
        app.buttons["phone-approval-deny"].tap()
        waitFor(app.staticTexts["phone-approval-result"])
        XCTAssertEqual(app.staticTexts["phone-approval-result"].label, "Denied preview-cli")
        XCTAssertEqual(app.staticTexts["pending-approval-header"].label.uppercased(), "WAITING FOR YOUR APPROVAL")
        XCTAssertFalse(app.buttons["pending-approval-more"].exists, "one request left, nothing more to show")

        app.buttons["pending-approval-approve-preview-ssh"].tap()
        waitFor(app.staticTexts["phone-approval-result"])
        XCTAssertEqual(app.staticTexts["phone-approval-result"].label, "Approved preview-ssh with Face ID")
        XCTAssertFalse(app.staticTexts["pending-approval-header"].exists, "no requests, no card")
        openTab("Approvals")
        waitFor(app.descendants(matching: .any)["approvals-empty"])
    }

    private func launchPreview(_ kind: String) {
        app.launchArguments = ["-HisarUITest", "-HisarApprovalPreview", kind]
        app.launch()
    }

    private func checkResult(_ expectedMessage: String, file: StaticString = #filePath, line: UInt = #line) {
        let result = app.staticTexts["phone-approval-result"]
        waitFor(result, file: file, line: line)
        XCTAssertEqual(result.label, expectedMessage, file: file, line: line)
        XCTAssertFalse(app.descendants(matching: .any)["phone-approval"].exists, "the sheet closed", file: file, line: line)
    }
}
