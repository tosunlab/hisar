import Foundation
import HisarKit
import UIKit
import UniformTypeIdentifiers

final class PasteboardClipboard: NSObject, HisarkitClipboardProtocol {
    static let expirationInterval: TimeInterval = 90

    private let pasteboard: UIPasteboard
    private let stateLock = NSLock()
    private var copiedText = ""
    private var copiedChangeCount = -1

    init(pasteboard: UIPasteboard = .general) {
        self.pasteboard = pasteboard
    }

    private func runOnMain<Value>(_ operation: () -> Value) -> Value {
        Thread.isMainThread ? operation() : DispatchQueue.main.sync(execute: operation)
    }

    func setText(_ text: String?) -> Bool {
        let text = text ?? ""
        let changeCount: Int = runOnMain {
            if text.isEmpty {
                pasteboard.items = []
            } else {
                pasteboard.setItems(
                    [[UTType.utf8PlainText.identifier: text]],
                    options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(Self.expirationInterval)]
                )
            }
            return pasteboard.changeCount
        }
        stateLock.lock()
        copiedText = text
        copiedChangeCount = changeCount
        stateLock.unlock()
        return true
    }

    // Reading another app's text would show the paste banner, so only report text Hisar itself copied and nobody replaced.
    func getText() -> String {
        let changeCount = runOnMain { pasteboard.changeCount }
        stateLock.lock()
        defer { stateLock.unlock() }
        return changeCount == copiedChangeCount ? copiedText : ""
    }
}

final class LockEventRelay: NSObject, HisarkitEventListenerProtocol {
    private let handlerLock = NSLock()
    private var handleLocked: (() -> Void)?
    private var handleApprovalEvent: ((String, String) -> Void)?
    private var handleRemoteChange: (() -> Void)?
    private var handleUpdateRequired: (() -> Void)?

    func setHandler(_ handler: @escaping () -> Void) {
        handlerLock.lock()
        handleLocked = handler
        handlerLock.unlock()
    }

    func setApprovalHandler(_ handler: @escaping (String, String) -> Void) {
        handlerLock.lock()
        handleApprovalEvent = handler
        handlerLock.unlock()
    }

    func setRemoteChangeHandler(_ handler: @escaping () -> Void) {
        handlerLock.lock()
        handleRemoteChange = handler
        handlerLock.unlock()
    }

    func onLocked() {
        handlerLock.lock()
        let handler = handleLocked
        handlerLock.unlock()
        DispatchQueue.main.async { handler?() }
    }

    func onApprovalEvent(_ eventName: String?, requestID: String?) {
        handlerLock.lock()
        let handler = handleApprovalEvent
        handlerLock.unlock()
        let eventName = eventName ?? ""
        let requestID = requestID ?? ""
        DispatchQueue.main.async { handler?(eventName, requestID) }
    }

    func onRemoteChange() {
        handlerLock.lock()
        let handler = handleRemoteChange
        handlerLock.unlock()
        DispatchQueue.main.async { handler?() }
    }

    func setUpdateRequiredHandler(_ handler: @escaping () -> Void) {
        handlerLock.lock()
        handleUpdateRequired = handler
        handlerLock.unlock()
    }

    func onUpdateRequired() {
        handlerLock.lock()
        let handler = handleUpdateRequired
        handlerLock.unlock()
        DispatchQueue.main.async { handler?() }
    }
}
