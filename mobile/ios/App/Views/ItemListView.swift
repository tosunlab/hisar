import SwiftUI

struct ItemListView: View {
    var vaultID: String

    @EnvironmentObject private var model: AppModel
    @State private var searchText = ""
    @State private var newItemCategory: String?
    @State private var isSharingShown = false
    @State private var isPersonalVaultNoteShown = false
    @State private var vaultActions: VaultActions?
    @State private var deletionActions: VaultActions?
    @State private var isRenameShown = false
    @State private var newName = ""
    @State private var isLeaveConfirmationShown = false
    @State private var blockedReason: String?
    @Environment(\.dismiss) private var dismiss

    private var vault: VaultSummary? { model.findVault(vaultID: vaultID) }
    private var isShared: Bool { vault?.type == "shared" }
    private var isOwner: Bool { vault?.role == Member.vaultRoleOwner }

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
        .applyThemedListStyle()
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
            if isShared && !isOwner {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isSharingShown = true
                    } label: {
                        Label("Members", systemImage: "person.2")
                    }
                    .accessibilityIdentifier("vault-members")
                }
            } else if vault != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        if isShared {
                            isSharingShown = true
                        } else {
                            isPersonalVaultNoteShown = true
                        }
                    } label: {
                        Label("Share", systemImage: "person.crop.circle.badge.plus")
                    }
                    .accessibilityIdentifier("share-vault")
                }
            }
            if isWritable {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button {
                            newItemCategory = ItemDetail.categoryLogin
                        } label: {
                            Label("Login", systemImage: "person.badge.key")
                        }
                        .accessibilityIdentifier("add-login")
                        Button {
                            newItemCategory = ItemDetail.categorySecureNote
                        } label: {
                            Label("Secure note", systemImage: "note.text")
                        }
                        .accessibilityIdentifier("add-secure-note")
                        Button {
                            newItemCategory = ItemDetail.categoryCreditCard
                        } label: {
                            Label("Credit card", systemImage: "creditcard")
                        }
                        .accessibilityIdentifier("add-credit-card")
                        Button {
                            newItemCategory = ItemDetail.categoryIdentity
                        } label: {
                            Label("Identity", systemImage: "person.text.rectangle")
                        }
                        .accessibilityIdentifier("add-identity")
                    } label: {
                        Label("Add item", systemImage: "plus")
                    }
                    .accessibilityIdentifier("add-item")
                }
            }
            if vault != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    VaultActionsMenu(
                        actions: vaultActions,
                        rename: {
                            newName = vault?.name ?? ""
                            isRenameShown = true
                        },
                        delete: { deletionActions = $0 },
                        leave: { isLeaveConfirmationShown = true },
                        showBlockedReason: { blockedReason = $0 }
                    )
                }
            }
        }
        .task(id: vault) {
            vaultActions = await model.loadVaultActions(vaultID: vaultID)
        }
        .onChange(of: vault == nil) { _, isGone in
            if isGone { dismiss() }
        }
        .sheet(item: $deletionActions) { actions in
            VaultDeletionSheet(actions: actions) {}
                .environmentObject(model)
        }
        .alert("Rename vault", isPresented: $isRenameShown) {
            TextField("Vault name", text: $newName)
                .accessibilityIdentifier("rename-vault-name")
            Button("Cancel", role: .cancel) {}
            Button("Rename") {
                let enteredName = newName
                Task { await model.renameVault(vaultID: vaultID, name: enteredName) }
            }
            .accessibilityIdentifier("rename-vault-confirm")
        } message: {
            Text("Everyone who shares the vault sees the new name.")
        }
        .alert("Can't do that", isPresented: Binding(get: { blockedReason != nil }, set: { if !$0 { blockedReason = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(blockedReason ?? "")
        }
        .confirmationDialog("Leave “\(vault?.name ?? "this vault")”?", isPresented: $isLeaveConfirmationShown, titleVisibility: .visible) {
            Button("Leave vault", role: .destructive) {
                Task { await model.leaveVault(vaultID: vaultID) }
            }
            .accessibilityIdentifier("leave-vault-confirm")
        } message: {
            Text("You lose access to its items on all your devices. An owner can invite you again.")
        }
        .sheet(isPresented: Binding(get: { newItemCategory != nil }, set: { if !$0 { newItemCategory = nil } })) {
            ItemEditorView(vaultID: vaultID, originalItem: nil, newCategory: newItemCategory ?? ItemDetail.categoryLogin) { _ in }
                .environmentObject(model)
        }
        .sheet(isPresented: $isSharingShown) {
            VaultSharingView(vaultID: vaultID)
                .environmentObject(model)
        }
        .alert("Personal vaults are private", isPresented: $isPersonalVaultNoteShown) {
            Button("OK", role: .cancel) {}
        } message: {
            Text("Only you can open the items in your Personal vault. To share items with other people, create a shared vault with + on the Vaults screen and share that.")
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
        .applyThemedListStyle()
        .overlay {
            if allReference.isEmpty {
                ContentUnavailableView(category.emptyTitle, systemImage: category.systemImage)
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
            .applyThemedListStyle()
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
