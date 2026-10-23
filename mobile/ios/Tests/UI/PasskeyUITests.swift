import CryptoKit
import XCTest

// Registers a passkey and signs in with it through the AutoFill preview (AutoFillPreviewView), then checks the
// attestation's public key and the ES256 signature over authenticatorData || clientDataHash as a relying party would.
final class PasskeyUITests: HisarUITestCase {
    private static let relyingPartyID = "example.com"

    func testRegisterPasskeyThenSignInWithIt() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_PASSKEY_INVITE_LINK")
        app.launch()
        _ = signUp(inviteLink: inviteLink, email: "ios-passkey@example.com", name: "iOS Passkey")
        addLogin(title: "Example", username: "passkey-user", password: "passkey-password-1", website: "login.example.com")

        let userHandle = Data("passkey-user-handle".utf8)
        let registrationClientDataHash = Data(SHA256.hash(data: Data(#"{"type":"webauthn.create","challenge":"cmVnaXN0ZXI","origin":"https://login.example.com","crossOrigin":false}"#.utf8)))
        launchPreviewAndUnlock([
            "-HisarAutoFillPasskeyRegistrationPreview", Self.relyingPartyID,
            "-HisarAutoFillPasskeyUserName", "passkey-user",
            "-HisarAutoFillPasskeyUserHandle", encodeBase64URL(userHandle),
            "-HisarAutoFillPasskeyClientDataHash", encodeBase64URL(registrationClientDataHash),
        ])
        let saveButton = app.buttons["autofill-save-passkey"]
        waitFor(saveButton)
        saveScreenshot("passkey-save")
        saveButton.tap()
        checkResult("Saved a passkey for example.com")
        let credentialID = try readResultValue("credential-id")
        let publicKey = try verifyAttestation(readResultValue("attestation-object"), credentialID: credentialID)

        let assertionClientDataHash = Data(SHA256.hash(data: Data(#"{"type":"webauthn.get","challenge":"c2lnbiBpbg","origin":"https://login.example.com","crossOrigin":false}"#.utf8)))
        launchPreviewAndUnlock([
            "-HisarAutoFillPasskeyPreview", Self.relyingPartyID,
            "-HisarAutoFillPasskeyClientDataHash", encodeBase64URL(assertionClientDataHash),
        ])
        let passkeyRow = app.buttons["autofill-passkey-Example"]
        waitFor(passkeyRow)
        saveScreenshot("passkey-picker")
        passkeyRow.tap()
        checkResult("Signed in to example.com with a passkey")
        XCTAssertEqual(try readResultValue("credential-id"), credentialID)
        XCTAssertEqual(try readResultValue("user-handle"), userHandle)
        let authenticatorData = try readResultValue("authenticator-data")
        XCTAssertEqual(authenticatorData.prefix(32), Data(SHA256.hash(data: Data(Self.relyingPartyID.utf8))))
        XCTAssertEqual(authenticatorData[32] & 0x04, 0, "an app unlock does not establish UV for this ceremony")
        let signature = try P256.Signing.ECDSASignature(derRepresentation: readResultValue("signature"))
        XCTAssertTrue(publicKey.isValidSignature(signature, for: authenticatorData + assertionClientDataHash))
        XCTAssertFalse(publicKey.isValidSignature(signature, for: authenticatorData + registrationClientDataHash), "the signature covers the request's client data hash")

        app.terminate()
        app.launchArguments = ["-HisarUITest"]
        app.launch()
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitFor(app.buttons["vault-Personal"])
        app.buttons["vault-Personal"].tap()
        app.buttons["item-Example"].tap()
        let passkeyItem = app.descendants(matching: .any)["passkey-example.com"]
        saveScreenshot("passkey-item-detail")
        app.buttons["delete-passkey-example.com"].tap()
        app.buttons["confirm-delete-passkey"].firstMatch.tap()
        XCTAssertTrue(passkeyItem.waitForNonExistence(timeout: 30), "the passkey is still listed after deleting it")
        XCTAssertEqual(app.staticTexts["username-value"].label, "passkey-user", "deleting the passkey keeps the login")
    }

    private func launchPreviewAndUnlock(_ allArgument: [String]) {
        app.terminate()
        app.launchArguments = ["-HisarUITest", "-HisarAutoFillPasskeyUserVerification", "discouraged"] + allArgument
        app.launch()
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
    }

    private func checkResult(_ expectedMessage: String, file: StaticString = #filePath, line: UInt = #line) {
        let result = app.descendants(matching: .any)["autofill-result"]
        waitFor(result, file: file, line: line)
        XCTAssertEqual(result.label, expectedMessage, file: file, line: line)
    }

    private func readResultValue(_ name: String) throws -> Data {
        let element = app.descendants(matching: .any)["autofill-result-\(name)"]
        return try XCTUnwrap(decodeBase64URL(element.label), "\(name) is not base64url: \(element.label)")
    }

    private func verifyAttestation(_ attestationObject: Data, credentialID: Data) throws -> P256.Signing.PublicKey {
        var attestationReader = CBORReader(data: attestationObject)
        guard case .map(let allEntry) = try attestationReader.readItem() else { throw CBORReader.InvalidError() }
        let allValueByKey = Dictionary(uniqueKeysWithValues: allEntry.compactMap { entry -> (String, CBORReader.Item)? in
            guard case .text(let key) = entry.key else { return nil }
            return (key, entry.value)
        })
        guard case .bytes(let authenticatorData) = allValueByKey["authData"] else { throw CBORReader.InvalidError() }
        XCTAssertEqual(authenticatorData[32] & 0x04, 0, "the simulator ceremony establishes presence without user verification")
        let credentialIDLength = Int(authenticatorData[53]) << 8 | Int(authenticatorData[54])
        XCTAssertEqual(authenticatorData[55..<(55 + credentialIDLength)], credentialID)
        var keyReader = CBORReader(data: authenticatorData.suffix(from: 55 + credentialIDLength))
        guard case .map(let allKeyEntry) = try keyReader.readItem() else { throw CBORReader.InvalidError() }
        let allKeyValue = Dictionary(uniqueKeysWithValues: allKeyEntry.compactMap { entry -> (Int, CBORReader.Item)? in
            guard case .integer(let key) = entry.key else { return nil }
            return (key, entry.value)
        })
        XCTAssertEqual(allKeyValue[3], .integer(-7), "ES256")
        guard case .bytes(let x) = allKeyValue[-2], case .bytes(let y) = allKeyValue[-3] else { throw CBORReader.InvalidError() }
        return try P256.Signing.PublicKey(x963Representation: Data([0x04]) + x + y)
    }

    private func encodeBase64URL(_ data: Data) -> String {
        data.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }

    private func decodeBase64URL(_ text: String) -> Data? {
        var base64 = text.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        base64 += String(repeating: "=", count: (4 - base64.count % 4) % 4)
        return Data(base64Encoded: base64)
    }
}

// Reads the few CBOR shapes of a "none" attestation: maps, byte and text strings, small integers.
struct CBORReader {
    struct InvalidError: Error {}

    indirect enum Item: Equatable {
        case integer(Int)
        case bytes(Data)
        case text(String)
        case map([Entry])
    }

    struct Entry: Equatable {
        var key: Item
        var value: Item
    }

    private let allByte: [UInt8]
    private var offset = 0

    init(data: Data) {
        allByte = Array(data)
    }

    private mutating func readByte() throws -> UInt8 {
        guard offset < allByte.count else { throw InvalidError() }
        defer { offset += 1 }
        return allByte[offset]
    }

    private mutating func readArgument(_ additionalInformation: UInt8) throws -> Int {
        switch additionalInformation {
        case 0..<24: return Int(additionalInformation)
        case 24: return Int(try readByte())
        case 25: return Int(try readByte()) << 8 | Int(try readByte())
        default: throw InvalidError()
        }
    }

    mutating func readItem() throws -> Item {
        let initialByte = try readByte()
        let argument = try readArgument(initialByte & 0x1f)
        switch initialByte >> 5 {
        case 0: return .integer(argument)
        case 1: return .integer(-1 - argument)
        case 2, 3:
            guard offset + argument <= allByte.count else { throw InvalidError() }
            let value = Data(allByte[offset..<(offset + argument)])
            offset += argument
            if initialByte >> 5 == 2 { return .bytes(value) }
            guard let text = String(data: value, encoding: .utf8) else { throw InvalidError() }
            return .text(text)
        case 5:
            var allEntry: [Entry] = []
            for _ in 0..<argument {
                allEntry.append(Entry(key: try readItem(), value: try readItem()))
            }
            return .map(allEntry)
        default:
            throw InvalidError()
        }
    }
}
