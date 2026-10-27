import SwiftUI

/// The Mac's four-segment strength meter with its label (Weak, Fair, Good, Strong), for core/item's rating of a password.
/// It shows nothing for an empty password.
struct PasswordStrengthMeter: View {
    var strength: PasswordStrength
    @Environment(\.themePalette) private var palette

    private var filledSegmentCount: Int {
        switch strength.level {
        case "weak": 1
        case "fair": 2
        case "good": 3
        case "strong": 4
        default: 0
        }
    }

    private var fillColor: Color {
        switch strength.level {
        case "weak": .red
        case "fair": palette.warning.color
        default: .green
        }
    }

    var body: some View {
        if !strength.level.isEmpty {
            HStack(spacing: 4) {
                ForEach(0..<4, id: \.self) { segment in
                    Capsule()
                        .fill(segment < filledSegmentCount ? fillColor : Color(uiColor: .systemFill))
                        .frame(width: 28, height: 4)
                }
                Text(strength.label)
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
                    .padding(.leading, 6)
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Password strength")
            .accessibilityValue(strength.label)
            .accessibilityIdentifier("password-strength-meter")
        }
    }
}
