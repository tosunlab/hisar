import SwiftUI

/// The tile of an item: the image or preset the user chose, else a login's website icon (which the app loads and passes
/// in), else the category's symbol or the title's initial.
struct ItemIcon: View {
    var item: ItemDetail
    var size: CGFloat = 32
    var websiteIcon: Data?
    var maxSize: CGFloat = .infinity

    @ScaledMetric(relativeTo: .body) private var scale: CGFloat = 1

    var body: some View {
        let initial = item.title.first.map { String($0).uppercased() } ?? "?"
        let scaledSize = min(size * scale, maxSize)
        let shape = RoundedRectangle(cornerRadius: scaledSize * 0.25, style: .continuous)
        ZStack {
            if let image = displayedImage {
                shape.fill(.white)
                Image(uiImage: image)
                    .resizable()
                    .interpolation(.high)
                    .scaledToFit()
            } else if let symbol = item.icon?.symbol {
                let hue = Self.getColor(item.icon?.color ?? "")
                shape.fill(hue.map { AnyShapeStyle($0.opacity(0.18)) } ?? AnyShapeStyle(.fill.tertiary))
                Image(systemName: symbol)
                    .font(size > 40 ? .title2 : .footnote)
                    .fontWeight(.semibold)
                    .foregroundStyle(hue.map { AnyShapeStyle($0) } ?? AnyShapeStyle(.themeSecondary))
            } else {
                shape.fill(.fill.tertiary)
                buildAutomaticContent(initial: initial)
            }
        }
        .foregroundStyle(.themeSecondary)
        .frame(width: scaledSize, height: scaledSize)
        .clipShape(shape)
        .accessibilityHidden(true)
    }

    private var displayedImage: UIImage? {
        if let image = item.icon?.image {
            return UIImage(data: image)
        }
        return item.icon == nil ? websiteIcon.flatMap { UIImage(data: $0) } : nil
    }

    /// The color of a preset; nil for the neutral tile.
    static func getColor(_ id: String) -> Color? {
        switch id {
        case "red": .red
        case "orange": .orange
        case "yellow": .yellow
        case "green": .green
        case "teal": .teal
        case "blue": .blue
        case "indigo": .indigo
        case "purple": .purple
        case "pink": .pink
        case "gray": .gray
        default: nil
        }
    }

    @ViewBuilder
    private func buildAutomaticContent(initial: String) -> some View {
        if let symbol = categorySymbol {
            Image(systemName: symbol)
                .font(size > 40 ? .title2 : .footnote)
                .fontWeight(.semibold)
        } else {
            Text(initial)
                .font(size > 40 ? .title.weight(.semibold) : .subheadline.weight(.semibold))
        }
    }

    /// The symbol of a category that shows one; a login shows its initial.
    private var categorySymbol: String? {
        if item.isSSHKey { return "key.horizontal" }
        if item.isSecureNote { return "note.text" }
        if item.isCreditCard { return "creditcard" }
        if item.isIdentity { return "person.text.rectangle" }
        if item.isAPICredential { return "curlybraces.square" }
        if item.isAddress { return "mappin.and.ellipse" }
        if item.isSoftwareLicense { return "checkmark.seal" }
        if item.isServer { return "server.rack" }
        if item.isDatabase { return "cylinder.split.1x2" }
        return nil
    }
}
