import AuthenticationServices
import Foundation

protocol CredentialIdentityUpdating: AnyObject {
    func replaceAllIdentity(_ allIdentity: [CredentialIdentityRecord]) async
    func removeAllIdentity() async
}

// The AutoFill extension adds or removes single identities, such as a passkey it just created, without listing the vault.
protocol CredentialIdentitySaving: AnyObject {
    func saveIdentity(_ allIdentity: [CredentialIdentityRecord]) async
    func removeIdentity(_ allIdentity: [CredentialIdentityRecord]) async
}

final class SystemCredentialIdentityStore: CredentialIdentityUpdating, CredentialIdentitySaving {
    private let store = ASCredentialIdentityStore.shared

    private func checkIsEnabled() async -> Bool {
        await store.state().isEnabled
    }

    private func checkIsUpdatable() async -> Bool {
        let state = await store.state()
        return state.isEnabled && state.supportsIncrementalUpdates
    }

    private func buildAllSystemIdentity(_ allIdentity: [CredentialIdentityRecord]) -> [ASCredentialIdentity] {
        allIdentity.map { identity -> ASCredentialIdentity in
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
    }

    func replaceAllIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        guard await checkIsEnabled() else { return }
        try? await store.replaceCredentialIdentities(buildAllSystemIdentity(allIdentity))
    }

    func saveIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        guard !allIdentity.isEmpty, await checkIsUpdatable() else { return }
        try? await store.saveCredentialIdentities(buildAllSystemIdentity(allIdentity))
    }

    func removeIdentity(_ allIdentity: [CredentialIdentityRecord]) async {
        guard !allIdentity.isEmpty, await checkIsUpdatable() else { return }
        try? await store.removeCredentialIdentities(buildAllSystemIdentity(allIdentity))
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
