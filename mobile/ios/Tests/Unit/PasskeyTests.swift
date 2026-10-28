import XCTest
@testable import Hisar

@MainActor
final class PasskeyTests: XCTestCase {
    func testPasskeyDeletionDescribesVaultWideConsequences() {
        for userName in ["alice", ""] {
            let passkey = ItemPasskey(credentialID: "key", relyingPartyID: "github.com", userName: userName, userDisplayName: "Alice")
            let account = userName.isEmpty ? "Alice" : "alice"
            let prefix = "You can no longer sign in to github.com as \(account) with it. "
            for type in ["personal", "shared", "organization"] {
                let vault = VaultSummary(id: "v", name: "Production", type: type, role: "owner", itemCount: 0)
                let consequence = type == "personal"
                    ? "It is removed from all your devices. Make sure you can sign in another way first."
                    : "It is removed from Production for everyone with access. Make sure they can sign in another way first."
                XCTAssertEqual(passkey.deletionMessage(vault: vault), prefix + consequence)
            }
        }
    }

    func testBase64URLRoundTrip() {
        let allValue = [Data(), Data([0xfb]), Data([0xfb, 0xff]), Data([0xfb, 0xff, 0x01]), Data((0..<64).map { UInt8($0 * 4) })]
        for value in allValue {
            let encoded = Base64URL.encode(value)
            XCTAssertFalse(encoded.contains("+") || encoded.contains("/") || encoded.contains("="), encoded)
            XCTAssertEqual(Base64URL.decode(encoded), value)
        }
        XCTAssertEqual(Base64URL.encode(Data([0xfb, 0xff])), "-_8")
        XCTAssertNil(Base64URL.decode("not base64!"))
    }

