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

    // Screenshots are taken only for HISAR_UI_TEST_SCREENSHOT_DIRECTORY: a test run records the screen anyway, and on a busy
    // simulator a screenshot can time out and fail the test.
    func saveScreenshot(_ name: String) {
        guard let directory = ProcessInfo.processInfo.environment["HISAR_UI_TEST_SCREENSHOT_DIRECTORY"], !directory.isEmpty else {
            return
        }
        let screenshot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        try? screenshot.pngRepresentation.write(to: URL(fileURLWithPath: directory).appendingPathComponent("ios-\(name).png"))
    }

    func waitFor(_ element: XCUIElement, timeout: TimeInterval = 30, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(element.waitForExistence(timeout: timeout), "\(element) did not appear", file: file, line: line)
    }

    // Waits until the element takes a tap: a row behind a closing sheet, or a button of a dialog still animating in, exists
    // before it is hittable. iOS's Save Password sheet can come up seconds after signing in or changing the master password,
    // later than dismissSavePasswordPrompt looks for it, and cover the app; it is dismissed here too.
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

    // A tap while a late Save Password sheet covers the app is lost, so the button is tappable first.
    func goBack(file: StaticString = #filePath, line: UInt = #line) {
        let backButton = app.navigationBars.buttons["BackButton"].exists ? app.navigationBars.buttons["BackButton"] : app.navigationBars.buttons.element(boundBy: 0)
        waitForHittable(backButton, file: file, line: line)
        backButton.tap()
    }

    enum SwipeDirection {
        case left, right
    }

    // Reveals a list row's swipe actions. swipeLeft() and swipeRight() are a flick whose moves can reach a loaded simulator
    // too late, and the row then takes it for a tap (job 120598941516 opened "AWS Console" instead of its actions). This drag
    // moves at a set speed and ends a third of the row away: far enough to open the actions, short of a full swipe, which
    // would run the first action at once.
    func revealSwipeActions(of row: XCUIElement, _ direction: SwipeDirection, file: StaticString = #filePath, line: UInt = #line) {
        waitForHittable(row, file: file, line: line)
        let (start, end): (CGFloat, CGFloat) = direction == .right ? (0.2, 0.55) : (0.8, 0.45)
        row.coordinate(withNormalizedOffset: CGVector(dx: start, dy: 0.5)).press(
            forDuration: 0.05, thenDragTo: row.coordinate(withNormalizedOffset: CGVector(dx: end, dy: 0.5)),
            withVelocity: .slow, thenHoldForDuration: 0.1)
    }

    func openTab(_ name: String, file: StaticString = #filePath, line: UInt = #line) {
        let tabButton = app.tabBars.buttons[name].exists ? app.tabBars.buttons[name] : app.buttons[name].firstMatch
        waitForHittable(tabButton, file: file, line: line)
        tabButton.tap()
    }

    func openSettings(_ identifier: String, file: StaticString = #filePath, line: UInt = #line) {
        openTab("Settings", file: file, line: line)
        let row = app.buttons[identifier]
        waitForHittable(row, file: file, line: line)
        row.tap()
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
    // `submenu` is the nested menu the item is in, such as the + menu's More….
    func choose(_ item: String, from menu: String, under submenu: String? = nil, file: StaticString = #filePath, line: UInt = #line) {
        let menuButton = app.buttons[menu]
        waitForHittable(menuButton, file: file, line: line)
        menuButton.tap()
        if let submenu {
            let submenuButton = app.buttons[submenu]
            waitForHittable(submenuButton, file: file, line: line)
            submenuButton.tap()
        }
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

    // On a slow simulator the keyboard focus comes a moment after the tap, and typing before it fails, so it waits for the focus
    // and taps again if it does not come. Only a field the form has not created yet is dragged to: tap() scrolls one that exists
    // just into view, while a drag can carry it under the navigation bar, where it still counts as hittable and taps land on
    // the bar.
    func type(_ text: String, into element: XCUIElement, normalizedBy normalize: ((String) -> String)? = nil, file: StaticString = #filePath, line: UInt = #line) {
        if focus(element, file: file, line: line) {
            typeText(text, into: element, normalizedBy: normalize, file: file, line: line)
        }
    }

    // A field whose placeholder is the very text to type, as the account deletion's email confirmation, cannot show that the
    // text is in, but the control the text unlocks can: typing waits until `control` turns enabled, and it must not be before.
    func type(_ text: String, into element: XCUIElement, untilEnabled control: XCUIElement, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertFalse(control.isEnabled, "\(control) is enabled before \"\(text)\" is typed", file: file, line: line)
        guard focus(element, file: file, line: line) else { return }
        element.typeText(text)
        let isEnabled = XCTNSPredicateExpectation(predicate: NSPredicate(format: "isEnabled == true"), object: control)
        XCTAssertEqual(XCTWaiter().wait(for: [isEnabled], timeout: 30), .completed, "\(control) did not turn enabled after typing \"\(text)\"", file: file, line: line)
    }

    private func focus(_ element: XCUIElement, file: StaticString, line: UInt) -> Bool {
        if !element.waitForExistence(timeout: 5) {
            scrollTo(element)
        }
        let hasFocus = NSPredicate(format: "hasKeyboardFocus == true")
        for _ in 0..<3 {
            element.tap()
            if hasFocus.evaluate(with: element)
                || XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: hasFocus, object: element)], timeout: 5) == .completed {
                return true
            }
        }
        XCTFail("\(element) did not take the keyboard focus", file: file, line: line)
        return false
    }

    // The simulator's keyboard can insert typed characters long after typeText returns: it holds them until its suggestions
    // come back, 19 seconds later in job 120358912010, and a tap meanwhile acts on part of the text. So typing waits until the
    // field shows exactly its end state (TypedText): `showing` when the typing also deletes, and both sides through the
    // field's own formatter when it has one (CardInput).
    func typeText(_ text: String, into element: XCUIElement, showing: String? = nil, normalizedBy normalize: ((String) -> String)? = nil, file: StaticString = #filePath, line: UInt = #line) {
        let typedText: TypedText
        do {
            typedText = try TypedText(typed: text, showing: showing, before: element.value as? String, placeholder: element.placeholderValue, isSecure: element.elementType == .secureTextField, normalize: normalize)
        } catch TypedText.UnshownError.secureFormattedNotEmpty {
            XCTFail("\(element) is secure and not empty, so what it holds cannot be formatted with \"\(text)\"; clear it first or pass `showing`", file: file, line: line)
            return
        } catch {
            XCTFail("\(element) would show its placeholder \"\(element.placeholderValue ?? "")\" whether or not \"\(text)\" is in; type something else", file: file, line: line)
            return
        }
        element.typeText(text)
        let isShown = waitUntil(timeout: 30) { typedText.isShown(byValue: element.value as? String, placeholder: element.placeholderValue) }
        XCTAssertTrue(isShown, "\(element) shows \"\(element.value as? String ?? "")\", not \"\(typedText.expected)\"", file: file, line: line)
    }

    // Waits until isTrue holds, checking it about once a second. An XCTNSPredicateExpectation with an element as its object
    // captures the app's whole accessibility hierarchy on the main thread whenever a check is false, to describe the element.
    // In job 120468910610 the typing wait's first check came before the keys landed, and that capture took 36.94 s, so the
    // 30 s wait ran out without checking again although the field had shown its typed text for 34 s. Here the predicate reads
    // the element itself, and XCTest has no element to describe.
    private func waitUntil(timeout: TimeInterval, _ isTrue: @escaping () -> Bool) -> Bool {
        let expectation = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in isTrue() }, object: nil)
        return XCTWaiter().wait(for: [expectation], timeout: timeout) == .completed
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
        let secretKey = finishPendingRecovery(email: email, password: Self.masterPassword)
        let notNowButton = app.buttons["biometric-offer-not-now"]
        if isBiometricOfferExpected {
            waitFor(notNowButton, timeout: 10)
            return secretKey
        }
        if notNowButton.waitForExistence(timeout: 2) {
            notNowButton.tap()
        }
        // The vaults exist under the Secret Key screen and the Face ID offer; they take a tap only once both have closed.
        waitForHittable(app.buttons["vault-Personal"])
        return secretKey
    }

    @discardableResult
    func finishPendingRecovery(email: String, password: String, restartAfterPreparingKit: Bool = false) -> String {
        let account = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'pending-recovery-' AND label CONTAINS %@ AND NOT label CONTAINS 'manual-unlock'", email)).firstMatch
        waitFor(account, timeout: 60)
        account.tap()
        let phase = app.staticTexts["pending-recovery-phase"]
        let isPrecommit = phase.label == "awaiting-kit"
        let passwordField = app.secureTextFields["pending-recovery-password"]
        type(password, into: passwordField)
        scrollTo(app.buttons["pending-recovery-export"])
        app.buttons["pending-recovery-export"].tap()
        let key = app.staticTexts["pending-recovery-secret-key"]
        waitFor(key, timeout: 60)
        let secretKey = key.label
        saveScreenshot("pending-recovery-kit")
        if restartAfterPreparingKit {
            app.launchArguments.removeAll { $0 == "-HisarResetState" }
            app.terminate()
            app.launch()
            waitFor(account, timeout: 60)
            account.tap()
        }
        type(password, into: passwordField)
        scrollTo(app.buttons["pending-recovery-acknowledge"])
        app.buttons["pending-recovery-acknowledge"].tap()
        if isPrecommit {
            let prepared = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label == 'prepared'"), object: phase)
            wait(for: [prepared], timeout: 60)
            type(secretKey, into: app.secureTextFields["pending-recovery-input-secret-key"])
            type(password, into: passwordField)
            scrollTo(app.buttons["pending-recovery-resume"])
            app.buttons["pending-recovery-resume"].tap()
            let committed = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label == 'delivery-pending'"), object: phase)
            wait(for: [committed], timeout: 60)
            type(password, into: passwordField)
            scrollTo(app.buttons["pending-recovery-acknowledge"])
            app.buttons["pending-recovery-acknowledge"].tap()
        }
        // Acknowledging runs while the recovery screen covers the app; the screen closes once the record is gone, or shows
        // manual-unlock. Until then the screen beneath is there but covered (job 120454298607 tapped its unlock field).
        let finished = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == NO OR label == 'manual-unlock'"), object: phase)
        wait(for: [finished], timeout: 60)
        if phase.exists {
            scrollTo(app.buttons["pending-recovery-later"])
            app.buttons["pending-recovery-later"].tap()
        }
        return secretKey
    }

    func addLogin(title: String, username: String, password: String, website: String, oneTimePassword: String = "") {
        waitForHittable(app.buttons["vault-Personal"])
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
