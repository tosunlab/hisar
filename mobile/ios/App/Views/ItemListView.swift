import SwiftUI

struct ItemListView: View {
    var vaultID: String

    @EnvironmentObject private var model: AppModel
    @State private var searchText = ""
    @State private var isEditorShown = false
    @State private var isMembersShown = false

    private var vault: VaultSummary? { model.findVault(vaultID: vaultID) }
    private var isShared: Bool { vault?.type == "shared" }

    private var isWritable: Bool {
        guard let vault else { return false }
        return vault.type == "personal" || vault.role != "reader"
    }

    var body: some View {
        let allMatch = VaultLogic.filterAllItem(model.listAllItemReference(vaultID: vaultID), searchText: searchText)
        List {
            if model.status.isOffline {
                Section {
                    OfflineNote()
                }
            }
            Section {
                ForEach(allMatch) { reference in
                    NavigationLink(value: ItemDestination(vaultID: vaultID, itemID: reference.item.id)) {
                        ItemRow(item: reference.item)
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .overlay {
            if allMatch.isEmpty {
                if searchText.isEmpty {
                    ContentUnavailableView {
                        Label("No items yet", systemImage: "tray")
                    } description: {
                        Text(isWritable ? "Tap + to add a login." : "Items appear here once an editor adds them.")
                    }
                } else {
                    ContentUnavailableView.search(text: searchText)
                }
            }
        }
        .refreshable { await model.refreshVault(vaultID: vaultID) }
        .navigationTitle(vault?.name ?? "Items")
        .searchable(text: $searchText, prompt: "Search \(vault?.name ?? "items")")
        .toolbar {
            if isShared {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isMembersShown = true
                    } label: {
                        Label("Members", systemImage: "person.2")
                    }
                    .accessibilityIdentifier("vault-members")
                }
            }
            if isWritable {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isEditorShown = true
                    } label: {
                        Label("Add item", systemImage: "plus")
                    }
                    .accessibilityIdentifier("add-item")
                }
            }
        }
        .sheet(isPresented: $isEditorShown) {
            ItemEditorView(vaultID: vaultID, originalItem: nil) { _ in }
                .environmentObject(model)
        }
        .sheet(isPresented: $isMembersShown) {
            VaultMembersView(vaultID: vaultID)
                .environmentObject(model)
        }
        .onAppear { model.reportActivity() }
    }
}

struct ItemCategoryListView: View {
    var category: ItemCategoryDestination

    @EnvironmentObject private var model: AppModel

    var body: some View {
        let allReference = model.listAllItemReference().filter { category.includes($0.item) }
        List {
            ForEach(allReference) { reference in
                NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item, vaultName: reference.vaultName)
                }
            }
        }
        .listStyle(.insetGrouped)
        .overlay {
            if allReference.isEmpty {
                ContentUnavailableView(category == .logins ? "No logins" : "No SSH keys", systemImage: category.systemImage)
            }
        }
        .refreshable { await model.refreshAll() }
        .navigationTitle(category.title)
    }
}

struct SearchView: View {
    @EnvironmentObject private var model: AppModel
    @State private var searchText = ""

    var body: some View {
        NavigationStack {
            let allMatch = VaultLogic.filterAllItem(model.listAllItemReference(), searchText: searchText)
            List {
                ForEach(searchText.isEmpty ? [] : allMatch) { reference in
                    NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                        ItemRow(item: reference.item, vaultName: reference.vaultName)
                    }
                }
            }
            .listStyle(.insetGrouped)
            .overlay {
                if searchText.isEmpty {
                    ContentUnavailableView("Search Hisar", systemImage: "magnifyingglass", description: Text("Find logins, SSH keys and notes in every vault."))
                } else if allMatch.isEmpty {
                    ContentUnavailableView.search(text: searchText)
                }
            }
            .navigationTitle("Search")
            .searchable(text: $searchText, prompt: "Logins, keys and notes")
            .navigationDestination(for: ItemDestination.self) { destination in
                ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
            }
        }
        .onChange(of: searchText) { _, _ in model.reportActivity() }
    }
}
