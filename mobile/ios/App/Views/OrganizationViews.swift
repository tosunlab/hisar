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
                Section("Invitations for you") {
                    ForEach(model.allOrganizationInvitation) { invitation in
                        OrganizationInvitationRow(invitation: invitation) {
                            Task { await accept(invitation) }
                        } decline: {
                            Task { await decline(invitation) }
                        }
                    }
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

    static func describeJoined(_ invitation: OrganizationInvitation) -> String {
        "You joined \(invitation.organizationName). Its vaults appear once an admin adds you to a team."
    }

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            row(at: context.date)
        }
    }

    private func row(at now: Date) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Label {
                VStack(alignment: .leading, spacing: 2) {
                    Text(invitation.message)
                    Text(invitation.invitedByEmail)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                    InvitationExpiryText(timestamp: invitation.expiresAt)
                }
            } icon: {
                Image(systemName: "building.2.fill")
                    .foregroundStyle(.tint)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("\(identifierPrefix)organization-invitation-text")
            HStack(spacing: 10) {
                Button(action: decline) {
                    Text("Decline").frame(maxWidth: .infinity)
                }
                .applyGlassButtonStyle(isProminent: false)
                .accessibilityIdentifier("\(identifierPrefix)decline-organization-invitation")
                Button(action: accept) {
                    Text("Accept").frame(maxWidth: .infinity)
                }
                .applyGlassButtonStyle(isProminent: true)
                .accessibilityIdentifier("\(identifierPrefix)accept-organization-invitation")
                .disabled(InvitationLifetime(invitation.expiresAt).isExpired(at: now))
            }
        }
        .padding(.vertical, 4)
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
        .navigationDestination(for: Member.self) { member in
            OrganizationMemberView(organizationID: organizationID, organizationName: name, member: member, isAdmin: isAdmin) {
                await load()
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
        .confirmationDialog(
            "Remove \(memberToRemove?.name ?? "") from \(name)?",
            isPresented: Binding(get: { memberToRemove != nil }, set: { if !$0 { memberToRemove = nil } }),
            titleVisibility: .visible,
            presenting: memberToRemove
        ) { member in
            Button("Remove from \(name)", role: .destructive) {
                Task { await change("Remove from \(name)") { await model.removeOrganizationMember(organizationID: organizationID, accountID: member.accountID) } }
            }
            .accessibilityIdentifier("confirm-remove-organization-member")
        } message: { member in
            Text(OrganizationMemberView.describeRemoval(memberName: member.name, organizationName: name))
        }
        .confirmationDialog(
            "Delete team \(teamToDelete?.name ?? "")?",
            isPresented: Binding(get: { teamToDelete != nil }, set: { if !$0 { teamToDelete = nil } }),
            titleVisibility: .visible,
            presenting: teamToDelete
        ) { team in
            Button("Delete team", role: .destructive) {
                Task { await change("Delete team") { await model.deleteTeam(organizationID: organizationID, teamID: team.id) } }
            }
            .accessibilityIdentifier("confirm-delete-team")
        } message: { team in
            Text("Its members lose the vaults they have only through \(team.name).")
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
        .confirmationDialog("Leave \(name)?", isPresented: $isLeaveShown, titleVisibility: .visible) {
            Button("Leave \(name)", role: .destructive) {
                Task { await finishRemoval("Leave \(name)") { await model.leaveOrganization(organizationID: organizationID) } }
            }
            .accessibilityIdentifier("confirm-leave-organization")
        } message: {
            Text("You lose the vaults you have through \(name)'s teams.")
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
        } footer: {
            Text("They accept in Hisar, or create an account from the email first. Every address gets the same answer.")
        }
    }

    private func memberSection(_ organization: Organization) -> some View {
        Section {
            ForEach(organization.allMember) { member in
                let allAction = OrganizationLogic.listAllMemberAction(organization: organization, member: member)
                NavigationLink(value: member) {
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
                            Button("Make admin", systemImage: "person.badge.shield.checkmark") { setRole(of: member, to: OrganizationRole.admin) }
                        case .makeMember:
                            Button("Make member", systemImage: "person") { setRole(of: member, to: OrganizationRole.member) }
                        case .remove:
                            Button("Remove from \(organization.name)…", systemImage: "person.badge.minus", role: .destructive) { memberToRemove = member }
                        }
                    }
                }
                .accessibilityIdentifier("organization-member-\(member.email)")
            }
        } header: {
            Text(OrganizationLogic.describeMemberCount(organization.allMember.count))
        } footer: {
            Text("Compare security codes with people once, in person or over a channel you trust. Tap a person to see theirs.")
        }
    }

    private func invitedSection(_ organization: Organization) -> some View {
        Section("Invited") {
            ForEach(organization.allInvitation) { invitation in
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(invitation.email)
                        Text("Waiting for them to accept")
                            .font(.footnote)
                            .foregroundStyle(.themeSecondary)
                        InvitationExpiryText(timestamp: invitation.expiresAt)
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
                    Text(describeVaultRole(vault.role))
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                }
                .accessibilityElement(children: .combine)
                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                    if organization.isAdmin {
                        Button("Take back") { takeBack(vault, from: team) }
                    }
                }
                .contextMenu {
                    if organization.isAdmin {
                        Button("Take back", systemImage: "arrow.uturn.backward") { takeBack(vault, from: team) }
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
        Task { await change(role == OrganizationRole.admin ? "Make admin" : "Make member") { await model.setOrganizationMemberRole(organizationID: organizationID, accountID: member.accountID, role: role) } }
    }

    private func addToTeam(_ member: Member, team: Team) {
        Task { await change("Add member") { await model.addTeamMember(organizationID: organizationID, teamID: team.id, accountID: member.accountID) } }
    }

    private func removeFromTeam(_ member: TeamMember, team: Team) {
        Task { await change("Remove") { await model.removeTeamMember(organizationID: organizationID, teamID: team.id, accountID: member.accountID) } }
    }

    private func takeBack(_ vault: TeamVault, from team: Team) {
        Task { await change("Take back") { await model.takeVaultFromTeam(organizationID: organizationID, teamID: team.id, vaultID: vault.vaultID) } }
    }
}

struct OrganizationMemberView: View {
    var organizationID: String
    var organizationName: String
    var member: Member
    var isAdmin: Bool
    var reload: () async -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var role: String
    @State private var isRemoveConfirmationShown = false
    @State private var refusal: Refusal?

    init(organizationID: String, organizationName: String, member: Member, isAdmin: Bool, reload: @escaping () async -> Void) {
        self.organizationID = organizationID
        self.organizationName = organizationName
        self.member = member
        self.isAdmin = isAdmin
        self.reload = reload
        _role = State(initialValue: member.role)
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
                        Text("\(member.name)'s key changed since you last shared with them. Hisar refuses to give them vault keys until you compare security codes again.")
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
                } footer: {
                    Text(member.isCurrentUser
                         ? "Others see this code for you. When they compare it with you, they know vaults they give you reach you and nobody else."
                         : "Compare it with \(member.name) once, in person or over a channel you trust. If it matches what their Hisar shows, vaults their teams get reach them and nobody else.")
                }
            }
            Section {
                LabeledContent("Role", value: OrganizationRole.describe(role))
                    .accessibilityIdentifier("organization-member-role")
                if isAdmin {
                    let isAdminRole = role == OrganizationRole.admin
                    Button(isAdminRole ? "Make member" : "Make admin", systemImage: isAdminRole ? "person" : "person.badge.shield.checkmark") {
                        setRole(isAdminRole ? OrganizationRole.member : OrganizationRole.admin)
                    }
                    .disabled(model.isBusy)
                    .accessibilityIdentifier("organization-member-toggle-role")
                }
            } footer: {
                Text(role == OrganizationRole.admin
                     ? "Admins manage members, teams and which teams have which vaults; being an admin gives no vault."
                     : "Members get the vaults of the teams they are in.")
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
        .confirmationDialog("Remove \(member.name) from \(organizationName)?", isPresented: $isRemoveConfirmationShown, titleVisibility: .visible) {
            Button("Remove from \(organizationName)", role: .destructive) {
                Task {
                    if await model.removeOrganizationMember(organizationID: organizationID, accountID: member.accountID) {
                        await reload()
                        dismiss()
                    } else {
                        refusal = model.takeRefusal(title: "Remove from \(organizationName)")
                    }
                }
            }
            .accessibilityIdentifier("confirm-remove-organization-member")
        } message: {
            Text(Self.describeRemoval(memberName: member.name, organizationName: organizationName))
        }
        .refusalAlert($refusal)
    }

    private func setRole(_ newRole: String) {
        Task {
            if await model.setOrganizationMemberRole(organizationID: organizationID, accountID: member.accountID, role: newRole) {
                role = newRole
                await reload()
            } else {
                refusal = model.takeRefusal(title: newRole == OrganizationRole.admin ? "Make admin" : "Make member")
            }
        }
    }
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
                    Text("Everyone in the team gets the vault with this role. Someone in several teams gets the strongest of their roles.")
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
                            NavigationLink {
                                if let organization {
                                    teamMemberList(access, organization: organization)
                                }
                            } label: {
                                HStack {
                                    Label {
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(access.team.name)
                                            Group {
                                                if access.team.allMember.count == 1 {
                                                    Text("1 member")
                                                } else {
                                                    Text("\(access.team.allMember.count) members")
                                                }
                                            }
                                            .font(.footnote)
                                            .foregroundStyle(.themeSecondary)
                                        }
                                    } icon: {
                                        Image(systemName: "person.3")
                                            .foregroundStyle(.themeSecondary)
                                    }
                                    Spacer()
                                    Text("Vault role: \(describeVaultRole(access.role))")
                                        .font(.subheadline)
                                        .foregroundStyle(.themeSecondary)
                                }
                            }
                            .accessibilityElement(children: .combine)
                            .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                if sharing.canChangeTeams {
                                    Button("Take back") { takeBack(from: access.team) }
                                }
                            }
                            .contextMenu {
                                if sharing.canChangeTeams {
                                    Button("Take back", systemImage: "arrow.uturn.backward") { takeBack(from: access.team) }
                                }
                            }
                            .accessibilityIdentifier("vault-team-\(access.team.name)")
                        }
                    } header: {
                        Text("Shared through teams in \(sharing.organizationName)")
                            .accessibilityIdentifier("shared-through-teams")
                    } footer: {
                        if let askAdminNote = sharing.askAdminNote {
                            Text(askAdminNote)
                                .accessibilityIdentifier("ask-an-admin")
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
            .refusalAlert($refusal)
            .task { await load() }
            .onChange(of: model.remoteChangeCount) { _, _ in
                Task { await load() }
            }
        }
        .presentationDetents([.medium, .large], selection: $presentationDetent)
        .interactiveDismissDisabled(model.isBusy)
    }

    private func teamMemberList(_ access: TeamAccess, organization: Organization) -> some View {
        List {
            Section {
                LabeledContent("Role for this vault", value: describeVaultRole(access.role))
            }
            Section {
                ForEach(OrganizationLogic.listAllTeamMember(organization: organization, team: access.team)) { member in
                    NavigationLink {
                        OrganizationMemberView(
                            organizationID: organization.id,
                            organizationName: organization.name,
                            member: member,
                            isAdmin: organization.isAdmin
                        ) {
                            await load()
                        }
                    } label: {
                        MemberRow(member: member, roleText: OrganizationRole.describe(member.role))
                    }
                    .accessibilityIdentifier("organization-member-\(member.email)")
                }
            } footer: {
                Text("Compare security codes with people once, in person or over a channel you trust. Tap a person to see theirs.")
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(access.team.name)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
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

    private func takeBack(from team: Team) {
        Task {
            if await model.takeVaultFromTeam(organizationID: organizationID, teamID: team.id, vaultID: vaultID) {
                await load()
            } else {
                refusal = model.takeRefusal(title: "Take back")
            }
        }
    }
}
