import Foundation

enum BackendFactory {
    static let resetStateArgument = "-HisarResetState"

    static func createBackend(configuration: HisarConfiguration, isClipboardIncluded: Bool, eventListener: LockEventRelay?) throws -> HisarKitBackend {
        let stateDirectory = try configuration.prepareStateDirectory()
        let secretKeyStore = KeychainSecretKeyStore(accessGroup: configuration.keychainAccessGroup)
        let biometric = BiometricKeychain(accessGroup: configuration.keychainAccessGroup)
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains(resetStateArgument) {
            try? FileManager.default.removeItem(at: stateDirectory)
            try FileManager.default.createDirectory(at: stateDirectory, withIntermediateDirectories: true)
            secretKeyStore.deleteAllSecretKey()
            biometric.deleteAllKey()
        }
        #endif
        return try HisarKitBackend(
            stateDirectory: stateDirectory,
            secretKeyStore: secretKeyStore,
            biometric: biometric,
            clipboard: isClipboardIncluded ? PasteboardClipboard() : nil,
            eventListener: eventListener
        )
    }
}
