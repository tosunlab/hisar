import CoreImage
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

    func testTagsAreListedWithCountsAndFilterItems() {
        var gitHub = makeReference("GitHub", username: "alice")
        gitHub.item.allTag = ["work", "dev"]
        var bank = makeReference("Bank", vaultName: "Family")
        bank.item.allTag = ["Finance"]
        var email = makeReference("Email")
        email.item.allTag = ["work"]
        let allItem = [gitHub, bank, email, makeReference("Untagged")]
        XCTAssertEqual(VaultLogic.listAllTag(allItem), [TagSummary(name: "dev", itemCount: 1), TagSummary(name: "Finance", itemCount: 1), TagSummary(name: "work", itemCount: 2)])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, tag: "work").map(\.item.title), ["GitHub", "Email"])
        XCTAssertTrue(VaultLogic.filterAllItem(allItem, tag: "nothing").isEmpty)
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "fin").map(\.item.title), ["Bank"])
        XCTAssertTrue(VaultLogic.listAllTag([makeReference("Untagged")]).isEmpty)
    }

    func testSplitTagsDropsBlanksAndRepeats() {
        XCTAssertEqual(ItemDetail.splitTags(" work, dev ,, work ,"), ["work", "dev"])
        XCTAssertEqual(ItemDetail.splitTags("   "), [])
    }

    func testItemDetailDecodesTagsAndDefaultsToNone() throws {
        let tagged = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"id":"1","title":"GitHub","tags":["work"]}"#.utf8))
        XCTAssertEqual(tagged.allTag, ["work"])
        let untagged = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"id":"2","title":"Bank"}"#.utf8))
        XCTAssertEqual(untagged.allTag, [])
        let encoded = try JSONSerialization.jsonObject(with: JSONEncoder().encode(tagged)) as? [String: Any]
        XCTAssertEqual(encoded?["tags"] as? [String], ["work"])
    }

    func testItemDetailDecodesAndroidAppsAndDefaultsToNone() throws {
        let json = #"{"id":"1","title":"Bank","urls":["https://bank.example"],"androidApps":[{"packageName":"com.bank.app","certificateSha256":"14:6D:E9"}]}"#
        let linked = try JSONDecoder().decode(ItemDetail.self, from: Data(json.utf8))
        XCTAssertEqual(linked.allAndroidApp, [ItemAndroidApp(packageName: "com.bank.app", certificateSHA256: "14:6D:E9")])
        XCTAssertEqual(linked.allURL, ["https://bank.example"])
        let older = try JSONDecoder().decode(ItemDetail.self, from: Data(#"{"id":"2","title":"Bank","androidApps":null}"#.utf8))
        XCTAssertEqual(older.allAndroidApp, [])
    }

    func testFilterFindsAnIdentityByItsNameAndEmail() {
        var passport = makeReference("Passport", category: ItemDetail.categoryIdentity)
        var identity = ItemIdentity()
        identity.firstName = "Alice"
        identity.lastName = "Example"
        passport.item.identity = identity
        var work = makeReference("Work", category: ItemDetail.categoryIdentity)
        identity = ItemIdentity()
        identity.email = "bob@example.com"
        work.item.identity = identity
        let allItem = [passport, work, makeReference("GitHub")]
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "alice example").map(\.item.title), ["Passport"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "bob@example").map(\.item.title), ["Work"])
    }

    func testFilterFindsAnIdentityByItsAddress() {
        var alice = makeReference("Alice", category: ItemDetail.categoryIdentity)
        var identity = ItemIdentity()
        identity.firstName = "Alice"
        identity.postalAddress = ItemAddress(street: "1 Main St", city: "Springfield", country: "US")
        alice.item.identity = identity
        let allItem = [alice, makeReference("GitHub")]
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "springfield").map(\.item.title), ["Alice"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "main st").map(\.item.title), ["Alice"])
    }

    func testFilterFindsAnAPICredentialByItsServiceKeyIDAndHost() {
        var stripe = makeReference("Billing key", category: ItemDetail.categoryAPICredential)
        var credential = ItemAPICredential()
        credential.service = "Stripe"
        credential.keyID = "key_live_1"
        credential.host = "api.stripe.com"
        stripe.item.apiCredential = credential
        let allItem = [stripe, makeReference("GitHub")]
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "stripe").map(\.item.title), ["Billing key"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "api.stripe").map(\.item.title), ["Billing key"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "key_live").map(\.item.title), ["Billing key"])
    }

    func testFilterFindsASoftwareLicenseByItsProductVersionAndOwnerButNotItsKey() {
        var sketch = makeReference("Design app", category: ItemDetail.categorySoftwareLicense)
        sketch.item.softwareLicense = ItemSoftwareLicense(product: "Sketch", version: "v100", licenseKey: "TEST-LICENSE-KEY", licensedTo: "Alice Example", email: "alice@example.com")
        let allItem = [sketch, makeReference("GitHub")]
        for searchText in ["sketch", "v100", "alice example", "alice@"] {
            XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: searchText).map(\.item.title), ["Design app"], searchText)
        }
        XCTAssertTrue(VaultLogic.filterAllItem(allItem, searchText: "test-license").isEmpty)
        XCTAssertTrue(ItemCategoryDestination.softwareLicenses.includes(sketch.item))
    }

    func testFilterFindsAnAddressByItsStreetCityAndCountry() {
        var home = makeReference("Home", category: ItemDetail.categoryAddress)
        home.item.address = ItemAddress(street: "1 Main St", city: "Springfield", country: "US")
        let allItem = [home, makeReference("GitHub")]
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "springfield").map(\.item.title), ["Home"])
        XCTAssertEqual(VaultLogic.filterAllItem(allItem, searchText: "main st").map(\.item.title), ["Home"])
        XCTAssertEqual(ItemCategoryDestination.addresses.includes(home.item), true)
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

    func testCredentialIdentitiesOfferAServerOnItsAdminConsole() {
        var server = makeReference("Build server", username: "admin", allURL: ["https://console.example.com"], category: ItemDetail.categoryServer)
        server.item.server = ItemServer(host: "build.example.com", port: "2222")
        let note = makeReference("Notes", username: "admin", allURL: ["https://notes.example.com"], category: ItemDetail.categorySecureNote)
        XCTAssertEqual(VaultLogic.buildAllCredentialIdentity([server, note]), [
            CredentialIdentityRecord(host: "console.example.com", username: "admin", recordIdentifier: "personal/build server"),
        ])
        XCTAssertEqual(VaultLogic.filterAllItem([server, note], searchText: "build.example").map(\.item.title), ["Build server"])
        XCTAssertEqual(VaultLogic.filterAllItem([server, note], searchText: "2222").map(\.item.title), ["Build server"])
        XCTAssertEqual(VaultLogic.filterAllItem([server, note], searchText: "build.example.com:2222").map(\.item.title), ["Build server"])
        var database = makeReference("Database", category: ItemDetail.categoryServer)
        database.item.server = ItemServer(host: "2001:db8::1", port: "5432")
        XCTAssertEqual(VaultLogic.filterAllItem([server, database], searchText: "[2001:db8::1]:5432").map(\.item.title), ["Database"])
        XCTAssertTrue(ItemCategoryDestination.servers.includes(server.item))
        XCTAssertFalse(ItemCategoryDestination.logins.includes(server.item))
    }

    func testDatabasesAreFoundByTheirEngineAddressAndName() {
        var database = makeReference("Prod db", username: "app", category: ItemDetail.categoryDatabase)
        database.item.database = ItemDatabase(engine: "postgresql", engineName: "PostgreSQL", host: "db.example.com", port: "5432", name: "orders")
        let note = makeReference("Notes", category: ItemDetail.categorySecureNote)
        for searchText in ["postgres", "db.example.com:5432/orders", "5432", "orders"] {
            XCTAssertEqual(VaultLogic.filterAllItem([database, note], searchText: searchText).map(\.item.title), ["Prod db"], searchText)
        }
        XCTAssertTrue(VaultLogic.buildAllCredentialIdentity([database]).isEmpty)
        XCTAssertTrue(ItemCategoryDestination.databases.includes(database.item))
        XCTAssertFalse(ItemCategoryDestination.logins.includes(database.item))
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

    func testPlainTextEmergencyKitPreservesBytesAndRejectsInvalidPayloads() {
        let payload = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL2hpc2FyLmV4YW1wbGUvYT94PW9uZSt0d29cdTAwMjZ5PSUyMCIsImVtYWlsIjoiYWxwZXIra2l0QHRyeWhpc2FyLmNvbSIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
        XCTAssertEqual(SignInLink.parse(payload), SignInLink(server: "https://hisar.example/a?x=one+two&y=%20", email: "alper+kit@tryhisar.com", secretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"))
        XCTAssertNil(InviteLink.parse(payload))
        for invalid in [payload + "=", payload + "\n", payload.replacingOccurrences(of: "HISARKIT1 ", with: "HISARKIT2 "), "HISARKIT1 e30", "HISARKIT1:", "HISARKIT1 _w"] {
            XCTAssertNil(SignInLink.parse(invalid), invalid)
        }
        for json in [#"{"server":"https://h","email":"a@b","secretKey":1}"#, #"{"server":"https://h","email":"a@b","secretKey":"H1-INVALID"}"#, #"{"server":"javascript:bad","email":"a@b","secretKey":"H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"}"#] {
            let encoded = Data(json.utf8).base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
            XCTAssertNil(SignInLink.parse("HISARKIT1 " + encoded))
        }
        let unicodeJSON = #"{"server":"https://hisar.example/İstanbul/東京","email":"élève+🔐@example.test","secretKey":"H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"}"#
        func encode(_ data: Data) -> String {
            "HISARKIT1 " + data.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
        }
        XCTAssertEqual(SignInLink.parse(encode(Data(unicodeJSON.utf8))), SignInLink(server: "https://hisar.example/İstanbul/東京", email: "élève+🔐@example.test", secretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"))
        for encoding in [String.Encoding.utf16, .utf16LittleEndian, .utf16BigEndian, .utf32, .utf32LittleEndian, .utf32BigEndian] {
            XCTAssertNil(SignInLink.parse(encode(unicodeJSON.data(using: encoding)!)), "\(encoding)")
        }
        let malformed = Data(unicodeJSON.utf8) + Data([0xC3, 0x28])
        XCTAssertNil(SignInLink.parse(encode(malformed)))
        XCTAssertEqual(SignInLink.parse("hisar://sign-in?secretKey=%20key%2Bbytes%20")?.secretKey, " key+bytes ")
    }

    func testPlainTextEmergencyKitWithInternationalServerName() {
        let payload = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL-S-i-OBiC50ZXN0Ojg0NDMiLCJlbWFpbCI6Iuadjumbt0DkvovjgYgudGVzdCIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
        XCTAssertEqual(SignInLink.parse(payload), SignInLink(server: "https://例え.test:8443", email: "李雷@例え.test", secretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"))
        let withUser = "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL3VzZXJA5L6L44GILnRlc3QiLCJlbWFpbCI6Iuadjumbt0DkvovjgYgudGVzdCIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ"
        XCTAssertNil(SignInLink.parse(withUser))
    }

    func testPlainTextAndLegacyKitFramesDecodeForSignIn() throws {
        let payloads = [
            "HISARKIT1 eyJzZXJ2ZXIiOiJodHRwczovL2hpc2FyLmV4YW1wbGUvYT94PW9uZSt0d29cdTAwMjZ5PSUyMCIsImVtYWlsIjoiYWxwZXIra2l0QHRyeWhpc2FyLmNvbSIsInNlY3JldEtleSI6IkgxLUdWQzJLQS1UNVhWWS1KUVoxRy00MkNFWS0xQzIzUC1WNUY4OU0ifQ",
            "hisar://sign-in?server=https%3A%2F%2Fhisar.example%2Fa%3Fx%3Done%2Btwo%26y%3D%2520&email=alper%2Bkit%40tryhisar.com&secretKey=H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M",
        ]
        for payload in payloads {
            let filter = try XCTUnwrap(CIFilter(name: "CIQRCodeGenerator", parameters: ["inputMessage": Data(payload.utf8), "inputCorrectionLevel": "M"]))
            let image = try XCTUnwrap(filter.outputImage?.transformed(by: CGAffineTransform(scaleX: 6, y: 6)))
            let detector = try XCTUnwrap(CIDetector(ofType: CIDetectorTypeQRCode, context: CIContext(options: [.useSoftwareRenderer: true]), options: [CIDetectorAccuracy: CIDetectorAccuracyHigh]))
            let decoded = try XCTUnwrap((detector.features(in: image).first as? CIQRCodeFeature)?.messageString)
            XCTAssertEqual(decoded, payload)
            XCTAssertEqual(SignInLink.parse(decoded)?.secretKey, "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M")
            XCTAssertEqual(SignInLink.parse(decoded)?.email, "alper+kit@tryhisar.com")
        }
    }

    func testInviteLinkFromInvitePage() {
        let expectedByLink: [String: InviteLink] = [
            "hisar://invite/abc_DEF-123": InviteLink(invite: "abc_DEF-123"),
            " hisar://invite/abc_DEF-123?server=https%3A%2F%2Feu.tryhisar.com ": InviteLink(invite: "abc_DEF-123"),
            "HISAR://INVITE/abc?server=https%3A%2F%2Fhisar.example.com": InviteLink(invite: "https://hisar.example.com/invite/abc", server: "https://hisar.example.com"),
            "hisar://invite/abc?server=http%3A%2F%2F127.0.0.1%3A19400": InviteLink(invite: "http://127.0.0.1:19400/invite/abc", server: "http://127.0.0.1:19400"),
            "hisar://invite/abc?server=hisar.example.com": InviteLink(invite: "https://hisar.example.com/invite/abc", server: "https://hisar.example.com"),
            "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com%2F&other=yes": InviteLink(invite: "https://hisar.example.com/invite/abc", server: "https://hisar.example.com"),
            // An organization invitation carries the kind and the normalized address.
            "hisar://invite/abc?invitation=organization&email=Alice%40Example.com": InviteLink(invite: "abc", invitation: "organization", email: "alice@example.com"),
            "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com&invitation=share&email=%20Bob%40Example.com%20": InviteLink(invite: "https://hisar.example.com/invite/abc", server: "https://hisar.example.com", invitation: "share", email: "bob@example.com"),
            // An unknown invitation is a plain invite, as in an older app.
            "hisar://invite/abc?invitation=team&email=alice%40example.com": InviteLink(invite: "abc"),
        ]
        for (link, expected) in expectedByLink {
            XCTAssertEqual(InviteLink.parse(link), expected, link)
        }
        for link in ["", "hisar://invite/", "hisar://invite/a/b", "hisar://sign-in?server=x", "https://eu.tryhisar.com/invite/abc", "hisar://invite/a%20b", "hisar://invite/abc?server=ftp%3A%2F%2Fx"] {
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

    func testCardNumberIsGroupedAsTyped() {
        XCTAssertEqual(CardInput.formatNumber("4444555555555555"), "4444 5555 5555 5555")
        XCTAssertEqual(CardInput.formatNumber("44445"), "4444 5")
        XCTAssertEqual(CardInput.formatNumber("4444 "), "4444")
        XCTAssertEqual(CardInput.formatNumber("4111-1111-1111-1111"), "4111 1111 1111 1111")
        XCTAssertEqual(CardInput.formatNumber("378282246310005"), "3782 822463 10005")
        XCTAssertEqual(CardInput.formatNumber("36227206271667"), "3622 720627 1667")
        XCTAssertEqual(CardInput.formatNumber("3056930009020004"), "3056 9300 0902 0004")
        XCTAssertEqual(CardInput.formatNumber("Store card #12"), "Store card #12")
    }

    func testExpiryIsWrittenAsMonthAndYearAsTyped() {
        let allCase = ["0": "0", "01": "01", "013": "01/3", "0130": "01/30", "01305": "01/30", "3": "03", "13": "01/3", "1/": "01/", "01/": "01/", "1/30": "01/30", "12/2030": "12/30", "2030-12": "12/30", "122030": "12/30", "": "", "soon": "soon"]
        for (typed, want) in allCase {
            XCTAssertEqual(CardInput.formatExpiry(typed), want, typed)
        }
    }
}
