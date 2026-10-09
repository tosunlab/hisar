#if DEBUG
import SwiftUI

// The extension's screens inside the app, for UI tests: the simulator does not offer a build without a team as a provider.
struct AutoFillPreviewView: View {
    static let argument = "-HisarAutoFillPreview"
    static let oneTimeCodeArgument = "-HisarAutoFillOneTimeCodePreview"

    static func readServiceIdentifier() -> String? {
        readArgumentValue(argument) ?? readArgumentValue(oneTimeCodeArgument)
    }

    static var isOneTimeCodeRequest: Bool {
        readArgumentValue(oneTimeCodeArgument) != nil
    }

    private static func readArgumentValue(_ name: String) -> String? {
        let allArgument = ProcessInfo.processInfo.arguments
        guard let index = allArgument.firstIndex(of: name), index + 1 < allArgument.count else { return nil }
        return allArgument[index + 1]
    }

    var serviceIdentifier: String
    @StateObject private var session = AutoFillPreviewSession()

    var body: some View {
        AutoFillRootView(model: session.model)
            .task { await session.model.prepareCredentialList(allServiceIdentifier: [serviceIdentifier], kind: Self.isOneTimeCodeRequest ? .oneTimeCode : .password) }
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
    private var copiedOneTimeCode: String?
    private(set) lazy var model = AutoFillModel(
        createBackend: { try BackendFactory.createBackend(configuration: .current, isClipboardIncluded: false, eventListener: nil) },
        completeRequest: { [weak self] credential in
            let copiedMessage = self?.copiedOneTimeCode.map { " and copied the one-time code \($0)" } ?? ""
            self?.resultMessage = "Filled \(credential.username) with a password of \(credential.password.count) characters\(copiedMessage)"
        },
        completeOneTimeCodeRequest: { [weak self] code in self?.resultMessage = "Filled the one-time code \(code)" },
        copyOneTimeCode: { [weak self] code in
            self?.copiedOneTimeCode = code
            _ = PasteboardClipboard().setText(code)
        },
        cancelRequest: { [weak self] in self?.resultMessage = "Cancelled" }
    )
}
#endif
