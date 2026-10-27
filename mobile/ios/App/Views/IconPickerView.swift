import PhotosUI
import SwiftUI

/// IconPickerView chooses an item's icon: Automatic (nil), a preset in a color, or a photo, which the backend turns into
/// the small image the item keeps. The choice is saved with the item.
struct IconPickerView: View {
    @Binding var icon: ChosenIcon?
    var item: ItemDetail

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var color: String
    @State private var photo: PhotosPickerItem?
    @State private var isPreparing = false

    init(icon: Binding<ChosenIcon?>, item: ItemDetail) {
        _icon = icon
        self.item = item
        _color = State(initialValue: icon.wrappedValue?.color ?? "")
    }

    private let columns = Array(repeating: GridItem(.flexible(), spacing: 8), count: 6)

    var body: some View {
        NavigationStack {
            List {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                Section {
                    Button {
                        model.clearError()
                        icon = nil
                    } label: {
                        HStack(spacing: 12) {
                            AppItemIcon(item: buildPreviewItem(icon: nil), size: 36)
                            VStack(alignment: .leading, spacing: 2) {
                                Text("Automatic")
                                    .foregroundStyle(.primary)
                                Text(item.isLogin && !item.allURL.isEmpty ? "The website's icon, or the initial" : "The initial or the item type's symbol")
                                    .font(.footnote)
                                    .foregroundStyle(.themeSecondary)
                            }
                            Spacer()
                            if icon == nil {
                                Image(systemName: "checkmark")
                                    .foregroundStyle(.tint)
                            }
                        }
                    }
                    .accessibilityIdentifier("icon-automatic")
                    PhotosPicker(selection: $photo, matching: .images) {
                        Label(isPreparing ? "Preparing…" : "Choose photo", systemImage: "photo")
                    }
                    .disabled(isPreparing)
                    .accessibilityIdentifier("icon-photo")
                }
                Section("Icons") {
                    LazyVGrid(columns: columns, spacing: 8) {
                        ForEach(ChosenIcon.allPreset, id: \.self) { preset in
                            let isChosen = icon?.image == nil && icon?.preset == preset
                            Button {
                                model.clearError()
                                icon = ChosenIcon(preset: preset, color: color)
                            } label: {
                                ItemIcon(item: buildPreviewItem(icon: ChosenIcon(preset: preset, color: color)), size: 40)
                                    .padding(3)
                                    .overlay {
                                        RoundedRectangle(cornerRadius: 13, style: .continuous)
                                            .strokeBorder(isChosen ? Color.accentColor : .clear, lineWidth: 2)
                                    }
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel(preset.capitalized)
                            .accessibilityAddTraits(isChosen ? .isSelected : [])
                            .accessibilityIdentifier("icon-preset-\(preset)")
                        }
                    }
                    .padding(.vertical, 4)
                }
                Section("Color") {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 10) {
                            buildSwatch("")
                            ForEach(ChosenIcon.allColor, id: \.self) { hue in
                                buildSwatch(hue)
                            }
                        }
                        .padding(.vertical, 4)
                    }
                    .disabled(icon?.image != nil)
                    if icon?.image != nil {
                        Text("Choose a preset icon to color it.")
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                    }
                }
            }
            .navigationTitle("Icon")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("icon-done")
                }
            }
            .onChange(of: photo) { _, newPhoto in
                guard let newPhoto else { return }
                Task { await preparePhoto(newPhoto) }
            }
        }
    }

    private func buildSwatch(_ hue: String) -> some View {
        Button {
            color = hue
            icon = ChosenIcon.applyingColor(hue, to: icon, category: item.category)
        } label: {
            Circle()
                .fill(ItemIcon.getColor(hue).map { AnyShapeStyle($0) } ?? AnyShapeStyle(.fill.tertiary))
                .frame(width: 28, height: 28)
                .overlay {
                    Circle().strokeBorder(color == hue ? Color.accentColor : .clear, lineWidth: 2.5)
                }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(hue.isEmpty ? "No color" : hue.capitalized)
        .accessibilityAddTraits(color == hue ? .isSelected : [])
        .accessibilityIdentifier("icon-color-\(hue.isEmpty ? "none" : hue)")
    }

    private func buildPreviewItem(icon: ChosenIcon?) -> ItemDetail {
        var preview = item
        preview.icon = icon
        return preview
    }

    private func preparePhoto(_ newPhoto: PhotosPickerItem) async {
        isPreparing = true
        defer {
            isPreparing = false
            photo = nil
        }
        guard let data = try? await newPhoto.loadTransferable(type: Data.self) else {
            model.errorMessage = AppModel.unreadablePhotoMessage
            return
        }
        if let image = await model.prepareItemIcon(data) {
            icon = ChosenIcon(image: image)
        }
    }
}
