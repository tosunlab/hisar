import SwiftUI

struct OrganizationDestination: Hashable {
    var organizationID: String
}

/// A refused change: the alert names the action, and its message is the server's sentence.
struct Refusal: Identifiable {
    var id = UUID()
    var title: String
    var message: String
}

extension View {
    func refusalAlert(_ refusal: Binding<Refusal?>) -> some View {
        alert(
            refusal.wrappedValue?.title ?? "",
            isPresented: Binding(get: { refusal.wrappedValue != nil }, set: { if !$0 { refusal.wrappedValue = nil } }),
            presenting: refusal.wrappedValue
        ) { _ in
            Button("OK", role: .cancel) {}
                .accessibilityIdentifier("refusal-ok")
        } message: { refusal in
            Text(refusal.message)
        }
    }
}

extension AppModel {
    /// The sentence of the change that just failed, for an alert instead of the screen's error banner.
    func takeRefusal(title: String) -> Refusal? {
        guard let message = errorMessage else { return nil }
        clearError()
        return Refusal(title: title, message: message)
    }
}

// Settings → Organizations: invitations for you, your organizations and Create organization….
struct OrganizationsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isCreateShown = false
    @State private var createdOrganization: OrganizationDestination?
    @State private var refusal: Refusal?

    var body: some View {
        List {
            if !model.allOrganizationInvitation.isEmpty {
                Section {
                    ForEach(model.allOrganizationInvitation) { invitation in
                        OrganizationInvitationRow(invitation: invitation) {
                            Task { await accept(invitation) }
                        } decline: {
                            Task { await decline(invitation) }
                        }
                    }
                } header: {
                    Text("Invitations for you")
                        .foregroundStyle(.themeSecondary)
                }
            }
            if !model.allOrganization.isEmpty {
                Section {
                    ForEach(model.allOrganization) { organization in
                        NavigationLink(value: OrganizationDestination(organizationID: organization.id)) {
                            Label {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(organization.name)
                                    Text("\(OrganizationRole.describe(organization.role)) · \(OrganizationLogic.describeMemberCount(organization.memberCount))")
                                        .font(.footnote)
                                        .foregroundStyle(.themeSecondary)
                                }
                            } icon: {
                                Image(systemName: "building.2")
                            }
                        }
                        .accessibilityIdentifier("organization-\(organization.name)")
                    }
                }
            }
            Section {
                Button {
                    isCreateShown = true
                } label: {
                    Label("Create organization…", systemImage: "plus.circle")
                }
                .accessibilityIdentifier("create-organization")
            } footer: {
                Text("An organization shares vaults through its teams instead of person by person. You become its first admin.")
                    .foregroundStyle(.themeSecondary)
            }
        }
        .applyThemedListStyle()
        .refreshable { await load() }
        .navigationTitle("Organizations")
        .navigationDestination(for: OrganizationDestination.self) { destination in
            OrganizationView(organizationID: destination.organizationID)
        }
        .navigationDestination(item: $createdOrganization) { destination in
            OrganizationView(organizationID: destination.organizationID)
        }
        .sheet(isPresented: $isCreateShown) {
            CreateOrganizationView { organization in
                createdOrganization = OrganizationDestination(organizationID: organization.id)
            }
            .environmentObject(model)
        }
        .refusalAlert($refusal)
        .task { await load() }
    }

    // Offline the list keeps the organizations it last loaded; the Vaults screen already says the app is offline.
    private func load() async {
        await model.loadOrganizations()
        model.clearError()
    }

    private func accept(_ invitation: OrganizationInvitation) async {
        if await model.acceptOrganizationInvitation(invitationID: invitation.id) {
            model.showNotice(OrganizationInvitationRow.describeJoined(invitation))
        } else {
            refusal = model.takeRefusal(title: "Accept")
        }
    }

    private func decline(_ invitation: OrganizationInvitation) async {
        if await model.declineOrganizationInvitation(invitationID: invitation.id) {
            model.showNotice("Invitation declined.")
        } else {
            refusal = model.takeRefusal(title: "Decline")
        }
    }
}

struct OrganizationInvitationRow: View {
    var invitation: OrganizationInvitation
    var identifierPrefix = ""
    var accept: () -> Void
    var decline: () -> Void
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static func describeJoined(_ invitation: OrganizationInvitation) -> String {
        "You joined \(invitation.organizationName). Its vaults appear once an admin adds you to a team."
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Label {
                    Text(invitation.message)
                } icon: {
                    Image(systemName: "building.2.fill")
                        .foregroundStyle(.tint)
                        .accessibilityHidden(true)
                }
                InvitationEmailText(email: invitation.invitedByEmail)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("\(identifierPrefix)organization-invitation-text")
            if dynamicTypeSize.isAccessibilitySize {
                VStack(spacing: 10) {
                    acceptButton
                    declineButton
                }
            } else {
                HStack(spacing: 10) {
                    declineButton
                    acceptButton
                }
            }
        }
        .padding(.vertical, 4)
    }

    private var acceptButton: some View {
        Button(action: accept) {
            Text("Accept").frame(maxWidth: .infinity)
        }
        .applyGlassButtonStyle(isProminent: true)
        .accessibilityIdentifier("\(identifierPrefix)accept-organization-invitation")
    }

    private var declineButton: some View {
        Button(action: decline) {
            Text("Decline").frame(maxWidth: .infinity)
        }
        .applyGlassButtonStyle(isProminent: false)
        .accessibilityIdentifier("\(identifierPrefix)decline-organization-invitation")
    }
}

struct CreateOrganizationView: View {
    var finish: (OrganizationSummary) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @FocusState private var isNameFocused: Bool

