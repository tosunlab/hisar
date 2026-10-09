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
        let allPasswordIdentity: [ASCredentialIdentity] = allIdentity.map { identity in
            ASPasswordCredentialIdentity(
                serviceIdentifier: ASCredentialServiceIdentifier(identifier: identity.host, type: .domain),
                user: identity.username,
                recordIdentifier: identity.recordIdentifier
            )
        }
        try? await store.replaceCredentialIdentities(allPasswordIdentity)
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

    // iOS 18 asks in place; iOS 17 can only open the Settings page.
    @MainActor
    static func requestToTurnOn() async -> Bool {
        if #available(iOS 18.0, *) {
            return await ASSettingsHelper.requestToTurnOnCredentialProviderExtension()
        }
        try? await ASSettingsHelper.openCredentialProviderAppSettings()
        return await checkIsEnabled()
    }
}
