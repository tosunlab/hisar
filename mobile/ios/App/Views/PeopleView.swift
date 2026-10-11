import SwiftUI

struct PeopleView: View {
    @EnvironmentObject private var model: AppModel
    @State private var allPerson: [Member]?
    @State private var shareInvitations = ShareInvitations()
    @State private var isInviteShown = false
    @State private var invitationToCancel: SentShareInvitation?

    var body: some View {
        List {
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                        .listRowInsets(EdgeInsets())
                        .listRowBackground(Color.clear)
                }
            }
            if !shareInvitations.allReceived.isEmpty {
                Section {
                    ForEach(shareInvitations.allReceived) { invitation in
                        ReceivedInvitationRow(invitation: invitation) {
                            Task {
                                if await model.acceptShareInvitation(invitationID: invitation.id) {
                                    model.showNotice("Accepted. The vault appears once \(invitation.inviterName)'s Hisar finishes sharing.")
                                    await load()
                                }
                            }
                        } decline: {
                            Task {
                                if await model.declineShareInvitation(invitationID: invitation.id) {
                                    model.showNotice("Invitation declined.")
                                    await load()
                                }
                            }
                        }
                        .listRowBackground(Theme.surface)
                        .accessibilityIdentifier("received-invitation-\(invitation.invitedByEmail)")
                    }
                } header: {
                    Text("Invitations for you")
                }
            }
            Section {
                if let allPerson {
                    ForEach(allPerson) { person in
                        MemberRow(member: person, roleText: "")
                            .listRowBackground(Theme.surface)
                            .accessibilityIdentifier("person-\(person.email)")
                    }
                } else {
                    HStack {
                        ProgressView().tint(Theme.textMuted)
                        Text("Loading people…")
                    }
                    .foregroundStyle(Theme.textMuted)
                    .listRowBackground(Theme.surface)
                }
            } header: {
                Text("People you share vaults with")
                    .textCase(nil)
                    .foregroundStyle(Theme.textMuted)
            } footer: {
                Text("People appear here once you share a vault with them. Compare security codes with them once, in person or over a channel you trust.")
            }
            if !shareInvitations.allSent.isEmpty || !shareInvitations.completionError.isEmpty {
                Section {
                    ForEach(shareInvitations.allSent) { invitation in
                        SentInvitationRow(invitation: invitation)
                            .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                Button("Cancel", role: .destructive) { invitationToCancel = invitation }
                            }
                            .contextMenu {
                                Button("Cancel invitation", role: .destructive) { invitationToCancel = invitation }
                            }
                            .listRowBackground(Theme.surface)
                            .accessibilityIdentifier("sent-invitation-\(invitation.email)")
                    }
                } header: {
                    Text("Invitations you sent")
                } footer: {
                    if !shareInvitations.completionError.isEmpty {
                        Text("Could not finish sharing: \(shareInvitations.completionError)")
                            .foregroundStyle(Theme.warning)
                    }
                }
            }
            if allPerson != nil {
                Section {
                    Button {
                        isInviteShown = true
                    } label: {
                        Label("Invite someone to Hisar", systemImage: "envelope.badge.person.crop")
                            .foregroundStyle(Theme.text)
                    }
                    .listRowBackground(Theme.surface)
                    .accessibilityIdentifier("invite-person")
                } footer: {
                    Text("The server emails them a link to create an account. Share a vault with them from the vault's members.")
                }
            }
        }
        .listStyle(.insetGrouped)
        .applyListTheme()
        .refreshable { await load() }
        .navigationTitle("People")
        .navigationBarTitleDisplayMode(.inline)
        .applyNavigationBarTheme()
        .confirmationDialog("Cancel the invitation to \(invitationToCancel?.email ?? "")?", isPresented: Binding(get: { invitationToCancel != nil }, set: { if !$0 { invitationToCancel = nil } }), titleVisibility: .visible) {
            Button("Cancel invitation", role: .destructive) {
                guard let invitation = invitationToCancel else { return }
                Task {
                    if await model.cancelShareInvitation(invitationID: invitation.id) {
                        model.showNotice("Invitation to \(invitation.email) cancelled.")
                        await load()
                    }
                }
            }
        }
        .sheet(isPresented: $isInviteShown) {
            InvitePersonView()
                .environmentObject(model)
        }
        .task { await load() }
    }

    private func load() async {
        model.clearError()
        allPerson = await model.loadAllPerson() ?? allPerson ?? []
        if let loadedInvitations = await model.loadShareInvitations() {
            shareInvitations = loadedInvitations
        }
    }
}

struct ReceivedInvitationRow: View {
    var invitation: ReceivedShareInvitation
    var accept: () -> Void
    var decline: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("\(invitation.inviterName) shared a vault with you as \(describeVaultRole(invitation.role).lowercased())")
                .foregroundStyle(Theme.text)
            Text(invitation.invitedByEmail)
                .font(.caption)
                .foregroundStyle(Theme.textMuted)
            HStack(spacing: 10) {
                Button("Decline", action: decline)
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityIdentifier("decline-invitation")
                Button("Accept", action: accept)
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityIdentifier("accept-invitation")
            }
        }
        .padding(.vertical, 4)
    }
}

struct SentInvitationRow: View {
    var invitation: SentShareInvitation

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: "envelope")
                .font(.title3)
                .foregroundStyle(Theme.textSecondary)
                .frame(width: 32)
            VStack(alignment: .leading, spacing: 2) {
                Text(invitation.email)
                    .foregroundStyle(Theme.text)
                Text("\(invitation.vaultName.isEmpty ? "A vault you own" : invitation.vaultName) · \(invitation.state == SentShareInvitation.stateAccepted ? "Accepted, finishing on your next sync" : "Waiting for them to accept")")
                    .font(.caption)
                    .foregroundStyle(Theme.textMuted)
            }
            Spacer()
            Text(describeVaultRole(invitation.role))
                .font(.subheadline)
                .foregroundStyle(Theme.textMuted)
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}

struct InvitePersonView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var email = ""
    @FocusState private var isEmailFocused: Bool

    private var canInvite: Bool { email.contains("@") }

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
                        .onSubmit(invite)
                        .accessibilityIdentifier("invite-email")
                        .listRowBackground(Theme.surface)
                } header: {
                    Text("Who")
                } footer: {
                    Text("The server emails them a sign-up link that works once and expires after 7 days. They sign up with this email address.")
                }
            }
            .applyListTheme()
            .navigationTitle("Invite someone")
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
                        Button("Send", action: invite)
                            .disabled(!canInvite)
                            .accessibilityIdentifier("invite-submit")
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

    private func invite() {
        guard canInvite, !model.isBusy else { return }
        let trimmedEmail = email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        Task {
            if await model.invitePerson(email: trimmedEmail) {
                model.showNotice("Invitation sent to \(trimmedEmail).")
                dismiss()
            }
        }
    }
}
