import XCTest

// Runs against a real hisar-server; scripts/test-ios-ui.sh starts one and passes the invite links.
class HisarUITestCase: XCTestCase {
    struct MissingEnvironmentError: Error {
        var name: String
    }

    static let masterPassword = "ui test master password"
    var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-HisarResetState", "-HisarUITest"]
    }

    func readEnvironment(_ name: String) throws -> String {
        guard let value = ProcessInfo.processInfo.environment[name], !value.isEmpty else {
            XCTFail("\(name) is not set; run scripts/test-ios-ui.sh, which starts a server and passes it as TEST_RUNNER_\(name)")
            throw MissingEnvironmentError(name: name)
        }
        return value
    }

    func saveScreenshot(_ name: String) {
        let screenshot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        if let directory = ProcessInfo.processInfo.environment["HISAR_UI_TEST_SCREENSHOT_DIRECTORY"], !directory.isEmpty {
            let url = URL(fileURLWithPath: directory).appendingPathComponent("ios-\(name).png")
            try? screenshot.pngRepresentation.write(to: url)
        }
    }

    func waitFor(_ element: XCUIElement, timeout: TimeInterval = 30, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(element.waitForExistence(timeout: timeout), "\(element) did not appear", file: file, line: line)
    }

    // iOS may show its Save Password sheet; on slow machines a tap during its animation is ignored, so tap until it is gone.
    func dismissSavePasswordPrompt(file: StaticString = #filePath, line: UInt = #line) {
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let allCandidate = [app.buttons["Not Now"], springboard.buttons["Not Now"]]
        guard allCandidate.contains(where: { $0.waitForExistence(timeout: 2) }) else {
            return
        }
        let deadline = Date().addingTimeInterval(20)
        while Date() < deadline {
            guard let candidate = allCandidate.first(where: { $0.exists }) else {
                return
            }
            if candidate.isHittable {
                candidate.tap()
            }
            _ = candidate.waitForNonExistence(timeout: 2)
        }
        XCTFail("the Save Password sheet did not go away", file: file, line: line)
    }

    func type(_ text: String, into element: XCUIElement) {
        waitFor(element)
        element.tap()
        element.typeText(text)
    }

    func signUp(inviteLink: String, email: String, name: String) -> String {
        waitFor(app.segmentedControls["sign-in-mode"])
        saveScreenshot("sign-in")
        app.segmentedControls["sign-in-mode"].buttons["Create account"].tap()
        type(inviteLink, into: app.textFields["sign-up-invite-link"])
        type(email, into: app.textFields["sign-up-email"])
        type(name, into: app.textFields["sign-up-name"])
        type(Self.masterPassword, into: app.secureTextFields["sign-up-password"])
        type(Self.masterPassword, into: app.secureTextFields["sign-up-repeated-password"])
        saveScreenshot("create-account")
        app.buttons["sign-in-submit"].tap()
        let secretKeyText = app.staticTexts["new-secret-key"]
        waitFor(secretKeyText, timeout: 60)
        let secretKey = secretKeyText.label
        XCTAssertTrue(secretKey.hasPrefix("H1-"), secretKey)
        saveScreenshot("new-secret-key")
        app.buttons["new-secret-key-done"].tap()
        waitFor(app.buttons["vault-Personal"])
        return secretKey
    }

    func addLogin(title: String, username: String, password: String, website: String, oneTimePassword: String = "") {
        app.buttons["vault-Personal"].tap()
        waitFor(app.buttons["add-item"])
        app.buttons["add-item"].tap()
        type(title, into: app.textFields["editor-title"])
        type(username, into: app.textFields["editor-username"])
        type(password, into: app.secureTextFields["editor-password"])
        type(website, into: app.textFields["editor-website-0"])
        if !oneTimePassword.isEmpty {
            type(oneTimePassword, into: app.textFields["editor-one-time-password"])
        }
        saveScreenshot("add-item")
        app.buttons["editor-save"].tap()
        waitFor(app.buttons["item-\(title)"])
    }
}