    private var canCreate: Bool { OrganizationLogic.isValidName(name) && !model.isBusy }

    var body: some View {
        NavigationStack {
            Form {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                Section {
                    TextField("Organization name", text: $name, prompt: Text(verbatim: "Acme").foregroundStyle(.themeSecondary))
                        .focused($isNameFocused)
                        .submitLabel(.done)
                        .onSubmit(create)
                        .accessibilityIdentifier("new-organization-name")
                } footer: {
                    Text("1 to 100 characters. The Hisar server sees the names of organizations and teams, but never the names of their vaults.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            .navigationTitle("Create organization")
            .navigationBarTitleDisplayMode(.inline)
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
                        Button("Create", action: create)
                            .disabled(!canCreate)
                            .accessibilityIdentifier("new-organization-create")
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .interactiveDismissDisabled(model.isBusy)
        .onAppear {
            model.clearError()
            isNameFocused = true
        }
    }

    private func create() {
        guard canCreate else { return }
        Task {
            if let organization = await model.createOrganization(name: name) {
                dismiss()
                finish(organization)
            }
        }
    }
}

// One organization: its members, invitations and teams, and for admins every change to them (§5.11 Screens).
struct OrganizationView: View {
    var organizationID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var organization: Organization?
    @State private var refusal: Refusal?
    @State private var inviteEmail = ""
    @State private var inviteRole = OrganizationRole.member
    @State private var sentEmail: String?
    @State private var isSending = false
    @FocusState private var isEmailFocused: Bool
    @State private var invitationToCancel: SentOrganizationInvitation?
    @State private var memberToRemove: Member?
    @State private var teamToDelete: Team?
    @State private var teamToRename: Team?
    @State private var teamGivenVault: Team?
    @State private var enteredName = ""
    @State private var isNewTeamShown = false
    @State private var isRenameShown = false
    @State private var isDeleteShown = false
    @State private var isLeaveShown = false
    @State private var memberRoleChange: Member?
    @State private var teamMemberToRemove: TeamMemberRemoval?
    @State private var vaultToTakeBack: TeamVaultTakeback?

    private var name: String {
        organization?.name ?? model.allOrganization.first { $0.id == organizationID }?.name ?? "Organization"
    }

    private var isAdmin: Bool { organization?.isAdmin ?? false }
    private var trimmedEmail: String { inviteEmail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
    private var canSend: Bool { trimmedEmail.contains("@") && !isSending }

    // buildContent is the loaded organization's sections, apart from the body so Swift type-checks it in time.
    @ViewBuilder
    private func buildContent(_ organization: Organization) -> some View {
        Section {
            LabeledContent("Your role", value: OrganizationRole.describe(organization.role))
                .accessibilityIdentifier("organization-role")
        } footer: {
            Text(organization.isAdmin
                 ? "Admins manage members, teams and which teams have which vaults; being an admin gives no vault."
                 : "Members get the vaults of the teams they are in. Admins of \(organization.name) manage its teams.")
                .foregroundStyle(.themeSecondary)
        }
        if organization.isAdmin {
            inviteSection
        }
        memberSection(organization)
        if organization.isAdmin && !organization.allInvitation.isEmpty {
            invitedSection(organization)
        }
        ForEach(organization.allTeam) { team in
            teamSection(team, organization: organization)
        }
        if organization.isAdmin {
            Section {
                Button {
                    enteredName = ""
                    isNewTeamShown = true
                } label: {
                    Label("New team…", systemImage: "person.3")
                }
                .accessibilityIdentifier("new-team")
            } footer: {
                if organization.allTeam.isEmpty {
                    Text("Teams get the organization's vaults. Create one, add members and give it a vault.")
                        .foregroundStyle(.themeSecondary)
                }
            }
        }
        Section {
            Button(role: .destructive) {
                isLeaveShown = true
            } label: {
                Label("Leave \(organization.name)…", systemImage: "rectangle.portrait.and.arrow.right")
            }
            .accessibilityIdentifier("leave-organization")
        }
    }

    // The body is split into steps, each a chain Swift type-checks in time.
    private var organizationList: some View {
        List {
            if let organization {
                buildContent(organization)
            } else {
                Label {
                    Text("Loading \(name)…")
                } icon: {
                    ProgressView()
                }
                .foregroundStyle(.themeSecondary)
            }
        }
        .applyThemedListStyle()
        .refreshable { await load() }
        .navigationTitle(name)
        .navigationDestination(for: SharingMemberRoute.self) { route in
            if let organization, let member = organization.allMember.first(where: { $0.accountID == route.accountID }) {
                OrganizationMemberView(organizationID: organizationID, organizationName: name, member: member, isAdmin: isAdmin, organization: organization) {
                    await load()
                }
            } else {
                SharingMemberUnavailableView()
            }
        }
        .toolbar {
            if isAdmin {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button("Rename organization…", systemImage: "pencil") {
                            enteredName = name
                            isRenameShown = true
                        }
                        .accessibilityIdentifier("rename-organization")
                        Button("Delete organization…", systemImage: "trash", role: .destructive) {
                            isDeleteShown = true
                        }
                        .accessibilityIdentifier("delete-organization")
                    } label: {
                        Label("Organization actions", systemImage: "ellipsis")
                    }
                    .accessibilityIdentifier("organization-actions")
                }
            }
        }
    }

    private var organizationListWithAlerts: some View {
        organizationList
        .alert("New team", isPresented: $isNewTeamShown) {
            TextField("Team name", text: $enteredName)
                .accessibilityIdentifier("new-team-name")
            Button("Cancel", role: .cancel) {}
            Button("Create") {
                let teamName = enteredName
                Task { await change("New team") { await model.createTeam(organizationID: organizationID, name: teamName) != nil } }
            }
            .accessibilityIdentifier("new-team-create")
        } message: {
            Text("A name of 1 to 100 characters, unique in \(name).")
        }
        .alert("Rename team", isPresented: Binding(get: { teamToRename != nil }, set: { if !$0 { teamToRename = nil } }), presenting: teamToRename) { team in
            TextField("Team name", text: $enteredName)
                .accessibilityIdentifier("rename-team-name")
            Button("Cancel", role: .cancel) {}
            Button("Rename") {
                let teamName = enteredName
                Task { await change("Rename team") { await model.renameTeam(organizationID: organizationID, teamID: team.id, name: teamName) } }
            }
            .accessibilityIdentifier("rename-team-confirm")
        }
        .alert("Rename organization", isPresented: $isRenameShown) {
            TextField("Organization name", text: $enteredName)
                .accessibilityIdentifier("rename-organization-name")
            Button("Cancel", role: .cancel) {}
            Button("Rename") {
                let organizationName = enteredName
                Task { await change("Rename organization") { await model.renameOrganization(organizationID: organizationID, name: organizationName) } }
            }
            .accessibilityIdentifier("rename-organization-confirm")
        } message: {
            Text("Everyone in \(name) sees the new name.")
        }
    }

    private var organizationListWithConfirmations: some View {
        organizationListWithAlerts
        .confirmationDialog(memberRoleChange.map { organizationRoleChangeTitle($0, organizationName: name) } ?? "", isPresented: Binding(get: { memberRoleChange != nil }, set: { if !$0 { memberRoleChange = nil } }), titleVisibility: .visible, presenting: memberRoleChange) { member in
            Button(member.role == OrganizationRole.admin ? (member.isCurrentUser ? "Make me a member" : "Make member") : "Make admin", role: member.role == OrganizationRole.admin ? .destructive : nil) {
                Task { await change(member.role == OrganizationRole.admin ? "Make member" : "Make admin") { await model.setOrganizationMemberRole(organizationID: organizationID, accountID: member.accountID, role: member.role == OrganizationRole.admin ? OrganizationRole.member : OrganizationRole.admin) } }
            }
            .accessibilityIdentifier("confirm-organization-role")
            Button("Cancel", role: .cancel) {}
        } message: { member in
            Text(organizationRoleChangeMessage(member))
        }
        .confirmationDialog(
            "Cancel the invitation to \(invitationToCancel?.email ?? "")?",
            isPresented: Binding(get: { invitationToCancel != nil }, set: { if !$0 { invitationToCancel = nil } }),
            titleVisibility: .visible,
            presenting: invitationToCancel
        ) { invitation in
            Button("Cancel invitation", role: .destructive) {
                Task { await change("Cancel invitation") { await model.cancelOrganizationInvitation(organizationID: organizationID, invitationID: invitation.id) } }
            }
            .accessibilityIdentifier("confirm-cancel-organization-invitation")
            Button("Keep invitation", role: .cancel) {}
        }
        .sheet(item: $memberToRemove) { member in
            if let organization {
                OrganizationAccessConfirmation(
                    title: "Remove \(member.name) from \(name)?",
                    message: OrganizationMemberView.describeRemoval(memberName: member.name, organizationName: name),
                    consequences: OrganizationLogic.accessConsequences(organization: organization, change: .removeMember(member.accountID)),
                    confirmLabel: "Remove from \(name)", confirmID: "confirm-remove-organization-member", refusal: $refusal, onCancel: { memberToRemove = nil }
                ) {
                    if await change("Remove from \(name)", { await model.removeOrganizationMember(organizationID: organizationID, accountID: member.accountID) }) { memberToRemove = nil }
            }
        }
            }
        .sheet(item: $teamToDelete) { team in
            if let organization {
                OrganizationAccessConfirmation(
                    title: "Delete team \(team.name)?",
                    message: "Its members lose the vaults they have only through \(team.name).",
                    consequences: OrganizationLogic.accessConsequences(organization: organization, change: .removeTeam(team.id)),
                    confirmLabel: "Delete team", confirmID: "confirm-delete-team", refusal: $refusal, onCancel: { teamToDelete = nil }
                ) {
                    if await change("Delete team", { await model.deleteTeam(organizationID: organizationID, teamID: team.id) }) { teamToDelete = nil }
                }
            }
        }
    }

    var body: some View {
        organizationListWithConfirmations
        .confirmationDialog("Delete \(name)?", isPresented: $isDeleteShown, titleVisibility: .visible) {
            Button("Delete organization", role: .destructive) {
                Task { await finishRemoval("Delete organization") { await model.deleteOrganization(organizationID: organizationID) } }
            }
            .accessibilityIdentifier("confirm-delete-organization")
        } message: {
            Text("Its teams and pending invitations are deleted with it.")
        }
        .sheet(isPresented: $isLeaveShown) {
            if let organization, let member = organization.allMember.first(where: \.isCurrentUser) {
                OrganizationAccessConfirmation(
                    title: "Leave \(name)?", message: "You lose the vaults you have through \(name)'s teams.",
                    consequences: OrganizationLogic.accessConsequences(organization: organization, change: .removeMember(member.accountID)),
                    confirmLabel: "Leave \(name)", confirmID: "confirm-leave-organization", refusal: $refusal, onCancel: { isLeaveShown = false }
                ) {
                    if await model.leaveOrganization(organizationID: organizationID) { isLeaveShown = false; dismiss() }
                    else { refusal = model.takeRefusal(title: "Leave \(name)") }
                }
            }
        }
        .sheet(item: $vaultToTakeBack) { takeback in
            if let organization {
                OrganizationAccessConfirmation(
                    title: "Take \(takeback.vault.displayName) back from \(takeback.team.name)?",
                    message: "Review who loses access or has a reduced role through the remaining teams.",
                    consequences: OrganizationLogic.accessConsequences(organization: organization, change: .takeVault(teamID: takeback.team.id, vaultID: takeback.vault.vaultID)),
                    confirmLabel: "Take back", confirmID: "confirm-take-back", refusal: $refusal, onCancel: { vaultToTakeBack = nil }
                ) {
                    if await change("Take back", { await model.takeVaultFromTeam(organizationID: organizationID, teamID: takeback.team.id, vaultID: takeback.vault.vaultID) }) {
                        vaultToTakeBack = nil
                    }
                }
            }
        }
        .sheet(item: $teamMemberToRemove) { removal in
            if let organization {
                OrganizationAccessConfirmation(
                    title: "Remove \(removal.member.displayName) from \(removal.team.name)?",
                    message: removal.member.isCurrentUser ? "You lose access to vaults this team alone gives you." : "Their vault access changes only where no other team gives the same role.",
                    consequences: OrganizationLogic.accessConsequences(organization: organization, change: .removeTeamMember(teamID: removal.team.id, accountID: removal.member.accountID)),
                    confirmLabel: "Remove from \(removal.team.name)", confirmID: "confirm-remove-team-member", refusal: $refusal,
                    onCancel: { teamMemberToRemove = nil }
                ) {
                    if await change("Remove", { await model.removeTeamMember(organizationID: organizationID, teamID: removal.team.id, accountID: removal.member.accountID) }) {
                        teamMemberToRemove = nil
                    }
                }
            }
        }
        .sheet(item: $teamGivenVault) { team in
            if let organization {
                GiveVaultSheet(
                    title: "Give a vault to \(team.name)",
                    choiceTitle: "Vault",
                    allChoice: OrganizationLogic.listAllGivableVault(organization: organization, team: team, allVault: model.allVault).map {
                        GiveVaultSheet.Choice(id: $0.vaultID, label: $0.displayName, teamID: team.id, vaultID: $0.vaultID)
                    },
                    organizationID: organizationID
                ) {
                    await load()
                }
                .environmentObject(model)
            }
        }
        .refusalAlert($refusal)
        .task { await load() }
        .onChange(of: model.remoteChangeCount) { _, _ in
            Task { await load() }
        }
    }

    private var inviteSection: some View {
        Section {
            TextField("Email", text: $inviteEmail, prompt: Text(verbatim: "name@example.com"))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.send)
                .onSubmit(send)
                .focused($isEmailFocused)
                .accessibilityIdentifier("organization-invite-email")
            Picker("Role", selection: $inviteRole) {
                ForEach(OrganizationRole.all, id: \.self) { role in
                    Text(OrganizationRole.describe(role)).tag(role)
                }
            }
            .accessibilityIdentifier("organization-invite-role")
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
            .accessibilityIdentifier("organization-invite-submit")
            if let sentEmail {
                Label("Invitation sent to \(sentEmail)", systemImage: "checkmark.circle.fill")
                    .symbolRenderingMode(.multicolor)
                    .font(.subheadline)
                    .accessibilityIdentifier("organization-invite-sent")
            }
        } header: {
            Text("Invite by email")
                .foregroundStyle(.themeSecondary)
        } footer: {
            Text("They accept in Hisar, or create an account from the email first. Every address gets the same answer.")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func memberSection(_ organization: Organization) -> some View {
        Section {
            ForEach(organization.allMember) { member in
                let allAction = OrganizationLogic.listAllMemberAction(organization: organization, member: member)
                NavigationLink(value: SharingMemberRoute(accountID: member.accountID)) {
                    MemberRow(member: member, roleText: OrganizationRole.describe(member.role))
                }
                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                    if allAction.contains(.remove) {
                        Button("Remove", role: .destructive) { memberToRemove = member }
                    }
                }
                .contextMenu {
                    ForEach(allAction, id: \.self) { action in
                        switch action {
                        case .makeAdmin:
                            Button("Make admin…", systemImage: "person.badge.shield.checkmark") { setRole(of: member, to: OrganizationRole.admin) }
                        case .makeMember:
                            Button("Make member…", systemImage: "person") { setRole(of: member, to: OrganizationRole.member) }
                        case .remove:
                            Button("Remove from \(organization.name)…", systemImage: "person.badge.minus", role: .destructive) { memberToRemove = member }
                        }
                    }
                }
                .accessibilityIdentifier("organization-member-\(member.email)")
            }
        } header: {
            Text(OrganizationLogic.describeMemberCount(organization.allMember.count))
                .foregroundStyle(.themeSecondary)
        } footer: {
            Text("Tap a person to see their security code. Ask them for the code Hisar shows for their own account in Settings → People, the row marked you. It must match the code you see for them.")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func invitedSection(_ organization: Organization) -> some View {
        Section {
            ForEach(organization.allInvitation) { invitation in
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(invitation.email)
                        Text("Waiting for them to accept")
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                    }
                } icon: {
                    Image(systemName: "envelope")
                }
                .badge(Text(OrganizationRole.describe(invitation.role)))
                .accessibilityElement(children: .combine)
                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                    Button("Cancel", role: .destructive) { invitationToCancel = invitation }
                }
                .contextMenu {
                    Button("Cancel invitation", systemImage: "xmark.circle", role: .destructive) { invitationToCancel = invitation }
                }
                .accessibilityIdentifier("organization-invitation-\(invitation.email)")
            }
        } header: {
            Text("Invited")
                .foregroundStyle(.themeSecondary)
        }
    }

    private func teamSection(_ team: Team, organization: Organization) -> some View {
        let allAddableMember = OrganizationLogic.listAllAddableMember(organization: organization, team: team)
        return Section {
            ForEach(team.allMember) { member in
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(member.displayName)
                        Text(member.email)
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                    }
                } icon: {
                    Image(systemName: member.isCurrentUser ? "person.crop.circle.fill" : "person.crop.circle")
                        .foregroundStyle(.themeSecondary)
                }
                .accessibilityElement(children: .combine)
                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                    if organization.isAdmin {
                        Button("Remove", role: .destructive) { removeFromTeam(member, team: team) }
                    }
                }
                .contextMenu {
                    if organization.isAdmin {
                        Button("Remove", systemImage: "person.badge.minus", role: .destructive) { removeFromTeam(member, team: team) }
                    }
                }
                .accessibilityIdentifier("team-\(team.name)-member-\(member.email)")
            }
            ForEach(team.allVault) { vault in
                HStack {
                    Label {
                        Text(vault.displayName)
                            .foregroundStyle(vault.canOpen ? .primary : .secondary)
                    } icon: {
                        Image(systemName: vault.canOpen ? "lock.square.stack" : "lock.slash")
                            .foregroundStyle(.themeSecondary)
                    }
                    Spacer()
                    if organization.isAdmin {
                        TeamVaultRolePicker(organization: organization, organizationID: organizationID, teamID: team.id, teamName: team.name, vaultID: vault.vaultID, vaultName: vault.displayName, role: vault.role, reload: load)
                    } else {
                    Text(describeVaultRole(vault.role))
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                }
                }
                .accessibilityElement(children: .contain)
                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                    if organization.isAdmin {
                        Button("Take back…", role: .destructive) { takeBack(vault, from: team) }
                    }
                }
                .contextMenu {
                    if organization.isAdmin {
                        Button("Take back…", systemImage: "arrow.uturn.backward", role: .destructive) { takeBack(vault, from: team) }
                    }
                }
                .accessibilityIdentifier("team-\(team.name)-vault-\(vault.displayName)")
            }
            if organization.isAdmin {
                Menu {
                    ForEach(allAddableMember) { member in
                        Button(member.isCurrentUser ? "\(member.name) (you)" : member.name) { addToTeam(member, team: team) }
                            .accessibilityIdentifier("team-\(team.name)-add-\(member.email)")
                    }
                } label: {
                    Label("Add member", systemImage: "person.badge.plus")
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .contentShape(Rectangle())
                }
                .disabled(allAddableMember.isEmpty)
                .accessibilityIdentifier("team-\(team.name)-add-member")
                Button {
                    teamGivenVault = team
                } label: {
                    Label("Give a vault…", systemImage: "plus.circle")
                }
                .disabled(OrganizationLogic.listAllGivableVault(organization: organization, team: team, allVault: model.allVault).isEmpty)
                .accessibilityIdentifier("team-\(team.name)-give-vault")
            }
        } header: {
            HStack {
                Text("Team \(team.name)")
                    .foregroundStyle(.themeSecondary)
                Spacer()
                if organization.isAdmin {
                    Menu {
                        Button("Rename…", systemImage: "pencil") {
                            enteredName = team.name
                            teamToRename = team
                        }
                        .accessibilityIdentifier("rename-team-\(team.name)")
                        Button("Delete team…", systemImage: "trash", role: .destructive) { teamToDelete = team }
                            .accessibilityIdentifier("delete-team-\(team.name)")
                    } label: {
                        Label("Team actions", systemImage: "ellipsis.circle")
                            .labelStyle(.iconOnly)
                    }
                    .accessibilityIdentifier("team-\(team.name)-actions")
                }
            }
        } footer: {
            if team.allMember.isEmpty && team.allVault.isEmpty {
                Text("No members or vaults yet.")
                    .foregroundStyle(.themeSecondary)
            }
            if organization.isAdmin && !team.allVault.isEmpty {
                Text("Someone in several teams gets the strongest of their roles.")
                    .foregroundStyle(.themeSecondary)
            }
        }
    }

    // The screen goes back only when the user is no longer in the organization (removed, or it was deleted elsewhere);
    // any other failure keeps it and says why.
    private func load() async {
        switch await model.loadOrganizationScreen(organizationID: organizationID) {
        case .loaded(let loaded):
            organization = loaded
        case .gone:
            dismiss()
        case .failed:
            refusal = model.takeRefusal(title: name)
        }
    }

    @discardableResult
    private func change(_ title: String, _ operation: () async -> Bool) async -> Bool {
        guard await operation() else {
            refusal = model.takeRefusal(title: title)
            return false
        }
        await load()
        await model.loadOrganizations()
        return true
    }

    // Deleting or leaving ends the screen, so it goes back instead of loading an organization the user is no longer in.
    private func finishRemoval(_ title: String, _ operation: () async -> Bool) async {
        if await operation() {
            dismiss()
        } else {
            refusal = model.takeRefusal(title: title)
        }
    }

    private func send() {
        guard canSend else { return }
        let invitedEmail = trimmedEmail
        let role = inviteRole
        isSending = true
        Task {
            defer { isSending = false }
            if await change("Send invitation", { await model.inviteToOrganization(organizationID: organizationID, email: invitedEmail, role: role) }) {
                sentEmail = invitedEmail
                inviteEmail = ""
                isEmailFocused = false
            }
        }
    }

    private func setRole(of member: Member, to role: String) {
        memberRoleChange = member
    }

    private func addToTeam(_ member: Member, team: Team) {
        Task { await change("Add member") { await model.addTeamMember(organizationID: organizationID, teamID: team.id, accountID: member.accountID) } }
    }

    private func removeFromTeam(_ member: TeamMember, team: Team) {
        teamMemberToRemove = TeamMemberRemoval(team: team, member: member)
    }

    private func takeBack(_ vault: TeamVault, from team: Team) {
        vaultToTakeBack = TeamVaultTakeback(team: team, vault: vault)
    }
}

