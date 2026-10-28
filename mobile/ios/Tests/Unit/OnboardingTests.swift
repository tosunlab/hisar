import XCTest
import UIKit
@testable import Hisar

@MainActor
final class OnboardingTests: XCTestCase {
    private var backend: FakeBackend!
    private var model: AppModel!

    override func setUp() async throws {
        backend = FakeBackend()
        model = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await model.start { self.backend }?.value
    }

    private func createAccount() async {
        let succeeded = await model.signUpWithoutInvite(server: "https://hisar.example.com", email: "ada@example.com", name: "Ada", password: FakeBackend.password)
        XCTAssertTrue(succeeded)
    }

    func testPostponedKitAuthorizedByPasswordUnlock() async throws {
        await createAccount()
        await model.postponeNewSecretKey()
        XCTAssertEqual(model.onboardingStep, .biometric)
        model.dismissNewSecretKey()
        model.lock()
        let restarted = AppModel(credentialIdentityStore: FakeCredentialIdentityStore())
        await restarted.start { self.backend }?.value
        XCTAssertEqual(restarted.screen, .locked)
        XCTAssertFalse(restarted.isPendingRecoveryShown)
        let unlocked = await restarted.unlock(password: FakeBackend.password)
        XCTAssertTrue(unlocked)
        XCTAssertNil(restarted.onboardingStep)
        restarted.openEmergencyKit()
        XCTAssertEqual(restarted.status.emergencyKit?.isAuthorized, true)
        let prepared = await restarted.exportNewAccountEmergencyKit()
        let export = try XCTUnwrap(prepared)
        defer { restarted.removeEmergencyKit(export) }
        let recorded = await restarted.recordEmergencyKitDelivery(exportID: try XCTUnwrap(export.exportId), method: "file", epoch: restarted.emergencyKitDeliveryEpoch)
        XCTAssertTrue(recorded)
        let finished = await restarted.finishNewSecretKey()
        XCTAssertTrue(finished)
        XCTAssertEqual(restarted.screen, .unlocked)
        XCTAssertNil(restarted.status.emergencyKit)
        XCTAssertFalse(backend.allCallName.contains("authorizeNewAccountEmergencyKit"))
        XCTAssertEqual(backend.allCallName.filter { $0 == "unlock" }.count, 1)
    }

    func testBiometricUnlockAsksOnceForKit() async throws {
        await createAccount()
        await model.postponeNewSecretKey()
        let enabled = await model.turnOnBiometricUnlockFromOffer()
        XCTAssertTrue(enabled)
        model.lock()
        let unlocked = await model.unlockWithBiometric()
        XCTAssertTrue(unlocked)
        model.openEmergencyKit()
        XCTAssertEqual(model.status.emergencyKit?.isAuthorized, false)
        let authorized = await model.authorizeEmergencyKit()
        XCTAssertTrue(authorized)
        model.closeEmergencyKit()
        model.openEmergencyKit()
        let prepared = await model.exportNewAccountEmergencyKit()
        let export = try XCTUnwrap(prepared)
        defer { model.removeEmergencyKit(export) }
        XCTAssertEqual(model.status.emergencyKit?.isAuthorized, true)
        XCTAssertEqual(backend.allCallName.filter { $0 == "authorizeNewAccountEmergencyKitWithBiometric" }.count, 1)
        XCTAssertFalse(backend.allCallName.contains("authorizeNewAccountEmergencyKit"))
    }

