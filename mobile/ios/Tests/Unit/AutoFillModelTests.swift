import AuthenticationServices
import XCTest
@testable import Hisar

private final class FakeIdentityStore: CredentialIdentitySaving {
    var allSaved: [CredentialIdentityRecord] = []
    var allRemoved: [CredentialIdentityRecord] = []
    var onRemove: (() -> Void)?

    func saveIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        allSaved += allIdentity
    }

    func removeIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        allRemoved += allIdentity
        onRemove?()
    }
}

private final class FakeLocalAuthenticator {
    var isSucceeding = true
    var allReason: [String] = []

    func authenticate(reason: String) -> Bool {
        allReason.append(reason)
        return isSucceeding
    }
}

private final class LockingAutoFillBackend: FakeBackend {
    var locksCredentialRead = false
    var locksCodeRead = false

    override func getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String) throws -> LoginCredential {
        let credential = try super.getLoginForService(vaultID: vaultID, itemID: itemID, serviceIdentifier: serviceIdentifier)
        if locksCredentialRead { _ = lock() }
        return credential
    }

    override func getOneTimeCode(vaultID: String, itemID: String) throws -> OneTimeCode {
        let code = try super.getOneTimeCode(vaultID: vaultID, itemID: itemID)
        if locksCodeRead { _ = lock() }
        return code
    }
}

@MainActor
final class AutoFillModelTests: XCTestCase {
    private var backend: FakeBackend!
    private var identityStore: FakeIdentityStore!
    private var localAuthenticator: FakeLocalAuthenticator!
    private var allCompletedCredential: [LoginCredential] = []
    private var allCompletedCode: [String] = []
    private var allCopiedCode: [String] = []
    private var allCancelCode: [ASExtensionError.Code] = []
    private var allAssertion: [PasskeyAssertionCredential] = []
    private var allRegistration: [PasskeyRegistrationCredential] = []
    private var rejectedRegistrationCount = 0

    override func setUp() async throws {
        backend = FakeBackend()
        identityStore = FakeIdentityStore()
        localAuthenticator = FakeLocalAuthenticator()
        backend.verifyPasskey = localAuthenticator.authenticate
        allCompletedCredential = []
        allCompletedCode = []
        allCopiedCode = []
        allCancelCode = []
        allAssertion = []
        allRegistration = []
        rejectedRegistrationCount = 0
    }

    private func createModel(source: VaultBackend? = nil) -> AutoFillModel {
        AutoFillModel(
            createBackend: { source ?? self.backend },
            completeRequest: { [unowned self] in allCompletedCredential.append($0) },
            completeOneTimeCodeRequest: { [unowned self] in allCompletedCode.append($0) },
            copyOneTimeCode: { [unowned self] in allCopiedCode.append($0) },
            cancelRequest: { [unowned self] in allCancelCode.append($0) },
            passkeyCompletion: AutoFillModel.PasskeyCompletion(
                completeAssertion: { [unowned self] in allAssertion.append($0) },
                completeRegistration: { [unowned self] in allRegistration.append($0) },
                rejectExcludedRegistration: { [unowned self] in rejectedRegistrationCount += 1 }
            ),
            credentialIdentityStore: identityStore
        )
    }

    func testSignedOutShowsMessage() async {
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        XCTAssertEqual(model.screen, .signedOut)
    }

    func testFollowsTheAppTheme() async {
        backend.theme = VaultTheme(appearance: .light, color: .green)
        let model = createModel()
        XCTAssertEqual(model.theme, .standard)
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        XCTAssertEqual(model.theme, VaultTheme(appearance: .light, color: .green))
    }