private struct TeamVaultTakeback: Identifiable {
    var team: Team
    var vault: TeamVault
    var id: String { "\(team.id)/\(vault.vaultID)" }
}

private struct TeamMemberRemoval: Identifiable {
    var team: Team
    var member: TeamMember
    var id: String { "\(team.id)/\(member.accountID)" }
}

struct OrganizationAccessConfirmation: View {
    var title: String
    var message: String
    var consequences: [String]
    var confirmLabel: String
    var confirmID: String
    @Binding var refusal: Refusal?
    var onCancel: () -> Void
    var onConfirm: () async -> Void
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(title).font(.headline)
                    Text(message)
                }
                Section {
                    ForEach(consequences, id: \.self) { Text($0) }
                } header: {
                    Text("Vault access")
                        .foregroundStyle(.themeSecondary)
                }
                Section {
                    Button(confirmLabel, role: .destructive) { Task { await onConfirm() } }
                        .disabled(model.isBusy)
                        .accessibilityIdentifier(confirmID)
                }
            }
            .navigationTitle("Review changes")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel", action: onCancel).disabled(model.isBusy) }
            }
            .refusalAlert($refusal)
        }
        .interactiveDismissDisabled(model.isBusy)
    }
}

struct OrganizationMemberView: View {
    var organizationID: String
    var organizationName: String
    var member: Member
    var isAdmin: Bool
    var reload: () async -> Void
    var organization: Organization
    @State private var isRoleConfirmationShown = false

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    private var role: String { member.role }
    @State private var isRemoveConfirmationShown = false
    @State private var refusal: Refusal?

