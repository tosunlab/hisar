import XCTest
@testable import Hisar

final class VaultLogicTests: XCTestCase {
    private func makeReference(_ title: String, username: String = "", allURL: [String] = [], category: String = "login", vaultName: String = "Personal") -> VaultItemReference {
        VaultItemReference(vaultID: vaultName.lowercased(), vaultName: vaultName, item: ItemDetail(id: title.lowercased(), category: category, title: title, username: username, allURL: allURL))
    }

    func testFilterMatchesEveryWordAcrossFields() {
        let allItem = [
            makeReference("GitHub", username: "alice", allURL: ["https://github.com"]),
            makeReference("Netflix", username: "family@example.com", vaultName: "Family"),
            makeReference("Bank", username: "alice"),
        ]
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "").count, 3)
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "ALICE").map(\.item.title), ["GitHub", "Bank"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "alice github.com").map(\.item.title), ["GitHub"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "family").map(\.item.title), ["Netflix"])
        XCTAssertTrue(VaultLogic.filterAllItem(allItem, searchText: "nothing").isEmpty)
    }

    func testSortByTitleThenUsername() {
        let allItem = [makeReference("b"), makeReference("A", username: "zed"), makeReference("a", username: "amy")]
        XCTAssertEqual(VaultLogic.sortAllItem(allItem).map(\.item.username), ["amy", "zed", ""])
    }

    func testOpenableURLAddsHTTPSAndRejectsOtherSchemes() {
        XCTAssertEqual(VaultLogic.convertToOpenableURL("github.com")?.absoluteString, "https://github.com")
        XCTAssertEqual(VaultLogic.convertToOpenableURL(" http://router.local:8080/x ")?.absoluteString, "http://router.local:8080/x")
        XCTAssertNil(VaultLogic.convertToOpenableURL("javascript://alert(1)"))
        XCTAssertNil(VaultLogic.convertToOpenableURL(""))
        XCTAssertEqual(VaultLogic.extractHost("HTTPS://Accounts.Google.com/signin"), "accounts.google.com")
    }

    func testCredentialIdentitiesSkipNonLoginsAndEmptyUsernames() {
        let allItem = [
            makeReference("GitHub", username: "alice", allURL: ["https://github.com", "github.com/login", "gist.github.com"]),
            makeReference("No user", allURL: ["https://example.com"]),
            makeReference("Server key", username: "root", allURL: ["https://server.example.com"], category: "ssh-key"),
            makeReference("No URL", username: "bob"),
        ]
        let allIdentity = VaultLogic.buildAllCredentialIdentity(allItem)
        XCTAssertEqual(allIdentity, [
            CredentialIdentityRecord(host: "github.com", username: "alice", recordIdentifier: "personal/github"),
            CredentialIdentityRecord(host: "gist.github.com", username: "alice", recordIdentifier: "personal/github"),
        ])
    }

    func testSignInLinkFromEmergencyKit() {
        let link = SignInLink.parse("hisar://sign-in?email=alice%40example.com&secretKey=H1-ABC123-AAAAA&server=https%3A%2F%2Fhisar.example.com")
        XCTAssertEqual(link, SignInLink(server: "https://hisar.example.com", email: "alice@example.com", secretKey: "H1-ABC123-AAAAA"))
        XCTAssertNil(SignInLink.parse("https://hisar.example.com/invite/abc"))
        XCTAssertNil(SignInLink.parse("hisar://sign-in"))
        XCTAssertNil(SignInLink.parse("hisar://other?server=x"))
    }

    func testSecretKeyFormatting() {
        XCTAssertEqual(VaultLogic.formatSecretKey(" h1-abc123 -aaaaa\n"), "H1-ABC123-AAAAA")
    }

    func testRecordIdentifierRoundTrip() {
        let recordIdentifier = LoginMatch.buildRecordIdentifier(vaultID: "vault-1", itemID: "item/with/slash")
        XCTAssertEqual(LoginMatch.parseRecordIdentifier(recordIdentifier)?.vaultID, "vault-1")
        XCTAssertEqual(LoginMatch.parseRecordIdentifier(recordIdentifier)?.itemID, "item/with/slash")
        XCTAssertNil(LoginMatch.parseRecordIdentifier("nothing"))
        XCTAssertNil(LoginMatch.parseRecordIdentifier("/item"))
    }
}

final class ModelDecodingTests: XCTestCase {
    func testDecodesGoStatus() throws {
        let json = #"{"state":"unlocked","server":"https://hisar.example.com","email":"a@example.com","name":"A","biometricAvailable":true,"biometricEnabled":true,"biometricReady":false,"isOffline":true}"#
        let status = try JSONDecoder().decode(VaultStatus.self, from: Data(json.utf8))
        XCTAssertTrue(status.isUnlocked)
        XCTAssertTrue(status.isOffline)
        XCTAssertTrue(status.biometricEnabled)
        XCTAssertFalse(status.biometricReady)
    }

    func testDecodesItemSummaryWithNullFields() throws {
        let json = #"[{"id":"i1","revision":"r","category":"","title":"T","username":"u","urls":["a.com"],"password":"","notes":"","fields":null,"publicKey":"","fingerprint":"","keyType":""}]"#
        let allItem = try JSONDecoder().decode([ItemDetail].self, from: Data(json.utf8))
        XCTAssertEqual(allItem.first?.allURL, ["a.com"])
        XCTAssertEqual(allItem.first?.allField, [])
        XCTAssertEqual(allItem.first?.isLogin, true)
    }

    func testEncodesItemWithGoKeys() throws {
        let item = ItemDetail(title: "T", allURL: ["a.com"], allField: [ItemField(name: "PIN", value: "1", concealed: true)])
        let object = try JSONSerialization.jsonObject(with: JSONEncoder().encode(item)) as? [String: Any]
        XCTAssertEqual(object?["urls"] as? [String], ["a.com"])
        XCTAssertEqual((object?["fields"] as? [[String: Any]])?.first?["concealed"] as? Bool, true)
        XCTAssertEqual(object?["id"] as? String, "")
    }

    func testDecodesLoginsAndSignUpResult() throws {
        let allLogin = try JSONDecoder().decode([LoginMatch].self, from: Data(#"[{"vaultID":"v","itemID":"i","title":"T","username":"u","vaultName":"Personal"}]"#.utf8))
        XCTAssertEqual(allLogin.first?.id, "v/i")
        let result = try JSONDecoder().decode(SignUpResult.self, from: Data(#"{"status":{"state":"unlocked"},"secretKey":"H1-X"}"#.utf8))
        XCTAssertEqual(result.secretKey, "H1-X")
        XCTAssertTrue(result.status.isUnlocked)
    }
}