    func testDecodesItemPasskeysFromGo() throws {
        let json = #"{"id":"i1","title":"GitHub","passkeys":[{"credentialId":"AQID","relyingPartyId":"github.com","userName":"alice","userDisplayName":"Alice","createdAt":"2026-10-09T12:00:00Z"}]}"#
        let item = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertEqual(item.allPasskey, [ItemPasskey(credentialID: "AQID", relyingPartyID: "github.com", userName: "alice", userDisplayName: "Alice", createdAt: "2026-10-09T12:00:00Z")])
        XCTAssertEqual(item.allPasskey.first?.createdDate, Date(timeIntervalSince1970: 1_791_547_200))
        let withoutPasskeys = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"id":"i1","passkeys":null}"#.utf8))
        XCTAssertEqual(withoutPasskeys.allPasskey, [])
        let encoded = try JSONSerialization.jsonObject(with: JSONEncoder().encode(item)) as? [String: Any]
        XCTAssertEqual((encoded?["passkeys"] as? [[String: Any]])?.first?["relyingPartyId"] as? String, "github.com")
    }

    func testDecodesStoredPasskeysAndRegistrationAnswers() throws {
        let allPasskey = try JSONDecoder().decode([StoredPasskey].self, from: Data(#"[{"credentialId":"AQID","relyingPartyId":"github.com","userName":"alice","userDisplayName":"","userHandle":"YWxpY2U","vaultID":"v","itemID":"i","title":"GitHub"}]"#.utf8))
        XCTAssertEqual(allPasskey.first?.recordIdentifier, "v/i")
        XCTAssertEqual(allPasskey.first.flatMap { Base64URL.decode($0.userHandle) }, Data("alice".utf8))
        let options = try JSONDecoder().decode(PasskeyRegistrationOptions.self, from: Data(#"{"isExcluded":false,"logins":[{"vaultID":"v","itemID":"i","title":"GitHub","username":"alice","vaultName":"Personal","hasOneTimePassword":false}]}"#.utf8))
        XCTAssertEqual(options.allLogin.map(\.id), ["v/i"])
        let registration = try JSONDecoder().decode(PasskeyRegistrationResult.self, from: Data(#"{"credentialId":"AQID","attestationObject":"o2Nm","authenticatorData":"AA","vaultID":"v","itemID":"i"}"#.utf8))
        let credential = PasskeyRegistrationCredential(relyingPartyID: "github.com", clientDataHash: Data([9]), result: registration)
        XCTAssertEqual(credential?.credentialID, Data([1, 2, 3]))
        XCTAssertEqual(credential?.attestationObject, Data([0xa3, 0x63, 0x66]))
        let assertion = PasskeyAssertionResult(credentialID: "AQID", authenticatorData: "AA", signature: "MEU", userHandle: "YWxpY2U")
        XCTAssertEqual(PasskeyAssertionCredential(relyingPartyID: "github.com", clientDataHash: Data([9]), result: assertion)?.userHandle, Data("alice".utf8))
        XCTAssertNil(PasskeyAssertionCredential(relyingPartyID: "github.com", clientDataHash: Data([9]), result: PasskeyAssertionResult(credentialID: "!", authenticatorData: "AA", signature: "MEU", userHandle: "")))
    }

    func testRegistrationNeedsES256() {
        var request = PasskeyRegistrationRequest(relyingPartyID: "github.com", userName: "alice", userHandle: Data([1]), clientDataHash: Data())
        XCTAssertTrue(request.isAlgorithmSupported, "an empty list leaves the choice to the authenticator")
        request.allSupportedAlgorithm = [-8, -257]
        XCTAssertFalse(request.isAlgorithmSupported)
        request.allSupportedAlgorithm = [-257, -7]
        XCTAssertTrue(request.isAlgorithmSupported)
    }

    func testPasskeyIdentitiesOnlyForListedLogins() {
        let allItem = [
            VaultItemReference(vaultID: "personal", vaultName: "Personal", item: ItemDetail(id: "github", title: "GitHub", username: "alice", allURL: ["https://github.com"])),
            VaultItemReference(vaultID: "personal", vaultName: "Personal", item: ItemDetail(id: "key", category: ItemDetail.categorySSHKey, title: "Key")),
        ]
        let allPasskey = [
            StoredPasskey(credentialID: "AQID", relyingPartyID: "github.com", userName: "alice", userHandle: "YWxpY2U", vaultID: "personal", itemID: "github", title: "GitHub"),
            StoredPasskey(credentialID: "AQID", relyingPartyID: "github.com", userName: "alice", userHandle: "YWxpY2U", vaultID: "personal", itemID: "github", title: "GitHub"),
            StoredPasskey(credentialID: "BAUG", relyingPartyID: "gone.example", userName: "x", userHandle: "eA", vaultID: "personal", itemID: "deleted", title: "Gone"),
            StoredPasskey(credentialID: "!", relyingPartyID: "github.com", userName: "bad", userHandle: "eA", vaultID: "personal", itemID: "github", title: "GitHub"),
            StoredPasskey(credentialID: "BwcH", relyingPartyID: "github.com", userName: "y", userHandle: "eA", vaultID: "personal", itemID: "key", title: "Key"),
        ]
        let allIdentity = VaultLogic.buildAllCredentialIdentity(allItem, allPasskey: allPasskey)
        XCTAssertEqual(allIdentity, [
            CredentialIdentityRecord(host: "github.com", username: "alice", recordIdentifier: "personal/github"),
            CredentialIdentityRecord(kind: .passkey, host: "github.com", username: "alice", recordIdentifier: "personal/github", credentialID: Data([1, 2, 3]), userHandle: Data("alice".utf8)),
        ])
    }

    func testAppPublishesPasskeyIdentitiesAndDeletesPasskeys() async {
        let backend = FakeBackend()
        backend.seedSignedIn()
        let identityStore = FakeCredentialIdentityStore()
        let model = AppModel(credentialIdentityStore: identityStore)
        await model.start { backend }?.value
        let passkeyIdentity = CredentialIdentityRecord(kind: .passkey, host: "github.com", username: "alice", recordIdentifier: "personal/github", credentialID: Data([1, 1, 1]), userHandle: Data("alice-handle".utf8))
        XCTAssertEqual(identityStore.allIdentity.filter { $0.kind == .passkey }, [passkeyIdentity])

        guard var item = await model.loadItem(vaultID: "personal", itemID: "github") else { return XCTFail("no item") }
        item.title = "GitHub work"
        let savedItem = await model.saveItem(vaultID: "personal", item: item)
        XCTAssertEqual(savedItem?.allPasskey.count, 1, "editing keeps the passkeys")
        XCTAssertEqual(identityStore.allIdentity.filter { $0.kind == .passkey }, [passkeyIdentity])

        let afterDelete = await model.deletePasskey(vaultID: "personal", itemID: "github", credentialID: FakeBackend.gitHubPasskey.credentialID)
        XCTAssertEqual(afterDelete?.allPasskey, [])
        XCTAssertTrue(identityStore.allIdentity.filter { $0.kind == .passkey }.isEmpty)
        XCTAssertEqual(model.listAllItemReference().first { $0.item.id == "github" }?.item.allPasskey, [])
        XCTAssertEqual(model.noticeMessage, "Passkey deleted.")
        XCTAssertTrue(identityStore.allIdentity.contains { $0.kind == .password && $0.host == "github.com" })

        let failedDelete = await model.deletePasskey(vaultID: "personal", itemID: "github", credentialID: "unknown")
        XCTAssertNil(failedDelete)
        XCTAssertEqual(model.errorMessage, "passkey not found")
    }

    func testDeletingAnItemRemovesItsPasskeyIdentities() async {
        let backend = FakeBackend()
        backend.seedSignedIn()
        let identityStore = FakeCredentialIdentityStore()
        let model = AppModel(credentialIdentityStore: identityStore)
        await model.start { backend }?.value
        XCTAssertTrue(identityStore.allIdentity.contains { $0.kind == .passkey })
        await model.deleteItem(vaultID: "personal", itemID: "github")
        XCTAssertFalse(identityStore.allIdentity.contains { $0.kind == .passkey })
    }
}
