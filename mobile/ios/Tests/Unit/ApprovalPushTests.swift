import CryptoKit
import XCTest
@testable import Hisar

final class ApprovalPushTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let accountId = "ACC-123"
    private let signingKey = Curve25519.Signing.PrivateKey()

    private var verifier: ApprovalPushVerifier {
        ApprovalPushVerifier(accountId: accountId, signingPublicKey: Base64URL.encode(signingKey.publicKey.rawRepresentation))
    }

    private func expiry(_ offset: TimeInterval) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: now.addingTimeInterval(offset))
    }

    /// signedMessage builds the decrypted push a Mac encrypts to the phone: the preview bytes and the account's
    /// signature over them, inside the {"kind":"approval","approval":{…}} envelope.
    private func signedMessage(_ preview: [String: Any], key: Curve25519.Signing.PrivateKey? = nil, account: String? = nil) -> Data {
        let previewBytes = try! JSONSerialization.data(withJSONObject: preview)
        let signature = try! (key ?? signingKey).signature(
            for: ApprovalPushText.encodeContext("hisar/v1/approval-preview", accountId: account ?? accountId, preview: previewBytes)
        )
        let message: [String: Any] = [
            "kind": "approval",
            "approval": ["preview": Base64URL.encode(previewBytes), "signature": Base64URL.encode(signature)],
        ]
        return try! JSONSerialization.data(withJSONObject: message)
    }

    private func titleAndBody(_ preview: [String: Any]) -> (title: String, body: String)? {
        ApprovalPushText.titleAndBody(decrypted: signedMessage(preview), verifier: verifier, now: now)
    }

    func testAnApprovalPushNamesTheMacTheAppAndTheRequest() {
        // An SSH key use.
        let ssh = titleAndBody([
            "requestId": "req-1", "requesterDeviceName": "MacBook Pro", "expiresAt": expiry(120),
            "kind": "ssh", "appName": "Terminal", "purpose": "push to git.tryhisar.com", "keyName": "GitHub",
        ])
        XCTAssertEqual(ssh?.title, "MacBook Pro is asking to use “GitHub”")
        XCTAssertEqual(ssh?.body, "Terminal wants to push to git.tryhisar.com")

        // The command-line tool.
        let cli = titleAndBody([
            "requestId": "req-2", "requesterDeviceName": "MacBook Pro", "expiresAt": expiry(120),
            "kind": "cli", "appName": "Terminal", "purpose": "run “git push”",
        ])
        XCTAssertEqual(cli?.title, "MacBook Pro is asking to use Hisar from the command line")
        XCTAssertEqual(cli?.body, "Terminal wants to run “git push”")

        // A launcher such as Raycast.
        let launcher = titleAndBody([
            "requestId": "req-3", "requesterDeviceName": "MacBook Pro", "expiresAt": expiry(120),
            "kind": "launcher", "appName": "Raycast", "purpose": "create a share link",
        ])
        XCTAssertEqual(launcher?.title, "MacBook Pro is asking to let Raycast use Hisar")
        XCTAssertEqual(launcher?.body, "Raycast wants to create a share link")

        // A forwarded SSH request names the computer it came from, not the local app.
        let forwarded = titleAndBody([
            "requestId": "req-4", "requesterDeviceName": "MacBook Pro", "expiresAt": expiry(120),
            "kind": "ssh", "appName": "Terminal", "purpose": "push to git.tryhisar.com", "keyName": "GitHub",
            "isForwarded": true, "forwardedFromHost": "laptop.local",
        ])
        XCTAssertEqual(forwarded?.title, "MacBook Pro is asking to use “GitHub”")
        XCTAssertEqual(forwarded?.body, "laptop.local wants to push to git.tryhisar.com")
    }

    func testAnApprovalPushWithoutAValidPreviewKeepsTheSentence() {
        // No approval part.
        XCTAssertNil(ApprovalPushText.titleAndBody(decrypted: Data(#"{"kind":"approval"}"#.utf8), verifier: verifier, now: now))

        // An expired request.
        XCTAssertNil(titleAndBody([
            "requestId": "req-1", "requesterDeviceName": "MacBook Pro", "expiresAt": expiry(-1),
            "kind": "ssh", "appName": "Terminal", "purpose": "push to git.tryhisar.com", "keyName": "GitHub",
        ]))

        // A signature by another key.
        let otherKey = signedMessage(["requestId": "req-1", "expiresAt": expiry(120), "kind": "ssh"], key: Curve25519.Signing.PrivateKey())
        XCTAssertNil(ApprovalPushText.titleAndBody(decrypted: otherKey, verifier: verifier, now: now))

        // A signature for another account.
        let otherAccount = signedMessage(["requestId": "req-1", "expiresAt": expiry(120), "kind": "ssh"], account: "someone-else")
        XCTAssertNil(ApprovalPushText.titleAndBody(decrypted: otherAccount, verifier: verifier, now: now))

        // An empty kind.
        XCTAssertNil(titleAndBody(["requestId": "req-1", "expiresAt": expiry(120), "kind": "  "]))

        // Garbage bytes.
        XCTAssertNil(ApprovalPushText.titleAndBody(decrypted: Data("not json".utf8), verifier: verifier, now: now))

        // A sharing push keeps its own path, not this one.
        XCTAssertNil(ApprovalPushText.titleAndBody(decrypted: Data(#"{"kind":"sharing","invitation":{}}"#.utf8), verifier: verifier, now: now))

        // Before the first unlock the verifier is empty, so nothing is shown.
        XCTAssertNil(ApprovalPushText.titleAndBody(
            decrypted: signedMessage(["requestId": "req-1", "expiresAt": expiry(120), "kind": "ssh"]),
            verifier: ApprovalPushVerifier(accountId: "", signingPublicKey: ""), now: now
        ))
    }

    func testAnApprovalPreviewVerifiesWithTheAccountsKey() {
        // The vector from core/crypt's SignApprovalPreview (vectors_test.go).
        let publicKey = Data(hexString: "2543b92ff1095511476adc8369db6ddc933665a11978dda1404ee1066ca9559d")
        let preview = Data(#"{"requestId":"req-1","kind":"ssh"}"#.utf8)
        let signature = Data(hexString: "a0e0f2aaa514dc42d0806710b4037c8e2e077ca23fb0b53b4bb8db37a00c609d54fc02d9dcce7ed34a6147b6e68f035196e1f42e3117f3ee86d1c5c8ce345808")

        let context = ApprovalPushText.encodeContext("hisar/v1/approval-preview", accountId: "ABC123", preview: preview)
        XCTAssertEqual(
            context.map { String(format: "%02x", $0) }.joined(),
            "68697361722f76312f617070726f76616c2d7072657669657700000006414243313233000000227b22726571756573744964223a227265712d31222c226b696e64223a22737368227d"
        )
        XCTAssertTrue(ApprovalPushText.verify(publicKeyData: publicKey, accountId: "ABC123", preview: preview, signature: signature))
        XCTAssertFalse(ApprovalPushText.verify(publicKeyData: publicKey, accountId: "OTHER1", preview: preview, signature: signature))
        XCTAssertFalse(ApprovalPushText.verify(
            publicKeyData: publicKey, accountId: "ABC123", preview: Data(#"{"requestId":"req-1","kind":"cli"}"#.utf8), signature: signature
        ))
    }
}

private extension Data {
    init(hexString: String) {
        var data = Data()
        var index = hexString.startIndex
        while index < hexString.endIndex {
            let next = hexString.index(index, offsetBy: 2)
            data.append(UInt8(hexString[index..<next], radix: 16)!)
            index = next
        }
        self = data
    }
}
