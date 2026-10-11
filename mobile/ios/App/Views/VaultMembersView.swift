import SwiftUI

struct VaultMembersView: View {
    var vaultID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var allVaultMember: [Member]?
    @State private var isShareShown = false
    @State private var memberToRemove: Member?
    @State private var allSentInvitation: [SentShareInvitation] = []

    private var vault: VaultSummary? { model.findVault(vaultID: vaultID) }
    private var isOwner: Bool { vault?.role == Member.vaultRoleOwner }

    var body: some View {
        NavigationStack {
            List {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                            .listRowInsets(EdgeInsets())
                            .listRowBackground(Color.clear)
                    }
                }
                Section {
                    if let allVaultMember {
                        ForEach(allVaultMember) { member in
                            MemberRow(member: member, roleText: describeVaultRole(member.role))
                                .contextMenu { buildMemberMenu(member) }
                                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                    if isOwner && !member.isCurrentUser {
                                        Button("Remove", role: .destructive) { memberToRemove = member }
                                    }
                                }
                                .listRowBackground(Theme.surface)
                                .accessibilityIdentifier("vault-member-\(member.email)")
                        }
                    } else {
                        HStack {
                            ProgressView().tint(Theme.textMuted)
                            Text("Loading members…")
                        }
                        .foregroundStyle(Theme.textMuted)
                        .listRowBackground(Theme.surface)
                    }
                } header: {
                    Text(vault?.name ?? "Vault")
                        .textCase(nil)
                        .foregroundStyle(Theme.textMuted)
                } footer: {
                    Text(isOwner
                         ? "Owners can share the vault, change roles and remove people. Editors change items; readers only see them. Touch and hold a person to change their role."
                         : "Only owners of this vault can share it or change who has access.")
                }
                if isOwner && !allSentInvitation.isEmpty {
                    Section {
                        ForEach(allSentInvitation) { invitation in
                            SentInvitationRow(invitation: invitation)
                                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                    Button("Cancel", role: .destructive) {
                                        Task {
                                            if await model.cancelShareInvitation(invitationID: invitation.id) {
                                                model.showNotice("Invitation to \(invitation.email) cancelled.")
                                                await load()
                                            }
                                        }
                                    }
                                }
                                .listRowBackground(Theme.surface)
                                .accessibilityIdentifier("vault-invitation-\(invitation.email)")
                        }
                    } header: {
                        Text("Invited")
                    }
                }
                if isOwner {
                    Section {
                        Button {
                            isShareShown = true
                        } label: {
                            Label("Invite by email", systemImage: "person.badge.plus")
                                .foregroundStyle(Theme.text)
                        }
                        .listRowBackground(Theme.surface)
                        .accessibilityIdentifier("share-vault")
                    }
                }
            }
            .listStyle(.insetGrouped)
            .applyListTheme()
            .refreshable { await load() }
            .navigationTitle("Members")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("vault-members-done")
                }
            }
            .confirmationDialog("Remove \(memberToRemove?.name ?? "") from “\(vault?.name ?? "")”?", isPresented: Binding(get: { memberToRemove != nil }, set: { if !$0 { memberToRemove = nil } }), titleVisibility: .visible) {
                Button("Remove", role: .destructive) {
                    guard let member = memberToRemove else { return }
                    Task {
                        if await model.removeVaultMember(vaultID: vaultID, accountID: member.accountID) {
                            model.showNotice("\(member.name) removed.")
                            await load()
                        }
                    }
                }
            } message: {
                Text("The vault is encrypted again with a new key that only the remaining members get.")
            }
            .sheet(isPresented: $isShareShown) {
                ShareVaultView(vaultID: vaultID) {
                    Task { await load() }
                }
                .environmentObject(model)
            }
            .task { await load() }
        }
        .preferredColorScheme(.dark)
        .interactiveDismissDisabled(model.isBusy)
    }

    @ViewBuilder
    private func buildMemberMenu(_ member: Member) -> some View {
        if isOwner && !member.isCurrentUser {
            ForEach(Member.allVaultRole, id: \.self) { role in
                Button {
                    Task {
                        if await model.changeVaultMemberRole(vaultID: vaultID, accountID: member.accountID, role: role) {
                            model.showNotice("\(member.name) is now \(describeVaultRole(role).lowercased()).")
                            await load()
                        }
                    }
                } label: {
                    if member.role == role {
                        Label(describeVaultRole(role), systemImage: "checkmark")
                    } else {
                        Text(describeVaultRole(role))
                    }
                }
                .disabled(member.role == role)
            }
            Divider()
            Button("Remove from vault", role: .destructive) { memberToRemove = member }
        }
    }

    private func load() async {
        model.clearError()
        allVaultMember = await model.loadAllVaultMember(vaultID: vaultID) ?? allVaultMember ?? []
        if isOwner, let shareInvitations = await model.loadShareInvitations() {
            allSentInvitation = shareInvitations.allSent.filter { $0.vaultID == vaultID }
        }
    }
}

