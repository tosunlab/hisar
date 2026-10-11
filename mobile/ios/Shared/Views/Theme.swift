import SwiftUI
import UIKit

// The logo's own colours; everything else uses system colours and the theme palette.
enum Theme {
    static let markBackground = Color(hexadecimal: 0xF2F3F5)
    static let markForeground = Color(hexadecimal: 0x0B0C0E)
}

// ThemePalettes.swift, generated from assets/themes/themes.json, defines one palette per colour.
struct ThemeColor {
    var light: UInt32
    var dark: UInt32

    var uiColor: UIColor {
        let light = light
        let dark = dark
        return UIColor { traits in UIColor(hexadecimal: traits.userInterfaceStyle == .dark ? dark : light) }
    }

    var color: Color { Color(uiColor: uiColor) }
}

struct ThemePalette {
    var tint: ThemeColor
    var onTint: ThemeColor
    var groupedBackground: ThemeColor
    var secondaryLabel: ThemeColor
    var warning: ThemeColor
}

extension VaultTheme {
    var palette: ThemePalette { ThemePalette.find(color: color) }

    var colorScheme: ColorScheme? {
        switch appearance {
        case .system: nil
        case .light: .light
        case .dark: .dark
        }
    }

    var userInterfaceStyle: UIUserInterfaceStyle {
        switch appearance {
        case .system: .unspecified
        case .light: .light
        case .dark: .dark
        }
    }
}

private struct ThemePaletteKey: EnvironmentKey {
    static let defaultValue = ThemePalette.neutral
}

extension EnvironmentValues {
    var themePalette: ThemePalette {
        get { self[ThemePaletteKey.self] }
        set { self[ThemePaletteKey.self] = newValue }
    }
}

// A solid secondary label: the translucent system grey falls below 4.5:1 on the tinted grouped backgrounds.
struct ThemeSecondaryLabelStyle: ShapeStyle {
    func resolve(in environment: EnvironmentValues) -> Color {
        environment.themePalette.secondaryLabel.color
    }
}

extension ShapeStyle where Self == ThemeSecondaryLabelStyle {
    static var themeSecondary: ThemeSecondaryLabelStyle { ThemeSecondaryLabelStyle() }
}

// The system orange is 2.2:1 on white; this amber keeps warnings readable.
struct ThemeWarningStyle: ShapeStyle {
    func resolve(in environment: EnvironmentValues) -> Color {
        environment.themePalette.warning.color
    }
}

extension ShapeStyle where Self == ThemeWarningStyle {
    static var themeWarning: ThemeWarningStyle { ThemeWarningStyle() }
}

struct ThemeModifier: ViewModifier {
    var theme: VaultTheme

    func body(content: Content) -> some View {
        content
            .environment(\.themePalette, theme.palette)
            .tint(theme.palette.tint.color)
            .preferredColorScheme(theme.colorScheme)
    }
}

struct ThemedListBackgroundModifier: ViewModifier {
    @Environment(\.themePalette) private var palette

    func body(content: Content) -> some View {
        content
            .scrollContentBackground(.hidden)
            .background(palette.groupedBackground.color.ignoresSafeArea())
    }
}

extension View {
    func applyTheme(_ theme: VaultTheme) -> some View {
        modifier(ThemeModifier(theme: theme))
    }

    // Lists and forms on full screens; sheets keep the system background and its glass.
    func applyThemedListStyle() -> some View {
        listStyle(.insetGrouped).modifier(ThemedListBackgroundModifier())
    }

    func applyThemedListBackground() -> some View {
        modifier(ThemedListBackgroundModifier())
    }
}

extension UIColor {
    convenience init(hexadecimal: UInt32) {
        self.init(
            red: CGFloat((hexadecimal >> 16) & 0xFF) / 255,
            green: CGFloat((hexadecimal >> 8) & 0xFF) / 255,
            blue: CGFloat(hexadecimal & 0xFF) / 255,
            alpha: 1
        )
    }
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

// Neutral's dark tint is near-white, so an enabled label takes the palette's text-on-tint colour.
struct ProminentGlassStyleModifier: ViewModifier {
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.themePalette) private var palette

    func body(content: Content) -> some View {
        content
            .buttonStyle(.glassProminent)
            .foregroundStyle(isEnabled ? AnyShapeStyle(palette.onTint.color) : AnyShapeStyle(.secondary))
    }
}
