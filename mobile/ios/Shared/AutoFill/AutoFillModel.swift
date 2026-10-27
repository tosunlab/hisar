import AuthenticationServices
import Foundation

@MainActor
final class AutoFillModel: ObservableObject {
    enum Screen: Equatable {
        case starting
        case failed(String)
        case signedOut
        case locked
        case list
        case passkeyRegistration
        case configuration
        case updateRequired
    }

    enum RegistrationDestination: Equatable {
        case idle
        case loading
        case loaded(PasskeyRegistrationOptions)
        case failed
    }

    enum RequestKind: Equatable {
        case password
        case oneTimeCode
    }

    struct PendingCredentialRequest: Equatable {
        var recordIdentifier: String
        var serviceIdentifier: String
        var kind: RequestKind
    }

    struct UnmatchedLoginConfirmation {
        var login: LoginMatch
        var allWebsite: [String]
        var lockGeneration: Int
        var requestEpoch: UInt64
    }

    struct PasskeyCompletion {
        var completeAssertion: (PasskeyAssertionCredential) -> Void
        var completeRegistration: (PasskeyRegistrationCredential) -> Void
        var rejectExcludedRegistration: () -> Void
    }

    @Published private(set) var screen: Screen = .starting
    @Published private(set) var status: VaultStatus = .signedOut
    @Published private(set) var isBusy = false
    @Published var errorMessage: String?
    @Published private(set) var allMatchingLogin: [LoginMatch] = []
    @Published private(set) var allOtherLogin: [LoginMatch] = []
    @Published private(set) var isOtherLoginLoaded = false
    @Published private(set) var unmatchedLoginConfirmation: UnmatchedLoginConfirmation?
    @Published var searchText = "" {
        didSet {
            if !searchText.isEmpty { Task { await loadAllOtherLogin() } }
        }
    }
    @Published private(set) var lockCount = 0
    @Published private(set) var requestKind = RequestKind.password
    @Published private(set) var allMatchingPasskey: [StoredPasskey] = []
    @Published private(set) var passkeyAssertionRequest: PasskeyAssertionRequest?
    @Published private(set) var passkeyRegistrationRequest: PasskeyRegistrationRequest?
    @Published private(set) var registrationDestination: RegistrationDestination = .idle

    var passkeyRegistrationOptions: PasskeyRegistrationOptions? {
        if case .loaded(let options) = registrationDestination { return options }
        return nil
    }
    @Published var selectedRegistrationLoginID: String?
    // The app's theme, read from the shared state in the app group.
    @Published private(set) var theme = VaultTheme.standard

    private(set) var allServiceIdentifier: [String] = []
    private var pendingRequest: PendingCredentialRequest?
    private var pendingPasskeyCredentialID: Data?
    private var requestEpoch: UInt64 = 0
    private var serviceIdentifierByLoginID: [String: String] = [:]
    private var backend: VaultBackend?
    private let createBackend: () throws -> VaultBackend
    private let completeRequest: (LoginCredential) -> Void
    private let completeOneTimeCodeRequest: (String) -> Void
    private let copyOneTimeCode: (String) -> Void
    private let cancelRequest: (ASExtensionError.Code) -> Void
    private let passkeyCompletion: PasskeyCompletion
    private let credentialIdentityStore: CredentialIdentitySaving

    init(
        createBackend: @escaping () throws -> VaultBackend,
        completeRequest: @escaping (LoginCredential) -> Void,
        completeOneTimeCodeRequest: @escaping (String) -> Void,
        copyOneTimeCode: @escaping (String) -> Void,
        cancelRequest: @escaping (ASExtensionError.Code) -> Void,
        passkeyCompletion: PasskeyCompletion,
        credentialIdentityStore: CredentialIdentitySaving = SystemCredentialIdentityStore()
    ) {
        self.createBackend = createBackend
        self.completeRequest = completeRequest
        self.completeOneTimeCodeRequest = completeOneTimeCodeRequest
        self.copyOneTimeCode = copyOneTimeCode
        self.cancelRequest = cancelRequest
        self.passkeyCompletion = passkeyCompletion
        self.credentialIdentityStore = credentialIdentityStore
    }

