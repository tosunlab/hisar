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
    @State private var roleChange: VaultRoleChange?
    @FocusState private var isEmailFocused: Bool
    @State private var presentationDetent = PresentationDetent.large
    @State private var path: [SharingMemberRoute] = []

    private var vaultName: String { sharing?.vault.name ?? model.findVault(vaultID: vaultID)?.name ?? "Vault" }
    private var isOwner: Bool { sharing?.isOwner ?? (model.findVault(vaultID: vaultID)?.role == Member.vaultRoleOwner) }
    private var trimmedEmail: String { email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
    private var canSend: Bool { trimmedEmail.contains("@") && !isSending }

    var body: some View {
        NavigationStack(path: $path) {
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
                        Text("Hisar shares the vault key only with the key it pinned for that person. Contact them over a channel you trust. Comparing security codes does not restore sharing.")
                            .foregroundStyle(.themeSecondary)
                    }
                }
                memberSection
                if isOwner, let allInvitation = sharing?.allInvitation, !allInvitation.isEmpty {
                    Section {
                        ForEach(allInvitation) { invitation in
                            SentInvitationRow(invitation: invitation, isVaultNameShown: false, identifier: "vault-invitation-\(invitation.email)", onConfirmed: { await load() })
                                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                    Button("Cancel", role: .destructive) { invitationToCancel = invitation }
                                }
                                .contextMenu {
                                    Button("Cancel invitation", systemImage: "xmark.circle", role: .destructive) { invitationToCancel = invitation }
                                }
                        }
                    } header: {
                        Text("Invited")
                            .foregroundStyle(.themeSecondary)
                    }
                }
            }
            .listStyle(.insetGrouped)
            .refreshable { await load() }
            .navigationTitle(isOwner ? "Share “\(vaultName)”" : "Members")
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: SharingMemberRoute.self) { route in
                if let member = sharing?.allMember.first(where: { $0.accountID == route.accountID }) {
                    VaultMemberView(vaultID: vaultID, vaultName: vaultName, member: member, isOwner: isOwner, close: { path.removeAll() }) {
                        await load()
                    }
                } else {
                    SharingMemberUnavailableView()
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
            .confirmationDialog(roleChange?.title(vaultName: vaultName) ?? "", isPresented: Binding(get: { roleChange != nil }, set: { if !$0 { roleChange = nil } }), titleVisibility: .visible) {
                Button(roleChange?.role == Member.vaultRoleOwner ? "Make owner" : "Change role", role: .destructive) {
                    if let change = roleChange { submitRole(of: change.member, to: change.role) }
                }
                .accessibilityIdentifier("confirm-role-change")
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(roleChange?.message ?? "")
            }
            .task { await load() }
            .onChange(of: model.remoteChangeCount) { _, _ in
                Task { await load() }
            }
        }
        .presentationDetents([.medium, .large], selection: $presentationDetent)
        .background(SharingDismissalPolicy(isDisabled: model.isBusy))
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
                .foregroundStyle(.themeSecondary)
        } footer: {
            Text("\(describeVaultRoleAbility(role)) They accept in Hisar, or create an account from the email first. For a new recipient, review their account after they accept before Hisar shares the vault key.")
                .foregroundStyle(.themeSecondary)
        }
    }

    private var memberSection: some View {
        Section {
            if let sharing {
                ForEach(sharing.allMember) { member in
                    NavigationLink(value: SharingMemberRoute(accountID: member.accountID)) {
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
                    .foregroundStyle(.themeSecondary)
            } else {
                Text("Members")
                    .foregroundStyle(.themeSecondary)
            }
        } footer: {
            Text(isOwner
                 ? "Everyone here can open the items in this vault. Tap a person to compare their security code, change their role or remove them."
                 : "Everyone here can open the items in this vault. Only its owners can share it or change who has access.")
                .foregroundStyle(.themeSecondary)
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
        if newRole == Member.vaultRoleOwner || member.role == Member.vaultRoleOwner {
            roleChange = VaultRoleChange(member: member, role: newRole)
        } else {
            submitRole(of: member, to: newRole)
        }
    }

    private func submitRole(of member: Member, to newRole: String) {
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

// Updating SwiftUI's interactiveDismissDisabled preference can recreate this sheet on iOS 27 and discard its form.
private struct SharingDismissalPolicy: UIViewControllerRepresentable {
    var isDisabled: Bool

    func makeUIViewController(context: Context) -> PolicyController {
        PolicyController()
    }

    func updateUIViewController(_ controller: PolicyController, context: Context) {
        controller.isDisabled = isDisabled
        controller.applyPolicy()
    }

    final class PolicyController: UIViewController {
        var isDisabled = false

        override func viewDidAppear(_ animated: Bool) {
            super.viewDidAppear(animated)
            applyPolicy()
        }

        func applyPolicy() {
            var sheet: UIViewController = self
            while let parent = sheet.parent {
                sheet = parent
            }
            sheet.isModalInPresentation = isDisabled
        }
    }
}

struct VaultMemberView: View {
    var vaultID: String
    var vaultName: String
    var member: Member
    var isOwner: Bool
    /// Returns to the members. The sheet owns the path: once a reload drops this member, the page's own DismissAction may
    /// belong to a replaced view and close the whole sheet instead.
    var close: () -> Void
    var reload: () async -> Void

    @EnvironmentObject private var model: AppModel
    @State private var pendingRole: String?
    @State private var isRemoveConfirmationShown = false
    @State private var roleChange: VaultRoleChange?

    private var selectedRole: String { pendingRole ?? member.role }

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
                        Text("\(member.name)'s key changed since this iPhone first shared with them, so Hisar stopped sharing with them. Contact them over a channel you trust. Comparing security codes does not restore sharing.")
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
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text(describeSecurityCode(member))
                        .foregroundStyle(.themeSecondary)
                }
            }
            Section {
                if isManageable {
                    Picker("Role", selection: Binding(get: { selectedRole }, set: chooseRole)) {
                        ForEach(Member.allVaultRole, id: \.self) { role in
                            Text(describeVaultRole(role)).tag(role)
                        }
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("member-role")
                } else {
                    LabeledContent("Role", value: describeVaultRole(member.role))
                }
            } footer: {
                Text(describeVaultRoleAbility(selectedRole))
                    .foregroundStyle(.themeSecondary)
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
        .refreshable { await reload() }
        .confirmationDialog(roleChange?.title(vaultName: vaultName) ?? "", isPresented: Binding(get: { roleChange != nil }, set: { if !$0 { roleChange = nil } }), titleVisibility: .visible) {
            Button(roleChange?.role == Member.vaultRoleOwner ? "Make owner" : "Change role", role: .destructive) {
                if let change = roleChange { submitRole(change.role) }
            }
            .accessibilityIdentifier("confirm-role-change")
            Button("Cancel", role: .cancel) {}
        } message: {
            Text(roleChange?.message ?? "")
        }
        .confirmationDialog("Remove \(member.name.isEmpty ? member.email : member.name) from “\(vaultName)”?", isPresented: $isRemoveConfirmationShown, titleVisibility: .visible) {
            Button("Remove", role: .destructive) {
                Task {
                    if await model.removeVaultMember(vaultID: vaultID, accountID: member.accountID) {
                        close()
                        await reload()
                    }
                }
            }
            .accessibilityIdentifier("confirm-remove-member")
        } message: {
            Text(describeVaultMemberRemoval(member: member, vaultName: vaultName))
        }
    }

    private func chooseRole(_ newRole: String) {
        guard newRole != member.role, pendingRole == nil else { return }
        if newRole == Member.vaultRoleOwner || member.role == Member.vaultRoleOwner {
            roleChange = VaultRoleChange(member: member, role: newRole)
        } else {
            submitRole(newRole)
        }
    }

    private func submitRole(_ newRole: String) {
        guard pendingRole == nil else { return }
        pendingRole = newRole
        Task {
            _ = await model.changeVaultMemberRole(vaultID: vaultID, accountID: member.accountID, role: newRole)
            let error = model.errorMessage
            await reload()
            if let error { model.errorMessage = error }
            pendingRole = nil
        }
    }
}

struct VaultRoleChange {
    var member: Member
    var role: String

    func title(vaultName: String) -> String {
        role == Member.vaultRoleOwner ? "Make \(member.name) an owner of \(vaultName)?" : "Change \(member.name)'s role from Owner to \(describeVaultRole(role))?"
    }

    var message: String {
        role == Member.vaultRoleOwner
            ? "Owners can share, rename and delete this vault, change roles and remove other owners, including you."
            : "They lose the ability to share, rename and delete this vault, change roles and remove owners."
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
    case Member.vaultRoleOwner: return "Owners view and change items, share, rename and delete the vault, change roles and remove people."
    case Member.vaultRoleEditor: return "Editors view and change items."
    case Member.vaultRoleReader: return "Readers view items but can't change them."
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
                    Label("Key changed. Hisar stopped sharing with them.", systemImage: "exclamationmark.triangle.fill")
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

struct SharingMemberRoute: Hashable {
    var accountID: String
}

struct SharingMemberUnavailableView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ProgressView()
            .task {
                model.showNotice("This person is no longer a member.")
                dismiss()
            }
    }
}