    init(organizationID: String, organizationName: String, member: Member, isAdmin: Bool, organization: Organization, reload: @escaping () async -> Void) {
        self.organizationID = organizationID
        self.organizationName = organizationName
        self.member = member
        self.isAdmin = isAdmin
        self.reload = reload
        self.organization = organization
    }
    static func describeRemoval(memberName: String, organizationName: String) -> String {
        "\(memberName) loses the vaults they have through \(organizationName)'s teams."
    }

    var body: some View {
        List {
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
                    .accessibilityIdentifier("organization-member-key-changed")
                }
            }
            if !member.securityCode.isEmpty {
                Section {
                    Text(member.securityCode)
                        .font(.title3.monospaced().weight(.medium))
                        .frame(maxWidth: .infinity)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("organization-member-security-code")
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
                LabeledContent("Role", value: OrganizationRole.describe(role))
                    .accessibilityIdentifier("organization-member-role")
                if isAdmin {
                    let isAdminRole = role == OrganizationRole.admin
                    Button(isAdminRole ? "Make member…" : "Make admin…", systemImage: isAdminRole ? "person" : "person.badge.shield.checkmark") {
                        isRoleConfirmationShown = true
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("organization-member-toggle-role")
                }
            } footer: {
                Text(role == OrganizationRole.admin
                     ? "Admins manage members, teams and which teams have which vaults; being an admin gives no vault."
                     : "Members get the vaults of the teams they are in.")
                    .foregroundStyle(.themeSecondary)
            }
            if isAdmin && !member.isCurrentUser {
                Section {
                    Button("Remove from \(organizationName)…", systemImage: "person.badge.minus", role: .destructive) {
                        isRemoveConfirmationShown = true
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("remove-organization-member")
                }
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(member.name)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await reload() }
        .confirmationDialog(organizationRoleChangeTitle(member, organizationName: organizationName), isPresented: $isRoleConfirmationShown, titleVisibility: .visible) {
            Button(role == OrganizationRole.admin ? (member.isCurrentUser ? "Make me a member" : "Make member") : "Make admin", role: role == OrganizationRole.admin ? .destructive : nil) {
                setRole(role == OrganizationRole.admin ? OrganizationRole.member : OrganizationRole.admin)
            }
            .accessibilityIdentifier("confirm-organization-role")
            Button("Cancel", role: .cancel) {}
        } message: {
            Text(organizationRoleChangeMessage(member))
        }
        .sheet(isPresented: $isRemoveConfirmationShown) {
            OrganizationAccessConfirmation(
                title: "Remove \(member.name) from \(organizationName)?",
                message: Self.describeRemoval(memberName: member.name, organizationName: organizationName),
                consequences: OrganizationLogic.accessConsequences(organization: organization, change: .removeMember(member.accountID)),
                confirmLabel: "Remove from \(organizationName)", confirmID: "confirm-remove-organization-member", refusal: $refusal, onCancel: { isRemoveConfirmationShown = false }
            ) {
                if await model.removeOrganizationMember(organizationID: organizationID, accountID: member.accountID) {
                    await reload()
                    isRemoveConfirmationShown = false
                    dismiss()
                } else {
                    refusal = model.takeRefusal(title: "Remove from \(organizationName)")
                }
            }
        }
        .refusalAlert($refusal)
    }

    private func setRole(_ newRole: String) {
        Task {
            if await model.setOrganizationMemberRole(organizationID: organizationID, accountID: member.accountID, role: newRole) {
                await reload()
            } else {
                refusal = model.takeRefusal(title: newRole == OrganizationRole.admin ? "Make admin" : "Make member")
                await reload()
            }
        }
    }
}

private func organizationRoleChangeTitle(_ member: Member, organizationName: String) -> String {
    if member.role != OrganizationRole.admin { return "Make \(member.name) an admin of \(organizationName)?" }
    return member.isCurrentUser ? "Make yourself a member of \(organizationName)?" : "Make \(member.name) a member of \(organizationName)?"
}

private func organizationRoleChangeMessage(_ member: Member) -> String {
    if member.role != OrganizationRole.admin {
        return "Admins manage members, teams and which teams have which vaults. Being an admin gives no vault by itself."
    }
    return member.isCurrentUser
        ? "You can no longer manage members, teams or which teams have which vaults. Another admin can make you an admin again."
        : "\(member.name) can no longer manage members, teams or which teams have which vaults. Another admin can make them an admin again."
}

/// Gives a team an organization vault with a role: from a team (pick the vault) or from the vault's sheet (pick the team).
struct GiveVaultSheet: View {
    struct Choice: Identifiable, Hashable {
        var id: String
        var label: String
        var teamID: String
        var vaultID: String
    }

    var title: String
    var choiceTitle: String
    var allChoice: [Choice]
    var organizationID: String
    var finish: () async -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var choiceID = ""
    @State private var role = Member.vaultRoleEditor
    @State private var refusal: Refusal?

    private var choice: Choice? { allChoice.first { $0.id == choiceID } }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker(choiceTitle, selection: $choiceID) {
                        ForEach(allChoice) { choice in
                            Text(choice.label).tag(choice.id)
                        }
                    }
                    .accessibilityIdentifier("give-vault-choice")
                    Picker("Role", selection: $role) {
                        ForEach(Member.allVaultRole, id: \.self) { role in
                            Text(describeVaultRole(role)).tag(role)
                        }
                    }
                    .accessibilityIdentifier("give-vault-role")
                } footer: {
                    Text("\(describeTeamVaultRoleAbility(role)) Everyone in the team gets the vault with this role. Someone in several teams gets the strongest of their roles.")
                        .foregroundStyle(.themeSecondary)
                }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.isBusy {
                        ProgressView()
                    } else {
                        Button("Give", action: give)
                            .disabled(choice == nil)
                            .accessibilityIdentifier("give-vault-confirm")
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .interactiveDismissDisabled(model.isBusy)
        .refusalAlert($refusal)
        .onAppear {
            model.clearError()
            if choice == nil {
                choiceID = allChoice.first?.id ?? ""
            }
        }
    }

    private func give() {
        guard let choice else { return }
        let selectedRole = role
        Task {
            if await model.giveVaultToTeam(organizationID: organizationID, teamID: choice.teamID, vaultID: choice.vaultID, role: selectedRole) {
                await finish()
                dismiss()
            } else {
                refusal = model.takeRefusal(title: title)
            }
        }
    }
}

// An organization vault's sheet: the teams that have it replace Invite by email and the member list (§5.11 Screens).
struct OrganizationVaultSharingView: View {
    var vaultID: String
    var organizationID: String

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var organization: Organization?
    @State private var isGiveShown = false
    @State private var refusal: Refusal?
    @State private var presentationDetent = PresentationDetent.large
    @State private var teamToTakeBack: Team?

