import SwiftUI

struct VaultDestination: Hashable {
    var vaultID: String
}

struct ItemDestination: Hashable {
    var vaultID: String
    var itemID: String
}

struct TagDestination: Hashable {
    var tag: String
}

// What a row of the Vaults list opens. One type, so every link in the list sets the split view's selection: beside the
// list on the open iPhone Duo, pushed over it on any other iPhone.
enum VaultsSelection: Hashable {
    case vault(VaultDestination)
    case item(ItemDestination)
    case category(ItemCategoryDestination)
    case tags
}

enum ItemCategoryDestination: String, Hashable, CaseIterable {
    case logins
    case secureNotes
    case creditCards
    case identities
    case apiCredentials
    case addresses
    case softwareLicenses
    case servers
    case databases
    case sshKeys

    var title: String {
        switch self {
        case .logins: "Logins"
        case .secureNotes: "Secure notes"
        case .creditCards: "Credit cards"
        case .identities: "Identities"
        case .apiCredentials: "API credentials"
        case .addresses: "Addresses"
        case .softwareLicenses: "Software licenses"
        case .servers: "Servers"
        case .databases: "Databases"
        case .sshKeys: "SSH keys"
        }
    }

    var systemImage: String {
        switch self {
        case .logins: "globe"
        case .secureNotes: "note.text"
        case .creditCards: "creditcard"
        case .identities: "person.text.rectangle"
        case .apiCredentials: "curlybraces.square"
        case .addresses: "mappin.and.ellipse"
        case .softwareLicenses: "checkmark.seal"
        case .servers: "server.rack"
        case .databases: "cylinder.split.1x2"
        case .sshKeys: "key.horizontal"
        }
    }

    var emptyTitle: String { "No \(title.lowercased())" }

    func includes(_ item: ItemDetail) -> Bool {
        switch self {
        case .logins: item.isLogin
        case .secureNotes: item.isSecureNote
        case .creditCards: item.isCreditCard
        case .identities: item.isIdentity
        case .apiCredentials: item.isAPICredential
        case .addresses: item.isAddress
        case .softwareLicenses: item.isSoftwareLicense
        case .servers: item.isServer
        case .databases: item.isDatabase
        case .sshKeys: item.isSSHKey
        }
    }
}

/// The item types the + menus create, in the Mac's order; the raw value names their buttons' identifiers.
enum NewItemType: String, CaseIterable, Identifiable {
    case login
    case secureNote = "secure-note"
    case creditCard = "credit-card"
    case identity
    case apiCredential = "api-credential"
    case address
    case softwareLicense = "software-license"
    case server
    case database

    /// Where a + menu is: Vaults, which creates in the Personal vault ("New login"), or a vault ("Login").
    enum Placement {
        case vaults
        case vault
    }

    /// The types on the menu itself; the others are under More….
    static let allOnMenu: [NewItemType] = [.login, .secureNote]
    static let allUnderMore: [NewItemType] = allCases.filter { !allOnMenu.contains($0) }

    var id: Self { self }

    var category: String {
        switch self {
        case .login: ItemDetail.categoryLogin
        case .secureNote: ItemDetail.categorySecureNote
        case .creditCard: ItemDetail.categoryCreditCard
        case .identity: ItemDetail.categoryIdentity
        case .apiCredential: ItemDetail.categoryAPICredential
        case .address: ItemDetail.categoryAddress
        case .softwareLicense: ItemDetail.categorySoftwareLicense
        case .server: ItemDetail.categoryServer
        case .database: ItemDetail.categoryDatabase
        }
    }

    /// The type's name within a sentence.
    private var name: String {
        switch self {
        case .login: "login"
        case .secureNote: "secure note"
        case .creditCard: "credit card"
        case .identity: "identity"
        case .apiCredential: "API credential"
        case .address: "address"
        case .softwareLicense: "software license"
        case .server: "server"
        case .database: "database"
        }
    }

    var systemImage: String {
        switch self {
        case .login: "person.badge.key"
        case .secureNote: "note.text"
        case .creditCard: "creditcard"
        case .identity: "person.text.rectangle"
        case .apiCredential: "curlybraces.square"
        case .address: "mappin.and.ellipse"
        case .softwareLicense: "checkmark.seal"
        case .server: "server.rack"
        case .database: "cylinder.split.1x2"
        }
    }

