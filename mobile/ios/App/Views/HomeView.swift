import SwiftUI

struct VaultDestination: Hashable {
    var vaultID: String
}

struct ItemDestination: Hashable {
    var vaultID: String
    var itemID: String
}

enum ItemCategoryDestination: String, Hashable, CaseIterable {
    case logins
    case sshKeys

    var title: String { self == .logins ? "Logins" : "SSH keys" }
    var systemImage: String { self == .logins ? "globe" : "key.horizontal" }

    func includes(_ item: ItemDetail) -> Bool {
        self == .sshKeys ? item.isSSHKey : item.isLogin
    }
}

struct VaultsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var navigationPath = NavigationPath()
    @State private var isNewVaultShown = false
    @State private var isNewLoginShown = false

    private var personalVault: VaultSummary? { model.allVault.first { $0.type == "personal" } }

    var body: some View {
        NavigationStack(path: $navigationPath) {
            List {
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
                receivedInvitationSection
                vaultSection
                if !model.allVault.isEmpty {
                    Section {
                        ForEach(ItemCategoryDestination.allCases, id: \.self) { category in
                            NavigationLink(value: category) {
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
            }
            .applyThemedListStyle()
            .refreshable { await model.refreshAll() }
            .navigationTitle("Vaults")
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
            .navigationDestination(for: VaultDestination.self) { destination in
                ItemListView(vaultID: destination.vaultID)
            }
            .navigationDestination(for: ItemCategoryDestination.self) { category in
                ItemCategoryListView(category: category)
            }
            .navigationDestination(for: ItemDestination.self) { destination in
                ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
            }
        }
        .onChange(of: navigationPath) { _, _ in model.reportActivity() }
        .onChange(of: model.allVault.map(\.id)) { allPreviousID, allCurrentID in
            if !Set(allPreviousID).isSubset(of: Set(allCurrentID)) && !navigationPath.isEmpty {
                navigationPath = NavigationPath()
            }
        }
        .sheet(isPresented: $isNewVaultShown) {
            NewVaultView { vault in
                navigationPath.append(VaultDestination(vaultID: vault.id))
            }
            .environmentObject(model)
        }
        .sheet(isPresented: $isNewLoginShown) {
            if let personalVault {
                ItemEditorView(vaultID: personalVault.id, originalItem: nil) { savedItem in
                    if let savedItem {
                        navigationPath.append(ItemDestination(vaultID: personalVault.id, itemID: savedItem.id))
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
    private var vaultSection: some View {
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
            ForEach(model.allVault) { vault in
                NavigationLink(value: VaultDestination(vaultID: vault.id)) {
                    VaultRow(vault: vault, itemCount: model.countItem(vaultID: vault.id))
                }
                .accessibilityIdentifier("vault-\(vault.name)")
            }
        } header: {
            Text("Vaults")
                .foregroundStyle(.themeSecondary)
        }
    }
}

struct VaultRow: View {
    var vault: VaultSummary
    var itemCount: Int

    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text(vault.name)
                Text(vault.type == "personal" ? "Only you" : "Shared · \(describeVaultRole(vault.role))")
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
            }
        } icon: {
            Image(systemName: vault.type == "personal" ? "person.crop.square" : "person.2.square.stack")
        }
        .badge(itemCount)
    }
}

struct ItemRow: View {
    var item: ItemDetail
    var vaultName: String?

    var body: some View {
        HStack(spacing: 12) {
            ItemIcon(item: item)
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
        let detail = item.isSSHKey ? (item.keyType.isEmpty ? "SSH key" : item.keyType) : item.username
        guard let vaultName else { return detail }
        return detail.isEmpty ? vaultName : "\(detail) · \(vaultName)"
    }
}