    private var vaultName: String { model.findVault(vaultID: vaultID)?.name ?? "Vault" }
    private var sharing: OrganizationVaultSharing? {
        organization.map { OrganizationLogic.describeVaultSharing(organization: $0, vaultID: vaultID) }
    }

    var body: some View {
        NavigationStack {
            List {
                if let sharing {
                    Section {
                        ForEach(sharing.allTeamAccess) { access in
                            HStack {
                                Label {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(access.team.name)
                                        Text(access.team.allMember.isEmpty ? "No members yet" : access.team.allMember.map(\.displayName).joined(separator: ", "))
                                            .font(.footnote)
                                            .foregroundStyle(.themeSecondary)
                                    }
                                } icon: {
                                    Image(systemName: "person.3")
                                        .foregroundStyle(.themeSecondary)
                                }
                                Spacer()
                                if sharing.canChangeTeams, let organization {
                                    TeamVaultRolePicker(organization: organization, organizationID: organizationID, teamID: access.team.id, teamName: access.team.name, vaultID: vaultID, vaultName: vaultName, role: access.role, reload: load)
                                } else {
                                Text(describeVaultRole(access.role))
                                    .font(.subheadline)
                                    .foregroundStyle(.themeSecondary)
                            }
                            }
                            .accessibilityElement(children: .contain)
                            .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                if sharing.canChangeTeams {
                                    Button("Take back…", role: .destructive) { teamToTakeBack = access.team }
                                }
                            }
                            .contextMenu {
                                if sharing.canChangeTeams {
                                    Button("Take back…", systemImage: "arrow.uturn.backward", role: .destructive) { teamToTakeBack = access.team }
                                }
                            }
                            .accessibilityIdentifier("vault-team-\(access.team.name)")
                        }
                    } header: {
                        Text("Shared through teams in \(sharing.organizationName)")
                            .foregroundStyle(.themeSecondary)
                            .accessibilityIdentifier("shared-through-teams")
                    } footer: {
                        if let askAdminNote = sharing.askAdminNote {
                            Text(askAdminNote)
                                .foregroundStyle(.themeSecondary)
                                .accessibilityIdentifier("ask-an-admin")
                        }
                        if sharing.canChangeTeams {
                            Text("Someone in several teams gets the strongest of their roles.")
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                    if sharing.canChangeTeams {
                        Section {
                            Button {
                                isGiveShown = true
                            } label: {
                                Label("Give to a team…", systemImage: "plus.circle")
                            }
                            .disabled(sharing.allTeamWithoutVault.isEmpty)
                            .accessibilityIdentifier("give-vault-to-team")
                        }
                    }
                } else {
                    Label {
                        Text("Loading teams…")
                    } icon: {
                        ProgressView()
                    }
                    .foregroundStyle(.themeSecondary)
                }
            }
            .listStyle(.insetGrouped)
            .refreshable { await load() }
            .navigationTitle(vaultName)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("vault-sharing-done")
                }
            }
            .sheet(isPresented: $isGiveShown) {
                if let sharing {
                    GiveVaultSheet(
                        title: "Give “\(vaultName)” to a team",
                        choiceTitle: "Team",
                        allChoice: sharing.allTeamWithoutVault.map { GiveVaultSheet.Choice(id: $0.id, label: $0.name, teamID: $0.id, vaultID: vaultID) },
                        organizationID: organizationID
                    ) {
                        await load()
                    }
                    .environmentObject(model)
                }
            }
            .sheet(item: $teamToTakeBack) { team in
                if let organization {
                    OrganizationAccessConfirmation(
                        title: "Take \(vaultName) back from \(team.name)?",
                        message: "Review who loses access or has a reduced role through the remaining teams.",
                        consequences: OrganizationLogic.accessConsequences(organization: organization, change: .takeVault(teamID: team.id, vaultID: vaultID)),
                        confirmLabel: "Take back", confirmID: "confirm-take-back", refusal: $refusal, onCancel: { teamToTakeBack = nil }
                    ) { await takeBack(from: team) }
                }
            }
            .refusalAlert($refusal)
            .task { await load() }
            .onChange(of: model.remoteChangeCount) { _, _ in
                Task { await load() }
            }
        }
        .presentationDetents([.medium, .large], selection: $presentationDetent)
        .interactiveDismissDisabled(model.isBusy)
    }

    private func load() async {
        if let loaded = await model.loadOrganization(organizationID: organizationID) {
            organization = loaded
        } else if organization == nil {
            refusal = model.takeRefusal(title: "Shared through teams")
        } else {
            model.clearError()
        }
    }

    private func takeBack(from team: Team) async {
            if await model.takeVaultFromTeam(organizationID: organizationID, teamID: team.id, vaultID: vaultID) {
                await load()
            teamToTakeBack = nil
            } else {
                refusal = model.takeRefusal(title: "Take back")
            }
        }
}

