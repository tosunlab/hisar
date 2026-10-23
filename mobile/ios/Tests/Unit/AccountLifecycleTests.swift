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

    func testNewPasswordRules() {
        XCTAssertNil(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "", repeatedPassword: ""))
        XCTAssertEqual(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "short", repeatedPassword: ""), "Use at least 10 characters.")
        XCTAssertEqual(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "old password", repeatedPassword: ""), "The new master password is the same as the current one.")
        XCTAssertEqual(AccountLifecycleLogic.describeNewPasswordProblem(currentPassword: "old password", newPassword: "a new master password", repeatedPassword: "a new master"), "The passwords do not match.")
        XCTAssertFalse(AccountLifecycleLogic.canChangePassword(currentPassword: "", newPassword: "a new master password", repeatedPassword: "a new master password"))
        XCTAssertFalse(AccountLifecycleLogic.canChangePassword(currentPassword: "old password", newPassword: "a new master password", repeatedPassword: ""))
        XCTAssertTrue(AccountLifecycleLogic.canChangePassword(currentPassword: "old password", newPassword: "a new master password", repeatedPassword: "a new master password"))
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
        let isDeleted = await model.deleteAccount(password: FakeBackend.password, email: "alice@example.com")
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
        let isDeletedWithWrongEmail = await model.deleteAccount(password: FakeBackend.password, email: "bob@example.com")
        XCTAssertFalse(isDeletedWithWrongEmail)
        XCTAssertEqual(model.screen, .unlocked)
        let isDeleted = await model.deleteAccount(password: FakeBackend.password, email: "alice@example.com")
        XCTAssertTrue(isDeleted)
        XCTAssertTrue(backend.isAccountDeleted)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertTrue(identityStore.allIdentity.isEmpty)
        XCTAssertTrue(model.allVault.isEmpty)
        XCTAssertEqual(model.noticeMessage, "Your Hisar account was deleted.")
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
}
