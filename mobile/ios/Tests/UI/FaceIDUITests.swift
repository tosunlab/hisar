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
        app.buttons["biometric-offer-turn-on"].tap()
        allowFaceIDIfAsked()
        sleep(1)
        matchFace()
        waitFor(app.buttons["vault-Personal"], timeout: 20)

        openSettings("open-security")
        let toggle = app.switches["biometric-toggle"]
        let enabledPredicate = NSPredicate(format: "value == '1'")
        expectation(for: enabledPredicate, evaluatedWith: toggle)
        waitForExpectations(timeout: 10)
        let knob = toggle.switches.firstMatch
        (knob.exists ? knob : toggle).tap()
        expectation(for: NSPredicate(format: "value == '0'"), evaluatedWith: toggle)
        waitForExpectations(timeout: 10)
        sleep(1)
        (knob.exists ? knob : toggle).tap()
        let passwordField = app.alerts.secureTextFields.firstMatch
        waitFor(passwordField)
        passwordField.typeText(Self.masterPassword)
        app.alerts.buttons["Turn on"].tap()
        allowFaceIDIfAsked()
        sleep(1)
        matchFace()
        expectation(for: enabledPredicate, evaluatedWith: toggle)
        waitForExpectations(timeout: 20)
        sleep(2)
        saveScreenshot("settings-face-id")
        app.buttons["settings-lock-now"].tap()
        saveScreenshot("lock-face-id")
        sleep(1)
        matchFace()
        waitFor(app.buttons["vault-Personal"], timeout: 15)
        XCTAssertFalse(springboard.otherElements["Face ID"].exists, "one Face ID prompt unlocks: the keychain reuses the evaluated context")
    }
}
