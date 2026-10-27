import SwiftUI

struct VaultSharingView: View {
    var vaultID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var sharing: VaultSharing?
    @State private var email = ""
    @State private var role = Member.vaultRoleEditor
    @State private var sentEmail: String?
    @State private var isSending = false
    @State private var invitationToCancel: SentShareInvitation?
    @State private var memberToRemove: Member?
    @FocusState private var isEmailFocused: Bool
    @State private var presentationDetent = PresentationDetent.large

    private var vaultName: String { sharing?.vault.name ?? model.findVault(vaultID: vaultID)?.name ?? "Vault" }
    private var isOwner: Bool { sharing?.isOwner ?? (model.findVault(vaultID: vaultID)?.role == Member.vaultRoleOwner) }
    private var trimmedEmail: String { email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
    private var canSend: Bool { trimmedEmail.contains("@") && !isSending }

    var body: some View {
        NavigationStack {
            List {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                if isOwner {
                    inviteSection
                }
                if let completionError = sharing?.completionError, !completionError.isEmpty {
                    Section {
                        Label {
                            Text("Could not finish sharing: \(completionError)")
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundStyle(.themeWarning)
                        }
                        .font(.subheadline)
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("share-completion-error")
                    } footer: {
                        Text("Hisar shares the vault key only with the key it pinned for that person. Compare security codes with them before inviting them again.")
                    }
                }
                memberSection
                if isOwner, let allInvitation = sharing?.allInvitation, !allInvitation.isEmpty {
                    Section("Invited") {
                        ForEach(allInvitation) { invitation in
                            SentInvitationRow(invitation: invitation, isVaultNameShown: false, identifier: "vault-invitation-\(invitation.email)", onConfirmed: { await load() })
                                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                    Button("Cancel", role: .destructive) { invitationToCancel = invitation }
                                }
                                .contextMenu {
                                    Button("Cancel invitation", systemImage: "xmark.circle", role: .destructive) { invitationToCancel = invitation }
                                }
                        }
                    }
                }
            }
            .listStyle(.insetGrouped)
            .refreshable { await load() }
            .navigationTitle(isOwner ? "Share “\(vaultName)”" : "Members")
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: Member.self) { member in
                VaultMemberView(vaultID: vaultID, vaultName: vaultName, member: member, isOwner: isOwner) {
                    await load()
                }
            }
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("vault-sharing-done")
                }
            }
            .confirmationDialog(
                "Cancel the invitation to \(invitationToCancel?.email ?? "")?",
                isPresented: Binding(get: { invitationToCancel != nil }, set: { if !$0 { invitationToCancel = nil } }),
                titleVisibility: .visible
            ) {
                Button("Cancel invitation", role: .destructive) {
                    guard let invitation = invitationToCancel else { return }
                    Task {
                        if await model.cancelShareInvitation(invitationID: invitation.id) {
                            await load()
                        }
                    }
                }
                .accessibilityIdentifier("confirm-cancel-invitation")
                Button("Keep invitation", role: .cancel) {}
            } message: {
                Text("Their invitation link stops working. You can invite them again later.")
            }
            .confirmationDialog(
                "Remove \(memberToRemove.map { $0.name.isEmpty ? $0.email : $0.name } ?? "") from “\(vaultName)”?",
                isPresented: Binding(get: { memberToRemove != nil }, set: { if !$0 { memberToRemove = nil } }),
                titleVisibility: .visible
            ) {
                Button("Remove", role: .destructive) {
                    guard let member = memberToRemove else { return }
                    Task {
                        if await model.removeVaultMember(vaultID: vaultID, accountID: member.accountID) {
                            await load()
                        }
                    }
                }
            } message: {
                Text(describeVaultMemberRemoval(member: memberToRemove, vaultName: vaultName))
            }
            .task { await load() }
            .onChange(of: model.remoteChangeCount) { _, _ in
                Task { await load() }
            }
        }
        .presentationDetents([.medium, .large], selection: $presentationDetent)
        .interactiveDismissDisabled(model.isBusy)
    }

    private var inviteSection: some View {
        Section {
            TextField("Email", text: $email, prompt: Text(verbatim: "name@example.com"))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.send)
                .onSubmit(send)
                .focused($isEmailFocused)
                .accessibilityIdentifier("share-email")
            Picker("Role", selection: $role) {
                ForEach(Member.allVaultRole, id: \.self) { role in
                    Text(describeVaultRole(role)).tag(role)
                }
            }
            .accessibilityIdentifier("share-role")
            Button(action: send) {
                HStack {
                    Label("Send invitation", systemImage: "paperplane")
                    if isSending {
                        Spacer()
                        ProgressView()
                    }
                }
            }
            .disabled(!canSend)
            .accessibilityIdentifier("share-vault-submit")
            if let sentEmail {
                Label("Invitation sent to \(sentEmail)", systemImage: "checkmark.circle.fill")
                    .symbolRenderingMode(.multicolor)
                    .font(.subheadline)
                    .accessibilityIdentifier("share-sent")
            }
        } header: {
            Text("Invite by email")
        } footer: {
            Text("\(describeVaultRoleAbility(role)) They accept in Hisar, or create an account from the email first; your Hisar then shares the vault key with them.")
        }
    }

    private var memberSection: some View {
        Section {
            if let sharing {
                ForEach(sharing.allMember) { member in
                    NavigationLink(value: member) {
                        MemberRow(member: member, roleText: describeVaultRole(member.role))
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        if isOwner && !member.isCurrentUser {
                            Button("Remove", role: .destructive) { memberToRemove = member }
                        }
                    }
                    .contextMenu { buildMemberMenu(member) }
                    .accessibilityIdentifier("vault-member-\(member.email)")
                }
            } else {
                Label {
                    Text("Loading members…")
                } icon: {
                    ProgressView()
                }
                .foregroundStyle(.themeSecondary)
            }
        } header: {
            if let sharing {
                Text(sharing.allMember.count == 1 ? "1 member" : "\(sharing.allMember.count) members")
            } else {
                Text("Members")
            }
        } footer: {
            Text(isOwner
                 ? "Everyone here can open the items in this vault. Tap a person to compare their security code, change their role or remove them."
                 : "Everyone here can open the items in this vault. Only its owners can share it or change who has access.")
        }
    }

    @ViewBuilder
    private func buildMemberMenu(_ member: Member) -> some View {
        if isOwner && !member.isCurrentUser {
            Picker("Role", selection: Binding(get: { member.role }, set: { newRole in changeRole(of: member, to: newRole) })) {
                ForEach(Member.allVaultRole, id: \.self) { role in
                    Text(describeVaultRole(role)).tag(role)
                }
            }
            Button("Remove from vault", systemImage: "person.badge.minus", role: .destructive) { memberToRemove = member }
        }
    }

    private func changeRole(of member: Member, to newRole: String) {
        guard newRole != member.role else { return }
        Task {
            if await model.changeVaultMemberRole(vaultID: vaultID, accountID: member.accountID, role: newRole) {
                await load()
            }
        }
    }

    private func send() {
        guard canSend else { return }
        let invitedEmail = trimmedEmail
        isSending = true
        Task {
            defer { isSending = false }
            if await model.shareVault(vaultID: vaultID, email: invitedEmail, role: role) {
                sentEmail = invitedEmail
                email = ""
                isEmailFocused = false
                await load()
            }
        }
    }

    private func load() async {
        if let loaded = await model.loadVaultSharing(vaultID: vaultID) {
            sharing = loaded
        }
    }
}

