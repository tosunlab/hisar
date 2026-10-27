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

        if let body = decryptedBody(from: request.content.userInfo) {
            content.body = body
        }
        contentHandler(content)
    }

    override func serviceExtensionTimeWillExpire() {
        if let contentHandler, let deliveredContent {
            contentHandler(deliveredContent)
        }
    }

    private func decryptedBody(from userInfo: [AnyHashable: Any]) -> String? {
        guard let payloadText = userInfo["payload"] as? String,
              let message = Base64URL.decode(payloadText) else { return nil }
        let accessGroup = HisarConfiguration.current.keychainAccessGroup
        // Only the app makes the key, since only it can give the server the public half.
        guard let key = try? PushPayloadKeyStore(accessGroup: accessGroup).load(),
              let plaintext = try? WebPushDecryptor.decrypt(message, privateKey: key.privateKey, auth: key.auth),
              let details = SharingPushDetails.decode(plaintext) else { return nil }
        return details.body
    }
}
