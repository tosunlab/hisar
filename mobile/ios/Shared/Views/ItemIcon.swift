import SwiftUI

struct ItemIcon: View {
    var item: ItemDetail
    var size: CGFloat = 32

    @ScaledMetric(relativeTo: .body) private var scale: CGFloat = 1

    var body: some View {
        let initial = item.title.first.map { String($0).uppercased() } ?? "?"
        let scaledSize = size * scale
        ZStack {
            RoundedRectangle(cornerRadius: scaledSize * 0.25, style: .continuous)
                .fill(.fill.tertiary)
            if let symbol = item.isSSHKey ? "key.horizontal" : (item.isSecureNote ? "note.text" : nil) {
                Image(systemName: symbol)
                    .font(size > 40 ? .title2 : .footnote)
                    .fontWeight(.semibold)
            } else {
                Text(initial)
                    .font(size > 40 ? .title.weight(.semibold) : .subheadline.weight(.semibold))
            }
        }
        .foregroundStyle(.themeSecondary)
        .frame(width: scaledSize, height: scaledSize)
        .accessibilityHidden(true)
    }
}
