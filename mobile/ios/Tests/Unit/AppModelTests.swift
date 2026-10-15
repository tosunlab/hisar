import XCTest
@testable import Hisar

@MainActor
final class AppModelTests: XCTestCase {
    private var backend: FakeBackend!
    private var identityStore: FakeCredentialIdentityStore!
    private var currentDate = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUp() async throws {
        backend = FakeBackend()
        identityStore = FakeCredentialIdentityStore()
    }

    private func createModel() -> AppModel {
        let model = AppModel(credentialIdentityStore: identityStore, getCurrentDate: { [unowned self] in currentDate })
        model.start { self.backend }
        return model
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        let model = createModel()
        await model.refreshAll()
        return model
    }

    func testStartsSignedOut() {
        let model = createModel()
        XCTAssertEqual(model.screen, .signedOut)
    }

    func testStartFailureShowsMessage() {
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { throw BackendError(message: "Disk full") }
        XCTAssertEqual(model.screen, .failed("Disk full"))
    }

    func testSignInLoadsVaultsAndItems() async {
        let model = createModel()
        let isSignedIn = await model.signIn(server: " http://127.0.0.1:18080 ", email: "alice@example.com", secretKey: "h1-abc123-aaaaa-bbbbb-ccccc-ddddd-eeeeee", password: FakeBackend.password)
        XCTAssertTrue(isSignedIn)
        XCTAssertEqual(model.screen, .unlocked)
        XCTAssertEqual(model.status.server, "http://127.0.0.1:18080")
        XCTAssertEqual(model.allVault.map(\.name), ["Personal", "Family"])
        XCTAssertEqual(model.listAllItemReference().map(\.item.title), ["Bank", "GitHub", "Netflix"])
        XCTAssertEqual(model.countItem(vaultID: "personal"), 2)
        XCTAssertNil(model.errorMessage)
        XCTAssertFalse(model.isBusy)
    }

    func testSignInFailureKeepsSignedOutWithMessage() async {
        let model = createModel()
        let isSignedIn = await model.signIn(server: "http://127.0.0.1:18080", email: "alice@example.com", secretKey: FakeBackend.secretKey, password: "wrong")
        XCTAssertFalse(isSignedIn)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertEqual(model.errorMessage, "Incorrect email, master password or Secret Key")
    }

    func testSignUpShowsNewSecretKey() async {
        let model = createModel()
        let isSignedUp = await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        XCTAssertTrue(isSignedUp)
        XCTAssertEqual(model.newSecretKey, FakeBackend.secretKey)
        XCTAssertEqual(model.screen, .unlocked)
        model.dismissNewSecretKey()
        XCTAssertNil(model.newSecretKey)
    }

