import SwiftUI

// The logo's own colours; everything else uses system colours and the asset catalog's accent colour.
enum Theme {
    static let markBackground = Color(hexadecimal: 0xF2F3F5)
    static let markForeground = Color(hexadecimal: 0x0B0C0E)
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

struct ErrorBanner: View {
    var message: String

    var body: some View {
        Label {
            Text(message)
                .foregroundStyle(.primary)
        } icon: {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.red)
        }
        .font(.subheadline)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("error-message")
    }
}

struct ProminentButtonStyleModifier: ViewModifier {
    var isProminent: Bool

    func body(content: Content) -> some View {
        if isProminent {
            content.applyProminentGlassStyle()
        } else {
            content.buttonStyle(.glass)
        }
    }
}

extension View {
    func applyGlassButtonStyle(isProminent: Bool) -> some View {
        modifier(ProminentButtonStyleModifier(isProminent: isProminent))
    }

    func applyProminentGlassStyle() -> some View {
        modifier(ProminentGlassStyleModifier())
    }
}

// The accent is near-white in dark mode, so an enabled label takes the background colour to stay readable.
struct ProminentGlassStyleModifier: ViewModifier {
    @Environment(\.isEnabled) private var isEnabled

    func body(content: Content) -> some View {
        content
            .buttonStyle(.glassProminent)
            .foregroundStyle(isEnabled ? AnyShapeStyle(Color(.systemBackground)) : AnyShapeStyle(.secondary))
    }
}
