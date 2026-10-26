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
    case tag(TagDestination)
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

struct VaultsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var selection: VaultsSelection?
    @State private var detailPath = NavigationPath()
    @State private var isNewVaultShown = false
    @State private var isNewLoginShown = false
    @State private var isNewSecureNoteShown = false
    @State private var isNewCreditCardShown = false
    @State private var isNewIdentityShown = false
    @State private var isNewAPICredentialShown = false
    @State private var isNewAddressShown = false
    @State private var isNewSoftwareLicenseShown = false
    @State private var isNewServerShown = false
    @State private var isNewDatabaseShown = false
    @State private var refusal: Refusal?

    private var personalVault: VaultSummary? { model.allVault.first { $0.type == "personal" } }

    // A favorite deleted from its detail screen, or on another device, leaves the detail column.
    private var isSelectedItemAvailable: Bool {
        guard case .item(let destination) = selection else { return true }
        return model.allItemByVaultID[destination.vaultID]?.contains { $0.id == destination.itemID } ?? false
    }

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                if model.status.isOffline {
                    Section {
                        OfflineNote()
                    }
                }
                PendingApprovalCardSection(
                    allApproval: model.allPendingApproval,
                    isBusy: model.isAnsweringApproval,
                    review: { model.reviewApproval($0) },
                    approve: { approval in Task { await model.approve(approval) } },
                    deny: { approval in Task { await model.deny(approval) } },
                    showAll: { model.selectedTab = .approvals }
                )
                RemovedVaultNoticeSection(allNotice: model.allRemovedVaultNotice) { model.dismissRemovedVaultNotice($0) }
                keyWarningSection
                receivedInvitationSection
                organizationInvitationSection
                favoriteSection
                let grouping = OrganizationLogic.groupVaults(model.allVault, allOrganization: model.allOrganization, allWaiting: model.waitingVaultKeys.allWaiting)
                vaultSection(grouping.allOtherVault)
                ForEach(grouping.allGroup) { group in
                    organizationVaultSection(group)
                }
                if !model.allVault.isEmpty {
                    Section {
                        ForEach(ItemCategoryDestination.allCases, id: \.self) { category in
                            NavigationLink(value: VaultsSelection.category(category)) {
                                Label(category.title, systemImage: category.systemImage)
                                    .badge(model.listAllItemReference().filter { category.includes($0.item) }.count)
                            }
                            .accessibilityIdentifier("category-\(category.rawValue)")
                        }
                    } header: {
                        Text("By type")
                            .foregroundStyle(.themeSecondary)
                    }
                }
                let allTag = VaultLogic.listAllTag(model.listAllItemReference())
                if !allTag.isEmpty {
                    Section {
                        ForEach(allTag) { tag in
                            NavigationLink(value: VaultsSelection.tag(TagDestination(tag: tag.name))) {
                                Label(tag.name, systemImage: "tag")
                                    .badge(tag.itemCount)
                            }
                            .accessibilityIdentifier("tag-\(tag.name)")
                        }
                    } header: {
                        Text("Tags")
                            .foregroundStyle(.themeSecondary)
                    }
                }
            }
            .applyThemedListStyle()
            .refreshable { await model.refreshAll() }
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
                        Button {
                            isNewLoginShown = true
                        } label: {
                            Label("New login", systemImage: "person.badge.key")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-login")
                        Button {
                            isNewSecureNoteShown = true
                        } label: {
                            Label("New secure note", systemImage: "note.text")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-secure-note")
                        Button {
                            isNewCreditCardShown = true
                        } label: {
                            Label("New credit card", systemImage: "creditcard")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-credit-card")
                        Button {
                            isNewIdentityShown = true
                        } label: {
                            Label("New identity", systemImage: "person.text.rectangle")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-identity")
                        Button {
                            isNewAPICredentialShown = true
                        } label: {
                            Label("New API credential", systemImage: "curlybraces.square")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-api-credential")
                        Button {
                            isNewAddressShown = true
                        } label: {
                            Label("New address", systemImage: "mappin.and.ellipse")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-address")
                        Button {
                            isNewSoftwareLicenseShown = true
                        } label: {
                            Label("New software license", systemImage: "checkmark.seal")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-software-license")
                        Button {
                            isNewServerShown = true
                        } label: {
                            Label("New server", systemImage: "server.rack")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-server")
                        Button {
                            isNewDatabaseShown = true
                        } label: {
                            Label("New database", systemImage: "cylinder.split.1x2")
                        }
                        .disabled(personalVault == nil)
                        .accessibilityIdentifier("new-database")
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
        .sheet(isPresented: $isNewLoginShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewAddressShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categoryAddress) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewDatabaseShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categoryDatabase) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewServerShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categoryServer) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewSoftwareLicenseShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categorySoftwareLicense) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewAPICredentialShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categoryAPICredential) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewIdentityShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categoryIdentity) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewCreditCardShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categoryCreditCard) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
        .sheet(isPresented: $isNewSecureNoteShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil, newCategory: ItemDetail.categorySecureNote) { savedItem in
                    if let savedItem {
                        selection = .item(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
                    }
                }
                .environmentObject(model)
            }
        }
    }

    @ViewBuilder
    private var receivedInvitationSection: some View {
        if !model.allReceivedInvitation.isEmpty || !model.allWaitingInvitation.isEmpty {
            Section {
                ForEach(model.allReceivedInvitation) { invitation in
                    ReceivedInvitationRow(invitation: invitation, identifierPrefix: "banner-") {
                        Task {
                            if await model.acceptShareInvitation(invitationID: invitation.id) {
                                model.showNotice("Accepted. The vault appears once \(invitation.inviterName)'s Hisar finishes sharing.")
                            }
                        }
                    } decline: {
                        Task {
                            if await model.declineShareInvitation(invitationID: invitation.id) {
                                model.showNotice("Invitation declined.")
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
            } header: {
                Text("Shared with you")
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    @ViewBuilder
    private var favoriteSection: some View {
        if !model.allVault.isEmpty {
            let allFavorite = model.listAllFavoriteReference()
            Section {
                if allFavorite.isEmpty {
                    Label("Star an item to keep it here, on all your devices. Only you see your favorites.", systemImage: "star")
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                        .accessibilityIdentifier("favorites-empty")
                }
                ForEach(allFavorite) { reference in
                    NavigationLink(value: VaultsSelection.item(ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id))) {
                        ItemRow(item: reference.item, vaultName: reference.vaultName)
                    }
                    .swipeActions(edge: .trailing) {
                        FavoriteSwipeButton(vaultID: reference.vaultID, item: reference.item)
                    }
                    .accessibilityIdentifier("favorite-\(reference.item.title)")
                }
            } header: {
                Text("Favorites")
                    .foregroundStyle(.themeSecondary)
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
            }
        }
    }

    @ViewBuilder
    private var organizationInvitationSection: some View {
        if !model.allOrganizationInvitation.isEmpty {
            Section {
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
        case .tag(let destination):
            ItemTagListView(tag: destination.tag)
        case nil:
            ContentUnavailableView("Choose a vault", systemImage: "lock.rectangle.stack", description: Text("Its items appear here."))
        }
    }
}

struct VaultRow: View {
    var vault: VaultSummary
    var itemCount: Int

    private var subtitle: String {
        if !vault.readIssues.isEmpty { return "\(vault.readIssues.count) unreadable records. Retry or repair." }
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

    @EnvironmentObject private var model: AppModel

    var body: some View {
        Button {
            Task { await model.setItemFavorite(vaultID: vaultID, itemID: item.id, isFavorite: !item.isFavorite) }
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

    var body: some View {
        HStack(spacing: 12) {
            AppItemIcon(item: item)
            VStack(alignment: .leading, spacing: 2) {
                Text(item.title)
                    .lineLimit(1)
                let subtitle = buildSubtitle()
                if !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                        .lineLimit(1)
                }
            }
        }
        .accessibilityElement(children: .combine)
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
        guard let vaultName else { return detail }
        return detail.isEmpty ? vaultName : "\(detail) · \(vaultName)"
    }
}
