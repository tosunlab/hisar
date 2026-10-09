import SwiftUI

enum Theme {
    static let background = Color(hexadecimal: 0x0D0E10)
    static let panel = Color(hexadecimal: 0x111214)
    static let surface = Color(hexadecimal: 0x16181B)
    static let surfaceRaised = Color(hexadecimal: 0x1C1F23)
    static let selected = Color(hexadecimal: 0x262A30)
    static let border = Color(hexadecimal: 0x23262B)
    static let borderStrong = Color(hexadecimal: 0x30343A)
    static let text = Color(hexadecimal: 0xE8EAED)
    static let textSecondary = Color(hexadecimal: 0xB4B9C0)
    static let textMuted = Color(hexadecimal: 0x7C838D)
    static let primary = Color(hexadecimal: 0xECEEF1)
    static let primaryText = Color(hexadecimal: 0x0B0C0E)
    static let danger = Color(hexadecimal: 0xE5484D)
    static let dangerBackground = Color(hexadecimal: 0x2A1416)
    static let warning = Color(hexadecimal: 0xD9A33C)
    static let success = Color(hexadecimal: 0x3FA66B)
    static let markBackground = Color(hexadecimal: 0xF2F3F5)
}

extension Color {
    init(hexadecimal: UInt32) {
        self.init(
            red: Double((hexadecimal >> 16) & 0xFF) / 255,
            green: Double((hexadecimal >> 8) & 0xFF) / 255,
            blue: Double(hexadecimal & 0xFF) / 255
        )
    }
}

struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.semibold))
            .frame(maxWidth: .infinity, minHeight: 50)
            .foregroundStyle(Theme.primaryText)
            .background(Theme.primary.opacity(isEnabled ? (configuration.isPressed ? 0.8 : 1) : 0.4))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.medium))
            .frame(maxWidth: .infinity, minHeight: 50)
            .foregroundStyle(Theme.text)
            .background(configuration.isPressed ? Theme.selected : Theme.surfaceRaised)
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(Theme.borderStrong, lineWidth: 1))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

struct FieldBackground: ViewModifier {
    func body(content: Content) -> some View {
        content
            .padding(.horizontal, 14)
            .frame(minHeight: 50)
            .background(Theme.surface)
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(Theme.border, lineWidth: 1))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

extension View {
    func applyFieldBackground() -> some View {
        modifier(FieldBackground())
    }

    // Scrolled content would otherwise run under the status bar, which has no bar background on these screens.
    func coverStatusBar() -> some View {
        mask {
            Rectangle().ignoresSafeArea(.all, edges: .bottom)
        }
    }

    func applyListTheme() -> some View {
        scrollContentBackground(.hidden)
            .background(Theme.background)
    }

    func applyNavigationBarTheme() -> some View {
        toolbarColorScheme(.dark, for: .navigationBar)
    }
}

struct LabeledInput<Content: View>: View {
    var label: String
    var systemImage: String
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.footnote.weight(.medium))
                .foregroundStyle(Theme.textSecondary)
            HStack(spacing: 10) {
                Image(systemName: systemImage)
                    .foregroundStyle(Theme.textMuted)
                    .frame(width: 20)
                content
                    .foregroundStyle(Theme.text)
            }
            .applyFieldBackground()
        }
    }
}

struct ErrorBanner: View {
    var message: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(Theme.danger)
            Text(message)
                .font(.subheadline)
                .foregroundStyle(Theme.text)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(12)
        .background(Theme.dangerBackground)
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(Theme.danger.opacity(0.5), lineWidth: 1))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .accessibilityIdentifier("error-message")
    }
}
