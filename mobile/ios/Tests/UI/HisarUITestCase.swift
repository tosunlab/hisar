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

    // Waits until the element takes a tap: a row behind a closing sheet, or a button of a dialog still animating in, exists
    // before it is hittable. iOS's Save Password sheet can come up seconds after a save, later than dismissSavePasswordPrompt
    // looks for it, and cover the app; it is dismissed here too.
    func waitForHittable(_ element: XCUIElement, timeout: TimeInterval = 30, file: StaticString = #filePath, line: UInt = #line) {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if element.exists && element.isHittable {
                return
            }
            if let notNow = savePasswordNotNowButtons.first(where: { $0.exists && $0.isHittable }) {
                notNow.tap()
            }
            _ = element.waitForExistence(timeout: 0.5)
        }
        XCTFail("\(element) did not become tappable", file: file, line: line)
    }

    private var savePasswordNotNowButtons: [XCUIElement] {
        [app.buttons["Not Now"], XCUIApplication(bundleIdentifier: "com.apple.springboard").buttons["Not Now"]]
    }

    // iOS may show its Save Password sheet; on slow machines a tap during its animation is ignored, so tap until it is gone.
    func dismissSavePasswordPrompt(file: StaticString = #filePath, line: UInt = #line) {
        let allCandidate = savePasswordNotNowButtons
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

    func goBack() {
        let backButton = app.navigationBars.buttons["BackButton"].exists ? app.navigationBars.buttons["BackButton"] : app.navigationBars.buttons.element(boundBy: 0)
        backButton.tap()
    }

    func openTab(_ name: String) {
        let tabButton = app.tabBars.buttons[name].exists ? app.tabBars.buttons[name] : app.buttons[name].firstMatch
        tabButton.tap()
    }

    func openSettings(_ identifier: String) {
        openTab("Settings")
        app.buttons[identifier].tap()
    }

    // A confirmation dialog is a sheet or, on iOS 26, a popover next to its button.
    func confirm(_ label: String, excludingIdentifier: String = "", file: StaticString = #filePath, line: UInt = #line) {
        let predicate = NSPredicate(format: "label == %@ AND identifier != %@", label, excludingIdentifier)
        let button = app.sheets.buttons[label].exists ? app.sheets.buttons[label] : app.buttons.matching(predicate).firstMatch
        waitFor(button, file: file, line: line)
        button.tap()
    }

    // On a slow machine a tap on an item of a Menu still animating in is ignored and the menu stays open, where the swipe
    // `type` makes to find the editor presses whichever item it crosses. So the item is tapped until the menu is gone.
    func choose(_ item: String, from menu: String, file: StaticString = #filePath, line: UInt = #line) {
        let menuButton = app.buttons[menu]
        waitFor(menuButton, file: file, line: line)
        menuButton.tap()
        let itemButton = app.buttons[item]
        waitFor(itemButton, file: file, line: line)
        for _ in 0..<5 {
            itemButton.tap()
            if itemButton.waitForNonExistence(timeout: 5) {
                return
            }
        }
        XCTFail("the \(menu) menu stayed open after choosing \(item)", file: file, line: line)
    }

    // A SwiftUI Form creates only the rows on screen, so a row further down does not exist until the form scrolls to it. The
    // drag stays in the top part of the form: lower down, the keyboard of the field being typed in covers it, and a swipe
    // there does not scroll the form in a sheet.
    func scrollTo(_ element: XCUIElement, file: StaticString = #filePath, line: UInt = #line) {
        let list = app.collectionViews.firstMatch
        for _ in 0..<8 {
            if element.waitForExistence(timeout: 1) && element.isHittable {
                return
            }
            guard list.exists else { break }
            let start = list.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            start.press(forDuration: 0.05, thenDragTo: list.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.15)))
        }
        waitFor(element, file: file, line: line)
    }

    func type(_ text: String, into element: XCUIElement) {
        if !element.waitForExistence(timeout: 5) || !element.isHittable {
            scrollTo(element)
        }
        element.tap()
        element.typeText(text)
    }

    // With isBiometricOfferExpected, it stops at the Face ID offer after the Secret Key; otherwise it declines any offer.
    @discardableResult
    func signUp(inviteLink: String, email: String, name: String, isBiometricOfferExpected: Bool = false) -> String {
        waitFor(app.segmentedControls["sign-in-mode"])
        saveScreenshot("sign-in")
        app.segmentedControls["sign-in-mode"].buttons["Create account"].tap()
        type(inviteLink, into: app.textFields["sign-up-invite-link"])
        type(email, into: app.textFields["sign-up-email"])
        type(name, into: app.textFields["sign-up-name"])
        type(Self.masterPassword, into: app.secureTextFields["sign-up-password"])
        type(Self.masterPassword, into: app.secureTextFields["sign-up-repeated-password"])
        // The note is the footer below the submit button, at the end of the form.
        let createAccountNote = app.staticTexts["create-account-note"]
        scrollTo(createAccountNote)
        saveScreenshot("create-account")
        scrollTo(app.buttons["sign-in-submit"])
        app.buttons["sign-in-submit"].tap()
        let secretKeyText = app.staticTexts["new-secret-key"]
        waitFor(secretKeyText, timeout: 60)
        let secretKey = secretKeyText.label
        saveScreenshot("new-secret-key")
        app.buttons["new-secret-key-done"].tap()
        let notNowButton = app.buttons["biometric-offer-not-now"]
        if isBiometricOfferExpected {
            waitFor(notNowButton, timeout: 10)
            return secretKey
        }
        if notNowButton.waitForExistence(timeout: 2) {
            notNowButton.tap()
        }
        waitFor(app.buttons["vault-Personal"])
        return secretKey
    }

    func addLogin(title: String, username: String, password: String, website: String, oneTimePassword: String = "") {
        app.buttons["vault-Personal"].tap()
        choose("add-login", from: "add-item")
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