    func testCopyOrCancelledShareDoesNotSaveKit() async throws {
        await createAccount()
        let prepared = await model.exportNewAccountEmergencyKit()
        let export = try XCTUnwrap(prepared)
        defer { model.removeEmergencyKit(export) }
        let exportID = try XCTUnwrap(export.exportId)
        for (activity, completed) in [(UIActivity.ActivityType.copyToPasteboard, true), (.print, false)] {
            let method = EmergencyKitShareButton.deliveryMethod(activity: activity, completed: completed, error: nil)
            let recorded = await model.recordEmergencyKitDelivery(exportID: exportID, method: method, epoch: model.emergencyKitDeliveryEpoch)
            XCTAssertFalse(recorded)
            XCTAssertEqual(model.status.emergencyKit?.savedBy, "")
        }
        let beforeDelivery = await model.finishNewSecretKey()
        XCTAssertFalse(beforeDelivery)
        for (activity, expected) in [(UIActivity.ActivityType("com.apple.DocumentManagerUICore.SaveToFiles"), "file"), (.print, "print"), (.mail, "share")] {
            let method = EmergencyKitShareButton.deliveryMethod(activity: activity, completed: true, error: nil)
            let recorded = await model.recordEmergencyKitDelivery(exportID: exportID, method: method, epoch: model.emergencyKitDeliveryEpoch)
            XCTAssertTrue(recorded)
            XCTAssertEqual(model.status.emergencyKit?.savedBy, expected)
        }
    }

    func testEmergencyKitLateCompletionAfterUnlockIsIgnored() async throws {
        await createAccount()
        let prepared = await model.exportNewAccountEmergencyKit()
        let export = try XCTUnwrap(prepared)
        defer { model.removeEmergencyKit(export) }
        let epoch = model.emergencyKitDeliveryEpoch
        model.lock()
        let unlocked = await model.unlock(password: FakeBackend.password)
        XCTAssertTrue(unlocked)
        let recorded = await model.recordEmergencyKitDelivery(exportID: try XCTUnwrap(export.exportId), method: "file", epoch: epoch)
        XCTAssertFalse(recorded)
        XCTAssertEqual(model.status.emergencyKit?.savedBy, "")
        XCTAssertEqual(model.onboardingStep, .kit)
        model.closeEmergencyKit()
        model.lock()
        _ = await model.unlock(password: FakeBackend.password)
        XCTAssertNil(model.onboardingStep)
    }

    func testEmergencyKitReminderDistinguishesDeliveredKit() async throws {
        await createAccount()
        for age in ["new", "3-days", "14-days"] {
            backend.status.emergencyKit?.reminder = age
            model.refreshStatus()
            XCTAssertTrue(model.status.emergencyKit?.reminderMessage.contains("only on") == false)
        }
        let prepared = await model.exportNewAccountEmergencyKit()
        let export = try XCTUnwrap(prepared)
        defer { model.removeEmergencyKit(export) }
        await model.recordEmergencyKitDelivery(exportID: try XCTUnwrap(export.exportId), method: "file", epoch: model.emergencyKitDeliveryEpoch)
        XCTAssertEqual(model.status.emergencyKit?.title, "Confirm your Emergency Kit")
        XCTAssertTrue(model.status.emergencyKit?.reminderMessage.hasPrefix("Emergency Kit saved.") == true)
        model.hideEmergencyKitReminder()
        XCTAssertEqual(model.status.emergencyKit?.isReminderHidden, true)
        model.closeEmergencyKit()
        model.lock()
        _ = await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(model.status.emergencyKit?.isReminderHidden, false)
    }

    func testSignUpPolicyHidesInviteFieldOnOpenServer() async {
        let form = SignUpForm()
        backend.signUpPolicyMode = "open"
        await form.checkPolicy(using: model)
        XCTAssertFalse(form.showsInviteField)
        XCTAssertTrue(form.showsAccountFields)
        backend.signUpPolicyMode = "unknown"
        await form.checkPolicy(using: model)
        XCTAssertTrue(form.showsInviteField)
        XCTAssertTrue(form.showsAccountFields)
        backend.signUpPolicyError = BackendError(message: "Network unavailable")
        await form.checkPolicy(using: model)
        XCTAssertFalse(form.canSubmit)
        guard case .failed = form.policy else { return XCTFail("Network errors must offer retry") }
        backend.signUpPolicyError = nil
        await form.checkPolicy(using: model)
        XCTAssertTrue(form.canSubmit)
    }

