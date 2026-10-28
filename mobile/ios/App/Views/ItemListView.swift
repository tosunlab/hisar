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
    @State private var issueToDelete: RecoveryDeletionReview?
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
                    Text(ReadIssueSummary(issues: vault.readIssues).text)
                    Button("Retry reads") { Task { await model.refreshAll() } }
                    ForEach(vault.readIssues) { issue in
                        VStack(alignment: .leading) {
                            Text(issue.explanation)
                            DisclosureGroup("Details") {
                                Text(issue.itemId ?? issue.vaultId).font(.footnote).textSelection(.enabled)
                            }
                            if issue.canRename {
                                Button("Repair vault name") { newName = ""; issueToRepair = issue; isRenameShown = true }
                            }
                            if issue.canDelete {
                                Button(issue.itemId == nil ? "Delete vault…" : "Delete unreadable data…", role: .destructive) {
                                    issueToDelete = RecoveryDeletionReview(vault: vault, issue: issue)
                                }
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
                    .itemRowActions(vaultID: vaultID, item: reference.item)
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
                        Text(isWritable ? "Tap + to add your first item." : "Items appear here once an editor adds them.")
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
            Text(issueToRepair != nil
                 ? "Secret references that use the previous name may need updating."
                 : "Secret references that use \(vault?.name ?? "") need the new name after you rename this vault."
                   + ((vault?.type == "shared" || vault?.isOrganization == true) ? " Everyone who has this vault sees the new name." : ""))
        }
        .alert("Can't do that", isPresented: Binding(get: { blockedReason != nil }, set: { if !$0 { blockedReason = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(blockedReason ?? "")
        }
        .sheet(item: $issueToDelete) { review in
            RecoveryDeletionSheet(review: review)
                .environmentObject(model)
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

struct CrossVaultReadWarning: View {
    @EnvironmentObject private var model: AppModel
    @State private var isReviewShown = false

    var body: some View {
        if !model.readIssueSummary.issues.isEmpty {
            Section {
                Text("Some saved data could not be opened. Results may be incomplete.")
                Text(model.readIssueSummary.text).font(.footnote)
                Button("Retry reads") { Task { await model.refreshAll() } }
                Button("Review affected vaults") { isReviewShown = true }
            }
            .accessibilityIdentifier("incomplete-results")
            .sheet(isPresented: $isReviewShown) {
                NavigationStack {
                    List(model.allVault.filter { !$0.readIssues.isEmpty }) { vault in
                        NavigationLink(vault.name) { ItemListView(vaultID: vault.id) }
                    }
                    .navigationDestination(for: ItemDestination.self) { destination in
                        ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
                    }
                    .navigationTitle("Affected vaults")
                    .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { isReviewShown = false } } }
                }
                .environmentObject(model)
            }
        }
    }
}

struct IncompleteSearchResults: View {
    var body: some View {
        ContentUnavailableView("No readable matches", systemImage: "magnifyingglass", description: Text("No readable items match this search. Some saved data could not be searched."))
    }
}

struct ItemCategoryListView: View {
    var category: ItemCategoryDestination

    @EnvironmentObject private var model: AppModel
    @State private var refreshFailure: String?
    @SceneStorage private var searchText: String

    init(category: ItemCategoryDestination) {
        self.category = category
        _searchText = SceneStorage(wrappedValue: "", "category-search-\(category.rawValue)")
    }

    var body: some View {
        let allReference = VaultLogic.filterAllItem(model.listAllItemReference().filter { category.includes($0.item) }, searchText: searchText)
        List {
            if let refreshFailure {
                Section {
                    ErrorBanner(message: refreshFailure)
                    Button("Retry") { Task { await refresh() } }
                        .accessibilityIdentifier("browse-refresh-retry")
                }
            }
            if model.status.isOffline { Section { OfflineNote() } }
            CrossVaultReadWarning()
            if refreshFailure == nil && allReference.isEmpty {
                if !searchText.isEmpty {
                    if model.readIssueSummary.issues.isEmpty { ContentUnavailableView.search(text: searchText) } else { IncompleteSearchResults() }
                } else if model.readIssueSummary.issues.isEmpty {
                    ContentUnavailableView(category.emptyTitle, systemImage: category.systemImage)
                } else {
                    ContentUnavailableView("No readable \(category.title.lowercased())", systemImage: category.systemImage, description: Text("Some saved data could not be searched."))
                }
            }
            ForEach(allReference) { reference in
                NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item, vaultName: reference.vaultName)
                }
                .itemRowActions(vaultID: reference.vaultID, item: reference.item)
            }
        }
        .applyThemedListStyle()
        .refreshable { await refresh() }
        .navigationTitle(category.title)
        .searchable(text: $searchText, prompt: "Search \(category.title.lowercased())")
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
    @SceneStorage private var searchText: String

    init(tag: String) {
        self.tag = tag
        _searchText = SceneStorage(wrappedValue: "", "tag-search-\(tag)")
    }

    var body: some View {
        let allReference = VaultLogic.filterAllItem(VaultLogic.filterAllItem(model.listAllItemReference(), tag: tag), searchText: searchText)
        List {
            if let refreshFailure {
                Section {
                    ErrorBanner(message: refreshFailure)
                    Button("Retry") { Task { await refresh() } }
                        .accessibilityIdentifier("browse-refresh-retry")
                }
            }
            if model.status.isOffline { Section { OfflineNote() } }
            CrossVaultReadWarning()
            if refreshFailure == nil && allReference.isEmpty {
                if !searchText.isEmpty {
                    if model.readIssueSummary.issues.isEmpty { ContentUnavailableView.search(text: searchText) } else { IncompleteSearchResults() }
                } else if model.readIssueSummary.issues.isEmpty {
                    ContentUnavailableView("No items are tagged \(tag)", systemImage: "tag")
                } else {
                    ContentUnavailableView("No readable items are tagged \(tag)", systemImage: "tag", description: Text("Some saved data could not be searched."))
                }
            }
            ForEach(allReference) { reference in
                NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item, vaultName: reference.vaultName)
                }
                .itemRowActions(vaultID: reference.vaultID, item: reference.item)
            }
        }
        .applyThemedListStyle()
        .refreshable { await refresh() }
        .navigationTitle(tag)
        .searchable(text: $searchText, prompt: "Search items tagged \(tag)")
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
                if model.status.isOffline { Section { OfflineNote() } }
                CrossVaultReadWarning()
                if searchText.isEmpty {
                    ContentUnavailableView("Search Hisar", systemImage: "magnifyingglass", description: Text("Find items in every vault."))
                } else if allMatch.isEmpty {
                    if model.readIssueSummary.issues.isEmpty {
                        ContentUnavailableView.search(text: searchText)
                    } else {
                        IncompleteSearchResults()
                    }
                }
                ForEach(searchText.isEmpty ? [] : allMatch) { reference in
                    NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                        ItemRow(item: reference.item, vaultName: reference.vaultName)
                    }
                    .itemRowActions(vaultID: reference.vaultID, item: reference.item)
                }
            }
            .applyThemedListStyle()
            .navigationTitle("Search")
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
        .searchable(text: $searchText, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search all items")
        .onChange(of: selection) { _, _ in detailPath = NavigationPath() }
        .onChange(of: searchText) { _, _ in model.reportActivity() }
    }
}

