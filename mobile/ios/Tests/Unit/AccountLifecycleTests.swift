import XCTest
@testable import Hisar

@MainActor
final class AccountLifecycleTests: XCTestCase {
    private var backend: FakeBackend!
    private var identityStore: FakeCredentialIdentityStore!

    override func setUp() async throws {
        backend = FakeBackend()
        identityStore = FakeCredentialIdentityStore()
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: identityStore)
        await model.start { self.backend }?.value
        return model
    }

    /// Waits for work the model starts in a Task, such as clearing AutoFill: a fixed sleep is too short on a busy CI runner.
    private func waitUntil(_ condition: () -> Bool, _ message: String) async {
        let deadline = Date().addingTimeInterval(5)
        while !condition() && Date() < deadline {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition(), message)
    }

    private let goodFeedback = MasterPasswordFeedback(level: "good", label: "Good", bits: 60, message: "A few random words you can remember make a strong master password.")

    func testNewPasswordRules() {
        XCTAssertNil(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "", repeatedPassword: "", feedback: goodFeedback))
        XCTAssertEqual(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "short", repeatedPassword: "", feedback: goodFeedback), "Use at least 10 characters.")
        XCTAssertEqual(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "old password", repeatedPassword: "", feedback: goodFeedback), "The new master password is the same as the current one.")
        XCTAssertEqual(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "a new master password", repeatedPassword: "a new master", feedback: goodFeedback), "The passwords do not match.")
        XCTAssertFalse(AccountLifecycleLogic.canChangePassword(currentPassword: "", newPassword: "a new master password", repeatedPassword: "a new master password", feedback: goodFeedback))
        XCTAssertFalse(AccountLifecycleLogic.canChangePassword(currentPassword: "old password", newPassword: "a new master password", repeatedPassword: "", feedback: goodFeedback))
        XCTAssertTrue(AccountLifecycleLogic.canChangePassword(currentPassword: "old password", newPassword: "a new master password", repeatedPassword: "a new master password", feedback: goodFeedback))
    }

    func testWeakMasterPasswordIsRefused() async {
        let model = await createUnlockedModel()
        let weakFeedback = await model.checkMasterPassword("password1234")
        XCTAssertEqual(weakFeedback.strength, PasswordStrength(level: "weak", label: "Weak", bits: 8))
        XCTAssertEqual(
            AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "password1234", repeatedPassword: "password1234", feedback: weakFeedback),
            "This password is too easy to guess. Add a few more words or characters."
        )
        XCTAssertFalse(AccountLifecycleLogic.canChangePassword(currentPassword: "old password", newPassword: "password1234", repeatedPassword: "password1234", feedback: weakFeedback))
        XCTAssertFalse(AccountLifecycleLogic.isMasterPasswordAccepted("password1234", feedback: weakFeedback))

        let allowedWeakFeedback = MasterPasswordFeedback(
            level: "weak", label: "Weak", bits: 33, message: "You can use it, but another word or a few more characters make it much harder to guess."
        )
        XCTAssertTrue(AccountLifecycleLogic.isMasterPasswordAccepted("orange cactus harbor", feedback: allowedWeakFeedback))
        XCTAssertEqual(AccountLifecycleLogic.describeMasterPasswordHint(feedback: allowedWeakFeedback), allowedWeakFeedback.message)
        XCTAssertNil(AccountLifecycleLogic.describeMasterPasswordHint(feedback: goodFeedback))
        XCTAssertFalse(AccountLifecycleLogic.isMasterPasswordAccepted("orange cactus harbor", feedback: nil), "refused until core has rated it")
    }

    func testUnknownSignedOutDeviceCount() {
        XCTAssertEqual(AccountLifecycleLogic.describeSignedOutDevices(-1), "Any other devices were signed out; their count could not be recovered.")
        XCTAssertEqual(AccountLifecycleLogic.describeSignedOutDevices(0), "No other devices were signed in.")
    }

    func testEmailConfirmationIgnoresCaseAndSpaces() {
        XCTAssertTrue(AccountLifecycleLogic.isEmailMatching(typedEmail: " Alice@Example.com ", accountEmail: "alice@example.com"))
        XCTAssertFalse(AccountLifecycleLogic.isEmailMatching(typedEmail: "bob@example.com", accountEmail: "alice@example.com"))
        XCTAssertFalse(AccountLifecycleLogic.isEmailMatching(typedEmail: "", accountEmail: ""))
    }

    func testDeletionConsequences() {
        let preview = AccountDeletionPreview(
            email: "alice@example.com",
            allOwnedVault: [
                DeletionVault(id: "personal", name: "Personal", type: "personal", role: "owner", otherMemberCount: 0),
                DeletionVault(id: "family", name: "Family", type: "shared", role: "owner", otherMemberCount: 2),
            ],
            allMemberVault: [DeletionVault(id: "work", name: "Work", type: "shared", role: "editor", otherMemberCount: 0)],
            shareLinkCount: 1, sentInvitationCount: 1, receivedInvitationCount: 1, deviceCount: 3
        )
        XCTAssertEqual(preview.peopleLosingAccessCount, 2)
        XCTAssertEqual(AccountLifecycleLogic.describeOwnedVault(preview.allOwnedVault[1]), "Deleted with its items. 2 people lose access.")
        XCTAssertEqual(AccountLifecycleLogic.describeMemberVault(preview.allMemberVault[0]), "Stays with its owners. Your access is removed.")
        XCTAssertEqual(AccountLifecycleLogic.describeAllOtherConsequence(preview), [
            "1 share link stops working.", "2 invitations are withdrawn.", "This iPhone and 2 other devices are signed out.",
        ])
    }

    func testPreviewDecodesTheBackendJSON() throws {
        let json = #"{"email":"a@example.com","ownedVaults":[{"id":"v1","name":"Family","type":"shared","role":"owner","otherMemberCount":2}],"memberVaults":[],"shareLinkCount":3,"sentInvitationCount":0,"receivedInvitationCount":0,"deviceCount":1}"#
        let preview = try JSONDecoder().decode(AccountDeletionPreview.self, from: Data(json.utf8))
        XCTAssertEqual(preview.allOwnedVault.first?.otherMemberCount, 2)
        XCTAssertEqual(preview.shareLinkCount, 3)
        let status = try JSONDecoder().decode(VaultStatus.self, from: Data(#"{"state":"signed-out","signedOutReason":"This device was signed out."}"#.utf8))
        XCTAssertEqual(status.signedOutReason, "This device was signed out.")
    }

    func testPendingResultPreservesCommittedKeyAndProblems() throws {
        let json = #"{"status":{"state":"signed-out"},"secretKey":"H1-NEW-SECRET","pendingOperationId":"pending-1","setupState":"needs-finishing","problems":["The keychain is unavailable."],"localError":"The offline copy could not finish."}"#
        let result = try JSONDecoder().decode(CredentialChangeResult.self, from: Data(json.utf8))
        XCTAssertEqual(result.secretKey, "H1-NEW-SECRET")
        XCTAssertEqual(result.pendingOperationId, "pending-1")
        XCTAssertEqual(result.setupState, "needs-finishing")
        XCTAssertEqual(result.problems, ["The keychain is unavailable."])
        XCTAssertEqual(result.localError, "The offline copy could not finish.")
        let signup = try JSONDecoder().decode(SignUpResult.self, from: Data(json.utf8))
        XCTAssertEqual(signup.pendingOperationId, "pending-1")
        XCTAssertEqual(signup.problems, result.problems)
    }

    func testSigningOutKeepsTheServerForSigningBackIn() async {
        let model = await createUnlockedModel()
        let server = model.status.server
        XCTAssertFalse(server.isEmpty)
        let isSignedOut = await model.signOut()
        XCTAssertTrue(isSignedOut)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertEqual(model.status.server, server, "the sign-in form opens on the server this iPhone used")
        XCTAssertEqual(model.status.email, "")
    }

    func testChangeMasterPassword() async {
        let model = await createUnlockedModel()
        let refused = await model.changeMasterPassword(currentPassword: "wrong", newPassword: "a new master password")
        XCTAssertNil(refused)
        XCTAssertEqual(model.errorMessage, "The master password is not correct")
        let result = await model.changeMasterPassword(currentPassword: FakeBackend.password, newPassword: "a new master password")
        XCTAssertEqual(result?.signedOutDeviceCount, 1)
        XCTAssertEqual(model.screen, .unlocked)
        XCTAssertEqual(model.noticeMessage, "Master password changed. 1 other device was signed out.")
        XCTAssertEqual(backend.currentPassword, "a new master password")
    }

    func testReplaceSecretKeyWritesAndRemovesTheNewEmergencyKit() async throws {
        let model = await createUnlockedModel()
        let replaced = await model.replaceSecretKey(password: FakeBackend.password)
        let result = try XCTUnwrap(replaced)
        XCTAssertNotEqual(result.secretKey, FakeBackend.secretKey)
        XCTAssertTrue(FileManager.default.fileExists(atPath: result.emergencyKitPath))
        model.finishSecretKeyReplacement(result)
        XCTAssertFalse(FileManager.default.fileExists(atPath: result.emergencyKitPath))
        await waitUntil({ backend.allCallName.contains("forgetNewAccountSecretKey") }, "the new Secret Key was not forgotten")
    }

    func testDeleteAccountCleanupErrorStillSignsOut() async {
        let model = await createUnlockedModel()
        backend.accountDeletionCleanupError = BackendError(message: "The offline copy could not be removed.")
        let isDeleted = await model.deleteAccount(password: FakeBackend.password, email: "alice@example.com", planVersion: backend.deletionPreview.planVersion)
        XCTAssertFalse(isDeleted)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertTrue(model.allVault.isEmpty)
        XCTAssertEqual(model.errorMessage, "The offline copy could not be removed.")
    }

    func testDeleteAccountSignsOutAndRemovesAutoFillIdentities() async {
        let model = await createUnlockedModel()
        XCTAssertFalse(identityStore.allIdentity.isEmpty)
        let preview = await model.loadAccountDeletionPreview()
        XCTAssertEqual(preview?.email, "alice@example.com")
        let isDeletedWithWrongEmail = await model.deleteAccount(password: FakeBackend.password, email: "bob@example.com", planVersion: preview!.planVersion)
        XCTAssertFalse(isDeletedWithWrongEmail)
        XCTAssertEqual(model.screen, .unlocked)
        let isDeleted = await model.deleteAccount(password: FakeBackend.password, email: "alice@example.com", planVersion: preview!.planVersion)
        XCTAssertTrue(isDeleted)
        XCTAssertTrue(backend.isAccountDeleted)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertTrue(identityStore.allIdentity.isEmpty)
        XCTAssertTrue(model.allVault.isEmpty)
        XCTAssertEqual(model.noticeMessage, "Your Hisar account was deleted.")
    }

    func testDeletionRejectsChangedPlanUntilReviewedAgain() async throws {
        let model = await createUnlockedModel()
        let loaded = await model.loadAccountDeletionPreview()
        let reviewed = try XCTUnwrap(loaded)
        backend.deletionPreview.planVersion = "new-plan"
        let refused = await model.deleteAccount(password: FakeBackend.password, email: reviewed.email, planVersion: reviewed.planVersion)
        XCTAssertFalse(refused)
        XCTAssertFalse(backend.isAccountDeleted)
        XCTAssertEqual(model.screen, .unlocked)
        let reloaded = await model.loadAccountDeletionPreview()
        let refreshed = try XCTUnwrap(reloaded)
        let deleted = await model.deleteAccount(password: FakeBackend.password, email: refreshed.email, planVersion: refreshed.planVersion)
        XCTAssertTrue(deleted)
    }

    private func seedPendingRecovery(phase: String = "outcome-unknown") {
        backend.allPendingRecovery = [
            PendingRecovery(operationId: "pending-1", kind: "signup", origin: "https://recovery.example", accountId: "pending-account", email: "pending@example.com", name: "Pending", phase: phase, kitSaved: false, problems: ["The keychain is unavailable. Use your master password to finish."]),
        ]
    }

    func testPendingRecoverySurvivesModelRestartWithoutSigningIn() async throws {
        seedPendingRecovery()
        let first = AppModel(credentialIdentityStore: identityStore)
        first.start { self.backend }
        await first.refreshPendingRecovery()
        XCTAssertTrue(first.isPendingRecoveryShown)
        first.closePendingRecovery()
        XCTAssertEqual(backend.allPendingRecovery.count, 1)
        let restarted = AppModel(credentialIdentityStore: identityStore)
        restarted.start { self.backend }
        await restarted.refreshPendingRecovery()
        XCTAssertTrue(restarted.isPendingRecoveryShown)
        XCTAssertEqual(restarted.allPendingRecovery.first?.email, "pending@example.com")
        XCTAssertEqual(restarted.screen, .signedOut)
        await restarted.resumePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(restarted.screen, .signedOut)
        XCTAssertEqual(restarted.allPendingRecovery.first?.phase, "delivery-pending")
        await restarted.exportPendingRecoveryKit(operationID: "pending-1", password: FakeBackend.password)
        let export = try XCTUnwrap(restarted.pendingRecoveryExport)
        XCTAssertEqual(export.secretKey, FakeBackend.secretKey)
        XCTAssertTrue(FileManager.default.fileExists(atPath: export.path))
        XCTAssertEqual(restarted.screen, .signedOut)
        await restarted.acknowledgePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertTrue(restarted.allPendingRecovery.isEmpty)
        XCTAssertFalse(FileManager.default.fileExists(atPath: export.path))
    }

    func testPendingRecoveryRequiresPasswordAndExplicitKitAcknowledgement() async {
        seedPendingRecovery(phase: "delivery-pending")
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { self.backend }
        await model.refreshPendingRecovery()
        await model.exportPendingRecoveryKit(operationID: "pending-1", password: "wrong")
        XCTAssertNil(model.pendingRecoveryExport)
        await model.acknowledgePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(backend.allPendingRecovery.count, 1)
        await model.exportPendingRecoveryKit(operationID: "pending-1", password: FakeBackend.password)
        model.closePendingRecovery()
        XCTAssertNil(model.pendingRecoveryExport)
        XCTAssertEqual(backend.allPendingRecovery.count, 1)
        XCTAssertTrue(backend.allPendingRecovery[0].kitSaved)
    }

    func testLockWhilePreparingTheKitHidesItAndSaysToPrepareItAgain() async {
        let model = await createUnlockedModel()
        seedPendingRecovery(phase: "delivery-pending")
        await model.refreshPendingRecovery()
        let isPreparing = DispatchSemaphore(value: 0)
        let mayFinish = DispatchSemaphore(value: 0)
        backend.onExportPendingRecoveryEmergencyKit = {
            isPreparing.signal()
            mayFinish.wait()
        }
        let export = Task { await model.exportPendingRecoveryKit(operationID: "pending-1", password: FakeBackend.password) }
        while isPreparing.wait(timeout: .now()) == .timedOut {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        model.lock()
        backend.onExportPendingRecoveryEmergencyKit = nil
        mayFinish.signal()
        await export.value
        XCTAssertNil(model.pendingRecoveryExport)
        XCTAssertEqual(model.errorMessage, "Hisar locked, signed out or switched accounts while it prepared the Emergency Kit. Prepare it again.")
        await model.exportPendingRecoveryKit(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(model.pendingRecoveryExport?.secretKey, FakeBackend.secretKey)
    }

    func testLaunchDeletesKitFilesTheLastProcessLeft() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("LaunchCleanup-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let allLeftover = ["PendingRecovery-1/Hisar Emergency Kit.pdf", "EmergencyKit-1/Hisar Emergency Kit.pdf", "Export-1/hisar-export.json"]
            .map { directory.appendingPathComponent($0) }
        let kept = directory.appendingPathComponent("Other-1/notes.txt")
        for file in allLeftover + [kept] {
            try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
            try Data("x".utf8).write(to: file)
        }

        AppModel.deleteLeftoverKitFiles(in: directory)

        for file in allLeftover {
            XCTAssertFalse(FileManager.default.fileExists(atPath: file.path), "\(file.lastPathComponent) outlived the process that wrote it")
        }
        XCTAssertTrue(FileManager.default.fileExists(atPath: kept.path))
    }

    func testPendingSignupKeepsProblemsWithoutApplyingStaleStatus() async {
        seedPendingRecovery()
        var staleStatus = VaultStatus.signedOut
        staleStatus.state = VaultStatus.stateUnlocked
        backend.pendingSignUpResult = SignUpResult(status: staleStatus, secretKey: "", pendingOperationId: "pending-1", setupState: "outcome-unknown", problems: ["The server response was lost."])
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { self.backend }
        await model.refreshPendingRecovery()
        let accepted = await model.signUp(inviteLink: "https://recovery.example/invite/test", email: "pending@example.com", name: "Pending", password: FakeBackend.password)
        XCTAssertTrue(accepted)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertNil(model.newSecretKey)
        XCTAssertTrue(model.isPendingRecoveryShown)
        XCTAssertEqual(model.errorMessage, "The server response was lost.")
    }

    func testPendingSignupThatInstalledTheAccountShowsItsVaults() async {
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { self.backend }
        backend.seedSignedIn()
        seedPendingRecovery(phase: "delivery-pending")
        backend.pendingSignUpResult = SignUpResult(status: backend.status, secretKey: "", pendingOperationId: "pending-1", setupState: "delivery-pending", problems: [])
        let accepted = await model.signUp(inviteLink: "https://recovery.example/invite/test", email: "pending@example.com", name: "Pending", password: FakeBackend.password)
        XCTAssertTrue(accepted)
        XCTAssertEqual(model.screen, .unlocked)
        XCTAssertEqual(model.allVault.map(\.name).sorted(), ["Family", "Personal"])
        XCTAssertTrue(model.isPendingRecoveryShown)
    }

    func testPendingReplacementKeepsRecoveryAndLocalProblems() async throws {
        let model = await createUnlockedModel()
        seedPendingRecovery(phase: "needs-finishing")
        backend.pendingCredentialChangeResult = CredentialChangeResult(
            status: backend.status, secretKey: FakeBackend.secretKey, localError: "The keychain is unavailable.",
            pendingOperationId: "pending-1", setupState: "needs-finishing", problems: ["The keychain is unavailable."]
        )
        let result = await model.replaceSecretKey(password: FakeBackend.password)
        XCTAssertEqual(result?.pendingOperationId, "pending-1")
        XCTAssertTrue(model.isPendingRecoveryShown)
        XCTAssertEqual(model.errorMessage, "The keychain is unavailable.")
        model.closePendingRecovery()
        XCTAssertEqual(backend.allPendingRecovery.count, 1)
        await model.refreshPendingRecovery()
        await model.resumePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        await model.exportPendingRecoveryKit(operationID: "pending-1", password: FakeBackend.password)
        let exported = try XCTUnwrap(model.pendingRecoveryExport)
        model.lock()
        XCTAssertNil(model.pendingRecoveryExport)
        XCTAssertFalse(FileManager.default.fileExists(atPath: exported.path))
        XCTAssertEqual(backend.allPendingRecovery.count, 1)
    }

    /// Holds a refresh at its last listing while change runs, as when the device is signed out or locks meanwhile.
    private func finishRefresh(of model: AppModel, after change: () async -> Void) async {
        let isListing = DispatchSemaphore(value: 0)
        let mayFinish = DispatchSemaphore(value: 0)
        backend.onListAllPasskey = {
            isListing.signal()
            mayFinish.wait()
        }
        let refresh = Task { await model.refreshAll() }
        while isListing.wait(timeout: .now()) == .timedOut {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        await change()
        backend.onListAllPasskey = nil
        mayFinish.signal()
        await refresh.value
    }

    func testRefreshFinishingAfterSignOutBringsNothingBack() async {
        let model = await createUnlockedModel()
        await backend.finishRefresh(of: model) {
            backend.status = .signedOut
            model.handleLockedEvent()
            await waitUntil({ identityStore.allIdentity.isEmpty }, "signing out did not clear AutoFill")
        }
        XCTAssertTrue(identityStore.allIdentity.isEmpty, "the refresh put the signed-out account's logins back into AutoFill")
        XCTAssertTrue(model.allVault.isEmpty, "the refresh restored the signed-out account's vaults")
        XCTAssertTrue(model.allItemByVaultID.isEmpty, "the refresh restored the signed-out account's items")
    }

    func testRefreshFinishingAfterLockingBringsNothingBackAndKeepsAutoFill() async {
        let model = await createUnlockedModel()
        let allPublishedIdentity = identityStore.allIdentity
        let replaceCount = identityStore.replaceCount
        await backend.finishRefresh(of: model) { model.lock() }
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(model.allVault.isEmpty, "the refresh restored the vaults behind the lock screen")
        XCTAssertTrue(model.allItemByVaultID.isEmpty, "the refresh restored the items behind the lock screen")
        XCTAssertEqual(identityStore.allIdentity, allPublishedIdentity, "a lock keeps the AutoFill identities")
        XCTAssertEqual(identityStore.replaceCount, replaceCount)
    }

    func testRefreshFinishingAfterANewerRefreshKeepsTheNewerLists() async {
        let model = await createUnlockedModel()
        await backend.finishRefresh(of: model) {
            backend.onListAllPasskey = nil
            backend.allFavoriteRecordID = ["family/netflix"]
            await model.refreshAll()
        }
        XCTAssertEqual(model.listAllFavoriteReference().map(\.item.title), ["Netflix"], "the older refresh brought back the older lists")
    }

    func testSignedOutByServerShowsReasonAndClearsAutoFill() async throws {
        let model = await createUnlockedModel()
        XCTAssertFalse(identityStore.allIdentity.isEmpty)
        backend.status = .signedOut
        backend.status.signedOutReason = "This device was signed out."
        model.handleLockedEvent()
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertEqual(model.status.signedOutReason, "This device was signed out.")
        await waitUntil({ identityStore.allIdentity.isEmpty }, "the AutoFill identities were not removed")
        model.dismissSignedOutReason()
        XCTAssertEqual(model.status.signedOutReason, "")
    }
    func testManualRecoveryRequiresSavedKitAfterRestart() async throws {
        seedPendingRecovery(phase: "awaiting-kit")
        backend.allPendingRecovery[0].manualUnlock = true
        let first = AppModel(credentialIdentityStore: identityStore)
        first.start { self.backend }
        await first.refreshPendingRecovery()
        await first.exportPendingRecoveryKit(operationID: "pending-1", password: FakeBackend.password)
        let kit = try XCTUnwrap(first.pendingRecoveryExport)
        await first.acknowledgePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(first.allPendingRecovery.first?.phase, "prepared")
        backend.isPendingRecoveryFactorAvailable = false
        let restarted = AppModel(credentialIdentityStore: identityStore)
        restarted.start { self.backend }
        await restarted.refreshPendingRecovery()
        await restarted.resumePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(restarted.allPendingRecovery.first?.phase, "prepared")
        await restarted.resumePendingRecovery(operationID: "pending-1", password: FakeBackend.password, secretKey: "wrong")
        XCTAssertEqual(restarted.allPendingRecovery.first?.phase, "prepared")
        await restarted.resumePendingRecovery(operationID: "pending-1", password: FakeBackend.password, secretKey: kit.secretKey)
        XCTAssertEqual(restarted.allPendingRecovery.first?.phase, "delivery-pending")
        await restarted.acknowledgePendingRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(restarted.allPendingRecovery.first?.phase, "manual-unlock")
        XCTAssertEqual(restarted.screen, .signedOut)
        XCTAssertTrue(backend.allCallName.contains("resumePendingRecoveryWithSecretKey"))
    }

    func testOnlyUncommittedRecoveryCanBeDiscarded() async {
        seedPendingRecovery(phase: "awaiting-kit")
        backend.isPendingRecoveryFactorAvailable = false
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { self.backend }
        await model.refreshPendingRecovery()
        await model.discardPreparedRecovery(operationID: "pending-1", password: "wrong")
        XCTAssertEqual(model.allPendingRecovery.count, 1)
        await model.discardPreparedRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertTrue(model.allPendingRecovery.isEmpty)
        seedPendingRecovery(phase: "outcome-unknown")
        await model.refreshPendingRecovery()
        await model.discardPreparedRecovery(operationID: "pending-1", password: FakeBackend.password)
        XCTAssertEqual(model.allPendingRecovery.count, 1)
        XCTAssertEqual(model.screen, .signedOut)
    }

    func testVersionCheckFinishingAfterSignOutDoesNotRestoreAccount() async {
        let model = await createUnlockedModel()
        await model.checkMinimumVersion()
        backend.onCheckMinimumVersion = {
            self.backend.status = .signedOut
            DispatchQueue.main.sync {
                MainActor.assumeIsolated { model.handleLockedEvent() }
            }
        }
        await model.checkMinimumVersion()
        backend.onCheckMinimumVersion = nil
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertTrue(model.allVault.isEmpty)
        XCTAssertNil(model.newSecretKey)
    }

    func testPendingRecoveryRefreshDoesNotDiscardActionError() async {
        let model = await createUnlockedModel()
        model.errorMessage = "The recovery password is not correct."
        await model.refreshPendingRecovery()
        XCTAssertEqual(model.errorMessage, "The recovery password is not correct.")
    }
}