func describeTeamVaultRoleAbility(_ role: String) -> String {
    if role == Member.vaultRoleOwner {
        return "Owners view and change items, share, rename and delete the vault. Access is managed through the organization's teams."
    }
    return describeVaultRoleAbility(role)
}

struct TeamVaultRolePicker: View {
    var organization: Organization
    var organizationID: String
    var teamID: String
    var teamName: String
    var vaultID: String
    var vaultName: String
    var role: String
    var reload: () async -> Void

    @EnvironmentObject private var model: AppModel
    @State private var pendingRole: String?
    @State private var isSubmitting = false
    @State private var refusal: Refusal?

    private var isPromotion: Bool { pendingRole == Member.vaultRoleOwner }
    private var confirmationMessage: String {
        let ability = isPromotion
            ? "Owners view and change items, share, rename and delete the vault. Access is managed through the organization's teams. Newly entitled members wait for an owner's Hisar to give them their keys."
            : "\(teamName)'s members lose Owner powers unless another team gives them Owner. Access is managed through the organization's teams."
        let effects = OrganizationLogic.accessConsequences(organization: organization, change: .teamRole(teamID: teamID, vaultID: vaultID, role: pendingRole ?? role))
        return ([ability, "Someone in several teams gets the strongest of their roles."] + effects).joined(separator: "\n\n")
    }

