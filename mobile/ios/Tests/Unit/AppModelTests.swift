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

    private func createModel() async -> AppModel {
        let model = AppModel(credentialIdentityStore: identityStore, getCurrentDate: { [unowned self] in currentDate })
        await model.start { self.backend }?.value
        return model
    }

    private func createUnlockedModel() async -> AppModel {
        backend.seedSignedIn()
        return await createModel()
    }

    func testStartsSignedOut() async {
        let model = await createModel()
        XCTAssertEqual(model.screen, .signedOut)
    }

    func testStartFailureShowsMessage() {
        let model = AppModel(credentialIdentityStore: identityStore)
        model.start { throw BackendError(message: "Disk full") }
        XCTAssertEqual(model.screen, .failed("Disk full"))
    }

    func testSignInLoadsVaultsAndItems() async {
        let model = await createModel()
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
        let model = await createModel()
        let isSignedIn = await model.signIn(server: "http://127.0.0.1:18080", email: "alice@example.com", secretKey: FakeBackend.secretKey, password: "wrong")
        XCTAssertFalse(isSignedIn)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertEqual(model.errorMessage, "Incorrect email, master password or Secret Key")
    }

    func testSignUpShowsNewSecretKey() async {
        let model = await createModel()
        let isSignedUp = await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        XCTAssertTrue(isSignedUp)
        XCTAssertEqual(model.newSecretKey, FakeBackend.secretKey)
        XCTAssertEqual(model.screen, .unlocked)
        model.dismissNewSecretKey()
        XCTAssertNil(model.newSecretKey)
    }

    func testLockHidesTheNewSecretKey() async {
        let model = await createModel()
        await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        model.lock()
        XCTAssertEqual(model.screen, .locked)
        XCTAssertNil(model.newSecretKey)
    }

    func testAutoLockHidesTheBiometricOffer() async {
        let model = await createModel()
        await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        model.finishNewSecretKey()
        XCTAssertTrue(model.isBiometricOfferShown)
        _ = backend.lock()
        model.handleLockedEvent()
        XCTAssertNil(model.newSecretKey)
        XCTAssertFalse(model.isBiometricOfferShown)
    }

    private func waitForCall(_ name: String) async {
        for _ in 0..<100 where !backend.allCallName.contains(name) {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(backend.allCallName.contains(name), "\(name) was not called")
    }

    func testSecretKeyScreenOffersFaceIDAndTurnsItOnInOneTap() async {
        let model = await createModel()
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
        let model = await createModel()
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
        let model = await createModel()
        await model.signUp(inviteLink: "http://127.0.0.1:18080/invite/abc", email: "bob@example.com", name: "Bob", password: FakeBackend.password)
        model.finishNewSecretKey()
        XCTAssertFalse(model.isBiometricOfferShown)
        XCTAssertNil(model.newSecretKey)
        await waitForCall("forgetNewAccountSecretKey")
    }

    func testSignInServerDefaultsToTheHostedService() {
        XCTAssertTrue(SignInView.isDefaultServer(""))
        XCTAssertTrue(SignInView.isDefaultServer("https://eu.tryhisar.com/"))
        XCTAssertFalse(SignInView.isDefaultServer("http://127.0.0.1:18080"))
        XCTAssertEqual(SignInView.describeServer(SignInView.defaultServer), "eu.tryhisar.com")
    }

    func testDiagnosticsHoldVersionsServerAndDeviceOnly() {
        let diagnostics = HelpView.buildDiagnostics(appVersion: "1.2 (34)", systemVersion: "26.0", deviceModel: "iPhone", server: "https://eu.tryhisar.com", deviceID: "device-1")
        XCTAssertEqual(diagnostics, "Hisar 1.2 (34)\niOS 26.0 on iPhone\nServer https://eu.tryhisar.com\nDevice ID device-1")
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

    func testSignOutErrorStillShowsTheSignedOutScreen() async {
        let model = await createUnlockedModel()
        backend.signOutError = BackendError(message: "The server did not confirm the sign-out.")
        let isSignedOut = await model.signOut()
        XCTAssertFalse(isSignedOut)
        XCTAssertEqual(model.screen, .signedOut)
        XCTAssertTrue(model.allVault.isEmpty)
        XCTAssertEqual(model.errorMessage, "The server did not confirm the sign-out.")
        await waitUntilIdentitiesAreCleared()
    }

    private let leftoverIdentity = CredentialIdentityRecord(host: "github.com", username: "alice", recordIdentifier: "personal/github")

    private func waitUntilIdentitiesAreCleared() async {
        for _ in 0..<100 where !identityStore.allIdentity.isEmpty {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(identityStore.allIdentity.isEmpty, "AutoFill still offers a signed-out account's logins")
    }

    func testSignedOutStartClearsLeftoverIdentities() async {
        identityStore.allIdentity = [leftoverIdentity]
        let model = await createModel()
        XCTAssertEqual(model.screen, .signedOut)
        await waitUntilIdentitiesAreCleared()
    }

    func testForegroundWhileSignedOutClearsLeftoverIdentities() async {
        let model = await createModel()
        await waitUntilIdentitiesAreCleared()
        identityStore.allIdentity = [leftoverIdentity]
        model.handleDidEnterBackground()
        model.handleWillEnterForeground()
        await waitUntilIdentitiesAreCleared()
    }

    func testLockedStartKeepsAutoFillIdentities() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        identityStore.allIdentity = [leftoverIdentity]
        let model = await createModel()
        XCTAssertEqual(model.screen, .locked)
        try? await Task.sleep(nanoseconds: 100_000_000)
        XCTAssertEqual(identityStore.allIdentity, [leftoverIdentity])
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

    func testSaveCreditCardShowsBrandAndLastFourWithoutSecrets() async {
        let model = await createUnlockedModel()
        var card = ItemCard()
        card.number = "4111 1111 1111 1111"
        card.securityCode = "123"
        let draft = ItemDetail(category: ItemDetail.categoryCreditCard, title: "Visa", card: card)
        guard let savedItem = await model.saveItem(vaultID: "personal", item: draft) else { return XCTFail("card not saved") }
        XCTAssertTrue(savedItem.isCreditCard)
        XCTAssertEqual(savedItem.card?.securityCode, "123")
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.card?.summary, "Visa •••• 1111")
        XCTAssertEqual(reference?.item.card?.number, "")
        XCTAssertEqual(reference?.item.card?.securityCode, "")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.creditCards.includes($0.item) }.map(\.item.title), ["Visa"])
        XCTAssertFalse(model.listAllItemReference().contains { ItemCategoryDestination.logins.includes($0.item) && $0.item.id == savedItem.id })
    }

    func testCardTypeIsFilledInFromTheNumberAndAChosenOneIsKept() async {
        let model = await createUnlockedModel()
        var card = ItemCard()
        card.number = "4111 1111 1111 1111"
        guard let filled = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categoryCreditCard, title: "Visa", card: card)) else { return XCTFail("card not saved") }
        XCTAssertEqual(filled.card?.type, "Visa")
        card.type = "Troy"
        guard let chosen = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categoryCreditCard, title: "Troy", card: card)) else { return XCTFail("card not saved") }
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == chosen.id }
        XCTAssertEqual(reference?.item.card?.summary, "Troy •••• 1111")
    }

    func testSaveIdentityKeepsOnlyTheNameAndEmailInTheList() async {
        let model = await createUnlockedModel()
        var identity = ItemIdentity()
        identity.firstName = "Alice"
        identity.lastName = "Example"
        identity.email = "alice@example.com"
        identity.passportNumber = "U1234567"
        identity.address = "1 Main St"
        guard let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categoryIdentity, title: "Passport", identity: identity)) else {
            return XCTFail("identity not saved")
        }
        XCTAssertTrue(savedItem.isIdentity)
        XCTAssertEqual(savedItem.identity?.passportNumber, "U1234567")
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.identity?.summary, "Alice Example")
        XCTAssertEqual(reference?.item.identity?.email, "alice@example.com")
        XCTAssertEqual(reference?.item.identity?.passportNumber, "")
        XCTAssertEqual(reference?.item.identity?.address, "")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.identities.includes($0.item) }.map(\.item.title), ["Passport"])
        XCTAssertFalse(model.listAllItemReference().contains { ItemCategoryDestination.logins.includes($0.item) && $0.item.id == savedItem.id })
    }

    func testSaveAPICredentialKeepsOnlyTheServiceKeyIDAndHostInTheList() async {
        let model = await createUnlockedModel()
        var credential = ItemAPICredential()
        credential.service = "Stripe"
        credential.keyID = "key_1"
        credential.secret = "sk_live_123"
        credential.host = "api.stripe.com"
        credential.expiry = "2031-01-01"
        guard let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categoryAPICredential, title: "Billing key", apiCredential: credential)) else {
            return XCTFail("API credential not saved")
        }
        XCTAssertTrue(savedItem.isAPICredential)
        XCTAssertEqual(savedItem.apiCredential?.secret, "sk_live_123")
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.apiCredential?.summary, "Stripe · api.stripe.com")
        XCTAssertEqual(reference?.item.apiCredential?.secret, "")
        XCTAssertEqual(reference?.item.apiCredential?.keyID, "key_1")
        XCTAssertEqual(reference?.item.apiCredential?.expiry, "")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.apiCredentials.includes($0.item) }.map(\.item.title), ["Billing key"])
        XCTAssertFalse(model.listAllItemReference().contains { ItemCategoryDestination.logins.includes($0.item) && $0.item.id == savedItem.id })
    }

    func testSaveServerKeepsItsLoginDataAndOnlyHostAndPortInTheList() async {
        let model = await createUnlockedModel()
        let server = ItemServer(host: "build.example.com", port: "2222", sshKeyID: "key-1")
        let item = ItemDetail(category: ItemDetail.categoryServer, title: "Build server", username: "deploy", allURL: ["https://console.example.com"], password: "server-password", server: server)
        guard let savedItem = await model.saveItem(vaultID: "personal", item: item) else {
            return XCTFail("server not saved")
        }
        XCTAssertTrue(savedItem.isServer)
        XCTAssertTrue(savedItem.offersLoginFill)
        XCTAssertEqual(savedItem.server, server)
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.server, ItemServer(host: "build.example.com", port: "2222"))
        XCTAssertEqual(reference?.item.server?.summary(username: reference?.item.username ?? ""), "build.example.com:2222 · deploy")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.servers.includes($0.item) }.map(\.item.title), ["Build server"])
    }

    func testSaveDatabaseKeepsItsLoginDataAndNoConnectionStringInTheList() async {
        let model = await createUnlockedModel()
        let database = ItemDatabase(engine: "postgresql", engineName: "PostgreSQL", host: "db.example.com", port: "5432", name: "app", options: "sslmode=require", connectionString: "postgresql://app:db-password@db.example.com:5432/app?sslmode=require")
        let item = ItemDetail(category: ItemDetail.categoryDatabase, title: "Prod db", username: "app", password: "db-password", database: database)
        guard let savedItem = await model.saveItem(vaultID: "personal", item: item) else {
            return XCTFail("database not saved")
        }
        XCTAssertTrue(savedItem.isDatabase)
        XCTAssertFalse(savedItem.offersLoginFill)
        XCTAssertEqual(savedItem.database, database)
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.database, ItemDatabase(engine: "postgresql", engineName: "PostgreSQL", host: "db.example.com", port: "5432", name: "app"))
        XCTAssertEqual(reference?.item.database?.summary, "PostgreSQL · db.example.com:5432/app")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.databases.includes($0.item) }.map(\.item.title), ["Prod db"])
        XCTAssertFalse(model.listAllItemReference().contains { ItemCategoryDestination.logins.includes($0.item) && $0.item.id == savedItem.id })
    }

    func testDatabaseDecodesFromTheBackend() throws {
        let json = #"{"category":"database","title":"Cache","username":"","database":{"engine":"redis","engineName":"Redis","host":"2001:db8::1","port":"6379","name":"","options":"","connectionString":"redis://[2001:db8::1]:6379","portWarning":""}}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertTrue(item.isDatabase)
        XCTAssertEqual(item.database?.connectionString, "redis://[2001:db8::1]:6379")
        XCTAssertEqual(item.database?.summary, "Redis · [2001:db8::1]:6379")
        XCTAssertEqual(ItemDatabase().summary, "Database")
        XCTAssertEqual(ItemDatabase(engineName: "DB2", name: "app").summary, "DB2 · app")
        XCTAssertTrue(DatabaseEngine.isKnown("sqlserver"))
        XCTAssertFalse(DatabaseEngine.isKnown("db2"))
        let encoded = try XCTUnwrap(String(data: JSONEncoder().encode(item), encoding: .utf8))
        XCTAssertTrue(encoded.contains(#""database""#) && encoded.contains(#""engine":"redis""#), encoded)
    }

    func testServerDecodesFromTheBackendAndWarnsAboutItsPort() throws {
        let json = #"{"category":"server","title":"Database","username":"","server":{"host":"2001:db8::1","port":"5432","sshKeyId":"key-1","sshKeyTitle":"Deploy key","sshKeyFingerprint":"SHA256:abc","portWarning":""}}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertTrue(item.isServer)
        XCTAssertEqual(item.server?.sshKeyTitle, "Deploy key")
        XCTAssertEqual(item.server?.address, "[2001:db8::1]:5432")
        XCTAssertEqual(item.server?.summary(username: ""), "[2001:db8::1]:5432")
        XCTAssertEqual(ItemServer().summary(username: ""), "Server")
        let encoded = try XCTUnwrap(String(data: JSONEncoder().encode(item), encoding: .utf8))
        XCTAssertTrue(encoded.contains(#""sshKeyId":"key-1""#) && encoded.contains(#""server""#), encoded)
        for (port, warning) in ["": "", "22": "", "65535": "", "0": ItemServer.portWarningText, "70000": ItemServer.portWarningText, "ssh": ItemServer.portWarningText, "２２": ItemServer.portWarningText] {
            XCTAssertEqual(ItemServer.describePort(port), warning, port)
        }
    }

    func testSaveSoftwareLicenseKeepsOnlyItsSummaryInTheList() async {
        let model = await createUnlockedModel()
        let license = ItemSoftwareLicense(product: "Sketch", version: "100", licenseKey: "TEST-LICENSE-KEY", licensedTo: "Alice Example", email: "alice@example.com", purchaseDate: "2024-03-09", downloadURL: "https://www.sketch.com/download")
        guard let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categorySoftwareLicense, title: "Sketch", softwareLicense: license)) else {
            return XCTFail("software license not saved")
        }
        XCTAssertTrue(savedItem.isSoftwareLicense)
        XCTAssertEqual(savedItem.softwareLicense, license)
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.softwareLicense, ItemSoftwareLicense(product: "Sketch", version: "100", licensedTo: "Alice Example", email: "alice@example.com"))
        XCTAssertEqual(reference?.item.softwareLicense?.summary, "Version 100 · Alice Example")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.softwareLicenses.includes($0.item) }.map(\.item.title), ["Sketch"])
        XCTAssertFalse(model.listAllItemReference().contains { ItemCategoryDestination.logins.includes($0.item) && $0.item.id == savedItem.id })
    }

    func testSoftwareLicenseDecodesFromTheBackendAndSummarizes() throws {
        let json = #"{"category":"software-license","title":"Nova","softwareLicense":{"product":"Nova","version":"v11","licenseKey":"K","licensedTo":"","email":"bob@example.com","purchaseDate":"2024-03-09","downloadUrl":"https://nova.app"}}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertTrue(item.isSoftwareLicense)
        XCTAssertEqual(item.softwareLicense?.downloadURL, "https://nova.app")
        XCTAssertEqual(item.softwareLicense?.summary, "v11 · bob@example.com")
        XCTAssertEqual(ItemSoftwareLicense(product: "Nova").summary, "Nova")
        XCTAssertEqual(ItemSoftwareLicense().summary, "Software license")
        let encoded = try XCTUnwrap(String(data: JSONEncoder().encode(item), encoding: .utf8))
        XCTAssertTrue(encoded.contains(#""downloadUrl":"https:\/\/nova.app""#) && encoded.contains(#""softwareLicense""#), encoded)
    }

    func testAPICredentialDecodesFromTheBackendAndSummarizes() throws {
        let json = #"{"category":"api-credential","title":"GitHub token","apiCredential":{"service":"","keyId":"ghp_key","secret":"s","host":"api.github.com","expiry":"2020-01-01","expiryWarning":"This credential has expired"}}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertTrue(item.isAPICredential)
        XCTAssertEqual(item.apiCredential?.keyID, "ghp_key")
        XCTAssertEqual(item.apiCredential?.expiryWarning, "This credential has expired")
        XCTAssertEqual(item.apiCredential?.summary, "api.github.com")
        XCTAssertEqual(ItemAPICredential().summary, "API credential")
        let encoded = try XCTUnwrap(String(data: JSONEncoder().encode(item.apiCredential), encoding: .utf8))
        XCTAssertTrue(encoded.contains(#""keyId":"ghp_key""#), encoded)
    }

    func testBirthDatePickerTakesOnlyDaysThatExist() {
        XCTAssertEqual(CalendarDate.parse("1990-04-12").map(CalendarDate.format), "1990-04-12")
        XCTAssertNotNil(CalendarDate.parse("2024-02-29"))
        XCTAssertNil(CalendarDate.parse("2023-02-31"))
        XCTAssertNil(CalendarDate.parse("2023-02-29"))
        XCTAssertNil(CalendarDate.parse("04/12/1990"))
    }

    func testBirthDateTodayFollowsTheDevicesCalendarDay() throws {
        var losAngeles = Calendar(identifier: .gregorian)
        losAngeles.timeZone = try XCTUnwrap(TimeZone(identifier: "America/Los_Angeles"))
        var tokyo = Calendar(identifier: .gregorian)
        tokyo.timeZone = try XCTUnwrap(TimeZone(identifier: "Asia/Tokyo"))
        let evening = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-15T20:00:00-07:00"))
        let morning = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-16T08:00:00+09:00"))

        XCTAssertEqual(CalendarDate.formatToday(now: evening, calendar: losAngeles), "2026-10-15")
        XCTAssertEqual(CalendarDate.format(CalendarDate.getToday(now: evening, calendar: losAngeles)), "2026-10-15")
        XCTAssertEqual(CalendarDate.formatToday(now: morning, calendar: tokyo), "2026-10-16")
        XCTAssertEqual(CalendarDate.format(CalendarDate.getToday(now: morning, calendar: tokyo)), "2026-10-16")
    }

    func testAPICredentialExpiryWarnsFromTheDayAfterOnTheDevicesCalendar() throws {
        var losAngeles = Calendar(identifier: .gregorian)
        losAngeles.timeZone = try XCTUnwrap(TimeZone(identifier: "America/Los_Angeles"))
        // October 16 in UTC, still October 15 in Los Angeles.
        let evening = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-15T20:00:00-07:00"))

        XCTAssertEqual(ItemAPICredential.describeExpiry("2026-10-14", now: evening, calendar: losAngeles), ItemAPICredential.expiredWarning)
        XCTAssertEqual(ItemAPICredential.describeExpiry("2026-10-15", now: evening, calendar: losAngeles), "")
        XCTAssertEqual(ItemAPICredential.describeExpiry("2031-01-01", now: evening, calendar: losAngeles), "")
        XCTAssertEqual(ItemAPICredential.describeExpiry("never", now: evening, calendar: losAngeles), "")
        XCTAssertEqual(ItemAPICredential.describeExpiry("", now: evening, calendar: losAngeles), "")
        XCTAssertEqual(ItemAPICredential.describeExpiry("2020-02-31", now: evening, calendar: losAngeles), "")
    }

    func testAPICredentialEditorWarningFollowsTheEditedExpiry() throws {
        let now = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-15T12:00:00Z"))
        var utc = Calendar(identifier: .gregorian)
        utc.timeZone = try XCTUnwrap(TimeZone(identifier: "UTC"))
        func describe(_ expiry: String, saved: ItemAPICredential?) -> String {
            ItemAPICredential.describeEditedExpiry(expiry, saved: saved, now: now, calendar: utc)
        }
        var expired = ItemAPICredential()
        expired.expiry = "2020-01-01"
        expired.expiryWarning = ItemAPICredential.expiredWarning
        var current = ItemAPICredential()
        current.expiry = "2031-01-01"

        XCTAssertEqual(describe("2020-01-01", saved: expired), ItemAPICredential.expiredWarning)
        XCTAssertEqual(describe("2031-01-01", saved: expired), "", "a future date clears the saved warning")
        XCTAssertEqual(describe("", saved: expired), "", "removing the expiry clears the saved warning")
        XCTAssertEqual(describe("2026-10-14", saved: current), ItemAPICredential.expiredWarning, "a past date warns before saving")
        XCTAssertEqual(describe("2026-10-14", saved: nil), ItemAPICredential.expiredWarning, "a new credential warns too")

        var imported = ItemAPICredential()
        imported.expiry = "2020/01/01"
        imported.expiryWarning = ItemAPICredential.expiredWarning
        XCTAssertEqual(describe("2020/01/01", saved: imported), ItemAPICredential.expiredWarning)
        XCTAssertEqual(describe("never", saved: imported), "")
    }

    func testSaveAddressKeepsOnlyItsFirstStreetLineCityAndCountryInTheList() async {
        let model = await createUnlockedModel()
        let address = ItemAddress(street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "12345", country: "US")
        guard let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categoryAddress, title: "Home", address: address)) else {
            return XCTFail("address not saved")
        }
        XCTAssertTrue(savedItem.isAddress)
        XCTAssertEqual(savedItem.address, ItemAddress(street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "12345", country: "US"))
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.address, ItemAddress(street: "1 Main St", city: "Springfield", country: "US"))
        XCTAssertEqual(reference?.item.address?.summary, "Springfield, US")
        XCTAssertEqual(model.listAllItemReference().filter { ItemCategoryDestination.addresses.includes($0.item) }.map(\.item.title), ["Home"])
    }

    func testSaveIdentityKeepsItsAddressInTheListForSearch() async {
        let model = await createUnlockedModel()
        var identity = ItemIdentity()
        identity.firstName = "Alice"
        identity.postalAddress = ItemAddress(street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "12345", country: "US")
        guard let savedItem = await model.saveItem(vaultID: "personal", item: ItemDetail(category: ItemDetail.categoryIdentity, title: "Alice", identity: identity)) else {
            return XCTFail("identity not saved")
        }
        let reference = model.listAllItemReference(vaultID: "personal").first { $0.item.id == savedItem.id }
        XCTAssertEqual(reference?.item.identity?.postalAddress, ItemAddress(street: "1 Main St", city: "Springfield", country: "US"))
        XCTAssertEqual(reference?.item.identity?.summary, "Alice")
    }

    func testIdentityAndCardCarryTheirAddressAndOldOneLineAddress() throws {
        let json = #"{"category":"identity","title":"Alice","identity":{"firstName":"Alice","street":"1 Main St","city":"Springfield","postalCode":"12345","country":"US","address":"1 Main St, Springfield"}}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertEqual(item.identity?.postalAddress, ItemAddress(street: "1 Main St", city: "Springfield", postalCode: "12345", country: "US"))
        XCTAssertEqual(item.identity?.address, "1 Main St, Springfield")
        let encoded = try XCTUnwrap(String(data: JSONEncoder().encode(item.identity), encoding: .utf8))
        XCTAssertTrue(encoded.contains(#""postalCode":"12345""#) && encoded.contains(#""address":"1 Main St, Springfield""#), encoded)

        var card = ItemCard()
        card.postalAddress = ItemAddress(street: "1 Main St", city: "Springfield")
        XCTAssertEqual(card.street, "1 Main St")
        XCTAssertEqual(card.city, "Springfield")
        XCTAssertEqual(ItemAddress(street: "1 Main St\nApt 4").summary, "1 Main St")
        XCTAssertEqual(ItemAddress().summary, "Address")
    }

    func testIdentityDecodesFromTheBackendAndSummarizes() throws {
        let json = #"{"category":"identity","title":"Work","identity":{"firstName":"","lastName":"","email":"alice@example.com","nationalIdNumber":"N1"}}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertTrue(item.isIdentity)
        XCTAssertEqual(item.identity?.nationalIDNumber, "N1")
        XCTAssertEqual(item.identity?.summary, "alice@example.com")
        XCTAssertEqual(ItemIdentity().summary, "Identity")
        let encoded = try XCTUnwrap(String(data: JSONEncoder().encode(item.identity), encoding: .utf8))
        XCTAssertTrue(encoded.contains(#""nationalIdNumber":"N1""#), encoded)
    }

    func testDescribeCardComesFromTheBackend() async {
        let model = await createUnlockedModel()
        let description = await model.describeCard(number: "4111 1111 1111 1112", expiry: "")
        XCTAssertEqual(description?.brand, "Visa")
        XCTAssertEqual(description?.numberWarning, "This doesn't look like a valid card number")
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

    func testConvertToSecureNoteTakesALoginWithOnlyCustomFields() async {
        let model = await createUnlockedModel()
        let draft = ItemDetail(title: "Router", allField: [ItemField(name: "PIN", value: "0000", concealed: true)], canConvertToSecureNote: true)
        guard let login = await model.saveItem(vaultID: "personal", item: draft) else { return XCTFail("login not saved") }
        let converted = await model.convertToSecureNote(vaultID: "personal", itemID: login.id)
        XCTAssertEqual(converted?.isSecureNote, true)
        XCTAssertEqual(converted?.allField.count, 1)
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
        let model = await createModel()
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

    func testSharingConfirmationBindsReviewedRecipient() async {
        let model = await createUnlockedModel()
        guard let vault = await model.createVault(name: "Team") else { return XCTFail("vault missing") }
        let invitation = SentShareInvitation(id: "accepted", vaultID: vault.id, vaultName: "Team", email: "new@example.com", role: "reader", state: "accepted", expiresAt: "2027-01-22T08:00:00Z", requiresConfirmation: true, recipientID: "new-account", securityCode: "3333 4444", reviewToken: "reviewed-identity")
        backend.shareInvitations = ShareInvitations(allSent: [invitation])
        var changed = invitation
        changed.reviewToken = "other-identity"
        let changedResult = await model.confirmShareRecipient(changed)
        XCTAssertFalse(changedResult)
        XCTAssertEqual(backend.shareInvitations.allSent.count, 1)
        let confirmed = await model.confirmShareRecipient(invitation)
        XCTAssertTrue(confirmed)
        XCTAssertTrue(backend.shareInvitations.allSent.isEmpty)
    }

    func testReceivedShareInvitationsAreAcceptedOrDeclined() async {
        backend.seedSignedIn()
        backend.shareInvitations = ShareInvitations(allReceived: [
            ReceivedShareInvitation(id: "from-carol", vaultID: "carol-vault", role: Member.vaultRoleEditor, invitedByName: "Carol", invitedByEmail: "carol@example.com", expiresAt: "2027-01-22T08:00:00Z"),
            ReceivedShareInvitation(id: "from-dave", vaultID: "dave-vault", role: Member.vaultRoleReader, invitedByName: "", invitedByEmail: "dave@example.com", expiresAt: "2027-01-22T08:00:00Z"),
        ])
        let model = await createModel()
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
        let model = await createModel()
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
        let model = await createModel()
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
        let model = await createModel()
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

    func testARemoteChangeDuringAnotherRefreshListsAgain() async {
        let model = await createUnlockedModel()
        await backend.finishRefresh(of: model) {
            backend.allVault.append(VaultSummary(id: "shared-with-me", name: "Deploy keys", type: "shared", role: Member.vaultRoleReader, itemCount: 0))
            model.handleRemoteChange()
        }
        XCTAssertNotNil(model.findVault(vaultID: "shared-with-me"), "the refresh listed the vaults before the change and nothing listed them again")
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
        await model.start { recordingBackend }?.value
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

    func testUpdateRequiredEventShowsTheBlockingScreen() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = await createModel()
        XCTAssertEqual(model.screen, .locked)
        let requirement = UpdateRequirement(currentVersion: "", minimumVersion: "0.2.0", updateURL: "https://tryhisar.com/download")
        backend.status.updateRequired = requirement
        model.handleUpdateRequired()
        XCTAssertEqual(model.screen, .updateRequired(requirement))
    }

    func testInviteLinkPrefillsCreateAccountWhenSignedOut() async {
        let model = await createModel()
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
