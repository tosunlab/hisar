import Foundation

@MainActor
final class AutoFillModel: ObservableObject {
    enum Screen: Equatable {
        case starting
        case failed(String)
        case signedOut
        case locked
        case list
        case configuration
    }

    struct PendingCredentialRequest: Equatable {
        var recordIdentifier: String
        var serviceIdentifier: String
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

    private(set) var allServiceIdentifier: [String] = []
    private var pendingRequest: PendingCredentialRequest?
    private var serviceIdentifierByLoginID: [String: String] = [:]
    private var backend: VaultBackend?
    private let createBackend: () throws -> VaultBackend
    private let completeRequest: (LoginCredential) -> Void
    private let cancelRequest: () -> Void

    init(createBackend: @escaping () throws -> VaultBackend, completeRequest: @escaping (LoginCredential) -> Void, cancelRequest: @escaping () -> Void) {
        self.createBackend = createBackend
        self.completeRequest = completeRequest
        self.cancelRequest = cancelRequest
    }

    var serviceName: String {
        allServiceIdentifier.lazy.compactMap { VaultLogic.extractHost($0) }.first ?? allServiceIdentifier.first ?? ""
    }

    var allVisibleMatchingLogin: [LoginMatch] {
        Self.filterAllLogin(allMatchingLogin, searchText: searchText)
    }

    var allVisibleOtherLogin: [LoginMatch] {
        guard !searchText.isEmpty else { return [] }
        let allMatchingID = Set(allMatchingLogin.map(\.id))
        return Self.filterAllLogin(allOtherLogin.filter { !allMatchingID.contains($0.id) }, searchText: searchText)
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
        case VaultStatus.stateUnlocked: screen = .list
        case VaultStatus.stateLocked: screen = .locked
        default: screen = .signedOut
        }
    }

    func prepareCredentialList(allServiceIdentifier: [String]) async {
        self.allServiceIdentifier = allServiceIdentifier
        pendingRequest = nil
        guard let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        if status.isUnlocked {
            await loadAllMatchingLogin()
        }
    }

    func prepareInterfaceToProvideCredential(recordIdentifier: String, serviceIdentifier: String) async {
        allServiceIdentifier = [serviceIdentifier]
        pendingRequest = PendingCredentialRequest(recordIdentifier: recordIdentifier, serviceIdentifier: serviceIdentifier)
        guard let backend = prepareBackend() else { return }
        applyStatus(backend.getStatus())
        if status.isUnlocked {
            await finishPendingRequest()
        }
    }

    // Without user interaction only an extension process that is still unlocked can answer.
    func provideCredentialWithoutUserInteraction(recordIdentifier: String, serviceIdentifier: String) async -> LoginCredential? {
        guard let backend, backend.getStatus().isUnlocked,
              let reference = LoginMatch.parseRecordIdentifier(recordIdentifier) else { return nil }
        return try? await runInBackground {
            try backend.getLoginForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier)
        }
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
        if pendingRequest != nil {
            await finishPendingRequest()
        } else {
            await loadAllMatchingLogin()
        }
    }

    private func finishPendingRequest() async {
        guard let pendingRequest, let reference = LoginMatch.parseRecordIdentifier(pendingRequest.recordIdentifier) else {
            cancel()
            return
        }
        let serviceIdentifier = pendingRequest.serviceIdentifier
        if let credential = await perform({ try $0.getLoginForService(vaultID: reference.vaultID, itemID: reference.itemID, serviceIdentifier: serviceIdentifier) }) {
            completeRequest(credential)
        } else {
            self.pendingRequest = nil
            await loadAllMatchingLogin()
        }
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
                    allLogin.append(LoginMatch(vaultID: vault.id, itemID: item.id, title: item.title, username: item.username, vaultName: vault.name))
                }
            }
            return allLogin.sorted { $0.title.localizedCaseInsensitiveCompare($1.title) == .orderedAscending }
        }) else { return }
        allOtherLogin = allLogin
        isOtherLoginLoaded = true
    }

    func select(_ login: LoginMatch) async {
        let credential: LoginCredential?
        if let serviceIdentifier = serviceIdentifierByLoginID[login.id] {
            credential = await perform { try $0.getLoginForService(vaultID: login.vaultID, itemID: login.itemID, serviceIdentifier: serviceIdentifier) }
        } else {
            credential = await perform { backend in
                let item = try backend.getItem(vaultID: login.vaultID, itemID: login.itemID)
                return LoginCredential(username: item.username, password: item.password)
            }
        }
        if let credential {
            completeRequest(credential)
        }
    }

    func cancel() {
        cancelRequest()
    }
}
