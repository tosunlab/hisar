import SwiftUI

struct ItemListView: View {
    var vaultID: String

    @EnvironmentObject private var model: AppModel
    @State private var searchText = ""
    @State private var isEditorShown = false

    private var isWritable: Bool {
        guard let vault = model.findVault(vaultID: vaultID) else { return false }
        return vault.type == "personal" || vault.role != "reader"
    }

    var body: some View {
        let allMatch = VaultLogic.filterAllItem(model.listAllItemReference(vaultID: vaultID), searchText: searchText)
        List {
            if allMatch.isEmpty {
                Text(searchText.isEmpty ? "No items yet. Tap + to add one." : "No items match “\(searchText)”.")
                    .foregroundStyle(Theme.textMuted)
                    .listRowBackground(Theme.surface)
            }
            ForEach(allMatch) { reference in
                NavigationLink(value: ItemDestination(vaultID: vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item)
                }
                .listRowBackground(Theme.surface)
            }
        }
        .listStyle(.insetGrouped)
        .applyListTheme()
        .refreshable { await model.refreshVault(vaultID: vaultID) }
        .safeAreaInset(edge: .top, spacing: 0) {
            if model.status.isOffline {
                OfflineBanner()
            }
        }
        .navigationTitle(model.findVault(vaultID: vaultID)?.name ?? "Items")
        .navigationBarTitleDisplayMode(.inline)
        .applyNavigationBarTheme()
        .searchable(text: $searchText, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search")
        .toolbar {
            if isWritable {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isEditorShown = true
                    } label: {
                        Image(systemName: "plus")
                    }
                    .accessibilityLabel("Add item")
                    .accessibilityIdentifier("add-item")
                }
            }
        }
        .sheet(isPresented: $isEditorShown) {
            ItemEditorView(vaultID: vaultID, originalItem: nil) { _ in }
                .environmentObject(model)
        }
        .onAppear { model.reportActivity() }
    }
}
