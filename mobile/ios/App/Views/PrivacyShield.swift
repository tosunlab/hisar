import SwiftUI
import UIKit

// A window above sheets and alerts, so the app switcher snapshot never shows vault content.
@MainActor
final class PrivacyShield {
    static let shared = PrivacyShield()

    private var window: UIWindow?

    func setVisible(_ isVisible: Bool) {
        if isVisible {
            showWindow()
        } else {
            window?.isHidden = true
            window = nil
        }
    }

    private func showWindow() {
        guard window == nil,
              let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else { return }
        let window = UIWindow(windowScene: scene)
        window.windowLevel = .alert + 1
        let controller = UIHostingController(rootView: PrivacyCover())
        controller.view.backgroundColor = UIColor(Theme.background)
        window.rootViewController = controller
        window.isHidden = false
        self.window = window
    }
}