    var serviceName: String {
        allServiceIdentifier.lazy.compactMap { VaultLogic.extractHost($0) }.first ?? allServiceIdentifier.first
            ?? passkeyAssertionRequest?.relyingPartyID ?? passkeyRegistrationRequest?.relyingPartyID ?? ""
    }

    var allVisibleMatchingPasskey: [StoredPasskey] {
        let allWord = searchText.lowercased().split(whereSeparator: \.isWhitespace).map(String.init)
        return allMatchingPasskey.filter { passkey in
            let searchable = "\(passkey.title) \(passkey.userName) \(passkey.userDisplayName)".lowercased()
            return allWord.allSatisfy { searchable.contains($0) }
        }
    }

    var allVisibleMatchingLogin: [LoginMatch] {
        Self.filterAllLogin(filterForRequest(allMatchingLogin), searchText: searchText)
    }

    var allVisibleOtherLogin: [LoginMatch] {
        guard !searchText.isEmpty else { return [] }
        let allMatchingID = Set(allMatchingLogin.map(\.id))
        return Self.filterAllLogin(filterForRequest(allOtherLogin).filter { !allMatchingID.contains($0.id) }, searchText: searchText)
    }

    private func filterForRequest(_ allLogin: [LoginMatch]) -> [LoginMatch] {
        requestKind == .oneTimeCode ? allLogin.filter(\.hasOneTimePassword) : allLogin
    }

    static func filterAllLogin(_ allLogin: [LoginMatch], searchText: String) -> [LoginMatch] {
        let allWord = searchText.lowercased().split(whereSeparator: \.isWhitespace).map(String.init)
        guard !allWord.isEmpty else { return allLogin }
        return allLogin.filter { login in
            let searchable = "\(login.title) \(login.username) \(login.vaultName)".lowercased()
            return allWord.allSatisfy { searchable.contains($0) }
        }
    }

    private func prepareBackend() -> VaultBackend? {
        if let backend { return backend }
        do {
            let backend = try createBackend()
            self.backend = backend
            theme = backend.getTheme()
            return backend
        } catch {
            screen = .failed(describeError(error))
            return nil
        }
    }

    private func perform<Value>(deliversCommittedRegistration: Bool = false, _ operation: @escaping (VaultBackend) throws -> Value) async -> Value? {
        guard let backend = prepareBackend() else { return nil }
        let epoch = requestEpoch
        let generation = lockCount
        isBusy = true
        defer { if epoch == requestEpoch { isBusy = false } }
        do {
            let value = try await runInBackground { try operation(backend) }
            guard epoch == requestEpoch else { return nil }
            if status.isUnlocked { refreshStatus() }
            guard deliversCommittedRegistration || generation == lockCount else { return nil }
            errorMessage = nil
            return value
        } catch {
            guard epoch == requestEpoch else { return nil }
            refreshStatus()
            guard generation == lockCount else { return nil }
            if (error as? BackendError)?.code == BackendError.passkeyVerificationFailed {
                cancelRequest(.userCanceled)
                return nil
            }
            errorMessage = describeError(error)
            return nil
        }
    }

    private func applyStatus(_ newStatus: VaultStatus) {
        if status.isUnlocked && !newStatus.isUnlocked {
            lockCount += 1
            forgetUnlockedData()
        }
        status = newStatus
        switch newStatus.state {
        case VaultStatus.stateUnlocked: screen = passkeyRegistrationRequest == nil ? .list : .passkeyRegistration
        case VaultStatus.stateLocked: screen = .locked
        default: screen = .signedOut
        }
        if newStatus.updateRequired != nil {
            screen = .updateRequired
        }
    }

