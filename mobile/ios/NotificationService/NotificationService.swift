import UserNotifications

/// NotificationService decrypts a sharing push's details, encrypted to a key only this phone holds, and sets the alert
/// body to who invited the user to what. If anything is missing or fails, the alert stays exactly as delivered: its
/// loc-key sentence from the app. It never imports HisarKit; all it needs is the shared keychain key.
final class NotificationService: UNNotificationServiceExtension {
    private var contentHandler: ((UNNotificationContent) -> Void)?
    private var deliveredContent: UNMutableNotificationContent?

    override func didReceive(
        _ request: UNNotificationRequest, withContentHandler contentHandler: @escaping (UNNotificationContent) -> Void
    ) {
        self.contentHandler = contentHandler
        let content = (request.content.mutableCopy() as? UNMutableNotificationContent) ?? UNMutableNotificationContent()
        deliveredContent = content

        if let update = decryptedUpdate(from: request.content.userInfo) {
            if let title = update.title { content.title = title }
            content.body = update.body
        }
        contentHandler(content)
    }

    override func serviceExtensionTimeWillExpire() {
        if let contentHandler, let deliveredContent {
            contentHandler(deliveredContent)
        }
    }

    // decryptedUpdate opens the push payload encrypted to this phone and returns the title and body to show: an
    // approval the account signed, or a sharing invitation (body only). nil keeps the alert's delivered sentence.
    private func decryptedUpdate(from userInfo: [AnyHashable: Any]) -> (title: String?, body: String)? {
        guard let payloadText = userInfo["payload"] as? String,
              let message = Base64URL.decode(payloadText) else { return nil }
        let accessGroup = HisarConfiguration.current.keychainAccessGroup
        // Only the app makes the key, since only it can give the server the public half.
        guard let key = try? PushPayloadKeyStore(accessGroup: accessGroup).load(),
              let plaintext = try? WebPushDecryptor.decrypt(message, privateKey: key.privateKey, auth: key.auth) else { return nil }
        // An approval the account signed names the Mac, the app and the request; the verifier is the account's key.
        if let verifier = try? ApprovalPushVerifierStore(accessGroup: accessGroup).load(),
           let text = ApprovalPushText.titleAndBody(decrypted: plaintext, verifier: verifier, now: Date()) {
            return (text.title, text.body)
        }
        // A sharing invitation sets only the body.
        if let body = SharingPushDetails.decode(plaintext)?.body {
            return (nil, body)
        }
        return nil
    }
}
