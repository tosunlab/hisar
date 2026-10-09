import AuthenticationServices
import SwiftUI

final class CredentialProviderViewController: ASCredentialProviderViewController {
    private lazy var model = AutoFillModel(
        createBackend: {
            try BackendFactory.createBackend(configuration: .current, isClipboardIncluded: false, eventListener: nil)
        },
        completeRequest: { [weak self] credential in
            self?.extensionContext.completeRequest(withSelectedCredential: ASPasswordCredential(user: credential.username, password: credential.password))
        },
        cancelRequest: { [weak self] in
            self?.extensionContext.cancelRequest(withError: NSError(domain: ASExtensionErrorDomain, code: ASExtensionError.userCanceled.rawValue))
        }
    )

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = UIColor(Theme.background)
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
    }

    override func prepareCredentialList(for allRequestedServiceIdentifier: [ASCredentialServiceIdentifier]) {
        let allServiceIdentifier = allRequestedServiceIdentifier.map(\.identifier)
        Task { await model.prepareCredentialList(allServiceIdentifier: allServiceIdentifier) }
    }

    override func provideCredentialWithoutUserInteraction(for credentialRequest: ASCredentialRequest) {
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
        guard let identity = credentialRequest.credentialIdentity as? ASPasswordCredentialIdentity,
              let recordIdentifier = identity.recordIdentifier else {
            Task { await model.prepareCredentialList(allServiceIdentifier: [credentialRequest.credentialIdentity.serviceIdentifier.identifier]) }
            return
        }
        let serviceIdentifier = identity.serviceIdentifier.identifier
        Task { await model.prepareInterfaceToProvideCredential(recordIdentifier: recordIdentifier, serviceIdentifier: serviceIdentifier) }
    }

    override func prepareInterfaceForExtensionConfiguration() {
        model.showConfiguration()
    }

    private func cancelForUserInteraction() {
        extensionContext.cancelRequest(withError: NSError(domain: ASExtensionErrorDomain, code: ASExtensionError.userInteractionRequired.rawValue))
    }
}
