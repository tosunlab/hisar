import CryptoKit
import XCTest
@testable import Hisar

final class SharingPushTests: XCTestCase {
    private func details(_ json: String) -> SharingPushDetails {
        SharingPushDetails.decode(Data(json.utf8))!
    }

    func testAnOrganizationInvitationPushNamesTheInviterAndOrganization() {
        let detail = details(
            #"{"kind":"sharing","invitation":{"type":"organization","inviterName":"Alper Tosun","inviterEmail":"alper@tryhisar.com","organizationName":"Acme","role":"member"}}"#
        )
        XCTAssertEqual(detail.body, "Alper Tosun (alper@tryhisar.com) invited you to join Acme. Open Hisar to answer it.")

        // The email alone when the name is blank.
        let emailOnly = details(
            #"{"invitation":{"type":"organization","inviterName":"","inviterEmail":"alper@tryhisar.com","organizationName":"Acme"}}"#
        )
        XCTAssertEqual(emailOnly.body, "alper@tryhisar.com invited you to join Acme. Open Hisar to answer it.")

        // The email alone when the name equals it case-insensitively.
        let sameAsEmail = details(
            #"{"invitation":{"type":"organization","inviterName":"Alper@Tryhisar.com","inviterEmail":"alper@tryhisar.com","organizationName":"Acme"}}"#
        )
        XCTAssertEqual(sameAsEmail.body, "alper@tryhisar.com invited you to join Acme. Open Hisar to answer it.")

        // The name alone when the email is blank.
        let nameOnly = details(
            #"{"invitation":{"type":"organization","inviterName":"Alper Tosun","inviterEmail":"","organizationName":"Acme"}}"#
        )
        XCTAssertEqual(nameOnly.body, "Alper Tosun invited you to join Acme. Open Hisar to answer it.")

        // A blank organization name keeps the alert unchanged.
        let noOrganization = details(
            #"{"invitation":{"type":"organization","inviterName":"Alper Tosun","inviterEmail":"alper@tryhisar.com","organizationName":""}}"#
        )
        XCTAssertNil(noOrganization.body)
    }

    func testAVaultInvitationPushNamesTheInviter() {
        let detail = details(
            #"{"kind":"sharing","invitation":{"type":"vault","inviterName":"Alper Tosun","inviterEmail":"alper@tryhisar.com"}}"#
        )
        XCTAssertEqual(detail.body, "Alper Tosun (alper@tryhisar.com) shared a vault with you. Open Hisar to answer it.")
    }

    func testAPushWithoutDetailsKeepsTheSentenceForItsKind() {
        // No invitation object.
        XCTAssertNil(details(#"{"kind":"sharing"}"#).body)
        // A blank inviter.
        XCTAssertNil(details(#"{"invitation":{"type":"vault","inviterName":"","inviterEmail":""}}"#).body)
        // An unknown type.
        XCTAssertNil(
            details(#"{"invitation":{"type":"future","inviterName":"Alper","inviterEmail":"a@b.com"}}"#).body
        )
    }

    func testAPushPayloadDecryptsWithThePhonesKey() throws {
        // The vector from core/crypt's EncryptWebPush, base64url without padding.
        let privateScalar = Base64URL.decode("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA")!
        let auth = Base64URL.decode("MDEyMzQ1Njc4OWFiY2RlZg")!
        let message = Base64URL.decode(
            "BRsGF0LXgP95kwpii0PUbAAAEABBBLNjAmqDExLPBQutpituh7sGbIF7XnrUHtkQuCwbmdlVX_uXzaEO5l5PeltZGuvUTFocrXMpNrfDNvGVFVFEKtRl80yyK9JrMrQo2i3SE7k0KWLEEZFxQf0UfjmTLlStXdtoLOsBoeNbZjlA7psAQB5iYyGbsc2sNK-lUa0fG_8Fdw67RGSkAP-1KsGpQokr3lbGdoASOQ6cOYpMNXre57SvF7rMyVTmFLNrsH0NkgB0iHcZXzqrtxvyf6FcXhYy9DTSpk-U2TmBJe3mrj6WNisvDfHf9FOsuTVHwZRmPiqUcEgHKOufdvoAnd4f2WiPnJgY"
        )!
        let privateKey = try P256.KeyAgreement.PrivateKey(rawRepresentation: privateScalar)

        let plaintext = try WebPushDecryptor.decrypt(message, privateKey: privateKey, auth: auth)
        let expected =
            #"{"kind":"sharing","invitation":{"type":"organization","inviterName":"Alper Tosun","inviterEmail":"alper@tryhisar.com","organizationName":"Acme","role":"member"}}"#
        XCTAssertEqual(String(data: plaintext, encoding: .utf8), expected)

        let detail = SharingPushDetails.decode(plaintext)
        XCTAssertEqual(detail?.body, "Alper Tosun (alper@tryhisar.com) invited you to join Acme. Open Hisar to answer it.")
    }
}
