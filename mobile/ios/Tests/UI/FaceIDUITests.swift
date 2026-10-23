import Darwin
import XCTest

// The simulator's biometric daemon listens to these notifications, the same ones its Features > Face ID menu posts.
final class FaceIDUITests: HisarUITestCase {
    private let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")

    private typealias NotifyPost = @convention(c) (UnsafePointer<CChar>) -> UInt32
    private typealias NotifyRegisterCheck = @convention(c) (UnsafePointer<CChar>, UnsafeMutablePointer<Int32>) -> UInt32
    private typealias NotifySetState = @convention(c) (Int32, UInt64) -> UInt32

    // notify.h is not exposed to Swift on iOS, so the libSystem functions are looked up at run time.
    private func loadFunction<Function>(_ name: String, as type: Function.Type) -> Function {
        let symbol = dlsym(UnsafeMutableRawPointer(bitPattern: -2), name)
        return unsafeBitCast(symbol, to: type)
    }

    private func postNotification(_ name: String) {
        _ = loadFunction("notify_post", as: NotifyPost.self)(name)
    }

    private func setFaceIDEnrolled(_ isEnrolled: Bool) {
        var token: Int32 = 0
        _ = loadFunction("notify_register_check", as: NotifyRegisterCheck.self)("com.apple.BiometricKit.enrollmentChanged", &token)
        _ = loadFunction("notify_set_state", as: NotifySetState.self)(token, isEnrolled ? 1 : 0)
        postNotification("com.apple.BiometricKit.enrollmentChanged")
    }

    private func matchFace() {
        postNotification("com.apple.BiometricKit_Sim.pearl.match")
    }

    // Face ID ignores a match posted before it asks, and its prompt is not an element the test can wait for, so the match
    // repeats until the app goes on. A step whose control stays disabled while Face ID runs passes its control: enabled
    // again, without the step done and with an error shown, it means Face ID ended in a failure; the app asks no more, so
    // the test stops there with the app's message. An error banner alone proves nothing: a background refresh can show one
    // while Face ID still runs, but then the control is still disabled.
    private func matchFace(until isDone: () -> Bool, failingWhenEnabled control: XCUIElement? = nil, file: StaticString = #filePath, line: UInt = #line) {
        let errorMessage = app.descendants(matching: .any)["error-message"].firstMatch
        let deadline = Date().addingTimeInterval(30)
        repeat {
            matchFace()
            let retry = Date().addingTimeInterval(2)
            while Date() < retry {
                if isDone() {
                    return
                }
                // Snapshots, because the offer can close between two queries, and a query of a gone element fails the test.
                if let control, let controlState = try? control.snapshot(), controlState.isEnabled, let error = try? errorMessage.snapshot() {
                    XCTFail("Face ID failed and the app shows \"\(error.label)\"", file: file, line: line)
                    return
                }
                Thread.sleep(forTimeInterval: 0.2)
            }
        } while Date() < deadline
        XCTFail("Face ID did not let the app go on", file: file, line: line)
    }

    private func allowFaceIDIfAsked() {
        let allowButton = springboard.alerts.buttons["Allow"].exists ? springboard.alerts.buttons["Allow"] : app.alerts.buttons["Allow"]
        if allowButton.waitForExistence(timeout: 3) {
            allowButton.tap()
        }
        let okButton = springboard.alerts.buttons["OK"]
        if okButton.waitForExistence(timeout: 1) {
            okButton.tap()
        }
    }

    override func tearDown() {
        setFaceIDEnrolled(false)
        super.tearDown()
    }

    func testFaceIDUnlockAfterTurningItOn() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_BIOMETRIC_INVITE_LINK")
        setFaceIDEnrolled(true)
        app.launch()
        signUp(inviteLink: inviteLink, email: "ios-face-id@example.com", name: "iOS Face ID", isBiometricOfferExpected: true)

        saveScreenshot("face-id-offer")
        let turnOnButton = app.buttons["biometric-offer-turn-on"]
        turnOnButton.tap()
        allowFaceIDIfAsked()
        // The vaults are already under the offer, so the offer closing is what shows that Face ID is on.
        matchFace(until: { !turnOnButton.exists }, failingWhenEnabled: turnOnButton)
        waitForHittable(app.buttons["vault-Personal"], timeout: 20)

        openSettings("open-security")
        let toggle = app.switches["biometric-toggle"]
        let enabledPredicate = NSPredicate(format: "value == '1'")
        expectation(for: enabledPredicate, evaluatedWith: toggle)
        waitForExpectations(timeout: 10)
        let knob = toggle.switches.firstMatch
        (knob.exists ? knob : toggle).tap()
        // The switch shows off at once; it stays disabled until the app has turned Face ID off.
        expectation(for: NSPredicate(format: "value == '0' AND enabled == true"), evaluatedWith: toggle)
        waitForExpectations(timeout: 10)
        (knob.exists ? knob : toggle).tap()
        let passwordField = app.alerts.secureTextFields.firstMatch
        waitFor(passwordField)
        typeText(Self.masterPassword, into: passwordField)
        app.alerts.buttons["Turn on"].tap()
        allowFaceIDIfAsked()
        matchFace(until: { toggle.value as? String == "1" }, failingWhenEnabled: toggle)
        expectation(for: enabledPredicate, evaluatedWith: toggle)
        waitForExpectations(timeout: 20)
        sleep(2)
        saveScreenshot("settings-face-id")
        app.buttons["settings-lock-now"].tap()
        saveScreenshot("lock-face-id")
        matchFace(until: { app.buttons["vault-Personal"].exists })
        waitFor(app.buttons["vault-Personal"], timeout: 15)
        XCTAssertFalse(springboard.otherElements["Face ID"].exists, "one Face ID prompt unlocks: the keychain reuses the evaluated context")
    }
}
