import SwiftUI

struct VaultDestination: Hashable {
    var vaultID: String
}

struct ItemDestination: Hashable {
    var vaultID: String
    var itemID: String
}

struct HomeView: View {
    @EnvironmentObject private var model: AppModel
    @State private var navigationPath = NavigationPath()
    @State private var searchText = ""
    @State private var isSettingsShown = false
    @State private var isNewVaultShown = false

    var body: some View {
        NavigationStack(path: $navigationPath) {
            List {
                if !model.allPendingApproval.isEmpty {
                    PendingApprovalSection(allApproval: model.allPendingApproval) { approval in
                        model.reviewApproval(approval)
                    }
                }
                if searchText.isEmpty {
                    vaultSection
                } else {
                    searchSection
                }
            }
            .listStyle(.insetGrouped)
            .applyListTheme()
            .refreshable { await model.refreshAll() }
            .safeAreaInset(edge: .top, spacing: 0) {
                if model.status.isOffline {
                    OfflineBanner()
                }
            }
            .navigationTitle("Vaults")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .searchable(text: $searchText, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search all items")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        isSettingsShown = true
                    } label: {
                        Image(systemName: "gearshape")
                    }
                    .accessibilityLabel("Settings")
                    .accessibilityIdentifier("open-settings")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        model.lock()
                    } label: {
                        Image(systemName: "lock")
                    }
                    .accessibilityLabel("Lock")
                    .accessibilityIdentifier("lock-now")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isNewVaultShown = true
                    } label: {
                        Image(systemName: "plus")
                    }
                    .accessibilityLabel("New vault")
                    .accessibilityIdentifier("new-vault")
                }
            }
            .navigationDestination(for: VaultDestination.self) { destination in
                ItemListView(vaultID: destination.vaultID)
            }
            .navigationDestination(for: ItemDestination.self) { destination in
                ItemDetailView(vaultID: destination.vaultID, itemID: destination.itemID)
            }
        }
        .onChange(of: navigationPath) { _, _ in model.reportActivity() }
        .onChange(of: searchText) { _, _ in model.reportActivity() }
        .sheet(isPresented: $isSettingsShown) {
            SettingsView()
                .environmentObject(model)
        }
        .sheet(isPresented: $isNewVaultShown) {
            NewVaultView { vault in
                navigationPath.append(VaultDestination(vaultID: vault.id))
            }
            .environmentObject(model)
        }
    }

    @ViewBuilder
    private var vaultSection: some View {
        Section {
            if model.allVault.isEmpty {
                HStack {
                    if model.isRefreshing {
                        ProgressView().tint(Theme.textMuted)
                        Text("Loading vaults…")
                    } else {
                        Text("No vaults")
                    }
                }
                .foregroundStyle(Theme.textMuted)
                .listRowBackground(Theme.surface)
            }
            ForEach(model.allVault) { vault in
                NavigationLink(value: VaultDestination(vaultID: vault.id)) {
                    HStack(spacing: 14) {
                        Image(systemName: vault.type == "personal" ? "person.crop.square" : "person.2.square.stack")
                            .font(.title3)
                            .foregroundStyle(Theme.textSecondary)
                            .frame(width: 32)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(vault.name)
                                .foregroundStyle(Theme.text)
                            Text(vault.type == "personal" ? "Personal" : "Shared · \(vault.role)")
                                .font(.caption)
                                .foregroundStyle(Theme.textMuted)
                        }
                        Spacer()
                        Text("\(model.countItem(vaultID: vault.id))")
                            .font(.subheadline.monospacedDigit())
                            .foregroundStyle(Theme.textMuted)
                    }
                    .padding(.vertical, 4)
                }
                .listRowBackground(Theme.surface)
                .accessibilityIdentifier("vault-\(vault.name)")
            }
        } header: {
            Text(model.status.email)
                .textCase(nil)
                .foregroundStyle(Theme.textMuted)
        }
    }

    @ViewBuilder
    private var searchSection: some View {
        let allMatch = VaultLogic.filterAllItem(model.listAllItemReference(), searchText: searchText)
        Section {
            if allMatch.isEmpty {
                Text("No items match “\(searchText)”.")
                    .foregroundStyle(Theme.textMuted)
                    .listRowBackground(Theme.surface)
            }
            ForEach(allMatch) { reference in
                NavigationLink(value: ItemDestination(vaultID: reference.vaultID, itemID: reference.item.id)) {
                    ItemRow(item: reference.item, vaultName: reference.vaultName)
                }
                .listRowBackground(Theme.surface)
            }
        }
    }
}

struct ItemRow: View {
    var item: ItemDetail
    var vaultName: String?

    var body: some View {
        HStack(spacing: 14) {
            ItemIcon(item: item)
            VStack(alignment: .leading, spacing: 2) {
                Text(item.title)
                    .foregroundStyle(Theme.text)
                    .lineLimit(1)
                Text(buildSubtitle())
                    .font(.caption)
                    .foregroundStyle(Theme.textMuted)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 2)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("item-\(item.title)")
    }

    private func buildSubtitle() -> String {
        let detail = item.isSSHKey ? (item.keyType.isEmpty ? "SSH key" : item.keyType) : item.username
        guard let vaultName else { return detail }
        return detail.isEmpty ? vaultName : "\(detail) · \(vaultName)"
    }
}