func describeVaultRole(_ role: String) -> String {
    switch role {
    case Member.vaultRoleOwner: return "Owner"
    case Member.vaultRoleEditor: return "Editor"
    case Member.vaultRoleReader: return "Reader"
    default: return role.capitalized
    }
}

struct MemberRow: View {
    var member: Member
    var roleText: String

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: member.isCurrentUser ? "person.crop.circle.fill" : "person.crop.circle")
                .font(.title3)
                .foregroundStyle(Theme.textSecondary)
                .frame(width: 32)
            VStack(alignment: .leading, spacing: 2) {
                Text(member.isCurrentUser ? "\(member.name) (you)" : member.name)
                    .foregroundStyle(Theme.text)
                Text(member.email)
                    .font(.caption)
                    .foregroundStyle(Theme.textMuted)
                if !member.securityCode.isEmpty {
                    Text("Security code \(member.securityCode)")
                        .font(.caption2.monospaced())
                        .foregroundStyle(Theme.textMuted)
                }
                if member.isKeyChanged {
                    Label("Their key changed since you last shared with them. Check the security code with them before sharing again.", systemImage: "exclamationmark.triangle.fill")
                        .font(.caption)
                        .foregroundStyle(Theme.warning)
                }
            }
            Spacer()
            Text(roleText)
                .font(.subheadline)
                .foregroundStyle(Theme.textMuted)
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}

struct ShareVaultView: View {
    var vaultID: String
    var finish: () -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var email = ""
    @State private var role = Member.vaultRoleEditor
    @FocusState private var isEmailFocused: Bool

    private var canShare: Bool { email.contains("@") }

    var body: some View {
        NavigationStack {
            Form {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                            .listRowInsets(EdgeInsets())
                            .listRowBackground(Color.clear)
                    }
                }
                Section {
                    TextField("Email", text: $email, prompt: Text(verbatim: "them@example.com").foregroundStyle(Theme.textMuted))
                        .keyboardType(.emailAddress)
                        .textContentType(.emailAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .focused($isEmailFocused)
                        .submitLabel(.send)
                        .onSubmit(share)
                        .accessibilityIdentifier("share-email")
                        .listRowBackground(Theme.surface)
                } header: {
                    Text("Who")
                } footer: {
                    Text("They get an email and accept in Hisar; without an account, the email lets them create one. Your Hisar then shares the vault key with them.")
                }
                Section {
                    Picker("Role", selection: $role) {
                        ForEach(Member.allVaultRole, id: \.self) { role in
                            Text(describeVaultRole(role)).tag(role)
                        }
                    }
                    .pickerStyle(.segmented)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                    .accessibilityIdentifier("share-role")
                } header: {
                    Text("Role")
                } footer: {
                    Text("Owners can share the vault and remove people, editors can change items, readers can only see them.")
                }
            }
            .applyListTheme()
            .navigationTitle("Share vault")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        model.clearError()
                        dismiss()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.isBusy {
                        ProgressView()
                    } else {
                        Button("Invite", action: share)
                            .disabled(!canShare)
                            .accessibilityIdentifier("share-vault-submit")
                    }
                }
            }
            .onAppear {
                model.clearError()
                isEmailFocused = true
            }
        }
        .preferredColorScheme(.dark)
        .interactiveDismissDisabled(model.isBusy)
    }

    private func share() {
        guard canShare, !model.isBusy else { return }
        let trimmedEmail = email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        Task {
            if await model.shareVault(vaultID: vaultID, email: trimmedEmail, role: role) {
                model.showNotice("Invitation sent to \(trimmedEmail).")
                dismiss()
                finish()
            }
        }
    }
}
