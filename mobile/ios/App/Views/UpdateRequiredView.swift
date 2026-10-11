import SwiftUI

struct UpdateRequiredView: View {
    var requirement: UpdateRequirement
    var isChecking: Bool
    var checkAgain: () async -> Void
    @Environment(\.openURL) private var openURL

    var body: some View {
        NavigationStack {
            ContentUnavailableView {
                Label("Update required", systemImage: "arrow.down.app")
            } description: {
                Text("This server needs Hisar \(requirement.minimumVersion) or later. This iPhone has \(requirement.currentVersionDescription).")
                    .accessibilityIdentifier("update-required-message")
            } actions: {
                VStack(spacing: 12) {
                    Button {
                        if let updateURL = URL(string: requirement.updateURL) {
                            openURL(updateURL)
                        }
                    } label: {
                        Text("Install the latest Hisar")
                            .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .controlSize(.large)
                    .accessibilityIdentifier("update-required-install")
                    Button {
                        Task { await checkAgain() }
                    } label: {
                        if isChecking {
                            ProgressView()
                        } else {
                            Text("Check again")
                        }
                    }
                    .disabled(isChecking)
                    .accessibilityIdentifier("update-required-check-again")
                }
                .padding(.horizontal, 32)
            }
            .applyThemedListBackground()
        }
        .accessibilityIdentifier("update-required")
    }
}

#if DEBUG
enum UpdateRequiredPreview {
    static let argument = "-HisarUpdateRequiredPreview"

    static func isRequested() -> Bool {
        ProcessInfo.processInfo.arguments.contains(argument)
    }

    static let requirement = UpdateRequirement(currentVersion: "0.1.0", minimumVersion: "0.2.0", updateURL: "https://tryhisar.com/download")
}
#endif
