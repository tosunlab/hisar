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
    @Published private(set) var passkeyRegistrationOptions: PasskeyRegistrationOptions?
    @Published var selectedRegistrationLoginID: String?
    // The app's theme, read from the shared state in the app group.
    @Published private(set) var theme = VaultTheme.standard

    private(set) var allServiceIdentifier: [String] = []
    private var pendingRequest: PendingCredentialRequest?
    private var pendingPasskeyCredentialID: Data?
    private var serviceIdentifierByLoginID: [String: String] = [:]
    private var backend: VaultBackend?
    private let createBackend: () throws -> VaultBackend
    private let completeRequest: (LoginCredential) -> Void
    private let completeOneTimeCodeRequest: (String) -> Void
    private let copyOneTimeCode: (String) -> Void
    private let cancelRequest: () -> Void
    private let passkeyCompletion: PasskeyCompletion

    init(
        createBackend: @escaping () throws -> VaultBackend,
        completeRequest: @escaping (LoginCredential) -> Void,
        completeOneTimeCodeRequest: @escaping (String) -> Void,
        copyOneTimeCode: @escaping (String) -> Void,
        cancelRequest: @escaping () -> Void,
        passkeyCompletion: PasskeyCompletion
    ) {
        self.createBackend = createBackend
        self.completeRequest = completeRequest
        self.completeOneTimeCodeRequest = completeOneTimeCodeRequest
        self.copyOneTimeCode = copyOneTimeCode
        self.cancelRequest = cancelRequest
        self.passkeyCompletion = passkeyCompletion
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

    private func perform<Value>(_ operation: @escaping (VaultBackend) throws -> Value) async -> Value? {
        guard let backend = prepareBackend() else { return nil }
        isBusy = true
        defer { isBusy = false }
        do {
            let value = try await runInBackground { try operation(backend) }
            errorMessage = nil
            return value
        } catch {
            errorMessage = describeError(error)
            return nil
        }
    }

    private func applyStatus(_ newStatus: VaultStatus) {
        if status.isUnlocked && !newStatus.isUnlocked { lockCount += 1 }
        status = newStatus
        switch newStatus.state {
        case VaultStatus.stateUnlocked: screen = passkeyRegistrationRequest == nil ? .list : .passkeyRegistration
        case VaultStatus.stateLocked: screen = .locked
        default: screen = .signedOut
        }
    }

    private func resetRequest() {
        pendingRequest = nil
        pendingPasskeyCredentialID = nil
        passkeyAssertionRequest = nil
        passkeyRegistrationRequest = nil
        passkeyRegistrationOptions = nil
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
        guard let backend, backend.getStatus().isUnlocked,
              let reference = LoginMatch.parseRecordIdentifier(recordIdentifier),
              let credential = try? await runInBackground({
                  try backend.getLoginForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier)
              }) else { return nil }
        if credential.hasOneTimePassword {
            await copyCode(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier)
        }
        return credential
    }

    func provideOneTimeCodeWithoutUserInteraction(recordIdentifier: String, serviceIdentifier: String) async -> String? {
        guard let backend, backend.getStatus().isUnlocked,
              let reference = LoginMatch.parseRecordIdentifier(recordIdentifier) else { return nil }
        return try? await runInBackground {
            try backend.getOneTimeCodeForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier).code
        }
    }

    // The code goes to the pasteboard after a password fill, for the site's next step; a failure never stops the fill.
    private func copyCode(vaultID: String, itemID: String, serviceIdentifier: String?) async {
        guard let backend else { return }
        let code = try? await runInBackground { () -> OneTimeCode in
            if let serviceIdentifier {
                return try backend.getOneTimeCodeForService(vaultID: vaultID, itemID: itemID, serviceIdentifier: serviceIdentifier)
            }
            return try backend.getOneTimeCode(vaultID: vaultID, itemID: itemID)
        }
        if let code {
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

    // Like passwords, only an extension process that is still unlocked signs without user interaction.
    func providePasskeyWithoutUserInteraction(request: PasskeyAssertionRequest, credentialID: Data) async -> PasskeyAssertionCredential? {
        guard let backend, backend.getStatus().isUnlocked,
              let result = try? await runInBackground({
                  try backend.assertPasskey(relyingPartyID: request.relyingPartyID, credentialID: credentialID, clientDataHash: request.clientDataHash)
              }) else { return nil }
        return PasskeyAssertionCredential(relyingPartyID: request.relyingPartyID, clientDataHash: request.clientDataHash, result: result)
    }

    func prepareInterfaceForPasskeyRegistration(_ request: PasskeyRegistrationRequest) async {
        resetRequest()
        guard request.isAlgorithmSupported else {
            cancel()
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
        guard let request = passkeyRegistrationRequest,
              let options = await perform({ try $0.preparePasskeyRegistration(relyingPartyID: request.relyingPartyID, allExcludedCredentialID: request.allExcludedCredentialID) }) else { return }
        passkeyRegistrationOptions = options
        let userName = request.userName.trimmingCharacters(in: .whitespaces)
        selectedRegistrationLoginID = options.allLogin.first { !userName.isEmpty && $0.username.caseInsensitiveCompare(userName) == .orderedSame }?.id
    }

    func savePasskey() async {
        guard let request = passkeyRegistrationRequest, let options = passkeyRegistrationOptions, !options.isExcluded else { return }
        let login = options.allLogin.first { $0.id == selectedRegistrationLoginID }
        guard let result = await perform({ try $0.createPasskey(request, vaultID: login?.vaultID ?? "", itemID: login?.itemID ?? "") }) else { return }
        guard let credential = PasskeyRegistrationCredential(relyingPartyID: request.relyingPartyID, clientDataHash: request.clientDataHash, result: result) else {
            errorMessage = "Hisar could not read the new passkey."
            return
        }
        passkeyCompletion.completeRegistration(credential)
    }

    func rejectExcludedPasskeyRegistration() {
        passkeyCompletion.rejectExcludedRegistration()
    }

    func selectPasskey(_ passkey: StoredPasskey) async {
        guard let request = passkeyAssertionRequest, let credentialID = Base64URL.decode(passkey.credentialID) else { return }
        _ = await assertPasskey(request: request, credentialID: credentialID)
    }

    private func assertPasskey(request: PasskeyAssertionRequest, credentialID: Data) async -> Bool {
        guard let result = await perform({ try $0.assertPasskey(relyingPartyID: request.relyingPartyID, credentialID: credentialID, clientDataHash: request.clientDataHash) }) else { return false }
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
            if let backend { status = backend.getStatus() }
            return
        }
        await handleUnlocked(newStatus)
    }

    private func handleUnlocked(_ newStatus: VaultStatus) async {
        applyStatus(newStatus)
        guard newStatus.isUnlocked else { return }
        if passkeyRegistrationRequest != nil {
            await loadPasskeyRegistrationOptions()
        } else if pendingRequest != nil || pendingPasskeyCredentialID != nil {
            await finishPendingRequest()
        } else {
            await loadAllMatchingCredential()
        }
    }

    private func finishPendingRequest() async {
        if let credentialID = pendingPasskeyCredentialID, let request = passkeyAssertionRequest {
            if await assertPasskey(request: request, credentialID: credentialID) {
                return
            }
            pendingPasskeyCredentialID = nil
            await loadAllMatchingCredential()
            return
        }
        guard let pendingRequest, let reference = LoginMatch.parseRecordIdentifier(pendingRequest.recordIdentifier) else {
            cancel()
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
            completeRequest(credential)
            return
        }
        self.pendingRequest = nil
        await loadAllMatchingLogin()
    }

    func loadAllMatchingCredential() async {
        await loadAllMatchingPasskey()
        await loadAllMatchingLogin()
    }

    func loadAllMatchingPasskey() async {
        guard let request = passkeyAssertionRequest else {
            allMatchingPasskey = []
            return
        }
        allMatchingPasskey = await perform({ try $0.listAllPasskeyForRelyingParty(relyingPartyID: request.relyingPartyID, allAllowedCredentialID: request.allAllowedCredentialID) }) ?? []
    }

    func loadAllMatchingLogin() async {
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
                for item in try backend.listAllItem(vaultID: vault.id) where item.isLogin {
                    allLogin.append(LoginMatch(vaultID: vault.id, itemID: item.id, title: item.title, username: item.username, vaultName: vault.name, hasOneTimePassword: item.hasOneTimePassword))
                }
            }
            return allLogin.sorted { $0.title.localizedCaseInsensitiveCompare($1.title) == .orderedAscending }
        }) else { return }
        allOtherLogin = allLogin
        isOtherLoginLoaded = true
    }

    func select(_ login: LoginMatch) async {
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
        completeRequest(credential)
    }

    func cancel() {
        cancelRequest()
    }
}
