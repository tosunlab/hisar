import CryptoKit
import Foundation

/// WebPushDecryptor opens an RFC 8291 Web Push message in the aes128gcm content coding (RFC 8188), as a single record,
/// with the subscription's private key and authentication secret. It mirrors core/crypt's DecryptWebPush.
enum WebPushDecryptor {
    private static let saltSize = 16
    private static let publicKeySize = 65
    private static let tagSize = 16
    // salt(16) | record size(4) | idlen(1) | sender public key(65)
    private static let headerSize = saltSize + 4 + 1 + publicKeySize

    struct DecryptionError: Error {}

    static func decrypt(_ message: Data, privateKey: P256.KeyAgreement.PrivateKey, auth: Data) throws -> Data {
        let bytes = [UInt8](message)
        guard bytes.count >= headerSize + tagSize, bytes[saltSize + 4] == UInt8(publicKeySize), auth.count == 16 else {
            throw DecryptionError()
        }
        let salt = Data(bytes[0..<saltSize])
        let senderPublicKeyData = Data(bytes[(saltSize + 5)..<headerSize])
        let body = Data(bytes[headerSize...])

        let senderPublicKey: P256.KeyAgreement.PublicKey
        do {
            senderPublicKey = try P256.KeyAgreement.PublicKey(x963Representation: senderPublicKeyData)
        } catch {
            throw DecryptionError()
        }
        let phonePublicKeyData = privateKey.publicKey.x963Representation

        let shared: SharedSecret
        do {
            shared = try privateKey.sharedSecretFromKeyAgreement(with: senderPublicKey)
        } catch {
            throw DecryptionError()
        }

        var keyInfo = Data("WebPush: info\0".utf8)
        keyInfo.append(phonePublicKeyData)
        keyInfo.append(senderPublicKeyData)
        let inputKey = shared.hkdfDerivedSymmetricKey(
            using: SHA256.self, salt: auth, sharedInfo: keyInfo, outputByteCount: 32
        )

        let contentKey = HKDF<SHA256>.deriveKey(
            inputKeyMaterial: inputKey, salt: salt,
            info: Data("Content-Encoding: aes128gcm\0".utf8), outputByteCount: 16
        )
        let nonce = HKDF<SHA256>.deriveKey(
            inputKeyMaterial: inputKey, salt: salt,
            info: Data("Content-Encoding: nonce\0".utf8), outputByteCount: 12
        )

        let bodyBytes = [UInt8](body)
        let ciphertext = Data(bodyBytes[0..<(bodyBytes.count - tagSize)])
        let tag = Data(bodyBytes[(bodyBytes.count - tagSize)...])
        let padded: Data
        do {
            let sealedNonce = try AES.GCM.Nonce(data: nonce.withUnsafeBytes { Data($0) })
            let box = try AES.GCM.SealedBox(nonce: sealedNonce, ciphertext: ciphertext, tag: tag)
            let cek = SymmetricKey(data: contentKey.withUnsafeBytes { Data($0) })
            padded = try AES.GCM.open(box, using: cek)
        } catch {
            throw DecryptionError()
        }

        // Strip trailing zero padding; the delimiter before it must be 0x02 (the last record).
        var index = padded.count - 1
        while index >= 0 {
            let value = padded[padded.startIndex + index]
            if value == 0 { index -= 1; continue }
            if value == 0x02 { return padded.prefix(index) }
            break
        }
        throw DecryptionError()
    }
}