struct VaultMemberView: View {
    var vaultID: String
    var vaultName: String
    var member: Member
    var isOwner: Bool
    var reload: () async -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var selectedRole: String
    @State private var savedRole: String
    @State private var isRemoveConfirmationShown = false

    init(vaultID: String, vaultName: String, member: Member, isOwner: Bool, reload: @escaping () async -> Void) {
        self.vaultID = vaultID
        self.vaultName = vaultName
        self.member = member
        self.isOwner = isOwner
        self.reload = reload
        _selectedRole = State(initialValue: member.role)
        _savedRole = State(initialValue: member.role)
    }

    private var isManageable: Bool { isOwner && !member.isCurrentUser }

    var body: some View {
        List {
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            Section {
                VStack(spacing: 6) {
                    Image(systemName: "person.crop.circle.fill")
                        .font(.largeTitle)
                        .imageScale(.large)
                        .foregroundStyle(.themeSecondary)
                        .accessibilityHidden(true)
                    Text(member.isCurrentUser ? "\(member.name) (you)" : member.name)
                        .font(.title2.weight(.bold))
                        .multilineTextAlignment(.center)
                    Text(member.email)
                        .foregroundStyle(.themeSecondary)
                        .multilineTextAlignment(.center)
                        .textSelection(.enabled)
                }
                .frame(maxWidth: .infinity)
                .listRowBackground(Color.clear)
            }
            if member.isKeyChanged {
                Section {
                    Label {
                        Text("\(member.name)'s key changed since you last shared with them. Hisar refuses to share with them until you compare security codes again.")
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundStyle(.themeWarning)
                    }
                    .accessibilityIdentifier("member-key-changed")
                }
            }
            if !member.securityCode.isEmpty {
                Section {
                    Text(member.securityCode)
                        .font(.title3.monospaced().weight(.medium))
                        .frame(maxWidth: .infinity)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("member-security-code")
                    if member.isCurrentUser {
                        ShareLink(item: "My Hisar security code: \(member.securityCode)") {
                            Label("Share my security code", systemImage: "square.and.arrow.up")
                        }
                    }
                } header: {
                    Text("Security code")
                } footer: {
                    Text(describeSecurityCode(member))
                }
            }
            Section {
                if isManageable {
                    Picker("Role", selection: $selectedRole) {
                        ForEach(Member.allVaultRole, id: \.self) { role in
                            Text(describeVaultRole(role)).tag(role)
                        }
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("member-role")
                } else {
                    LabeledContent("Role", value: describeVaultRole(savedRole))
                }
            } footer: {
                Text(describeVaultRoleAbility(selectedRole))
            }
            if isManageable {
                Section {
                    Button("Remove from vault", systemImage: "person.badge.minus", role: .destructive) {
                        isRemoveConfirmationShown = true
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("remove-member")
                }
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(member.name)
        .navigationBarTitleDisplayMode(.inline)
        .onChange(of: selectedRole) { _, newRole in
            guard newRole != savedRole else { return }
            Task {
                if await model.changeVaultMemberRole(vaultID: vaultID, accountID: member.accountID, role: newRole) {
                    savedRole = newRole
                    await reload()
                } else {
                    selectedRole = savedRole
                }
            }
        }
        .confirmationDialog("Remove \(member.name.isEmpty ? member.email : member.name) from “\(vaultName)”?", isPresented: $isRemoveConfirmationShown, titleVisibility: .visible) {
            Button("Remove", role: .destructive) {
                Task {
                    if await model.removeVaultMember(vaultID: vaultID, accountID: member.accountID) {
                        await reload()
                        dismiss()
                    }
                }
            }
            .accessibilityIdentifier("confirm-remove-member")
        } message: {
            Text(describeVaultMemberRemoval(member: member, vaultName: vaultName))
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

func describeVaultRoleAbility(_ role: String) -> String {
    switch role {
    case Member.vaultRoleOwner: return "Owners open and change items, share the vault, change roles and remove people."
    case Member.vaultRoleEditor: return "Editors open and change items."
    case Member.vaultRoleReader: return "Readers open items but cannot change them."
    default: return ""
    }
}

struct MemberRow: View {
    var member: Member
    var roleText: String

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: member.isCurrentUser ? "person.crop.circle.fill" : "person.crop.circle")
                .font(.title3)
                .foregroundStyle(.themeSecondary)
                .frame(width: 32)
            VStack(alignment: .leading, spacing: 2) {
                Text(member.isCurrentUser ? "\(member.name) (you)" : member.name)
                    .foregroundStyle(.primary)
                Text(member.email)
                    .font(.caption)
                    .foregroundStyle(.themeSecondary)
                if member.isKeyChanged {
                    Label("Their key changed since you last shared with them. Check the security code with them before sharing again.", systemImage: "exclamationmark.triangle.fill")
                        .font(.caption)
                        .foregroundStyle(.themeWarning)
                }
            }
            Spacer()
            Text(roleText)
                .font(.subheadline)
                .foregroundStyle(.themeSecondary)
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}

func describeVaultMemberRemoval(member: Member?, vaultName: String) -> String {
    let name = member.map { $0.name.isEmpty ? $0.email : $0.name } ?? ""
    return "\(name) is removed from “\(vaultName)”, and Hisar encrypts the vault again with a new key for the people who remain. \(name) can still read copies their devices already downloaded. Change any password or secret they should no longer know."
}

func describeSecurityCode(_ member: Member) -> String {
    if member.isCurrentUser {
        return "This is your security code. Read it to someone who is checking the code they see for you."
    }
    let name = member.name.isEmpty ? member.email : member.name
    return "Ask \(name) for the security code Hisar shows for their own account (Settings → People, the row marked you), in person or over a channel you trust. It must match this code."
}
