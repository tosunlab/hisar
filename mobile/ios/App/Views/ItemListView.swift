import SwiftUI

struct ItemListView: View {
    var vaultID: String

    @EnvironmentObject private var model: AppModel
    @State private var searchText = ""
    @State private var newItemCategory: String?
    @State private var isSharingShown = false
    @State private var vaultActions: VaultActions?
    @State private var deletionActions: VaultActions?
    @State private var isRenameShown = false
    @State private var newName = ""
    @State private var isLeaveConfirmationShown = false
    @State private var blockedReason: String?
    @State private var issueToDelete: ReadIssue?
    @State private var issueToRepair: ReadIssue?
    @State private var refusal: Refusal?
    @State private var refreshFailure: String?
    @State private var failedRenameName: String?
    @Environment(\.dismiss) private var dismiss

    private var vault: VaultSummary? { model.findVault(vaultID: vaultID) }
    private var isShared: Bool { vault?.type == VaultSummary.typeShared || vault?.isOrganization == true }
    private var isOwner: Bool { vault?.role == Member.vaultRoleOwner }

    private var isWritable: Bool {
        guard let vault else { return false }
        if vault.readIssues.contains(where: { $0.kind == "vault-key" }) { return false }
        return vault.type == "personal" || vault.role != "reader"
    }

    var body: some View {
        let allMatch = VaultLogic.filterAllItem(model.listAllItemReference(vaultID: vaultID), searchText: searchText).filter { $0.item.readIssue == nil }
        List {
            if let refreshFailure {
                Section {
                    ErrorBanner(message: refreshFailure)
                    Button("Retry") { Task { await refresh() } }
                        .accessibilityIdentifier("browse-refresh-retry")
                }
            }
            if model.status.isOffline {
                Section {
                    OfflineNote()
                }
            }
            if let vault, !vault.readIssues.isEmpty {
                Section {
                    Text("\(vault.readIssues.count) records could not be opened. Healthy items remain available.")
                    Button("Retry reads") { Task { await model.refreshAll() } }
                    ForEach(vault.readIssues) { issue in
                        VStack(alignment: .leading) {
                            Text("\(issue.kind). \(issue.itemId ?? issue.vaultId)")
                            if issue.canRename {
                                Button("Repair vault name") { newName = ""; issueToRepair = issue; isRenameShown = true }
                            }
                            if issue.canDelete {
                                Button("Delete unreadable data…", role: .destructive) { issueToDelete = issue }
                            }
                        }
                    }
                }
            }
            Section {
                ForEach(allMatch) { reference in
                    NavigationLink(value: ItemDestination(vaultID: vaultID, itemID: reference.item.id)) {
                        ItemRow(item: reference.item)
                    }
                    .swipeActions(edge: .leading) {
                        FavoriteSwipeButton(vaultID: vaultID, item: reference.item) { refusal = $0 }
                    }
                }
            }
        }
        .applyThemedListStyle()
        .overlay {
            if refreshFailure == nil && allMatch.isEmpty && vault?.readIssues.isEmpty != false {
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
        .refreshable { await refresh() }
        .navigationTitle(vault?.name ?? "Items")
        .searchable(text: $searchText, prompt: "Search \(vault?.name ?? "items")")
        .refusalAlert($refusal)
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
            } else if isShared {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isSharingShown = true
                    } label: {
                        Label("Share", systemImage: "person.crop.circle.badge.plus")
                    }
                    .accessibilityIdentifier("share-vault")
                }
            }
            if isWritable {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        ForEach(NewItemType.allOnMenu) { type in
                            NewItemButton(type: type, placement: .vault) { newItemCategory = $0.category }
                        }
                        NewItemMoreMenu(placement: .vault) { newItemCategory = $0.category }
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
                            newName = failedRenameName ?? vault?.name ?? ""
                            issueToRepair = nil
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
            if vault?.readIssues.contains(where: { $0.kind == "vault-key" }) != true {
                vaultActions = await model.loadVaultActions(vaultID: vaultID)
            }
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
            Button("Cancel", role: .cancel) { issueToRepair = nil }
            Button("Rename") {
                let enteredName = newName
                if let issue = issueToRepair {
                    Task { await model.repairVaultName(issue, name: enteredName) }
                } else {
                    Task {
                        if await model.renameVault(vaultID: vaultID, name: enteredName, onFailure: { message in
                            failedRenameName = enteredName
                            refusal = Refusal(title: "Rename vault", message: message)
                            model.clearError()
                        }) {
                            failedRenameName = nil
                        }
                    }
                }
                issueToRepair = nil
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
        .alert("Permanently delete unreadable data?", isPresented: Binding(get: { issueToDelete != nil }, set: { if !$0 { issueToDelete = nil } })) {
            Button("Cancel", role: .cancel) { issueToDelete = nil }
            Button("Permanently delete", role: .destructive) {
                if let issue = issueToDelete { Task { await model.deleteUnreadableRecord(issue) } }
                issueToDelete = nil
            }
        } message: {
            Text(issueToDelete?.itemId == nil ? "This deletes the vault and all its items on every device. This cannot be undone." : "This deletes this unreadable item on every device. This cannot be undone.")
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
            ItemEditorView(vaultID: vaultID, originalItem: nil, newCategory: newItemCategory ?? ItemDetail.categoryLogin) { _, _ in }
                .environmentObject(model)
        }
        .sheet(isPresented: $isSharingShown) {
            if let vault, vault.isOrganization {
                OrganizationVaultSharingView(vaultID: vaultID, organizationID: vault.organizationID)
                    .environmentObject(model)
            } else {
                VaultSharingView(vaultID: vaultID)
                    .environmentObject(model)
            }
        }
        .onAppear { model.reportActivity() }
    }

    private func refresh() async {
        refreshFailure = nil
        await model.refreshVault(vaultID: vaultID) { message in
            refreshFailure = message
            model.clearError()
        }
    }
}

struct ItemCategoryListView: View {
    var category: ItemCategoryDestination

