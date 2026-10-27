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
    @StateObject private var preparation = IconPhotoPreparation()

    init(icon: Binding<ChosenIcon?>, item: ItemDetail) {
        _icon = icon
        self.item = item
        _color = State(initialValue: icon.wrappedValue?.color ?? "")
    }

    var body: some View {
        let photoLabel = preparation.isPreparing ? "Preparing…" : "Choose photo"
        NavigationStack {
            List {
                if let errorMessage = preparation.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                Section {
                    Button {
                        preparation.cancel()
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
                        Label(photoLabel, systemImage: "photo")
                    }
                    .accessibilityIdentifier("icon-photo")
                    if preparation.isPreparing {
                        Button("Cancel preparation") {
                            preparation.cancel()
                            photo = nil
                        }
                        .accessibilityIdentifier("icon-cancel-preparation")
                    }
                }
                Section {
                    IconPresetGrid(item: item, icon: icon, color: color) { preset in
                        preparation.cancel()
                        icon = ChosenIcon(preset: preset, color: color)
                    }
                } header: {
                    Text("Icons")
                        .foregroundStyle(.themeSecondary)
                }
                Section {
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
                } header: {
                    Text("Color")
                        .foregroundStyle(.themeSecondary)
                }
            }
            .navigationTitle("Icon")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .disabled(preparation.isPreparing)
                        .accessibilityIdentifier("icon-done")
                }
            }
            .onAppear { preparation.activate() }
            .onDisappear { preparation.dismiss() }
            .onChange(of: photo) { _, newPhoto in
                guard let newPhoto else { return }
                preparation.prepare(
                    load: { try await newPhoto.loadTransferable(type: Data.self) },
                    convert: { try await model.prepareItemIcon($0) },
                    apply: { icon = $0 }
                )
            }
        }
    }

    private func buildSwatch(_ hue: String) -> some View {
        Button {
            color = hue
            if icon?.image == nil { preparation.cancel() }
            icon = ChosenIcon.applyingColor(hue, to: icon, category: item.category)
        } label: {
            Circle()
                .fill(ItemIcon.getColor(hue).map { AnyShapeStyle($0) } ?? AnyShapeStyle(.fill.tertiary))
                .frame(width: 28, height: 28)
                .overlay {
                    Circle().strokeBorder(color == hue ? Color.accentColor : .clear, lineWidth: 2.5)
                }
                .iconButtonHitArea()
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

}

struct IconPresetGrid: View {
    var item: ItemDetail
    var icon: ChosenIcon?
    var color: String
    var onChoose: (String) -> Void

    @ScaledMetric(relativeTo: .body) private var bodyRelativeScale: CGFloat = 1

    var body: some View {
        let minimum = max(44, 40 * bodyRelativeScale + 6)
        LazyVGrid(columns: [GridItem(.adaptive(minimum: minimum), spacing: 8)], spacing: 8) {
            ForEach(ChosenIcon.allPreset, id: \.self) { preset in
                let isChosen = icon?.image == nil && icon?.preset == preset
                Button {
                    onChoose(preset)
                } label: {
                    ItemIcon(item: previewItem(preset), size: 40)
                        .padding(3)
                        .frame(minWidth: 44, minHeight: 44)
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

    private func previewItem(_ preset: String) -> ItemDetail {
        var preview = item
        preview.icon = ChosenIcon(preset: preset, color: color)
        return preview
    }
}
