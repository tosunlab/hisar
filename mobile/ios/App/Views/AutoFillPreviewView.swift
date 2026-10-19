#if DEBUG
import SwiftUI

// The extension's screens inside the app, for UI tests: the simulator does not offer a build without a team as a provider.
struct AutoFillPreviewView: View {
    enum Mode {
        case password(String)
        case oneTimeCode(String)
        case passkeyAssertion(PasskeyAssertionRequest)
        case passkeyRegistration(PasskeyRegistrationRequest)
    }

    static let argument = "-HisarAutoFillPreview"
    static let oneTimeCodeArgument = "-HisarAutoFillOneTimeCodePreview"
    static let passkeyArgument = "-HisarAutoFillPasskeyPreview"
    static let passkeyRegistrationArgument = "-HisarAutoFillPasskeyRegistrationPreview"
    static let passkeyUserNameArgument = "-HisarAutoFillPasskeyUserName"
    static let passkeyUserHandleArgument = "-HisarAutoFillPasskeyUserHandle"
    static let passkeyClientDataHashArgument = "-HisarAutoFillPasskeyClientDataHash"

    static func readMode() -> Mode? {
        if let serviceIdentifier = readArgumentValue(argument) {
            return .password(serviceIdentifier)
        }
        if let serviceIdentifier = readArgumentValue(oneTimeCodeArgument) {
            return .oneTimeCode(serviceIdentifier)
        }
        let clientDataHash = readArgumentValue(passkeyClientDataHashArgument).flatMap(Base64URL.decode) ?? Data(repeating: 0, count: 32)
        if let relyingPartyID = readArgumentValue(passkeyArgument) {
            return .passkeyAssertion(PasskeyAssertionRequest(relyingPartyID: relyingPartyID, clientDataHash: clientDataHash))
        }
        if let relyingPartyID = readArgumentValue(passkeyRegistrationArgument) {
            return .passkeyRegistration(PasskeyRegistrationRequest(
                relyingPartyID: relyingPartyID,
                userName: readArgumentValue(passkeyUserNameArgument) ?? "",
                userHandle: readArgumentValue(passkeyUserHandleArgument).flatMap(Base64URL.decode) ?? Data("preview-user".utf8),
                clientDataHash: clientDataHash,
                allSupportedAlgorithm: [PasskeyRegistrationRequest.algorithmES256]
            ))
        }
        return nil
    }

    private static func readArgumentValue(_ name: String) -> String? {
        let allArgument = ProcessInfo.processInfo.arguments
        guard let index = allArgument.firstIndex(of: name), index + 1 < allArgument.count else { return nil }
        return allArgument[index + 1]
    }

    var mode: Mode
    @StateObject private var session = AutoFillPreviewSession()

    var body: some View {
        AutoFillRootView(model: session.model)
            .task { await prepare() }
            .overlay(alignment: .bottom) {
                if let resultMessage = session.resultMessage {
                    VStack(spacing: 6) {
                        Text(resultMessage)
                            .padding()
                            .background(.regularMaterial, in: .capsule)
                            .accessibilityIdentifier("autofill-result")
                        ForEach(session.allResultValue, id: \.name) { value in
                            Text(value.text)
                                .font(.system(size: 6, design: .monospaced))
                                .foregroundStyle(.themeSecondary)
                                .lineLimit(1)
                                .accessibilityIdentifier("autofill-result-\(value.name)")
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.bottom, 40)
                }
            }
    }

    private func prepare() async {
        switch mode {
        case .password(let serviceIdentifier):
            await session.model.prepareCredentialList(allServiceIdentifier: [serviceIdentifier])
        case .oneTimeCode(let serviceIdentifier):
            await session.model.prepareCredentialList(allServiceIdentifier: [serviceIdentifier], kind: .oneTimeCode)
        case .passkeyAssertion(let request):
            await session.model.prepareCredentialList(allServiceIdentifier: ["https://\(request.relyingPartyID)"], passkeyRequest: request)
        case .passkeyRegistration(let request):
            await session.model.prepareInterfaceForPasskeyRegistration(request)
        }
    }
}

@MainActor
private final class AutoFillPreviewSession: ObservableObject {
    struct ResultValue {
        var name: String
        var text: String
    }

    @Published var resultMessage: String?
    @Published var allResultValue: [ResultValue] = []
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
        cancelRequest: { [weak self] _ in self?.resultMessage = "Cancelled" },
        passkeyCompletion: AutoFillModel.PasskeyCompletion(
            completeAssertion: { [weak self] credential in
                self?.allResultValue = [
                    ResultValue(name: "credential-id", text: Base64URL.encode(credential.credentialID)),
                    ResultValue(name: "authenticator-data", text: Base64URL.encode(credential.authenticatorData)),
                    ResultValue(name: "signature", text: Base64URL.encode(credential.signature)),
                    ResultValue(name: "user-handle", text: Base64URL.encode(credential.userHandle)),
                ]
                self?.resultMessage = "Signed in to \(credential.relyingPartyID) with a passkey"
            },
            completeRegistration: { [weak self] credential in
                self?.allResultValue = [
                    ResultValue(name: "credential-id", text: Base64URL.encode(credential.credentialID)),
                    ResultValue(name: "attestation-object", text: Base64URL.encode(credential.attestationObject)),
                ]
                self?.resultMessage = "Saved a passkey for \(credential.relyingPartyID)"
            },
            rejectExcludedRegistration: { [weak self] in self?.resultMessage = "Rejected: Hisar already has this passkey" }
        )
    )
}
#endif