    private func waitForCall(_ name: String) async {
        for _ in 0..<100 where !backend.allCallName.contains(name) {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(backend.allCallName.contains(name), "\(name) was not called")
    }

    func testSecretKeyScreenOffersFaceIDAndTurnsItOnInOneTap() async {
        let model = createModel()
        await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        model.finishNewSecretKey()
        XCTAssertTrue(model.isBiometricOfferShown)
        XCTAssertNotNil(model.newSecretKey, "the cover stays up for the offer")
        let isTurnedOn = await model.turnOnBiometricUnlockFromOffer()
        XCTAssertTrue(isTurnedOn)
        XCTAssertTrue(model.status.biometricEnabled)
        XCTAssertFalse(model.isBiometricOfferShown)
        XCTAssertNil(model.newSecretKey)
        XCTAssertFalse(backend.allCallName.contains("enableBiometricUnlock"), "no master password asked again")
        await waitForCall("forgetNewAccountSecretKey")
    }

    func testFaceIDOfferFailureStaysOnTheOffer() async {
        let model = createModel()
        await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        model.finishNewSecretKey()
        backend.isBiometricSucceeding = false
        let isTurnedOn = await model.turnOnBiometricUnlockFromOffer()
        XCTAssertFalse(isTurnedOn)
        XCTAssertTrue(model.isBiometricOfferShown)
        XCTAssertEqual(model.errorMessage, "Face ID was cancelled.")
        model.dismissNewSecretKey()
        XCTAssertFalse(model.isBiometricOfferShown)
        XCTAssertFalse(model.status.biometricEnabled)
        await waitForCall("forgetNewAccountSecretKey")
    }

    func testSecretKeyScreenSkipsTheOfferWithoutFaceID() async {
        backend.isBiometricAvailable = false
        let model = createModel()
        await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        model.finishNewSecretKey()
        XCTAssertFalse(model.isBiometricOfferShown)
        XCTAssertNil(model.newSecretKey)
        await waitForCall("forgetNewAccountSecretKey")
    }

    func testSignInServerDefaultsToTheHostedService() {
        XCTAssertTrue(SignInView.isDefaultServer(""))
        XCTAssertTrue(SignInView.isDefaultServer("https://api.tryhisar.com/"))
        XCTAssertFalse(SignInView.isDefaultServer("http://127.0.0.1:18080"))
        XCTAssertEqual(SignInView.describeServer(SignInView.defaultServer), "api.tryhisar.com")
    }

    func testDiagnosticsHoldVersionsServerAndDeviceOnly() {
        let diagnostics = HelpView.buildDiagnostics(appVersion: "1.2 (34)", systemVersion: "26.0", deviceModel: "iPhone", server: "https://api.tryhisar.com", deviceID: "device-1")
        XCTAssertEqual(diagnostics, "Hisar 1.2 (34)\niOS 26.0 on iPhone\nServer https://api.tryhisar.com\nDevice ID device-1")
        XCTAssertEqual(HelpView.buildDiagnostics(appVersion: "1.2 (34)", systemVersion: "26.0", deviceModel: "iPhone", server: "", deviceID: nil), "Hisar 1.2 (34)\niOS 26.0 on iPhone\nServer not signed in")
        XCTAssertEqual(HelpView.supportEmail, "support@tryhisar.com")
    }

    func testLockAndUnlockWithPassword() async {
        let model = await createUnlockedModel()
        model.lock()
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(model.allVault.isEmpty, "vault data is dropped on lock")
        let isWrongUnlocked = await model.unlock(password: "nope")
        XCTAssertFalse(isWrongUnlocked)
        XCTAssertEqual(model.screen, .locked)
        XCTAssertNotNil(model.errorMessage)
        let isUnlocked = await model.unlock(password: FakeBackend.password)
        XCTAssertTrue(isUnlocked)
        XCTAssertEqual(model.screen, .unlocked)
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(model.allVault.count, 2)
    }

    func testLockedEventFromBackendShowsLockScreen() async {
        let model = await createUnlockedModel()
        let lockCountBefore = model.lockCount
        _ = backend.lock()
        model.handleLockedEvent()
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.lockCount, lockCountBefore + 1)
    }

    func testBackgroundLongerThanAutoLockLocks() async {
        let model = await createUnlockedModel()
        model.handleDidEnterBackground()
        currentDate += model.autoLockInterval + 1
        model.handleWillEnterForeground()
        XCTAssertEqual(model.screen, .locked)
    }

    func testShortBackgroundStaysUnlocked() async {
        let model = await createUnlockedModel()
        model.handleDidEnterBackground()
        currentDate += 30
        model.handleWillEnterForeground()
        XCTAssertEqual(model.screen, .unlocked)
    }

    func testBiometricUnlockNeedsReadyStatus() async {
        let model = await createUnlockedModel()
        model.lock()
        let isUnlockedWithoutEnabling = await model.unlockWithBiometric()
        XCTAssertFalse(isUnlockedWithoutEnabling)
        XCTAssertFalse(backend.allCallName.contains("unlockWithBiometric"))
    }

    func testEnableBiometricRequiresMasterPasswordThenUnlocks() async {
        let model = await createUnlockedModel()
        let isEnabledWithWrongPassword = await model.setBiometricUnlock(isEnabled: true, password: "wrong")
        XCTAssertFalse(isEnabledWithWrongPassword)
        XCTAssertFalse(model.status.biometricEnabled)
        let isEnabled = await model.setBiometricUnlock(isEnabled: true, password: FakeBackend.password)
        XCTAssertTrue(isEnabled)
        XCTAssertTrue(model.status.biometricEnabled)
        model.lock()
        let isUnlocked = await model.unlockWithBiometric()
        XCTAssertTrue(isUnlocked)
        XCTAssertEqual(model.screen, .unlocked)
        _ = await model.setBiometricUnlock(isEnabled: false, password: "")
        XCTAssertFalse(model.status.biometricEnabled)
    }