    func title(in placement: Placement) -> String {
        switch placement {
        case .vaults: "New \(name)"
        case .vault: name.prefix(1).uppercased() + name.dropFirst()
        }
    }

    func identifier(in placement: Placement) -> String {
        switch placement {
        case .vaults: "new-\(rawValue)"
        case .vault: "add-\(rawValue)"
        }
    }
}

struct NewItemButton: View {
    var type: NewItemType
    var placement: NewItemType.Placement
    var choose: (NewItemType) -> Void

    var body: some View {
        Button {
            choose(type)
        } label: {
            Label(type.title(in: placement), systemImage: type.systemImage)
        }
        .accessibilityIdentifier(type.identifier(in: placement))
    }
}

struct NewItemMoreMenu: View {
    var placement: NewItemType.Placement
    var choose: (NewItemType) -> Void

    var body: some View {
        Menu {
            ForEach(NewItemType.allUnderMore) { type in
                NewItemButton(type: type, placement: placement, choose: choose)
            }
        } label: {
            Label("More…", systemImage: "ellipsis")
        }
        .accessibilityIdentifier(placement == .vaults ? "new-more" : "add-more")
    }
}

struct VaultsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var selection: VaultsSelection?
    @State private var detailPath = NavigationPath()
    @State private var isNewVaultShown = false
    @State private var newItemType: NewItemType?
    @State private var refusal: Refusal?
    @State private var refreshFailure: String?

    private var personalVault: VaultSummary? { model.allVault.first { $0.type == "personal" } }

    // A favorite deleted from its detail screen, or on another device, leaves the detail column.
    private var isSelectedItemAvailable: Bool {
        guard case .item(let destination) = selection else { return true }
        return model.allItemByVaultID[destination.vaultID]?.contains { $0.id == destination.itemID } ?? false
    }

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
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
                PendingApprovalCardSection(
                    allApproval: model.allPendingApproval,
                    isBusy: model.isAnsweringApproval,
                    isBiometricAvailable: model.status.biometricAvailable,
                    rememberChoice: { approval in Binding(get: { model.rememberChoice(for: approval) }, set: { model.setRememberChoice($0, for: approval) }) },
                    review: { model.reviewApproval($0) },
                    approve: { approval, isRemembered in Task { await model.approve(approval, isRemembered: isRemembered) } },
                    deny: { approval in Task { await model.deny(approval) } },
                    showAll: { model.selectedTab = .approvals }
                )
                RemovedVaultNoticeSection(allNotice: model.allRemovedVaultNotice) { model.dismissRemovedVaultNotice($0) }
                keyWarningSection
                invitationSection
                favoriteSection
                let grouping = OrganizationLogic.groupVaults(model.allVault, allOrganization: model.allOrganization, allWaiting: model.waitingVaultKeys.allWaiting)
                vaultSection(grouping.allOtherVault)
                ForEach(grouping.allGroup) { group in
                    organizationVaultSection(group)
                }
                let allItemReference = model.listAllItemReference()
                let allCategoryCount = ItemCategoryDestination.allCases
                    .map { category in (category: category, count: allItemReference.filter { category.includes($0.item) }.count) }
                    .filter { $0.count > 0 }
                if !allCategoryCount.isEmpty {
                    Section {
                        ForEach(allCategoryCount, id: \.category) { categoryCount in
                            NavigationLink(value: VaultsSelection.category(categoryCount.category)) {
                                Label(categoryCount.category.title, systemImage: categoryCount.category.systemImage)
                                    .badge(categoryCount.count)
                            }
                            .accessibilityIdentifier("category-\(categoryCount.category.rawValue)")
                        }
                    } header: {
                        Text("By type")
                            .foregroundStyle(.themeSecondary)
                    }
                }
                let allTag = VaultLogic.listAllTag(allItemReference)
                if !allTag.isEmpty {
                    Section {
                        NavigationLink(value: VaultsSelection.tags) {
                            Label("Tags", systemImage: "tag")
                                .badge(allTag.count)
                        }
                        .accessibilityIdentifier("open-tags")
                    }
                }
            }
            .applyThemedListStyle()
            .refreshable { await refresh() }
            .navigationTitle("Vaults")
            .refusalAlert($refusal)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        model.lock()
                    } label: {
                        Label("Lock", systemImage: "lock")
                    }
                    .accessibilityIdentifier("lock-now")
                }
                ToolbarSpacer(.fixed, placement: .topBarTrailing)
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        ForEach(NewItemType.allOnMenu) { type in
                            NewItemButton(type: type, placement: .vaults) { newItemType = $0 }
                        }
                        .disabled(personalVault == nil)
                        NewItemMoreMenu(placement: .vaults) { newItemType = $0 }
                            .disabled(personalVault == nil)
                        Divider()
                        Button {
                            isNewVaultShown = true
                        } label: {
                            Label("New vault", systemImage: "square.stack.3d.up.badge.plus")
                        }
                        .accessibilityIdentifier("new-vault")
                    } label: {
                        Label("New", systemImage: "plus")
                    }
                    .accessibilityIdentifier("new-menu")
                }
            }
        } detail: {
            NavigationStack(path: $detailPath) {
                VaultsDetailView(selection: selection)
                    .navigationDestination(for: TagDestination.self) { destination in
                        ItemTagListView(tag: destination.tag)
                    }
                    .navigationDestination(for: ItemDestination.self) { destination in
                        ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
                    }
            }
        }
        .navigationSplitViewStyle(.balanced)
        .onChange(of: selection) { _, _ in
            detailPath = NavigationPath()
            model.reportActivity()
        }
        .onChange(of: detailPath) { _, _ in model.reportActivity() }
        .onChange(of: model.allVault.map(\.id)) { allPreviousID, allCurrentID in
            if !Set(allPreviousID).isSubset(of: Set(allCurrentID)) {
                selection = nil
            }
        }
        .onChange(of: isSelectedItemAvailable) { _, isAvailable in
            if !isAvailable {
                selection = nil
            }
        }
        .sheet(isPresented: $isNewVaultShown) {
            NewVaultView { vault in
                selection = .vault(VaultDestination(vaultID: vault.id))
            }
            .environmentObject(model)
        }
        .sheet(item: $newItemType) { type in
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: type.category) { savedItem, vaultID in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: vaultID, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
    }

    private func refresh() async {
        refreshFailure = nil
        await model.refreshAll { message in
            refreshFailure = message
            model.clearError()
        }
    }

    @ViewBuilder
    private var invitationSection: some View {
        if !model.allReceivedInvitation.isEmpty || !model.allWaitingInvitation.isEmpty || !model.allOrganizationInvitation.isEmpty {
            Section {
                ForEach(model.allReceivedInvitation) { invitation in
                    ReceivedInvitationRow(invitation: invitation, identifierPrefix: "banner-") {
                        Task {
                            if await model.acceptShareInvitation(invitationID: invitation.id) {
                                model.showNotice("Accepted. The vault appears once \(invitation.inviterName)'s Hisar finishes sharing.")
                            } else {
                                refusal = model.takeRefusal(title: "Accept")
                            }
                        }
                    } decline: {
                        Task {
                            if await model.declineShareInvitation(invitationID: invitation.id) {
                                model.showNotice("Invitation declined.")
                            } else {
                                refusal = model.takeRefusal(title: "Decline")
                            }
                        }
                    }
                }
                ForEach(model.allWaitingInvitation) { invitation in
                    Label(invitation.describe(now: Date()), systemImage: "clock")
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                        .accessibilityIdentifier("waiting-\(invitation.id)")
                }
                ForEach(model.allOrganizationInvitation) { invitation in
                    OrganizationInvitationRow(invitation: invitation, identifierPrefix: "banner-") {
                        Task {
                            if await model.acceptOrganizationInvitation(invitationID: invitation.id) {
                                model.showNotice(OrganizationInvitationRow.describeJoined(invitation))
                            } else {
                                refusal = model.takeRefusal(title: "Accept")
                            }
                        }
                    } decline: {
                        Task {
                            if await model.declineOrganizationInvitation(invitationID: invitation.id) {
                                model.showNotice("Invitation declined.")
                            } else {
                                refusal = model.takeRefusal(title: "Decline")
                            }
                        }
                    }
                }
            } header: {
                Text("Invitations for you")
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    @ViewBuilder
    private var favoriteSection: some View {
        let allFavorite = model.listAllFavoriteReference()
        if !allFavorite.isEmpty {
            Section {
                ForEach(allFavorite) { reference in
                    NavigationLink(value: VaultsSelection.item(ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id))) {
                        ItemRow(item: reference.item, vaultName: reference.vaultName, showsFavorite: false)
                    }
                    .itemRowActions(vaultID: reference.vaultID, item: reference.item)
                    .swipeActions(edge: .trailing) {
                        FavoriteSwipeButton(vaultID: reference.vaultID, item: reference.item) { refusal = $0 }
                    }
                    .accessibilityIdentifier("favorite-\(reference.item.title)")
                }
            } header: {
                Text("Favorites")
                    .foregroundStyle(.themeSecondary)
                    .accessibilityIdentifier("favorites-header")
            }
        }
    }

    @ViewBuilder
    private var keyWarningSection: some View {
        let allWarning = OrganizationLogic.describeKeyWarnings(model.waitingVaultKeys)
        if !allWarning.isEmpty {
            Section {
                ForEach(allWarning, id: \.self) { warning in
                    Label {
                        Text(warning)
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundStyle(.themeWarning)
                    }
                    .font(.subheadline)
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("vault-key-warning")
                }
            } footer: {
                Text("Hisar gives vault keys only to the keys it pinned for each person. Compare security codes in Settings → Organizations.")
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    @ViewBuilder
    private func vaultSection(_ allVault: [VaultSummary]) -> some View {
        Section {
            if model.allVault.isEmpty {
                if model.isRefreshing {
                    Label {
                        Text("Loading vaults…")
                    } icon: {
                        ProgressView()
                    }
                    .foregroundStyle(.themeSecondary)
                } else {
                    Text("No vaults")
                        .foregroundStyle(.themeSecondary)
                }
            }
            ForEach(allVault) { vault in
                NavigationLink(value: VaultsSelection.vault(VaultDestination(vaultID: vault.id))) {
                    VaultRow(vault: vault, itemCount: model.countItem(vaultID: vault.id))
                }
                .accessibilityIdentifier("vault-\(vault.name)")
            }
        } header: {
            Text("Vaults")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func organizationVaultSection(_ group: OrganizationVaultGroup) -> some View {
        Section {
            ForEach(group.allVault) { vault in
                NavigationLink(value: VaultsSelection.vault(VaultDestination(vaultID: vault.id))) {
                    VaultRow(vault: vault, itemCount: model.countItem(vaultID: vault.id))
                }
                .accessibilityIdentifier("vault-\(vault.name)")
            }
            if !group.waitingMessage.isEmpty {
                Label(group.waitingMessage, systemImage: "key")
                    .font(.subheadline)
                    .foregroundStyle(.themeSecondary)
                    .accessibilityIdentifier("waiting-vault-key-\(group.name)")
            }
        } header: {
            Text(group.name)
                .foregroundStyle(.themeSecondary)
                .accessibilityIdentifier("organization-section-\(group.name)")
        }
    }
}

// The detail column of Vaults. Only the open iPhone Duo shows the empty column; a closed phone, and every other iPhone,
// collapses the split view into one stack and shows the detail pushed over the list.
struct VaultsDetailView: View {
    var selection: VaultsSelection?

    var body: some View {
        switch selection {
        case .vault(let destination):
            ItemListView(vaultID: destination.vaultID)
        case .item(let destination):
            ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
        case .category(let category):
            ItemCategoryListView(category: category)
        case .tags:
            TagListView()
        case nil:
            ContentUnavailableView("Choose a vault", systemImage: "lock.rectangle.stack", description: Text("Its items appear here."))
        }
    }
}

struct TagListView: View {
    @EnvironmentObject private var model: AppModel
    @State private var refreshFailure: String?

    var body: some View {
        let allTag = VaultLogic.listAllTag(model.listAllItemReference())
        List {
            if let refreshFailure {
                Section {
                    ErrorBanner(message: refreshFailure)
                    Button("Retry") { Task { await refresh() } }
                        .accessibilityIdentifier("browse-refresh-retry")
                }
            }
            TagRows(allTag: allTag)
        }
        .applyThemedListStyle()
        .refreshable { await refresh() }
        .navigationTitle("Tags")
    }

    private func refresh() async {
        refreshFailure = nil
        await model.refreshAll { message in
            refreshFailure = message
            model.clearError()
        }
    }
}

struct TagRows: View {
    var allTag: [TagSummary]
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
            ForEach(allTag) { tag in
                NavigationLink(value: TagDestination(tag: tag.name)) {
                if dynamicTypeSize.isAccessibilitySize {
                    HStack(alignment: .top, spacing: 12) {
                        Image(systemName: "tag")
                            .font(.system(size: 28))
                            .frame(width: 32, height: 32, alignment: .top)
                        VStack(alignment: .leading, spacing: 4) {
                            Text(tag.name)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .fixedSize(horizontal: false, vertical: true)
                            Text(tag.itemCount == 1 ? "1 item" : "\(tag.itemCount) items")
                                .font(.subheadline)
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                } else {
                    Label(tag.name, systemImage: "tag")
                        .badge(tag.itemCount)
                }
            }
                .accessibilityIdentifier("tag-\(tag.name)")
            }
        }
}

struct VaultRow: View {
    var vault: VaultSummary
    var itemCount: Int

    private var subtitle: String {
        if !vault.readIssues.isEmpty { return "\(ReadIssueSummary(issues: vault.readIssues).text) Retry or repair." }
        if vault.isPersonalExtra { return "Legacy Personal vault. Recover items before deleting." }
        switch vault.type {
        case VaultSummary.typePersonal: return "Only you"
        case VaultSummary.typeOrganization: return "Organization · \(describeVaultRole(vault.role))"
        default: return "Shared · \(describeVaultRole(vault.role))"
        }
    }

    private var systemImage: String {
        switch vault.type {
        case VaultSummary.typePersonal: return "person.crop.square"
        case VaultSummary.typeOrganization: return "building.2"
        default: return "person.2.square.stack"
        }
    }

    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text(vault.name)
                Text(subtitle)
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
            }
        } icon: {
            Image(systemName: systemImage)
        }
        .badge(itemCount)
    }
}

/// Stars or unstars an item from a list row's swipe actions.
struct FavoriteSwipeButton: View {
    var vaultID: String
    var item: ItemDetail
    var onRefusal: (Refusal) -> Void

    @EnvironmentObject private var model: AppModel

    var body: some View {
        Button {
            Task {
                await model.setItemFavorite(vaultID: vaultID, itemID: item.id, isFavorite: !item.isFavorite) { message in
                    onRefusal(Refusal(title: item.isFavorite ? "Remove from Favorites" : "Add to Favorites", message: message))
                    model.clearError()
                }
            }
        } label: {
            Label(item.isFavorite ? "Remove from Favorites" : "Add to Favorites", systemImage: item.isFavorite ? "star.slash" : "star")
        }
        .tint(.orange)
        .accessibilityIdentifier(item.isFavorite ? "unfavorite-\(item.title)" : "favorite-swipe-\(item.title)")
    }
}

struct ItemRow: View {
    var item: ItemDetail
    var vaultName: String?
    var showsFavorite = true

    var body: some View {
        HStack(spacing: 12) {
            AppItemIcon(item: item)
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(item.title).lineLimit(1)
                    if showsFavorite && item.isFavorite {
                        Image(systemName: "star.fill")
                            .font(.caption)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityHidden(true)
                    }
                }
                let subtitle = buildSubtitle()
                if !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                        .lineLimit(1)
                }
                if let vaultName {
                    Text(vaultName)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityValue(showsFavorite && item.isFavorite ? "In Favorites" : "")
        .accessibilityIdentifier("item-\(item.title)")
    }

    private func buildSubtitle() -> String {
        let detail: String
        if item.isSSHKey {
            detail = item.keyType.isEmpty ? "SSH key" : item.keyType
        } else if item.isSecureNote {
            detail = "Secure note"
        } else if item.isCreditCard {
            detail = (item.card ?? ItemCard()).summary
        } else if item.isIdentity {
            detail = (item.identity ?? ItemIdentity()).summary
        } else if item.isAPICredential {
            detail = (item.apiCredential ?? ItemAPICredential()).summary
        } else if item.isAddress {
            detail = (item.address ?? ItemAddress()).summary
        } else if item.isSoftwareLicense {
            detail = (item.softwareLicense ?? ItemSoftwareLicense()).summary
        } else if item.isServer {
            detail = (item.server ?? ItemServer()).summary(username: item.username)
        } else if item.isDatabase {
            detail = (item.database ?? ItemDatabase()).summary
        } else {
            detail = item.username
        }
        return detail
    }
}