extension View {
    func itemRowActions(vaultID: String, item: ItemDetail) -> some View {
        modifier(ItemRowActions(vaultID: vaultID, itemID: item.id, title: item.title))
    }
}

private struct ItemRowActions: ViewModifier {
    var vaultID: String
    var itemID: String
    var title: String
    @State private var isCopyShown = false

    func body(content: Content) -> some View {
        content
            .contextMenu {
                Button("Copy…", systemImage: "doc.on.doc") { isCopyShown = true }
                    .accessibilityIdentifier("row-copy-actions")
            }
            .accessibilityAction(named: "Item actions") { isCopyShown = true }
            .sheet(isPresented: $isCopyShown) {
                ItemRowCopyChooser(vaultID: vaultID, itemID: itemID, title: title)
            }
    }
}

private struct ItemRowCopyChooser: View {
    var vaultID: String
    var itemID: String
    var title: String
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var actions: [ItemCopyAction]?
    @State private var error: String?
    @State private var isCopying = false

    var body: some View {
        NavigationStack {
            List {
                if let error { Text(error).foregroundStyle(.red).accessibilityIdentifier("row-copy-error") }
                if let actions {
                    if actions.isEmpty { Text("No copyable fields are available.") }
                    ForEach(actions) { action in
                        Button("Copy \(action.label.lowercased())") {
                            Task {
                                guard model.status.isUnlocked else { dismiss(); return }
                                isCopying = true
                                let before = model.copyCount
                                await model.copyField(vaultID: vaultID, itemID: itemID, fieldName: action.fieldName, label: action.label)
                                isCopying = false
                                if model.copyCount > before { dismiss() } else { error = model.errorMessage ?? "The field could not be copied." }
                            }
                        }
                        .disabled(isCopying)
                        .accessibilityIdentifier("row-copy-\(action.fieldName)")
                    }
                } else if error == nil {
                    ProgressView("Loading item actions")
                }
            }
            .navigationTitle(title)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } } }
            .task {
                guard let item = await model.loadItem(vaultID: vaultID, itemID: itemID) else {
                    error = model.errorMessage ?? "This item could not be opened."
                    return
                }
                guard !Task.isCancelled, model.status.isUnlocked else { return }
                actions = ItemCopyAction.available(for: item)
            }
            .onChange(of: model.status.isUnlocked) { _, unlocked in
                if !unlocked { actions = nil; dismiss() }
            }
        }
    }
}
