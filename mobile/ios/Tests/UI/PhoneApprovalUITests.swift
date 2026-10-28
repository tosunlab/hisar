import XCTest

// Drives the approval sheet through the debug-only preview (PhoneApprovalPreviewView): the Mac side of the flow
// cannot run in the simulator, and Go tests cover it end to end.
final class PhoneApprovalUITests: HisarUITestCase {
    func testApproveSSHRequestWithFaceID() {
        launchPreview("ssh")
        waitFor(app.descendants(matching: .any)["phone-approval"])
        XCTAssertEqual(app.staticTexts["phone-approval-summary"].label, "Terminal wants to push to git.tryhisar.com")
        XCTAssertEqual(app.buttons["phone-approval-approve"].label, "Approve with Face ID")
        XCTAssertTrue(app.descendants(matching: .any)["phone-approval-biometry-faceid"].exists)
        let rememberSwitch = app.switches["phone-approval-remember"]
        XCTAssertEqual(rememberSwitch.value as? String, "1", "the switch starts as the Mac's checkbox does")
        saveScreenshot("phone-approval-ssh-faceid")
        rememberSwitch.switches.firstMatch.tap()
        app.buttons["phone-approval-approve"].tap()
        checkResult("Approved preview-ssh with Face ID")
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
