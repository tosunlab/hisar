import AuthenticationServices
import Foundation

protocol CredentialIdentityUpdating: AnyObject {
    func replaceAllIdentity(_ allIdentity: [CredentialIdentityRecord]) async
    func removeAllIdentity() async
}

final class SystemCredentialIdentityStore: CredentialIdentityUpdating {
    private let store = ASCredentialIdentityStore.shared

    private func checkIsEnabled() async -> Bool {
        await store.state().isEnabled
    }

    func replaceAllIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        guard await checkIsEnabled() else { return }
        let allSystemIdentity: [ASCredentialIdentity] = allIdentity.compactMap { identity in
            let serviceIdentifier = ASCredentialServiceIdentifier(identifier: identity.host, type: .domain)
            switch identity.kind {
            case .password:
                return ASPasswordCredentialIdentity(serviceIdentifier: serviceIdentifier, user: identity.username, recordIdentifier: identity.recordIdentifier)
            case .oneTimeCode:
                return ASOneTimeCodeCredentialIdentity(serviceIdentifier: serviceIdentifier, label: identity.username, recordIdentifier: identity.recordIdentifier)
            case .passkey:
                return ASPasskeyCredentialIdentity(relyingPartyIdentifier: identity.host, userName: identity.username, credentialID: identity.credentialID, userHandle: identity.userHandle, recordIdentifier: identity.recordIdentifier)
            }
        }
        try? await store.replaceCredentialIdentities(allSystemIdentity)
    }

    func removeAllIdentity() async {
        guard await checkIsEnabled() else { return }
        try? await store.removeAllCredentialIdentities()
    }
}

enum AutoFillSettings {
    static func checkIsEnabled() async -> Bool {
        await ASCredentialIdentityStore.shared.state().isEnabled
    }

    @MainActor
    static func requestToTurnOn() async -> Bool {
        await ASSettingsHelper.requestToTurnOnCredentialProviderExtension()
    }

    @MainActor
    static func openSettings() async {
        try? await ASSettingsHelper.openCredentialProviderAppSettings()
    }
}
