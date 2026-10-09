import SwiftUI

struct ItemIcon: View {
    var item: ItemDetail

    var body: some View {
        let initial = item.title.first.map { String($0).uppercased() } ?? "?"
        ZStack {
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .fill(Theme.surfaceRaised)
                .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(Theme.borderStrong, lineWidth: 1))
            if item.isSSHKey {
                Image(systemName: "terminal")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(Theme.textSecondary)
            } else {
                Text(initial)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Theme.textSecondary)
            }
        }
        .frame(width: 34, height: 34)
    }
}
