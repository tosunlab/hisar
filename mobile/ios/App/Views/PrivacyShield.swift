import SwiftUI
import UIKit

// A window above sheets and alerts, so the app switcher snapshot never shows vault content.
@MainActor
final class PrivacyShield {
    static let shared = PrivacyShield()

    private var window: UIWindow?

    func setVisible(_ isVisible: Bool, theme: VaultTheme) {
        if isVisible {
            showWindow(theme: theme)
        } else {
            window?.isHidden = true
            window = nil
        }
    }

    private func showWindow(theme: VaultTheme) {
        guard window == nil,
              let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else { return }
        let window = UIWindow(windowScene: scene)
        window.windowLevel = .alert + 1
        window.overrideUserInterfaceStyle = theme.userInterfaceStyle
        window.tintColor = theme.palette.tint.uiColor
        let controller = UIHostingController(rootView: PrivacyCover().applyTheme(theme))
        controller.view.backgroundColor = theme.palette.groupedBackground.uiColor
        window.rootViewController = controller
        window.isHidden = false
        self.window = window
    }
}