    var body: some View {
        Picker("Role", selection: Binding(get: { role }, set: choose)) {
            ForEach(Member.allVaultRole, id: \.self) { choice in
                Text(describeVaultRole(choice)).tag(choice)
            }
        }
        .pickerStyle(.menu)
        .disabled(model.isBusy || isSubmitting)
        .accessibilityLabel("Role for \(teamName) in \(vaultName)")
        .accessibilityValue(describeVaultRole(role))
        .accessibilityHint("Someone in several teams gets the strongest of their roles.")
        .accessibilityIdentifier("team-role-\(teamID)-\(vaultID)")
        .confirmationDialog(
            isPromotion ? "Make \(teamName) an owner of “\(vaultName)”?" : "Change \(teamName)'s role from Owner to \(describeVaultRole(pendingRole ?? role))?",
            isPresented: Binding(get: { pendingRole != nil }, set: { if !$0 { pendingRole = nil } }),
            titleVisibility: .visible
        ) {
            if let selected = pendingRole {
                Button(isPromotion ? "Make owner" : "Change role", role: isPromotion ? nil : .destructive) {
                    submit(selected)
                }
                .accessibilityIdentifier("confirm-team-role")
            }
            Button("Cancel", role: .cancel) { pendingRole = nil }
        } message: {
            Text(confirmationMessage)
        }
        .refusalAlert($refusal)
    }

    private func choose(_ newRole: String) {
        guard newRole != role, !isSubmitting else { return }
        if role == Member.vaultRoleOwner || newRole == Member.vaultRoleOwner {
            pendingRole = newRole
        } else {
            submit(newRole)
        }
    }

    private func submit(_ newRole: String) {
        isSubmitting = true
        pendingRole = nil
        Task {
            if !(await model.giveVaultToTeam(organizationID: organizationID, teamID: teamID, vaultID: vaultID, role: newRole)) {
                refusal = model.takeRefusal(title: "Change role")
            }
            await reload()
            isSubmitting = false
        }
    }
}