    func testFailedBiometricStaysLocked() async {
        let model = await createUnlockedModel()
        _ = await model.setBiometricUnlock(isEnabled: true, password: FakeBackend.password)
        model.lock()
        backend.isBiometricSucceeding = false
        let isUnlocked = await model.unlockWithBiometric()
        XCTAssertFalse(isUnlocked)
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.errorMessage, "Face ID was cancelled.")
    }

    func testSignOutClearsIdentities() async {
        let model = await createUnlockedModel()
        XCTAssertFalse(identityStore.allIdentity.isEmpty)
        let isSignedOut = await model.signOut()
        XCTAssertTrue(isSignedOut)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertTrue(identityStore.allIdentity.isEmpty)
    }

    func testRefreshPublishesCredentialIdentities() async {
        _ = await createUnlockedModel()
        let allHost = Set(identityStore.allIdentity.map(\.host))
        XCTAssertEqual(allHost, ["github.com", "bank.example.com", "www.netflix.com"])
        XCTAssertTrue(identityStore.allIdentity.contains(CredentialIdentityRecord(host: "github.com", username: "alice", recordIdentifier: "personal/github")))
        XCTAssertEqual(identityStore.allIdentity.filter { $0.kind == .oneTimeCode }, [CredentialIdentityRecord(kind: .oneTimeCode, host: "github.com", username: "alice", recordIdentifier: "personal/github")])
    }

    func testOneTimePasswordIsSavedLoadedAndCopied() async {
        let model = await createUnlockedModel()
        XCTAssertEqual(model.listAllItemReference(vaultID: "personal").first { $0.item.id == "github" }?.item.hasOneTimePassword, true)
        XCTAssertEqual(model.listAllItemReference(vaultID: "personal").first { $0.item.id == "github" }?.item.oneTimePassword, "", "summaries never carry the secret")
        let code = await model.loadOneTimeCode(vaultID: "personal", itemID: "github")
        XCTAssertEqual(code, FakeBackend.oneTimeCode)
        let noCode = await model.loadOneTimeCode(vaultID: "personal", itemID: "bank")
        XCTAssertNil(noCode)
        await model.copyField(vaultID: "personal", itemID: "github", fieldName: "one-time code", label: "One-time code")
        XCTAssertEqual(backend.allCopiedField, ["github.one-time code"])
        XCTAssertEqual(model.noticeMessage, "One-time code copied. It clears in 90 seconds.")

        let draft = ItemDetail(title: "Mastodon", username: "alice", allURL: ["mastodon.social"], password: "secret", oneTimePassword: "jbsw y3dp ehpk 3pxp")
        let savedItem = await model.saveItem(vaultID: "personal", item: draft)
        XCTAssertEqual(savedItem?.oneTimePassword, "otpauth://totp/?secret=JBSWY3DPEHPK3PXP")
        let summary = model.listAllItemReference(vaultID: "personal").first { $0.item.title == "Mastodon" }?.item
        XCTAssertEqual(summary?.oneTimePassword, "")
        XCTAssertEqual(summary?.hasOneTimePassword, true)
        XCTAssertTrue(identityStore.allIdentity.contains { $0.kind == .oneTimeCode && $0.host == "mastodon.social" })
    }

    func testInvalidOneTimePasswordShowsTheDesktopMessage() async {
        let model = await createUnlockedModel()
        let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(title: "Broken", oneTimePassword: "not a secret!"))
        XCTAssertNil(savedItem)
        XCTAssertEqual(model.errorMessage, "The one-time password is not a valid secret or otpauth:// link")
    }

    func testSaveNewItemAddsItWithoutSecretsAndUpdatesIdentities() async {
        let model = await createUnlockedModel()
        let draft = ItemDetail(title: "Mastodon", username: "alice", allURL: ["mastodon.social"], password: "secret")
        let savedItem = await model.saveItem(vaultID: "personal", item: draft)
        XCTAssertEqual(savedItem?.password, "secret")
        let summary = model.listAllItemReference(vaultID: "personal").first { $0.item.title == "Mastodon" }
        XCTAssertNotNil(summary)
        XCTAssertEqual(summary?.item.password, "", "list summaries never keep the password")
        XCTAssertTrue(identityStore.allIdentity.contains { $0.host == "mastodon.social" })
    }

    func testSaveWithoutTitleShowsError() async {
        let model = await createUnlockedModel()
        let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(title: "  "))
        XCTAssertNil(savedItem)
        XCTAssertEqual(model.errorMessage, "Enter a title")
    }

    func testEditExistingItemReplacesIt() async {
        let model = await createUnlockedModel()
        guard var item = await model.loadItem(vaultID: "personal", itemID: "github") else { return XCTFail("item missing") }
        XCTAssertEqual(item.password, "github-password")
        item.title = "GitHub Work"
        _ = await model.saveItem(vaultID: "personal", item: item)
        XCTAssertEqual(model.listAllItemReference(vaultID: "personal").map(\.item.title), ["Bank", "GitHub Work"])
    }

    func testDeleteItemRemovesIt() async {
        let model = await createUnlockedModel()
        let isDeleted = await model.deleteItem(vaultID: "family", itemID: "netflix")
        XCTAssertTrue(isDeleted)
        XCTAssertTrue(model.listAllItemReference(vaultID: "family").isEmpty)
        XCTAssertFalse(identityStore.allIdentity.contains { $0.host == "www.netflix.com" })
    }

    func testSaveSecureNoteKeepsItsCategoryAndIsNotALogin() async {
        let model = await createUnlockedModel()
        let draft = ItemDetail(category: ItemDetail.categorySecureNote, title: "Recovery codes", notes: "one\ntwo")
        guard let savedItem = await model.saveItem(vaultID: "personal", item: draft) else { return XCTFail("note not saved") }
        XCTAssertTrue(savedItem.isSecureNote)
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.notes, "")
        XCTAssertEqual(reference?.item.isSecureNote, true)
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.secureNotes.includes($0.item) }.map(\.item.title), ["Recovery codes"])
        XCTAssertFalse(model.listAllItemReference().contains { ItemCategoryDestination.logins.includes($0.item) && $0.item.id == savedItem.id })
    }

    func testConvertToSecureNoteUpdatesTheItem() async {
        let model = await createUnlockedModel()
        let draft = ItemDetail(title: "Door codes", notes: "Front 1234", canConvertToSecureNote: true)
        guard let login = await model.saveItem(vaultID: "personal", item: draft) else { return XCTFail("login not saved") }
        guard let converted = await model.convertToSecureNote(vaultID: "personal", itemID: login.id) else { return XCTFail("not converted") }
        XCTAssertTrue(converted.isSecureNote)
        XCTAssertFalse(converted.canConvertToSecureNote)
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == login.id }
        XCTAssertEqual(reference?.item.category, ItemDetail.categorySecureNote)
        XCTAssertEqual(reference?.item.canConvertToSecureNote, false)
        XCTAssertEqual(model.noticeMessage, "Converted to a secure note.")
    }

    func testConvertToSecureNoteRefusesALoginWithAPassword() async {
        let model = await createUnlockedModel()
        let converted = await model.convertToSecureNote(vaultID: "personal", itemID: "github")
        XCTAssertNil(converted)
        XCTAssertNotNil(model.errorMessage)
        XCTAssertEqual(model.listAllItemReference(vaultID: "personal").first { $0.item.id == "github" }?.item.isLogin, true)
    }

    func testCopyFieldShowsNotice() async {
        let model = await createUnlockedModel()
        await model.copyField(vaultID: "personal", itemID: "github", fieldName: "password", label: "Password")
        XCTAssertEqual(backend.allCopiedField, ["github.password"])
        XCTAssertEqual(model.noticeMessage, "Password copied. It clears in 90 seconds.")
    }

    func testOfflineStatusIsPublished() async {
        backend.isOffline = true
        let model = await createUnlockedModel()
        XCTAssertTrue(model.status.isOffline)
    }

    func testGeneratePasswordUsesOptions() async {
        let model = await createUnlockedModel()
        let password = await model.generatePassword(length: 12, isDigitIncluded: true, isSymbolIncluded: false)
        XCTAssertEqual(password, "111111111111")
    }

    func testCreateVaultAddsASharedVaultOwnedByTheUser() async {
        let model = await createUnlockedModel()
        let vault = await model.createVault(name: "  Streaming ")
        XCTAssertEqual(vault?.name, "Streaming")
        XCTAssertEqual(vault?.type, "shared")
        XCTAssertEqual(vault?.role, "owner")
        XCTAssertEqual(model.allVault.map(\.name), ["Personal", "Family", "Streaming"], "personal first, then shared vaults by name")
        XCTAssertEqual(model.countItem(vaultID: vault?.id ?? ""), 0)
        let allMember = await model.loadAllVaultMember(vaultID: vault?.id ?? "")
        XCTAssertEqual(allMember?.map(\.email), ["alice@example.com"])
        XCTAssertEqual(allMember?.first?.role, "owner")
    }

    func testCreateVaultWithoutNameShowsError() async {
        let model = await createUnlockedModel()
        let vault = await model.createVault(name: "   ")
        XCTAssertNil(vault)
        XCTAssertEqual(model.errorMessage, "Enter a vault name")
        XCTAssertEqual(model.allVault.count, 2)
    }

    func testAnyoneInvitesByEmailWithTrimmedAddress() async {
        backend.seedSignedIn()
        backend.allMember[0].role = Member.roleMember
        let model = createModel()
        let isInvited = await model.invitePerson(email: " dave@example.com ")
        XCTAssertTrue(isInvited)
        XCTAssertEqual(backend.allInvitedEmail, ["dave@example.com"])
        XCTAssertNil(model.errorMessage)
        let isInvalidInvited = await model.invitePerson(email: "dave")
        XCTAssertFalse(isInvalidInvited)
        XCTAssertEqual(model.errorMessage, "Enter the email address of the person")
    }

    func testPeopleAreOnlyThoseWhoShareAVault() async {
        let model = await createUnlockedModel()
        let allPerson = await model.loadAllPerson()
        XCTAssertEqual(allPerson?.map(\.email), ["alice@example.com", "bob@example.com"])
    }

    func testOwnerSendsShareInvitationThenChangesRoleAndRemovesMember() async {
        let model = await createUnlockedModel()
        guard let vault = await model.createVault(name: "Streaming") else { return XCTFail("vault missing") }
        let isShared = await model.shareVault(vaultID: vault.id, email: " bob@example.com ", role: Member.vaultRoleReader)
        XCTAssertTrue(isShared)
        let shareInvitations = await model.loadShareInvitations()
        XCTAssertEqual(shareInvitations?.allSent.map(\.email), ["bob@example.com"])
        XCTAssertEqual(shareInvitations?.allSent.first?.vaultName, "Streaming")
        var allMember = await model.loadAllVaultMember(vaultID: vault.id)
        XCTAssertEqual(allMember?.map(\.email), ["alice@example.com"], "nobody gets access before accepting")

        backend.allVaultMemberByVaultID[vault.id]?.append(Member(accountID: "bob-id", name: "Bob", email: "bob@example.com", role: Member.vaultRoleReader, securityCode: "3333 4444"))
        let isRoleChanged = await model.changeVaultMemberRole(vaultID: vault.id, accountID: "bob-id", role: Member.vaultRoleEditor)
        XCTAssertTrue(isRoleChanged)
        allMember = await model.loadAllVaultMember(vaultID: vault.id)
        XCTAssertEqual(allMember?.last?.role, Member.vaultRoleEditor)

        let isRemoved = await model.removeVaultMember(vaultID: vault.id, accountID: "bob-id")
        XCTAssertTrue(isRemoved)
        allMember = await model.loadAllVaultMember(vaultID: vault.id)
        XCTAssertEqual(allMember?.map(\.email), ["alice@example.com"])
        guard let removeIndex = backend.allCallName.firstIndex(of: "removeVaultMember") else { return XCTFail("remove not called") }
        XCTAssertEqual(backend.allCallName[removeIndex + 1], "listAllItem", "items are loaded again after the vault key changed")

        let isCancelled = await model.cancelShareInvitation(invitationID: shareInvitations?.allSent.first?.id ?? "")
        XCTAssertTrue(isCancelled)
        let remaining = await model.loadShareInvitations()
        XCTAssertEqual(remaining?.allSent.count, 0)
    }

    func testReceivedShareInvitationsAreAcceptedOrDeclined() async {
        backend.seedSignedIn()
        backend.shareInvitations = ShareInvitations(allReceived: [
            ReceivedShareInvitation(id: "from-carol", vaultID: "carol-vault", role: Member.vaultRoleEditor, invitedByName: "Carol", invitedByEmail: "carol@example.com", expiresAt: "2027-01-22T08:00:00Z"),
            ReceivedShareInvitation(id: "from-dave", vaultID: "dave-vault", role: Member.vaultRoleReader, invitedByName: "", invitedByEmail: "dave@example.com", expiresAt: "2027-01-22T08:00:00Z"),
        ])
        let model = createModel()
        let shareInvitations = await model.loadShareInvitations()
        XCTAssertEqual(shareInvitations?.allReceived.map(\.inviterName), ["Carol", "dave@example.com"])
        let isAccepted = await model.acceptShareInvitation(invitationID: "from-carol")
        XCTAssertTrue(isAccepted)
        let isDeclined = await model.declineShareInvitation(invitationID: "from-dave")
        XCTAssertTrue(isDeclined)
        XCTAssertEqual(backend.allAnsweredInvitation, ["from-carol accept", "from-dave decline"])
        let isAcceptedTwice = await model.acceptShareInvitation(invitationID: "from-carol")
        XCTAssertFalse(isAcceptedTwice)
        XCTAssertEqual(model.errorMessage, "This invitation no longer exists")
    }

    func testRefreshLoadsReceivedInvitationsForTheBannerAndAnsweringRemovesThem() async {
        backend.seedSignedIn()
        backend.shareInvitations = ShareInvitations(allReceived: [
            ReceivedShareInvitation(id: "from-carol", vaultID: "carol-vault", role: Member.vaultRoleEditor, invitedByName: "Carol", invitedByEmail: "carol@example.com", expiresAt: "2027-01-22T08:00:00Z"),
            ReceivedShareInvitation(id: "from-dave", vaultID: "dave-vault", role: Member.vaultRoleReader, invitedByName: "", invitedByEmail: "dave@example.com", expiresAt: "2027-01-22T08:00:00Z"),
        ])
        let model = createModel()
        await model.refreshAll()
        XCTAssertEqual(model.allReceivedInvitation.map(\.id), ["from-carol", "from-dave"])
        XCTAssertEqual(model.receivedInvitationCount, 2)
        XCTAssertLessThan(backend.allCallName.firstIndex(of: "listAllShareInvitation") ?? .max, backend.allCallName.firstIndex(of: "listAllVault") ?? 0, "accepted shares complete before the vaults load")

        let isAccepted = await model.acceptShareInvitation(invitationID: "from-carol")
        XCTAssertTrue(isAccepted)
        XCTAssertEqual(model.allReceivedInvitation.map(\.id), ["from-dave"])
        let isDeclined = await model.declineShareInvitation(invitationID: "from-dave")
        XCTAssertTrue(isDeclined)
        XCTAssertTrue(model.allReceivedInvitation.isEmpty)

        model.lock()
        XCTAssertTrue(model.allReceivedInvitation.isEmpty)
    }

    func testLockScreenAsksTheOwnerToUnlockToFinishAcceptedShares() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        backend.allShareAwaitingUnlock = [
            ShareAwaitingUnlock(invitationID: "to-ada", recipientName: "Ada", recipientEmail: "ada@example.com", acceptedAt: "2027-01-15T06:00:00Z"),
            ShareAwaitingUnlock(invitationID: "to-bo", recipientName: "", recipientEmail: "bo@example.com", acceptedAt: "2027-01-15T07:00:00Z"),
        ]
        let model = createModel()
        XCTAssertEqual(model.screen, .locked)
        await model.refreshShareAwaitingUnlock()
        XCTAssertEqual(model.allShareAwaitingUnlock.map(\.message), [
            "Ada accepted your invitation to a vault — unlock to finish sharing",
            "bo@example.com accepted your invitation to a vault — unlock to finish sharing",
        ])
        XCTAssertNil(model.errorMessage)

        backend.isShareAwaitingUnlockFailing = true
        await model.refreshShareAwaitingUnlock()
        XCTAssertEqual(model.allShareAwaitingUnlock.count, 2, "a failed refresh keeps the last banner")
        XCTAssertNil(model.errorMessage, "the lock screen stays quiet offline")

        let isUnlocked = await model.unlock(password: FakeBackend.password)
        XCTAssertTrue(isUnlocked)
        XCTAssertTrue(model.allShareAwaitingUnlock.isEmpty, "unlocking finishes the shares, so the banner goes")
        let callCount = backend.allCallName.filter { $0 == "listAllShareAwaitingUnlock" }.count
        await model.refreshShareAwaitingUnlock()
        XCTAssertEqual(backend.allCallName.filter { $0 == "listAllShareAwaitingUnlock" }.count, callCount, "an unlocked app does not ask")
    }

    func testRecipientSeesWhoTheAcceptedShareIsWaitingFor() async {
        backend.seedSignedIn()
        backend.shareInvitations = ShareInvitations(allWaiting: [
            WaitingShareInvitation(id: "from-carol", vaultID: "carol-vault", role: Member.vaultRoleEditor, invitedByName: "Carol", invitedByEmail: "carol@example.com", acceptedAt: "2027-01-15T06:00:00Z"),
        ])
        let model = createModel()
        await model.refreshAll()
        XCTAssertEqual(model.allWaitingInvitation.map(\.id), ["from-carol"])
        let now = ISO8601DateFormatter().date(from: "2027-01-15T08:05:00Z")!
        XCTAssertEqual(model.allWaitingInvitation.first?.describe(now: now), "Waiting for Carol's Hisar to finish sharing · accepted 2 h ago")
        model.lock()
        XCTAssertTrue(model.allWaitingInvitation.isEmpty)
    }

    func testFormatTimeAgo() {
        let now = ISO8601DateFormatter().date(from: "2027-01-15T12:00:00Z")!
        XCTAssertEqual(formatTimeAgo("2027-01-15T11:59:40Z", now: now), "just now")
        XCTAssertEqual(formatTimeAgo("2027-01-15T11:55:00Z", now: now), "5 min ago")
        XCTAssertEqual(formatTimeAgo("2027-01-14T09:00:00Z", now: now), "yesterday")
        XCTAssertEqual(formatTimeAgo("2027-01-12T12:00:00Z", now: now), "3 days ago")
        XCTAssertEqual(formatTimeAgo("", now: now), "")
    }

    func testShareAwaitingUnlockDecodesFromHisarKit() throws {
        let json = #"[{"invitationId":"i","recipientName":"Ada","recipientEmail":"ada@example.com","acceptedAt":"2027-01-15T06:00:00Z"}]"#
        let decoded = try JSONDecoder().decode([ShareAwaitingUnlock].self, from: Data(json.utf8))
        XCTAssertEqual(decoded.first?.id, "i")
        XCTAssertEqual(decoded.first?.recipientDisplayName, "Ada")
        let invitations = try JSONDecoder().decode(ShareInvitations.self, from: Data(#"{"received":null,"sent":null,"waiting":[{"id":"w","vaultId":"v","role":"reader","invitedByName":"","invitedByEmail":"carol@example.com","acceptedAt":"2027-01-15T06:00:00Z"}],"completionError":""}"#.utf8))
        XCTAssertEqual(invitations.allWaiting.first?.inviterName, "carol@example.com")
    }

    func testRemoteChangeReloadsVaultsAndInvitations() async {
        let model = await createUnlockedModel()
        backend.allVault.append(VaultSummary(id: "shared-with-me", name: "Deploy keys", type: "shared", role: Member.vaultRoleReader, itemCount: 0))
        backend.shareInvitations.allReceived = [
            ReceivedShareInvitation(id: "from-carol", vaultID: "carol-vault", role: Member.vaultRoleEditor, invitedByName: "Carol", invitedByEmail: "carol@example.com", expiresAt: "2027-01-22T08:00:00Z"),
        ]
        let remoteChangeCount = model.remoteChangeCount
        model.handleRemoteChange()
        XCTAssertEqual(model.remoteChangeCount, remoteChangeCount + 1, "an open sharing sheet reloads on this count")
        let deadline = Date().addingTimeInterval(5)
        while model.findVault(vaultID: "shared-with-me") == nil && Date() < deadline {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertNotNil(model.findVault(vaultID: "shared-with-me"))
        XCTAssertEqual(model.allReceivedInvitation.map(\.id), ["from-carol"])

        model.lock()
        model.handleRemoteChange()
        XCTAssertEqual(model.remoteChangeCount, remoteChangeCount + 1, "a locked app ignores remote changes")
    }

    func testVaultSharingShowsMembersAndOnlyThisVaultsInvitations() async {
        let model = await createUnlockedModel()
        guard let streaming = await model.createVault(name: "Streaming"), let deploy = await model.createVault(name: "Deploy keys") else { return XCTFail("vaults missing") }
        _ = await model.shareVault(vaultID: streaming.id, email: "bob@example.com", role: Member.vaultRoleReader)
        _ = await model.shareVault(vaultID: deploy.id, email: "carol@example.com", role: Member.vaultRoleEditor)
        let sharing = await model.loadVaultSharing(vaultID: streaming.id)
        XCTAssertEqual(sharing?.vault.name, "Streaming")
        XCTAssertEqual(sharing?.isOwner, true)
        XCTAssertEqual(sharing?.allMember.map(\.email), ["alice@example.com"])
        XCTAssertEqual(sharing?.allInvitation.map(\.email), ["bob@example.com"])

        let editorSharing = await model.loadVaultSharing(vaultID: "family")
        XCTAssertEqual(editorSharing?.isOwner, false, "an editor sees the members only")
        XCTAssertEqual(editorSharing?.allInvitation.count, 0)

        guard let personal = model.allVault.first(where: { $0.type == "personal" }) else { return XCTFail("no personal vault") }
        let personalSharing = await model.loadVaultSharing(vaultID: personal.id)
        XCTAssertNil(personalSharing)
        XCTAssertEqual(model.errorMessage, "Personal vaults are private and cannot be shared; create a shared vault instead")
    }

    func testVaultSharingDecodesFromHisarKit() throws {
        let json = #"{"vault":{"id":"v","name":"Deploy keys","type":"shared","role":"owner","itemCount":2},"isOwner":true,"members":[{"accountId":"a","name":"Ada","email":"ada@example.com","role":"owner","securityCode":"3F2A 91C0 7B44 E1D8","isCurrentUser":true,"isKeyChanged":false}],"invitations":null,"completionError":"bob@example.com: the signing key of bob@example.com changed since you last shared with them; stopping"}"#
        let decoded = try JSONDecoder().decode(VaultSharing.self, from: Data(json.utf8))
        XCTAssertEqual(decoded.vault.name, "Deploy keys")
        XCTAssertTrue(decoded.isOwner)
        XCTAssertEqual(decoded.allMember.first?.securityCode, "3F2A 91C0 7B44 E1D8")
        XCTAssertEqual(decoded.allInvitation.count, 0)
        XCTAssertTrue(decoded.completionError.contains("changed since you last shared"))
    }

    func testShareInvitationsDecodeFromHisarKit() throws {
        let json = #"{"received":[{"id":"a","vaultId":"v","role":"editor","invitedByName":"Carol","invitedByEmail":"carol@example.com","expiresAt":"2027-01-22T08:00:00Z"}],"sent":null,"completionError":""}"#
        let decoded = try JSONDecoder().decode(ShareInvitations.self, from: Data(json.utf8))
        XCTAssertEqual(decoded.allReceived.first?.vaultID, "v")
        XCTAssertEqual(decoded.allSent.count, 0)
    }

    func testSharingWithAStrangerOrAnEditedVaultFails() async {
        let model = await createUnlockedModel()
        let isSharedWithStranger = await model.shareVault(vaultID: "family", email: "nobody@example.com", role: Member.vaultRoleEditor)
        XCTAssertFalse(isSharedWithStranger)
        XCTAssertEqual(model.errorMessage, "Only owners of this vault can share it, change roles or remove members")
        let isPersonalShared = await model.shareVault(vaultID: "personal", email: "bob@example.com", role: Member.vaultRoleEditor)
        XCTAssertFalse(isPersonalShared)
        XCTAssertEqual(model.errorMessage, "Personal vaults are private and cannot be shared; create a shared vault instead")
    }

    func testOperationsRunOffTheMainThread() async {
        final class ThreadRecordingBackend: FakeBackend {
            var isListCalledOnMainThread: Bool?
            override func listAllVault() throws -> [VaultSummary] {
                isListCalledOnMainThread = Thread.isMainThread
                return try super.listAllVault()
            }
        }
        let recordingBackend = ThreadRecordingBackend()
        recordingBackend.seedSignedIn()
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { recordingBackend }
        await model.refreshAll()
        XCTAssertEqual(recordingBackend.isListCalledOnMainThread, false)
    }

    func testUpdateRequiredBlocksTheAppUntilTheServerAcceptsIt() async {
        let model = await createUnlockedModel()
        let requirement = UpdateRequirement(currentVersion: "0.1.0", minimumVersion: "0.2.0", updateURL: "https://tryhisar.com/download")
        backend.serverUpdateRequirement = requirement
        await model.checkMinimumVersion()
        XCTAssertEqual(model.screen, .updateRequired(requirement))
        XCTAssertTrue(model.allPendingApproval.isEmpty)

        backend.serverUpdateRequirement = nil
        await model.checkMinimumVersion()
        XCTAssertEqual(model.screen, .unlocked)
        XCTAssertFalse(model.isCheckingMinimumVersion)
    }

    func testUpdateRequiredEventShowsTheBlockingScreen() {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        XCTAssertEqual(model.screen, .locked)
        let requirement = UpdateRequirement(currentVersion: "", minimumVersion: "0.2.0", updateURL: "https://tryhisar.com/download")
        backend.status.updateRequired = requirement
        model.handleUpdateRequired()
        XCTAssertEqual(model.screen, .updateRequired(requirement))
    }

    func testInviteLinkPrefillsCreateAccountWhenSignedOut() {
        let model = createModel()
        let link = InviteLink(invite: "https://hisar.example.com/invite/abc")
        model.openInviteLink(link)
        XCTAssertEqual(model.pendingInviteLink, link)
        XCTAssertNil(model.inviteLinkMessage)
    }

    func testInviteLinkWhileSignedInExplainsInstead() async {
        let model = await createUnlockedModel()
        model.openInviteLink(InviteLink(invite: "abc"))
        XCTAssertNil(model.pendingInviteLink)
        XCTAssertEqual(model.inviteLinkMessage, "You're signed in as alice@example.com. Sign out to create an account with this invite.")
        XCTAssertEqual(model.screen, .unlocked)
    }
}
