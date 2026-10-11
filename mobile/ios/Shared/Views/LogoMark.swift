import SwiftUI

// The tower path of assets/logo/hisar-mark.svg on its 32-unit grid.
struct TowerShape: Shape {
    func path(in rect: CGRect) -> Path {
        let scale = min(rect.width, rect.height) / 32
        func makePoint(_ x: CGFloat, _ y: CGFloat) -> CGPoint {
            CGPoint(x: rect.minX + x * scale, y: rect.minY + y * scale)
        }
        var path = Path()
        path.move(to: makePoint(8, 26))
        path.addLine(to: makePoint(8, 12))
        path.addLine(to: makePoint(11, 12))
        path.addLine(to: makePoint(11, 9))
        path.addLine(to: makePoint(13.5, 9))
        path.addLine(to: makePoint(13.5, 12))
        path.addLine(to: makePoint(18.5, 12))
        path.addLine(to: makePoint(18.5, 9))
        path.addLine(to: makePoint(21, 9))
        path.addLine(to: makePoint(21, 12))
        path.addLine(to: makePoint(24, 12))
        path.addLine(to: makePoint(24, 26))
        path.addLine(to: makePoint(18, 26))
        path.addLine(to: makePoint(18, 21))
        path.addArc(center: makePoint(16, 21), radius: 2 * scale, startAngle: .degrees(0), endAngle: .degrees(180), clockwise: true)
        path.addLine(to: makePoint(14, 26))
        path.closeSubpath()
        return path
    }
}

struct LogoMark: View {
    var size: CGFloat = 56

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: size / 4, style: .continuous)
                .fill(Theme.markBackground)
                .strokeBorder(.separator, lineWidth: 1)
            TowerShape()
                .fill(Theme.markForeground)
        }
        .frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}
