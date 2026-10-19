import AuthenticationServices
import Combine
import SwiftUI

final class CredentialProviderViewController: ASCredentialProviderViewController {
    private var themeSubscription: AnyCancellable?
    private lazy var model = AutoFillModel(
        createBackend: {
            try BackendFactory.createBackend(configuration: .current, isClipboardIncluded: false, eventListener: nil)
        },
        completeRequest: { [weak self] credential in
            self?.extensionContext.completeRequest(withSelectedCredential: ASPasswordCredential(user: credential.username, password: credential.password))
        },
        completeOneTimeCodeRequest: { [weak self] code in
            self?.completeOneTimeCodeRequest(code: code)
        },
        copyOneTimeCode: { code in
            _ = PasteboardClipboard().setText(code)
        },
        cancelRequest: { [weak self] code in
            self?.cancelRequest(code: code)
        },
        passkeyCompletion: AutoFillModel.PasskeyCompletion(
            completeAssertion: { [weak self] credential in
                self?.completePasskeyAssertion(credential)
            },
            completeRegistration: { [weak self] credential in
                self?.extensionContext.completeRegistrationRequest(using: ASPasskeyRegistrationCredential(
                    relyingParty: credential.relyingPartyID, clientDataHash: credential.clientDataHash,
                    credentialID: credential.credentialID, attestationObject: credential.attestationObject
                ))
            },
            rejectExcludedRegistration: { [weak self] in
                self?.cancelRequest(code: .matchedExcludedCredential)
            }
        )
    )

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .systemGroupedBackground
        let hostingController = UIHostingController(rootView: AutoFillRootView(model: model))
        hostingController.view.backgroundColor = .clear
        addChild(hostingController)
        hostingController.view.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(hostingController.view)
        NSLayoutConstraint.activate([
            hostingController.view.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            hostingController.view.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            hostingController.view.topAnchor.constraint(equalTo: view.topAnchor),
            hostingController.view.bottomAnchor.constraint(equalTo: view.bottomAnchor),
        ])
        hostingController.didMove(toParent: self)
        themeSubscription = model.$theme.sink { [weak self] theme in
            self?.overrideUserInterfaceStyle = theme.userInterfaceStyle
            self?.view.tintColor = theme.palette.tint.uiColor
            self?.view.backgroundColor = theme.palette.groupedBackground.uiColor
        }
    }

    override func prepareCredentialList(for allRequestedServiceIdentifier: [ASCredentialServiceIdentifier]) {
        let allServiceIdentifier = allRequestedServiceIdentifier.map(\.identifier)
        Task { await model.prepareCredentialList(allServiceIdentifier: allServiceIdentifier) }
    }

    override func prepareCredentialList(for allRequestedServiceIdentifier: [ASCredentialServiceIdentifier], requestParameters: ASPasskeyCredentialRequestParameters) {
        let allServiceIdentifier = allRequestedServiceIdentifier.map(\.identifier)
        let passkeyRequest = PasskeyAssertionRequest(
            relyingPartyID: requestParameters.relyingPartyIdentifier, clientDataHash: requestParameters.clientDataHash, allAllowedCredentialID: requestParameters.allowedCredentials,
            isUserVerificationDiscouraged: requestParameters.userVerificationPreference == .discouraged
        )
        Task { await model.prepareCredentialList(allServiceIdentifier: allServiceIdentifier, passkeyRequest: passkeyRequest) }
    }

    override func prepareOneTimeCodeCredentialList(for allRequestedServiceIdentifier: [ASCredentialServiceIdentifier]) {
        let allServiceIdentifier = allRequestedServiceIdentifier.map(\.identifier)
        Task { await model.prepareCredentialList(allServiceIdentifier: allServiceIdentifier, kind: .oneTimeCode) }
    }

    override func provideCredentialWithoutUserInteraction(for credentialRequest: ASCredentialRequest) {
        if let passkeyRequest = readPasskeyRequest(credentialRequest) {
            Task {
                if let credential = await model.providePasskeyWithoutUserInteraction(request: passkeyRequest.request, credentialID: passkeyRequest.credentialID) {
                    completePasskeyAssertion(credential)
                } else {
                    cancelForUserInteraction()
                }
            }
            return
        }
        if let oneTimeCodeRequest = readOneTimeCodeRequest(credentialRequest) {
            Task {
                if let code = await model.provideOneTimeCodeWithoutUserInteraction(recordIdentifier: oneTimeCodeRequest.recordIdentifier, serviceIdentifier: oneTimeCodeRequest.serviceIdentifier) {
                    completeOneTimeCodeRequest(code: code)
                } else {
                    cancelForUserInteraction()
                }
            }
            return
        }
        guard let identity = credentialRequest.credentialIdentity as? ASPasswordCredentialIdentity,
              let recordIdentifier = identity.recordIdentifier else {
            cancelForUserInteraction()
            return
        }
        let serviceIdentifier = identity.serviceIdentifier.identifier
        Task {
            if let credential = await model.provideCredentialWithoutUserInteraction(recordIdentifier: recordIdentifier, serviceIdentifier: serviceIdentifier) {
                extensionContext.completeRequest(withSelectedCredential: ASPasswordCredential(user: credential.username, password: credential.password))
            } else {
                cancelForUserInteraction()
            }
        }
    }

    override func prepareInterfaceToProvideCredential(for credentialRequest: ASCredentialRequest) {
        if let passkeyRequest = readPasskeyRequest(credentialRequest) {
            Task { await model.prepareInterfaceToProvidePasskey(request: passkeyRequest.request, credentialID: passkeyRequest.credentialID) }
            return
        }
        if let oneTimeCodeRequest = readOneTimeCodeRequest(credentialRequest) {
            Task {
                await model.prepareInterfaceToProvideCredential(
                    recordIdentifier: oneTimeCodeRequest.recordIdentifier,
                    serviceIdentifier: oneTimeCodeRequest.serviceIdentifier,
                    kind: .oneTimeCode
                )
            }
            return
        }
        guard let identity = credentialRequest.credentialIdentity as? ASPasswordCredentialIdentity,
              let recordIdentifier = identity.recordIdentifier else {
            Task { await model.prepareCredentialList(allServiceIdentifier: [credentialRequest.credentialIdentity.serviceIdentifier.identifier]) }
            return
        }
        let serviceIdentifier = identity.serviceIdentifier.identifier
        Task { await model.prepareInterfaceToProvideCredential(recordIdentifier: recordIdentifier, serviceIdentifier: serviceIdentifier) }
    }

    override func prepareInterface(forPasskeyRegistration registrationRequest: ASCredentialRequest) {
        guard let passkeyRequest = registrationRequest as? ASPasskeyCredentialRequest,
              let identity = passkeyRequest.credentialIdentity as? ASPasskeyCredentialIdentity else {
            cancelRequest(code: .failed)
            return
        }
        let allExcludedCredentialID = passkeyRequest.excludedCredentials?.map(\.credentialID) ?? []
        let request = PasskeyRegistrationRequest(
            relyingPartyID: identity.relyingPartyIdentifier, userName: identity.userName, userHandle: identity.userHandle,
            clientDataHash: passkeyRequest.clientDataHash, allSupportedAlgorithm: passkeyRequest.supportedAlgorithms.map(\.rawValue),
            allExcludedCredentialID: allExcludedCredentialID, isUserVerificationDiscouraged: passkeyRequest.userVerificationPreference == .discouraged
        )
        Task { await model.prepareInterfaceForPasskeyRegistration(request) }
    }

    override func prepareInterfaceForExtensionConfiguration() {
        model.showConfiguration()
    }

    private func readPasskeyRequest(_ credentialRequest: ASCredentialRequest) -> (request: PasskeyAssertionRequest, credentialID: Data)? {
        guard credentialRequest.type == .passkeyAssertion,
              let passkeyRequest = credentialRequest as? ASPasskeyCredentialRequest,
              let identity = passkeyRequest.credentialIdentity as? ASPasskeyCredentialIdentity else { return nil }
        return (PasskeyAssertionRequest(
            relyingPartyID: identity.relyingPartyIdentifier, clientDataHash: passkeyRequest.clientDataHash,
            isUserVerificationDiscouraged: passkeyRequest.userVerificationPreference == .discouraged
        ), identity.credentialID)
    }

    private func completePasskeyAssertion(_ credential: PasskeyAssertionCredential) {
        extensionContext.completeAssertionRequest(using: ASPasskeyAssertionCredential(
            userHandle: credential.userHandle, relyingParty: credential.relyingPartyID, signature: credential.signature,
            clientDataHash: credential.clientDataHash, authenticatorData: credential.authenticatorData, credentialID: credential.credentialID
        ))
    }

    private func cancelRequest(code: ASExtensionError.Code) {
        extensionContext.cancelRequest(withError: NSError(domain: ASExtensionErrorDomain, code: code.rawValue))
    }

    private func readOneTimeCodeRequest(_ credentialRequest: ASCredentialRequest) -> (recordIdentifier: String, serviceIdentifier: String)? {
        guard credentialRequest.type == .oneTimeCode,
              let identity = credentialRequest.credentialIdentity as? ASOneTimeCodeCredentialIdentity,
              let recordIdentifier = identity.recordIdentifier else { return nil }
        return (recordIdentifier, identity.serviceIdentifier.identifier)
    }

    private func completeOneTimeCodeRequest(code: String) {
        extensionContext.completeOneTimeCodeRequest(using: ASOneTimeCodeCredential(code: code))
    }

    private func cancelForUserInteraction() {
        cancelRequest(code: .userInteractionRequired)
    }
}
