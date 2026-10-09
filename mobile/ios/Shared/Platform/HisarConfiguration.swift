import Foundation

struct HisarConfiguration {
    var appGroupIdentifier: String?
    var keychainAccessGroup: String?

    static let current = HisarConfiguration(bundle: .main)

    init(appGroupIdentifier: String?, keychainAccessGroup: String?) {
        self.appGroupIdentifier = appGroupIdentifier
        self.keychainAccessGroup = keychainAccessGroup
    }

    init(bundle: Bundle) {
        appGroupIdentifier = Self.readNonEmptyValue(bundle: bundle, key: "HisarAppGroupIdentifier")
        keychainAccessGroup = Self.readNonEmptyValue(bundle: bundle, key: "HisarKeychainAccessGroup")
    }

    private static func readNonEmptyValue(bundle: Bundle, key: String) -> String? {
        guard let value = bundle.object(forInfoDictionaryKey: key) as? String else { return nil }
        let trimmedValue = value.trimmingCharacters(in: .whitespaces)
        return trimmedValue.isEmpty || trimmedValue.contains("$(") ? nil : trimmedValue
    }

    var isAppGroupAvailable: Bool { getAppGroupContainer() != nil }

    func getAppGroupContainer() -> URL? {
        guard let appGroupIdentifier else { return nil }
        return FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupIdentifier)
    }

    // The app group container when signed for it, otherwise the app's own Application Support (free personal team).
    func prepareStateDirectory() throws -> URL {
        let baseDirectory: URL
        if let container = getAppGroupContainer() {
            baseDirectory = container.appendingPathComponent("Library/Application Support", isDirectory: true)
        } else {
            baseDirectory = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
        }
        var stateDirectory = baseDirectory.appendingPathComponent("Hisar", isDirectory: true)
        try FileManager.default.createDirectory(at: stateDirectory, withIntermediateDirectories: true)
        var resourceValues = URLResourceValues()
        resourceValues.isExcludedFromBackup = true
        try? stateDirectory.setResourceValues(resourceValues)
        return stateDirectory
    }
}