    @EnvironmentObject private var model: AppModel
    @State private var refreshFailure: String?

    var body: some View {
        let allReference = model.listAllItemReference().filter { category.includes($0.item) }
        List {
            if let refreshFailure {
                Section {
                    ErrorBanner(message: refreshFailure)
                    Button("Retry") { Task { await refresh() } }
                        .accessibilityIdentifier("browse-refresh-retry")
                }
            }
            ForEach(allReference) { reference in
                NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item, vaultName: reference.vaultName)
                }
            }
        }
        .applyThemedListStyle()
        .overlay {
            if refreshFailure == nil && allReference.isEmpty {
                ContentUnavailableView(category.emptyTitle, systemImage: category.systemImage)
            }
        }
        .refreshable { await refresh() }
        .navigationTitle(category.title)
    }

    private func refresh() async {
        refreshFailure = nil
        await model.refreshAll { message in
            refreshFailure = message
            model.clearError()
        }
    }
}

struct ItemTagListView: View {
    var tag: String

    @EnvironmentObject private var model: AppModel
    @State private var refreshFailure: String?

    var body: some View {
        let allReference = VaultLogic.filterAllItem(model.listAllItemReference(), tag: tag)
        List {
            if let refreshFailure {
                Section {
                    ErrorBanner(message: refreshFailure)
                    Button("Retry") { Task { await refresh() } }
                        .accessibilityIdentifier("browse-refresh-retry")
                }
            }
            ForEach(allReference) { reference in
                NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item, vaultName: reference.vaultName)
                }
            }
        }
        .applyThemedListStyle()
        .overlay {
            if refreshFailure == nil && allReference.isEmpty {
                ContentUnavailableView("No items are tagged \(tag)", systemImage: "tag")
            }
        }
        .refreshable { await refresh() }
        .navigationTitle(tag)
    }

    private func refresh() async {
        refreshFailure = nil
        await model.refreshAll { message in
            refreshFailure = message
            model.clearError()
        }
    }
}

struct SearchView: View {
    @EnvironmentObject private var model: AppModel
    @State private var searchText = ""
    @State private var selection: ItemDestination?
    @State private var detailPath = NavigationPath()

    var body: some View {
        NavigationSplitView {
            let allMatch = VaultLogic.filterAllItem(model.listAllItemReference(), searchText: searchText)
            List(selection: $selection) {
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
        } detail: {
            // A server's SSH key link pushes within the detail column.
            NavigationStack(path: $detailPath) {
                Group {
                    if let selection {
                        ItemDetailView(vaultID: selection.vaultID, itemID: selection.itemID)
                    } else {
                        ContentUnavailableView("No item chosen", systemImage: "magnifyingglass", description: Text("A result opens here."))
                    }
                }
                .navigationDestination(for: ItemDestination.self) { destination in
                    ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
                }
            }
        }
        .navigationSplitViewStyle(.balanced)
        .onChange(of: selection) { _, _ in detailPath = NavigationPath() }
        .onChange(of: searchText) { _, _ in model.reportActivity() }
    }
}