    func testLockedThenUnlockListsMatchingLogins() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["https://github.com/login", "github.com"])
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.serviceName, "github.com")
        let isUnlocked = await model.unlock(password: FakeBackend.password)
        XCTAssertTrue(isUnlocked)
        XCTAssertEqual(model.screen, .list)
        XCTAssertEqual(model.allMatchingLogin.map(\.title), ["GitHub"], "the same login matched twice is listed once")
    }

    func testOpenListHidesItsLoginsWhenHisarLocks() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"], passkeyRequest: PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash))
        model.searchText = "bank"
        await model.loadAllOtherLogin()
        _ = backend.lock()
        model.handleLocked()
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(model.allMatchingLogin.isEmpty)
        XCTAssertTrue(model.allOtherLogin.isEmpty)
        XCTAssertTrue(model.allMatchingPasskey.isEmpty)
        XCTAssertEqual(model.searchText, "")
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(model.allMatchingLogin.map(\.title), ["GitHub"])
        XCTAssertEqual(model.allMatchingPasskey.count, 1)
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "alice", userHandle: Data([5]), clientDataHash: clientDataHash))
        XCTAssertNotNil(model.passkeyRegistrationOptions)
        _ = backend.lock()
        model.refreshStatus()
        XCTAssertEqual(model.screen, .locked)
        XCTAssertNil(model.passkeyRegistrationOptions)
        XCTAssertNil(model.selectedRegistrationLoginID)
    }

    func testCredentialReadBeforeLockCannotCompleteAfterLock() async {
        let source = LockingAutoFillBackend()
        source.seedSignedIn()
        source.locksCredentialRead = true
        let model = createModel(source: source)
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        await model.select(model.allMatchingLogin[0])
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(allCompletedCredential.isEmpty)
        XCTAssertTrue(allCopiedCode.isEmpty)
    }

    func testPasswordDoesNotCompleteWhenItsAutomaticCodeReadCrossesLock() async {
        let source = LockingAutoFillBackend()
        source.seedSignedIn()
        source.locksCodeRead = true
        let model = createModel(source: source)
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        await model.select(model.allMatchingLogin[0])
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(allCopiedCode.isEmpty)
        XCTAssertTrue(allCompletedCredential.isEmpty)
    }

    func testConfirmedRegistrationCommittedBeforeLock() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "shop.example", userName: "buyer", userHandle: Data([5]), clientDataHash: clientDataHash))
        identityStore.onRemove = { [unowned self] in
            _ = backend.lock()
            model.handleLocked()
        }
        await model.savePasskey()
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(allRegistration.count, 1)
        XCTAssertEqual(backend.allPasskey.last?.relyingPartyID, "shop.example")
    }

    func testWrongPasswordStaysLocked() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        let isUnlocked = await model.unlock(password: "nope")
        XCTAssertFalse(isUnlocked)
        XCTAssertEqual(model.screen, .locked)
        XCTAssertNotNil(model.errorMessage)
    }

    func testSelectingMatchingLoginCompletesWithPassword() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["www.netflix.com"])
        XCTAssertEqual(model.screen, .list)
        guard let login = model.allMatchingLogin.first else { return XCTFail("no match") }
        await model.select(login)
        XCTAssertEqual(allCompletedCredential, [LoginCredential(username: "family@example.com", password: "netflix-password")])
        XCTAssertTrue(backend.allCallName.contains("getLoginForService"))
        XCTAssertEqual(allCopiedCode, [], "a login without a one-time password copies nothing")
    }

    func testSearchFindsOtherLoginsAndSelectsThem() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        model.searchText = "bank"
        await model.loadAllOtherLogin()
        XCTAssertTrue(model.isOtherLoginLoaded)
        XCTAssertTrue(model.allVisibleMatchingLogin.isEmpty)
        XCTAssertEqual(model.allVisibleOtherLogin.map(\.title), ["Bank"])
        await model.select(model.allVisibleOtherLogin[0])
        XCTAssertTrue(allCompletedCredential.isEmpty)
        XCTAssertEqual(model.unmatchedLoginConfirmation?.login.title, "Bank")
        XCTAssertTrue(model.unmatchedFillTitle.contains("github.com"))
        XCTAssertFalse(model.unmatchedLoginConfirmation?.allWebsite.isEmpty ?? true)
        model.dismissUnmatchedLogin()
        XCTAssertTrue(allCompletedCredential.isEmpty)
        await model.select(model.allVisibleOtherLogin[0])
        let savedWebsites = try? backend.getItem(vaultID: "personal", itemID: "bank").allURL
        await model.fillUnmatchedLogin()
        XCTAssertEqual(try? backend.getItem(vaultID: "personal", itemID: "bank").allURL, savedWebsites)
        XCTAssertEqual(allCompletedCredential, [LoginCredential(username: "alice@example.com", password: "bank-password")])
        await model.prepareCredentialList(allServiceIdentifier: [])
        model.searchText = "bank"
        await model.loadAllOtherLogin()
        if let index = backend.allItemByVaultID["personal"]?.firstIndex(where: { $0.id == "bank" }) {
            backend.allItemByVaultID["personal"]?[index].allURL = []
        }
        await model.select(model.allVisibleOtherLogin[0])
        XCTAssertTrue(model.unmatchedFillTitle.contains("without a website"))
        XCTAssertTrue(model.unmatchedFillMessage.contains("This request has no website."))
        XCTAssertTrue(model.unmatchedFillMessage.contains("No website saved"))
        let staleConfirmation = model.unmatchedLoginConfirmation
        _ = backend.lock()
        model.handleLocked()
        await model.unlock(password: FakeBackend.password)
        await model.fillUnmatchedLogin(staleConfirmation)
        XCTAssertEqual(allCompletedCredential.count, 1, "a confirmation cannot release a login across a lock")
    }

    func testSearchHidesMatchingLoginsFromOtherSection() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        model.searchText = "git"
        await model.loadAllOtherLogin()
        XCTAssertEqual(model.allVisibleMatchingLogin.map(\.title), ["GitHub"])
        XCTAssertTrue(model.allVisibleOtherLogin.isEmpty)
    }

    func testQuickTypeRequestUnlocksThenCompletes() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareInterfaceToProvideCredential(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(allCompletedCredential.isEmpty)
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(allCompletedCredential, [LoginCredential(username: "alice", password: "github-password", hasOneTimePassword: true)])
        XCTAssertEqual(allCopiedCode, [FakeBackend.oneTimeCode.code], "the code is on the pasteboard for the site's next step")
    }

    func testFillingALoginWithACodeCopiesTheCodeFirst() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        await model.select(model.allMatchingLogin[0])
        XCTAssertEqual(allCopiedCode, [FakeBackend.oneTimeCode.code])
        XCTAssertEqual(allCompletedCredential.map(\.password), ["github-password"])
        XCTAssertTrue(backend.allCallName.contains("getOneTimeCodeForService"))
    }

    func testOneTimeCodeListOffersOnlyLoginsWithACode() async {
        backend.seedSignedIn()
        backend.allItemByVaultID["personal"]?.append(ItemDetail(id: "github-2", category: "login", title: "GitHub second", username: "bob", allURL: ["github.com"], password: "x"))
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"], kind: .oneTimeCode)
        XCTAssertEqual(model.requestKind, .oneTimeCode)
        XCTAssertEqual(model.allMatchingLogin.count, 2)
        XCTAssertEqual(model.allVisibleMatchingLogin.map(\.title), ["GitHub"])
        await model.select(model.allVisibleMatchingLogin[0])
        XCTAssertEqual(allCompletedCode, [FakeBackend.oneTimeCode.code])
        XCTAssertTrue(allCompletedCredential.isEmpty, "a one-time code request never hands out the password")
        XCTAssertFalse(backend.allCallName.contains("getLoginForService"))

        await model.prepareCredentialList(allServiceIdentifier: ["www.netflix.com"], kind: .oneTimeCode)
        XCTAssertTrue(model.allVisibleMatchingLogin.isEmpty)
        model.searchText = "hub"
        await model.loadAllOtherLogin()
        XCTAssertEqual(model.allVisibleOtherLogin.map(\.title), ["GitHub"])
        await model.select(model.allVisibleOtherLogin[0])
        XCTAssertTrue(model.unmatchedFillTitle.contains("one-time code"))
        XCTAssertTrue(model.unmatchedFillTitle.contains("netflix.com"))
        await model.fillUnmatchedLogin()
        XCTAssertEqual(allCompletedCode.count, 2)
        XCTAssertTrue(allCompletedCredential.isEmpty)
    }

    func testOneTimeCodeQuickTypeRequestUnlocksThenCompletes() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareInterfaceToProvideCredential(recordIdentifier: "personal/github", serviceIdentifier: "github.com", kind: .oneTimeCode)
        XCTAssertEqual(model.screen, .locked)
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(allCompletedCode, [FakeBackend.oneTimeCode.code])
        XCTAssertTrue(allCompletedCredential.isEmpty)
    }

    func testOneTimeCodeWithoutInteractionNeedsAnUnlockedProcessAndAMatchingSite() async {
        backend.seedSignedIn()
        let model = createModel()
        let beforeStart = await model.provideOneTimeCodeWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertNil(beforeStart)
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"], kind: .oneTimeCode)
        let code = await model.provideOneTimeCodeWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertEqual(code, FakeBackend.oneTimeCode.code)
        let otherSite = await model.provideOneTimeCodeWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "evil.example")
        XCTAssertNil(otherSite)
        let withoutCode = await model.provideOneTimeCodeWithoutUserInteraction(recordIdentifier: "personal/bank", serviceIdentifier: "bank.example.com")
        XCTAssertNil(withoutCode)
    }

    func testWithoutInteractionNeedsAnUnlockedProcess() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        let lockedCredential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertNil(lockedCredential, "no backend exists yet in a fresh extension process")
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        await model.unlock(password: FakeBackend.password)
        let credential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "github.com")
        XCTAssertEqual(credential?.password, "github-password")
        XCTAssertEqual(allCopiedCode, [FakeBackend.oneTimeCode.code])
        let mismatchedCredential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: "personal/github", serviceIdentifier: "evil.example")
        XCTAssertNil(mismatchedCredential)
    }

    func testCancel() {
        let model = createModel()
        model.cancel()
        XCTAssertEqual(allCancelCode, [.userCanceled])
    }

    func testBiometricUnlock() async {
        backend.seedSignedIn()
        _ = try? backend.enableBiometricUnlock(password: FakeBackend.password)
        _ = backend.lock()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        XCTAssertTrue(model.status.biometricReady)
        await model.unlockWithBiometric()
        XCTAssertEqual(model.screen, .list)
        XCTAssertEqual(model.allMatchingLogin.count, 1)
    }

    private let clientDataHash = Data((0..<32).map { UInt8($0) })

    func testPasskeyListShowsPasskeysForTheRelyingPartyAndSigns() async {
        backend.seedSignedIn()
        let model = createModel()
        let request = PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash)
        await model.prepareCredentialList(allServiceIdentifier: ["https://github.com"], passkeyRequest: request)
        XCTAssertEqual(model.allVisibleMatchingPasskey.map(\.title), ["GitHub"])
        XCTAssertEqual(model.allMatchingLogin.map(\.title), ["GitHub"], "passwords are still offered")
        await model.selectPasskey(model.allVisibleMatchingPasskey[0])
        XCTAssertEqual(allAssertion.count, 1)
        XCTAssertEqual(allAssertion.first?.relyingPartyID, "github.com")
        XCTAssertEqual(allAssertion.first?.clientDataHash, clientDataHash)
        XCTAssertEqual(allAssertion.first?.credentialID, Data([1, 1, 1]))
        XCTAssertEqual(allAssertion.first?.userHandle, Data("alice-handle".utf8))
        XCTAssertEqual(allAssertion.first?.signature, Data("signature over ".utf8) + clientDataHash)
        XCTAssertTrue(allCompletedCredential.isEmpty)
    }

    func testPasskeyListAppliesAllowedCredentialsAndOtherRelyingParties() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["https://github.com"], passkeyRequest: PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash, allAllowedCredentialID: [Data([9])]))
        XCTAssertTrue(model.allVisibleMatchingPasskey.isEmpty)
        await model.prepareCredentialList(allServiceIdentifier: ["https://gist.github.com"], passkeyRequest: PasskeyAssertionRequest(relyingPartyID: "gist.github.com", clientDataHash: clientDataHash))
        XCTAssertTrue(model.allVisibleMatchingPasskey.isEmpty, "only passkeys whose RP ID equals the request's")
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        XCTAssertNil(model.passkeyAssertionRequest)
        XCTAssertTrue(model.allMatchingPasskey.isEmpty, "a password-only request lists no passkeys")
    }

    func testLockedPasskeyListUnlocksThenLists() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: [], passkeyRequest: PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash))
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.serviceName, "github.com")
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(model.screen, .list)
        XCTAssertEqual(model.allVisibleMatchingPasskey.count, 1)
        model.searchText = "nobody"
        XCTAssertTrue(model.allVisibleMatchingPasskey.isEmpty)
    }

    func testChosenPasskeyUnlocksThenSigns() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        let request = PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash)
        await model.prepareInterfaceToProvidePasskey(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertEqual(model.screen, .locked)
        XCTAssertTrue(allAssertion.isEmpty)
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(allAssertion.map(\.credentialID), [Data([1, 1, 1])])
        XCTAssertEqual(localAuthenticator.allReason, ["Use your passkey for github.com"], "the final concrete ceremony requires fresh verification")
    }

    func testChosenPasskeyInAnUnlockedExtensionAsksTheDeviceOwner() async {
        backend.seedSignedIn()
        let model = createModel()
        let request = PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash)
        await model.prepareInterfaceToProvidePasskey(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertEqual(localAuthenticator.allReason, ["Use your passkey for github.com"])
        XCTAssertEqual(allAssertion.map(\.credentialID), [Data([1, 1, 1])])
    }

    func testDeclinedDeviceOwnerCheckCancels() async {
        backend.seedSignedIn()
        localAuthenticator.isSucceeding = false
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: [], passkeyRequest: PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash))
        guard let passkey = model.allMatchingPasskey.first else { return XCTFail("no passkey listed") }
        await model.selectPasskey(passkey)
        XCTAssertTrue(allAssertion.isEmpty)
        XCTAssertEqual(allCancelCode, [.userCanceled])
    }

    func testDiscouragedUserVerificationSkipsTheDeviceOwnerCheck() async {
        backend.seedSignedIn()
        localAuthenticator.isSucceeding = false
        let model = createModel()
        let request = PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash, userVerification: "discouraged")
        await model.prepareInterfaceToProvidePasskey(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertTrue(localAuthenticator.allReason.isEmpty)
        XCTAssertEqual(allAssertion.count, 1)
    }

    func testChosenPasskeyThatIsGoneCancels() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareInterfaceToProvidePasskey(request: PasskeyAssertionRequest(relyingPartyID: "evil.example", clientDataHash: clientDataHash), credentialID: Data([1, 1, 1]))
        XCTAssertTrue(allAssertion.isEmpty)
        XCTAssertEqual(allCancelCode, [.credentialIdentityNotFound])
        XCTAssertFalse(backend.allCallName.contains("assertPasskey"))
    }

    func testPasskeyWithoutInteractionNeedsAnUnlockedProcess() async {
        backend.seedSignedIn()
        let model = createModel()
        let request = PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash, userVerification: "discouraged")
        let beforeStart = await model.providePasskeyWithoutUserInteraction(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertNil(beforeStart, "no backend exists yet in a fresh extension process")
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        let credential = await model.providePasskeyWithoutUserInteraction(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertEqual(credential?.userHandle, Data("alice-handle".utf8))
        _ = backend.lock()
        let whileLocked = await model.providePasskeyWithoutUserInteraction(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertNil(whileLocked)
    }

    func testPasskeyWithoutInteractionNeedsDiscouragedUserVerification() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareCredentialList(allServiceIdentifier: ["github.com"])
        let request = PasskeyAssertionRequest(relyingPartyID: "github.com", clientDataHash: clientDataHash)
        let credential = await model.providePasskeyWithoutUserInteraction(request: request, credentialID: Data([1, 1, 1]))
        XCTAssertNil(credential, "the controller then cancels with userInteractionRequired")
        XCTAssertFalse(backend.allCallName.contains("assertPasskey"))
    }

    func testPasskeyRegistrationPreselectsTheMatchingLoginAndSaves() async {
        backend.seedSignedIn()
        backend.allItemByVaultID["personal"]?.append(ItemDetail(id: "github-bob", category: "login", title: "GitHub Bob", username: "bob", allURL: ["github.com"]))
        let model = createModel()
        let request = PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "Alice", userHandle: Data("alice-handle".utf8), clientDataHash: clientDataHash, allSupportedAlgorithm: [-8, -7])
        await model.prepareInterfaceForPasskeyRegistration(request)
        XCTAssertEqual(model.screen, .passkeyRegistration)
        XCTAssertEqual(model.passkeyRegistrationOptions?.allLogin.map(\.title), ["GitHub", "GitHub Bob"])
        XCTAssertEqual(model.selectedRegistrationLoginID, "personal/github", "the login whose username equals the user name")
        await model.savePasskey()
        XCTAssertEqual(localAuthenticator.allReason, ["Create a passkey for github.com"])
        XCTAssertEqual(allRegistration.count, 1)
        XCTAssertEqual(allRegistration.first?.relyingPartyID, "github.com")
        XCTAssertEqual(allRegistration.first?.clientDataHash, clientDataHash)
        XCTAssertEqual(allRegistration.first?.attestationObject, Data("attestation for github.com".utf8))
        let savedItem = try? backend.getItem(vaultID: "personal", itemID: "github")
        XCTAssertEqual(savedItem?.allPasskey.count, 1, "the passkey with the same user handle is replaced")
        XCTAssertEqual(savedItem?.allPasskey.first.flatMap { Base64URL.decode($0.credentialID) }, allRegistration.first?.credentialID)
    }

    func testPasskeyRegistrationUpdatesTheCredentialIdentities() async {
        backend.seedSignedIn()
        let model = createModel()
        let userHandle = Data("alice-handle".utf8)
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "alice", userHandle: userHandle, clientDataHash: clientDataHash))
        await model.savePasskey()
        guard let credentialID = allRegistration.first?.credentialID else { return XCTFail("no passkey created") }
        XCTAssertEqual(identityStore.allSaved, [CredentialIdentityRecord(kind: .passkey, host: "github.com", username: "alice", recordIdentifier: "personal/github", credentialID: credentialID, userHandle: userHandle)])
        XCTAssertEqual(identityStore.allRemoved.map(\.credentialID), [Data([1, 1, 1])], "the passkey Hisar replaced is no longer offered")
        XCTAssertEqual(identityStore.allRemoved.first?.recordIdentifier, "personal/github")
    }

    func testDeclinedDeviceOwnerCheckCancelsRegistration() async {
        backend.seedSignedIn()
        localAuthenticator.isSucceeding = false
        let model = createModel()
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "shop.example", userName: "buyer", userHandle: Data([5]), clientDataHash: clientDataHash))
        await model.savePasskey()
        XCTAssertEqual(allCancelCode, [.userCanceled])
        XCTAssertTrue(identityStore.allSaved.isEmpty)
    }

    func testPasskeyRegistrationToANewLogin() async {
        backend.seedSignedIn(state: VaultStatus.stateLocked)
        let model = createModel()
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "shop.example", userName: "buyer", userHandle: Data([5]), clientDataHash: clientDataHash))
        XCTAssertEqual(model.screen, .locked)
        XCTAssertEqual(model.serviceName, "shop.example")
        await model.unlock(password: FakeBackend.password)
        XCTAssertEqual(model.screen, .passkeyRegistration)
        XCTAssertEqual(model.passkeyRegistrationOptions?.allLogin, [])
        XCTAssertNil(model.selectedRegistrationLoginID, "New login is the only choice")
        await model.savePasskey()
        XCTAssertEqual(localAuthenticator.allReason, ["Create a passkey for shop.example"], "the final concrete ceremony verifies afresh")
        XCTAssertEqual(allRegistration.count, 1)
        XCTAssertTrue(backend.allItemByVaultID["personal"]?.contains { $0.title == "shop.example" && $0.allPasskey.count == 1 } ?? false)
        XCTAssertTrue(identityStore.allRemoved.isEmpty)
    }

    func testExcludedPasskeyRegistrationIsRejected() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "alice", userHandle: Data([5]), clientDataHash: clientDataHash, allExcludedCredentialID: [Data([1, 1, 1])]))
        XCTAssertEqual(model.passkeyRegistrationOptions?.isExcluded, true)
        await model.savePasskey()
        XCTAssertTrue(allRegistration.isEmpty)
        XCTAssertFalse(backend.allCallName.contains("createPasskey"))
        model.rejectExcludedPasskeyRegistration()
        XCTAssertEqual(rejectedRegistrationCount, 1)
    }

    func testPasskeyRegistrationWithoutES256Fails() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "alice", userHandle: Data([5]), clientDataHash: clientDataHash, allSupportedAlgorithm: [-8, -257]))
        XCTAssertEqual(allCancelCode, [.failed])
        XCTAssertFalse(backend.allCallName.contains("preparePasskeyRegistration"))
    }

    func testPasskeyRegistrationErrorIsShown() async {
        backend.seedSignedIn()
        let model = createModel()
        await model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "alice", userHandle: Data([5]), clientDataHash: Data([1])))
        await model.savePasskey()
        XCTAssertTrue(allRegistration.isEmpty)
        XCTAssertEqual(model.errorMessage, "the request is not valid")
        XCTAssertEqual(model.screen, .passkeyRegistration)
        XCTAssertTrue(identityStore.allSaved.isEmpty)
    }

    func testSharedPasskeyDestinationRequiresExplicitSelection() async {
        backend.seedSignedIn()
        backend.allItemByVaultID["family"]?.append(ItemDetail(id: "attacker", category: "login", title: "AAA shared", username: "victim", allURL: ["https://example.com"]))
        let model = createModel()
        let request = PasskeyRegistrationRequest(relyingPartyID: "example.com", userName: "victim", userHandle: Data([9]), clientDataHash: clientDataHash)
        await model.prepareInterfaceForPasskeyRegistration(request)
        XCTAssertNil(model.selectedRegistrationLoginID)
        XCTAssertEqual(model.passkeyRegistrationOptions?.allLogin.first?.isPersonal, false)
        await model.savePasskey()
        XCTAssertEqual(backend.allPasskey.last?.vaultID, "personal")
        await model.prepareInterfaceForPasskeyRegistration(request)
        model.selectedRegistrationLoginID = "family/attacker"
        await model.savePasskey()
        XCTAssertEqual(backend.allPasskey.last?.vaultID, "family")
        XCTAssertEqual(allRegistration.count, 2)
    }
}