    func testSignUpPolicyRequiresInviteFirst() async {
        let form = SignUpForm()
        for mode in ["invite", "setup"] {
            backend.signUpPolicyMode = mode
            await form.checkPolicy(using: model)
            XCTAssertTrue(form.showsInviteField)
            XCTAssertFalse(form.showsAccountFields)
            form.inviteText = "not a link or code"
            form.parseInvitation()
            XCTAssertFalse(form.showsAccountFields)
            form.inviteText = "an-invite-code"
            form.parseInvitation()
            XCTAssertTrue(form.showsAccountFields)
            XCTAssertEqual(form.invitation?.server, InviteLink.defaultServer)
            form.removeInvitation()
        }
    }

    func testSignUpPolicyIgnoresStaleAnswer() async {
        let form = SignUpForm()
        let entered = DispatchSemaphore(value: 0)
        let release = DispatchSemaphore(value: 0)
        backend.onGetSignUpPolicy = { server in
            if server.contains("first") {
                entered.signal()
                _ = release.wait(timeout: .now() + 10)
                return SignUpPolicy(server: server, host: "first.example", mode: "open")
            }
            return SignUpPolicy(server: server, host: "second.example/sub", mode: "invite")
        }
        form.changeServer("https://first.example")
        let first = Task { await form.checkPolicy(using: model) }
        while entered.wait(timeout: .now()) == .timedOut { await Task.yield() }
        form.changeServer("https://second.example/sub")
        await form.checkPolicy(using: model)
        release.signal()
        await first.value
        XCTAssertEqual(form.mode, "invite")
        XCTAssertEqual(form.canonicalServer, "https://second.example/sub")
        XCTAssertFalse(form.showsAccountFields)
    }

    func testRegistrationClosedAfterCheckKeepsForm() async {
        let form = SignUpForm()
        backend.signUpPolicyMode = "open"
        await form.checkPolicy(using: model)
        form.name = "Ada"
        form.email = "ada@example.com"
        form.password = FakeBackend.password
        form.repeatedPassword = FakeBackend.password
        backend.signUpWithoutInviteErrorMessage = "Sign-ups are not open on this server. Ask an admin for an invitation."
        backend.signUpPolicyMode = "invite"
        let succeeded = await form.submit(using: model)
        XCTAssertFalse(succeeded)
        XCTAssertTrue(form.showsInviteField)
        XCTAssertTrue(form.showsAccountFields)
        XCTAssertEqual(form.name, "Ada")
        XCTAssertEqual(form.email, "ada@example.com")
        XCTAssertEqual(form.password, FakeBackend.password)
        XCTAssertEqual(form.repeatedPassword, FakeBackend.password)
        XCTAssertNotNil(form.invitationMessage)
        form.changeServer("https://another.example")
        XCTAssertNil(form.invitationMessage)
        XCTAssertEqual(form.password, FakeBackend.password)
    }

    func testInvitationEmailAndServerStayBound() async throws {
        let form = SignUpForm()
        form.email = "typed@example.com"
        form.password = FakeBackend.password
        let link = try XCTUnwrap(InviteLink.parse("hisar://invite/abc?server=https%3A%2F%2Fhisar.example%2Fsub&email=ADA%40example.com"))
        form.applyInvitation(link)
        XCTAssertEqual(form.boundEmail, "ada@example.com")
        XCTAssertEqual(form.server, "https://hisar.example/sub")
        XCTAssertTrue(form.showsAccountFields)
        form.changeServer("https://other.example")
        XCTAssertNil(form.invitation)
        XCTAssertNil(form.boundEmail)
        XCTAssertEqual(form.email, "typed@example.com")
        XCTAssertEqual(form.password, FakeBackend.password)
        form.inviteText = "abc"
        form.parseInvitation()
        XCTAssertEqual(form.invitation?.invite, "https://other.example/invite/abc")
        XCTAssertNil(form.boundEmail)
    }
}
