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

    func testOneTimeCodeIdentitiesForLoginsWithACode() {
        var withCode = makeReference("GitHub", username: "alice", allURL: ["https://github.com", "github.com/login"])
        withCode.item.hasOneTimePassword = true
        var withCodeWithoutUsername = makeReference("Router", allURL: ["https://router.example.com"])
        withCodeWithoutUsername.item.hasOneTimePassword = true
        var sshKeyWithCode = makeReference("Server key", username: "root", allURL: ["https://server.example.com"], category: "ssh-key")
        sshKeyWithCode.item.hasOneTimePassword = true
        XCTAssertEqual(VaultLogic.buildAllCredentialIdentity([withCode, withCodeWithoutUsername, sshKeyWithCode]), [
            CredentialIdentityRecord(host: "github.com", username: "alice", recordIdentifier: "personal/github"),
            CredentialIdentityRecord(kind: .oneTimeCode, host: "github.com", username: "alice", recordIdentifier: "personal/github"),
            CredentialIdentityRecord(kind: .oneTimeCode, host: "router.example.com", username: "Router", recordIdentifier: "personal/router"),
        ])
    }

    func testOneTimeCodeFormattingAndCountdown() throws {
        let code = OneTimeCode(code: "123456", expiresAt: "2026-10-09T12:00:30Z", period: 30)
        XCTAssertEqual(code.formattedCode, "123 456")
        XCTAssertEqual(OneTimeCode(code: "12345678", expiresAt: "", period: 30).formattedCode, "1234 5678")
        XCTAssertEqual(OneTimeCode(code: "1234567", expiresAt: "", period: 30).formattedCode, "1234 567")
        let now = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-09T12:00:20Z"))
        XCTAssertEqual(code.countSecondsLeft(at: now.addingTimeInterval(0.4)), 10)
        XCTAssertEqual(code.countSecondsLeft(at: now.addingTimeInterval(11)), 0)
        XCTAssertEqual(OneTimeCode(code: "1", expiresAt: "not a date", period: 30).countSecondsLeft(at: now), 0)
    }

    func testSignInLinkFromEmergencyKit() {
        let link = SignInLink.parse("hisar://sign-in?email=alice%40example.com&secretKey=H1-ABC123-AAAAA&server=https%3A%2F%2Fhisar.example.com")
        XCTAssertEqual(link, SignInLink(server: "https://hisar.example.com", email: "alice@example.com", secretKey: "H1-ABC123-AAAAA"))
        XCTAssertNil(SignInLink.parse("https://hisar.example.com/invite/abc"))
        XCTAssertNil(SignInLink.parse("hisar://sign-in"))
        XCTAssertNil(SignInLink.parse("hisar://other?server=x"))
    }

    func testInviteLinkFromInvitePage() {
        let expectedByLink = [
            "hisar://invite/abc_DEF-123": "abc_DEF-123",
            " hisar://invite/abc_DEF-123?server=https%3A%2F%2Fapi.tryhisar.com ": "abc_DEF-123",
            "HISAR://INVITE/abc?server=https%3A%2F%2Fhisar.example.com": "https://hisar.example.com/invite/abc",
            "hisar://invite/abc?server=http%3A%2F%2F127.0.0.1%3A19400": "http://127.0.0.1:19400/invite/abc",
            "hisar://invite/abc?server=hisar.example.com": "https://hisar.example.com/invite/abc",
            "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com%2F&other=yes": "https://hisar.example.com/invite/abc",
        ]
        for (link, expected) in expectedByLink {
            XCTAssertEqual(InviteLink.parse(link), InviteLink(invite: expected), link)
        }
        for link in ["", "hisar://invite/", "hisar://invite/a/b", "hisar://sign-in?server=x", "https://api.tryhisar.com/invite/abc", "hisar://invite/a%20b", "hisar://invite/abc?server=ftp%3A%2F%2Fx"] {
            XCTAssertNil(InviteLink.parse(link), link)
        }
        XCTAssertNil(SignInLink.parse("hisar://invite/abc"))
    }

    func testStatusDecodesUpdateRequirement() throws {
        let json = #"{"state":"locked","updateRequired":{"currentVersion":"","minimumVersion":"0.2.0","updateURL":"https://tryhisar.com/download"}}"#
        let status = try JSONDecoder().decode(VaultStatus.self, from: Data(json.utf8))
        XCTAssertEqual(status.updateRequired, UpdateRequirement(currentVersion: "", minimumVersion: "0.2.0", updateURL: "https://tryhisar.com/download"))
        XCTAssertEqual(status.updateRequired?.currentVersionDescription, "a development build")
        XCTAssertNil(try JSONDecoder().decode(VaultStatus.self, from: Data(#"{"state":"locked","updateRequired":null}"#.utf8)).updateRequired)
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
        XCTAssertEqual(allItem.first?.hasOneTimePassword, false)
        let withCode = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"id":"i2","oneTimePassword":"","hasOneTimePassword":true}"#.utf8))
        XCTAssertTrue(withCode.hasOneTimePassword)
    }

    func testEncodesItemWithGoKeys() throws {
        let item = ItemDetail(title: "T", allURL: ["a.com"], allField: [ItemField(name: "PIN", value: "1", concealed: true)])
        let object = try JSONSerialization.jsonObject(with: JSONEncoder().encode(item)) as? [String: Any]
        XCTAssertEqual(object?["urls"] as? [String], ["a.com"])
        XCTAssertEqual((object?["fields"] as? [[String: Any]])?.first?["concealed"] as? Bool, true)
        XCTAssertEqual(object?["id"] as? String, "")
        let withCode = ItemDetail(title: "T", oneTimePassword: "JBSWY3DPEHPK3PXP")
        let encodedWithCode = try JSONSerialization.jsonObject(with: JSONEncoder().encode(withCode)) as? [String: Any]
        XCTAssertEqual(encodedWithCode?["oneTimePassword"] as? String, "JBSWY3DPEHPK3PXP")
    }

    func testDecodesLoginsAndSignUpResult() throws {
        let allLogin = try JSONDecoder().decode([LoginMatch].self, from: Data(#"[{"vaultID":"v","itemID":"i","title":"T","username":"u","vaultName":"Personal"},{"vaultID":"v","itemID":"j","title":"T","username":"u","vaultName":"Personal","hasOneTimePassword":true}]"#.utf8))
        XCTAssertEqual(allLogin.first?.id, "v/i")
        XCTAssertEqual(allLogin.map(\.hasOneTimePassword), [false, true])
        let credential = try JSONDecoder().decode(LoginCredential.self, from: Data(#"{"username":"u","password":"p","hasOneTimePassword":true}"#.utf8))
        XCTAssertEqual(credential, LoginCredential(username: "u", password: "p", hasOneTimePassword: true))
        let code = try JSONDecoder().decode(OneTimeCode.self, from: Data(#"{"code":"287082","expiresAt":"1970-01-01T00:01:00Z","period":30}"#.utf8))
        XCTAssertEqual(code.expiryDate, Date(timeIntervalSince1970: 60))
        let result = try JSONDecoder().decode(SignUpResult.self, from: Data(#"{"status":{"state":"unlocked"},"secretKey":"H1-X"}"#.utf8))
        XCTAssertEqual(result.secretKey, "H1-X")
        XCTAssertTrue(result.status.isUnlocked)
    }
}
