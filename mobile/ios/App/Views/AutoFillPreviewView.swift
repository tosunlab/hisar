#if DEBUG
import SwiftUI

// The extension's screens inside the app, for UI tests: the simulator does not offer a build without a team as a provider.
struct AutoFillPreviewView: View {
    static let argument = "-HisarAutoFillPreview"

    static func readServiceIdentifier() -> String? {
        let allArgument = ProcessInfo.processInfo.arguments
        guard let index = allArgument.firstIndex(of: argument), index + 1 < allArgument.count else { return nil }
        return allArgument[index + 1]
    }

    var serviceIdentifier: String
    @StateObject private var session = AutoFillPreviewSession()

    var body: some View {
        AutoFillRootView(model: session.model)
            .task { await session.model.prepareCredentialList(allServiceIdentifier: [serviceIdentifier]) }
            .overlay(alignment: .bottom) {
                if let resultMessage = session.resultMessage {
                    Text(resultMessage)
                        .padding()
                        .background(Theme.surfaceRaised)
                        .foregroundStyle(Theme.text)
                        .clipShape(Capsule())
                        .padding(.bottom, 40)
                        .accessibilityIdentifier("autofill-result")
                }
            }
    }
}

@MainActor
private final class AutoFillPreviewSession: ObservableObject {
    @Published var resultMessage: String?
    private(set) lazy var model = AutoFillModel(
        createBackend: { try BackendFactory.createBackend(configuration: .current, isClipboardIncluded: false, eventListener: nil) },
        completeRequest: { [weak self] credential in self?.resultMessage = "Filled \(credential.username) with a password of \(credential.password.count) characters" },
        cancelRequest: { [weak self] in self?.resultMessage = "Cancelled" }
    )
}
#endif