    func handleLocked() {
        lockCount += 1
        forgetUnlockedData()
        if let backend {
            status = backend.getStatus()
            applyStatus(status)
        }
    }

    func refreshStatus() {
        guard screen != .configuration, let backend else { return }
        applyStatus(backend.getStatus())
    }

    private func forgetUnlockedData() {
        allMatchingLogin = []
        allOtherLogin = []
        isOtherLoginLoaded = false
        unmatchedLoginConfirmation = nil
        allMatchingPasskey = []
        searchText = ""
        serviceIdentifierByLoginID = [:]
        passkeyRegistrationOptions = nil
        selectedRegistrationLoginID = nil
        errorMessage = nil
        isBusy = false
    }

    private func resetRequest() {
        endPasskeyCeremony()
        forgetUnlockedData()
        pendingRequest = nil
        pendingPasskeyCredentialID = nil
        passkeyAssertionRequest = nil
        passkeyRegistrationRequest = nil
        registrationDestination = .idle
        selectedRegistrationLoginID = nil
        allMatchingPasskey = []
    }

    func prepareCredentialList(allServiceIdentifier: [String], kind: RequestKind = .password, passkeyRequest: PasskeyAssertionRequest? = nil) async {
        resetRequest()
        self.allServiceIdentifier = allServiceIdentifier
        requestKind = kind
        passkeyAssertionRequest = passkeyRequest
        guard let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        if status.isUnlocked {
            await loadAllMatchingCredential()
        }
    }

    func prepareInterfaceToProvideCredential(recordIdentifier: String, serviceIdentifier: String, kind: RequestKind = .password) async {
        resetRequest()
        allServiceIdentifier = [serviceIdentifier]
        requestKind = kind
        pendingRequest = PendingCredentialRequest(recordIdentifier: recordIdentifier, serviceIdentifier: serviceIdentifier, kind: kind)
        guard let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        if status.isUnlocked {
            await finishPendingRequest()
        }
    }

    // Without user interaction only an extension process that is still unlocked can answer.
    func provideCredentialWithoutUserInteraction(recordIdentifier: String, serviceIdentifier: String) async -> LoginCredential? {
        let generation = lockCount
        let epoch = requestEpoch
        guard let backend, backend.getStatus().isUnlocked, backend.getStatus().updateRequired == nil,
              let reference = LoginMatch.parseRecordIdentifier(recordIdentifier),
              let credential = try? await runInBackground({
                  try backend.getLoginForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier)
              }) else { return nil }
        guard generation == lockCount, epoch == requestEpoch else { return nil }
        if credential.hasOneTimePassword {
            await copyCode(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier)
        }
        refreshStatus()
        guard generation == lockCount, epoch == requestEpoch, status.isUnlocked else { return nil }
        return credential
    }

    func provideOneTimeCodeWithoutUserInteraction(recordIdentifier: String, serviceIdentifier: String) async -> String? {
        guard let backend, backend.getStatus().isUnlocked, backend.getStatus().updateRequired == nil,
              let reference = LoginMatch.parseRecordIdentifier(recordIdentifier) else { return nil }
        let generation = lockCount
        let epoch = requestEpoch
        let code = try? await runInBackground {
            try backend.getOneTimeCodeForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier).code
        }
        refreshStatus()
        return generation == lockCount && epoch == requestEpoch && status.isUnlocked ? code : nil
    }

    // The code goes to the pasteboard after a password fill, for the site's next step; a failure never stops the fill.
    private func copyCode(vaultID: String, itemID: String, serviceIdentifier: String?) async {
        guard let backend else { return }
        let generation = lockCount
        let epoch = requestEpoch
        let code = try? await runInBackground { () -> OneTimeCode in
            if let serviceIdentifier {
                return try backend.getOneTimeCodeForService(vaultID: vaultID, itemID: itemID, serviceIdentifier: serviceIdentifier)
            }
            return try backend.getOneTimeCode(vaultID: vaultID, itemID: itemID)
        }
        refreshStatus()
        if let code, generation == lockCount, epoch == requestEpoch, status.isUnlocked {
            copyOneTimeCode(code.code)
        }
    }

    func prepareInterfaceToProvidePasskey(request: PasskeyAssertionRequest, credentialID: Data) async {
        resetRequest()
        allServiceIdentifier = []
        requestKind = .password
        passkeyAssertionRequest = request
        pendingPasskeyCredentialID = credentialID
        guard let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        if status.isUnlocked {
            await finishPendingRequest()
        }
    }

    // Like passwords, only an extension process that is still unlocked signs without user interaction, and only when the
    // site does not ask for user verification: otherwise iOS shows the extension so the user confirms.
    func providePasskeyWithoutUserInteraction(request: PasskeyAssertionRequest, credentialID: Data) async -> PasskeyAssertionCredential? {
        let generation = lockCount
        let epoch = requestEpoch
        guard request.isUserVerificationDiscouraged, let backend, backend.getStatus().isUnlocked, backend.getStatus().updateRequired == nil,
              let result = try? await runInBackground({
                  try backend.assertPasskey(relyingPartyID: request.relyingPartyID, credentialID: credentialID, clientDataHash: request.clientDataHash, userVerification: request.userVerification)
              }) else { return nil }
        refreshStatus()
        guard generation == lockCount, epoch == requestEpoch, status.isUnlocked else { return nil }
        return PasskeyAssertionCredential(relyingPartyID: request.relyingPartyID, clientDataHash: request.clientDataHash, result: result)
    }

    func prepareInterfaceForPasskeyRegistration(_ request: PasskeyRegistrationRequest) async {
        resetRequest()
        guard request.isAlgorithmSupported else {
            cancelRequest(.failed)
            return
        }
        allServiceIdentifier = []
        requestKind = .password
        passkeyRegistrationRequest = request
        guard let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        if status.isUnlocked {
            await loadPasskeyRegistrationOptions()
        }
    }

    func loadPasskeyRegistrationOptions() async {
        guard !isBusy, let request = passkeyRegistrationRequest, let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        guard screen == .passkeyRegistration else { return }
        let epoch = requestEpoch
        registrationDestination = .loading
        errorMessage = nil
        let options = await perform { try $0.preparePasskeyRegistration(relyingPartyID: request.relyingPartyID, allExcludedCredentialID: request.allExcludedCredentialID) }
        guard epoch == requestEpoch else { return }
        applyStatus(backend.getStatus())
        guard screen == .passkeyRegistration else {
            registrationDestination = .idle
            errorMessage = nil
            return
        }
        guard let options else {
            registrationDestination = .failed
            return
        }
        registrationDestination = .loaded(options)
        let userName = request.userName.trimmingCharacters(in: .whitespaces)
        selectedRegistrationLoginID = options.allLogin.first { $0.isPersonal && !userName.isEmpty && $0.username.caseInsensitiveCompare(userName) == .orderedSame }?.id
    }

    func savePasskey() async {
        guard let request = passkeyRegistrationRequest, let options = passkeyRegistrationOptions, !options.isExcluded else { return }
        let login = options.allLogin.first { $0.id == selectedRegistrationLoginID }
        guard let saved = await perform(deliversCommittedRegistration: true, { backend -> (result: PasskeyRegistrationResult, allPrevious: [StoredPasskey], allCurrent: [StoredPasskey]) in
            let allPrevious = try backend.listAllPasskeyForRelyingParty(relyingPartyID: request.relyingPartyID, allAllowedCredentialID: [])
            let result = try backend.createPasskey(request, vaultID: login?.vaultID ?? "", itemID: login?.itemID ?? "")
            let allCurrent = (try? backend.listAllPasskeyForRelyingParty(relyingPartyID: request.relyingPartyID, allAllowedCredentialID: [])) ?? allPrevious
            return (result, allPrevious, allCurrent)
        }) else { return }
        guard let credential = PasskeyRegistrationCredential(relyingPartyID: request.relyingPartyID, clientDataHash: request.clientDataHash, result: saved.result) else {
            errorMessage = "Hisar could not read the new passkey."
            return
        }
        await updateCredentialIdentity(request: request, result: saved.result, credentialID: credential.credentialID, allPrevious: saved.allPrevious, allCurrent: saved.allCurrent)
        passkeyCompletion.completeRegistration(credential)
    }

    // The new passkey is offered in QuickType and the passkey sheet right away; one it replaced in the same login goes.
    private func updateCredentialIdentity(request: PasskeyRegistrationRequest, result: PasskeyRegistrationResult, credentialID: Data, allPrevious: [StoredPasskey], allCurrent: [StoredPasskey]) async {
        let allCurrentID = Set(allCurrent.map(\.credentialID))
        let userHandle = Base64URL.encode(request.userHandle)
        let allReplaced = allPrevious.filter { $0.userHandle == userHandle && !allCurrentID.contains($0.credentialID) }
        await credentialIdentityStore.removeIdentity(allReplaced.compactMap { passkey in
            Base64URL.decode(passkey.credentialID).map {
                CredentialIdentityRecord(kind: .passkey, host: passkey.relyingPartyID, username: passkey.userName, recordIdentifier: passkey.recordIdentifier, credentialID: $0, userHandle: request.userHandle)
            }
        })
        await credentialIdentityStore.saveIdentity([CredentialIdentityRecord(
            kind: .passkey, host: request.relyingPartyID, username: request.userName,
            recordIdentifier: LoginMatch.buildRecordIdentifier(vaultID: result.vaultID, itemID: result.itemID),
            credentialID: credentialID, userHandle: request.userHandle
        )])
    }

    func rejectExcludedPasskeyRegistration() {
        passkeyCompletion.rejectExcludedRegistration()
    }

    func selectPasskey(_ passkey: StoredPasskey) async {
        guard let request = passkeyAssertionRequest, let credentialID = Base64URL.decode(passkey.credentialID) else { return }
        _ = await assertPasskey(request: request, credentialID: credentialID)
    }


    private func assertPasskey(request: PasskeyAssertionRequest, credentialID: Data) async -> Bool {
        guard let result = await perform({ try $0.assertPasskey(relyingPartyID: request.relyingPartyID, credentialID: credentialID, clientDataHash: request.clientDataHash, userVerification: request.userVerification) }) else { return false }
        guard let credential = PasskeyAssertionCredential(relyingPartyID: request.relyingPartyID, clientDataHash: request.clientDataHash, result: result) else {
            errorMessage = "Hisar could not read the passkey signature."
            return false
        }
        passkeyCompletion.completeAssertion(credential)
        return true
    }

    func showConfiguration() {
        screen = .configuration
    }

    @discardableResult
    func unlock(password: String) async -> Bool {
        guard let newStatus = await perform({ try $0.unlock(password: password) }) else { return false }
        await handleUnlocked(newStatus)
        return true
    }

    func unlockWithBiometric() async {
        guard status.biometricReady, let newStatus = await perform({ try $0.unlockWithBiometric() }) else {
            refreshStatus()
            return
        }
        await handleUnlocked(newStatus)
    }

    private func handleUnlocked(_ newStatus: VaultStatus) async {
        let currentStatus = backend?.getStatus() ?? newStatus
        applyStatus(currentStatus)
        guard currentStatus.isUnlocked else { return }
        if passkeyRegistrationRequest != nil {
            await loadPasskeyRegistrationOptions()
        } else if pendingRequest != nil || pendingPasskeyCredentialID != nil {
            await finishPendingRequest()
        } else {
            await loadAllMatchingCredential()
        }
    }

    private func finishPendingRequest() async {
        guard status.isUnlocked else { return }
        let generation = lockCount
        let epoch = requestEpoch
        if let credentialID = pendingPasskeyCredentialID, let request = passkeyAssertionRequest {
            // The identity iOS offered is gone, deleted or moved since the store was last updated.
            guard let allFound = await perform({ try $0.listAllPasskeyForRelyingParty(relyingPartyID: request.relyingPartyID, allAllowedCredentialID: [credentialID]) }) else { return }
            guard !allFound.isEmpty else {
                cancelRequest(.credentialIdentityNotFound)
                return
            }
            if await assertPasskey(request: request, credentialID: credentialID) {
                return
            }
            guard generation == lockCount, epoch == requestEpoch, status.isUnlocked else { return }
            pendingPasskeyCredentialID = nil
            await loadAllMatchingCredential()
            return
        }
        guard let pendingRequest, let reference = LoginMatch.parseRecordIdentifier(pendingRequest.recordIdentifier) else {
            cancelRequest(.userCanceled)
            return
        }
        let serviceIdentifier = pendingRequest.serviceIdentifier
        if pendingRequest.kind == .oneTimeCode {
            if let code = await perform({ try $0.getOneTimeCodeForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier) }) {
                completeOneTimeCodeRequest(code.code)
                return
            }
        } else if let credential = await perform({ try $0.getLoginForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier) }) {
            if credential.hasOneTimePassword {
                await copyCode(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier)
            }
            guard generation == lockCount, epoch == requestEpoch, status.isUnlocked else { return }
            completeRequest(credential)
            return
        }
        guard generation == lockCount, epoch == requestEpoch, status.isUnlocked else { return }
        self.pendingRequest = nil
        await loadAllMatchingLogin()
    }

    func loadAllMatchingCredential() async {
        await loadAllMatchingPasskey()
        guard status.isUnlocked else { return }
        await loadAllMatchingLogin()
    }

    func loadAllMatchingPasskey() async {
        guard status.isUnlocked else { return }
        guard let request = passkeyAssertionRequest else {
            allMatchingPasskey = []
            return
        }
        allMatchingPasskey = await perform({ try $0.listAllPasskeyForRelyingParty(relyingPartyID: request.relyingPartyID, allAllowedCredentialID: request.allAllowedCredentialID) }) ?? []
    }

    func loadAllMatchingLogin() async {
        guard status.isUnlocked else { return }
        let allServiceIdentifier = allServiceIdentifier
        guard let result = await perform({ backend -> [(String, LoginMatch)] in
            var allPair: [(String, LoginMatch)] = []
            for serviceIdentifier in allServiceIdentifier {
                for login in try backend.listAllLoginForService(serviceIdentifier: serviceIdentifier) {
                    allPair.append((serviceIdentifier, login))
                }
            }
            return allPair
        }) else { return }
        var allLogin: [LoginMatch] = []
        serviceIdentifierByLoginID = [:]
        for (serviceIdentifier, login) in result where serviceIdentifierByLoginID[login.id] == nil {
            serviceIdentifierByLoginID[login.id] = serviceIdentifier
            allLogin.append(login)
        }
        allMatchingLogin = allLogin
    }

    func loadAllOtherLogin() async {
        guard !isOtherLoginLoaded, status.isUnlocked, !isBusy else { return }
        guard let allLogin = await perform({ backend -> [LoginMatch] in
            var allLogin: [LoginMatch] = []
            for vault in try backend.listAllVault() {
                for item in try backend.listAllItem(vaultID: vault.id) where item.offersLoginFill {
                    allLogin.append(LoginMatch(vaultID: vault.id, itemID: item.id, title: item.title, username: item.username, vaultName: vault.name, hasOneTimePassword: item.hasOneTimePassword))
                }
            }
            return allLogin.sorted { $0.title.localizedCaseInsensitiveCompare($1.title) == .orderedAscending }
        }) else { return }
        allOtherLogin = allLogin
        isOtherLoginLoaded = true
    }

    func select(_ login: LoginMatch) async {
        guard status.isUnlocked else { return }
        if serviceIdentifierByLoginID[login.id] == nil {
            guard let item = await perform({ try $0.getItem(vaultID: login.vaultID, itemID: login.itemID) }) else { return }
            unmatchedLoginConfirmation = UnmatchedLoginConfirmation(login: login, allWebsite: item.allURL.filter { VaultLogic.extractHost($0) != nil }, lockGeneration: lockCount, requestEpoch: requestEpoch)
            return
        }
        await fill(login)
    }

    func dismissUnmatchedLogin() {
        unmatchedLoginConfirmation = nil
    }

    func fillUnmatchedLogin(_ selected: UnmatchedLoginConfirmation? = nil) async {
        guard status.isUnlocked, let confirmation = selected ?? unmatchedLoginConfirmation,
              confirmation.lockGeneration == lockCount, confirmation.requestEpoch == requestEpoch else { return }
        unmatchedLoginConfirmation = nil
        await fill(confirmation.login)
    }

    var unmatchedFillTitle: String {
        guard let login = unmatchedLoginConfirmation?.login else { return "" }
        let account = login.username.isEmpty ? login.title : "\(login.title) (\(login.username))"
        let subject = requestKind == .oneTimeCode ? "a one-time code for \(account)" : account
        let host = allServiceIdentifier.lazy.compactMap { VaultLogic.extractHost($0) }.first
        return host.map { "Fill \(subject) into \($0)?" } ?? "Fill \(subject) without a website?"
    }

    var unmatchedFillMessage: String {
        guard let confirmation = unmatchedLoginConfirmation else { return "" }
        let hasWebsite = allServiceIdentifier.contains { VaultLogic.extractHost($0) != nil }
        let target = hasWebsite ? "Not saved for this site." : "This request has no website."
        let websites = confirmation.allWebsite.isEmpty ? "No website saved" : "Saved websites:\n" + confirmation.allWebsite.joined(separator: "\n")
        return "\(target)\n\(confirmation.login.vaultName)\n\n\(websites)"
    }

    private func fill(_ login: LoginMatch) async {
        guard status.isUnlocked else { return }
        let generation = lockCount
        let epoch = requestEpoch
        let serviceIdentifier = serviceIdentifierByLoginID[login.id]
        if requestKind == .oneTimeCode {
            let code = await perform { backend -> OneTimeCode in
                if let serviceIdentifier {
                    return try backend.getOneTimeCodeForService(vaultID: login.vaultID, itemID: login.itemID, serviceIdentifier: serviceIdentifier)
                }
                return try backend.getOneTimeCode(vaultID: login.vaultID, itemID: login.itemID)
            }
            if let code {
                completeOneTimeCodeRequest(code.code)
            }
            return
        }
        let credential: LoginCredential?
        if let serviceIdentifier {
            credential = await perform { try $0.getLoginForService(vaultID: login.vaultID, itemID: login.itemID, serviceIdentifier: serviceIdentifier) }
        } else {
            credential = await perform { backend in
                let item = try backend.getItem(vaultID: login.vaultID, itemID: login.itemID)
                return LoginCredential(username: item.username, password: item.password, hasOneTimePassword: item.hasOneTimePassword)
            }
        }
        guard let credential else { return }
        if credential.hasOneTimePassword {
            await copyCode(vaultID: login.vaultID, itemID: login.itemID, serviceIdentifier: serviceIdentifier)
        }
        guard generation == lockCount, epoch == requestEpoch, status.isUnlocked else { return }
        completeRequest(credential)
    }

    func endPasskeyCeremony() {
        requestEpoch &+= 1
        backend?.cancelPasskeyCeremonies()
    }

    func cancel() {
        endPasskeyCeremony()
        cancelRequest(.userCanceled)
    }
}
