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
                }
            }
            if !shareInvitations.allReceived.isEmpty {
                Section("Invitations for you") {
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
                    }
                }
            }
            Section {
                if let allPerson {
                    ForEach(allPerson) { person in
                        NavigationLink {
                            PersonView(person: person)
                        } label: {
                            MemberRow(member: person, roleText: "")
                        }
                        .accessibilityIdentifier("person-\(person.email)")
                    }
                } else {
                    Label {
                        Text("Loading people…")
                    } icon: {
                        ProgressView()
                    }
                    .foregroundStyle(.themeSecondary)
                }
            } header: {
                Text("People you share vaults with")
            } footer: {
                Text("People appear here once you share a vault with them. Compare security codes with them once, in person or over a channel you trust.")
            }
            if !shareInvitations.allSent.isEmpty || !shareInvitations.completionError.isEmpty {
                Section {
                    ForEach(shareInvitations.allSent) { invitation in
                        SentInvitationRow(invitation: invitation, identifier: "sent-invitation-\(invitation.email)", onConfirmed: { await load() })
                            .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                Button("Cancel", role: .destructive) { invitationToCancel = invitation }
                            }
                            .contextMenu {
                                Button("Cancel invitation", systemImage: "xmark.circle", role: .destructive) { invitationToCancel = invitation }
                            }
                    }
                } header: {
                    Text("Invitations you sent")
                } footer: {
                    if !shareInvitations.completionError.isEmpty {
                        Text("Could not finish sharing: \(shareInvitations.completionError)")
                            .foregroundStyle(.themeWarning)
                    }
                }
            }
            if allPerson != nil {
                Section {
                    Button {
                        isInviteShown = true
                    } label: {
                        Label("Invite to Hisar", systemImage: "envelope.badge.person.crop")
                    }
                    .accessibilityIdentifier("invite-person")
                } footer: {
                    Text("Creates a Hisar account: the server emails them a sign-up link. To give them a vault, share it from Share in the vault's toolbar.")
                }
            }
        }
        .applyThemedListStyle()
        .refreshable { await load() }
        .navigationTitle("People")
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
            Button("Keep invitation", role: .cancel) {}
        } message: {
            Text("Their invitation link stops working. You can invite them again later.")
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
        await model.refreshReceivedInvitationCount()
    }
}

struct PersonView: View {
    var person: Member

    var body: some View {
        List {
            Section {
                VStack(spacing: 8) {
                    Image(systemName: "person.crop.circle.fill")
                        .font(.system(.largeTitle))
                        .imageScale(.large)
                        .foregroundStyle(.themeSecondary)
                        .accessibilityHidden(true)
                    Text(person.isCurrentUser ? "\(person.name) (you)" : person.name)
                        .font(.title2.weight(.bold))
                    Text(person.email)
                        .foregroundStyle(.themeSecondary)
                        .textSelection(.enabled)
                }
                .frame(maxWidth: .infinity)
                .listRowBackground(Color.clear)
            }
            if person.isKeyChanged {
                Section {
                    Label {
                        Text("Their key changed since you last shared with them. Check the security code with them before sharing again.")
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundStyle(.themeWarning)
                    }
                    .accessibilityIdentifier("person-key-changed")
                }
            }
            if !person.securityCode.isEmpty {
                Section {
                    Text(person.securityCode)
                        .font(.title3.monospaced().weight(.medium))
                        .frame(maxWidth: .infinity)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("person-security-code")
                    if person.isCurrentUser {
                        ShareLink(item: "My Hisar security code: \(person.securityCode)") {
                            Label("Share my security code", systemImage: "square.and.arrow.up")
                        }
                    }
                } header: {
                    Text("Security code")
                } footer: {
                    Text(person.isCurrentUser
                         ? "This is your security code. Read it to someone who is checking the code they see for you."
                         : "Compare it with \(person.name) once, in person or over a channel you trust. If it matches what their Hisar shows, vaults you share reach them and nobody else.")
                }
            }
        }
        .applyThemedListStyle()
        .navigationTitle(person.name)
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct InvitationExpiryText: View {
    var timestamp: String

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            let lifetime = InvitationLifetime(timestamp)
            Text(lifetime.describe(at: context.date))
                .font(.footnote)
                .foregroundStyle(.themeSecondary)
                .accessibilityValue(lifetime.exactDate)
        }
    }
}

struct ReceivedInvitationRow: View {
    var invitation: ReceivedShareInvitation
    var identifierPrefix = ""
    var accept: () -> Void
    var decline: () -> Void

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            row(at: context.date)
        }
    }

    private func row(at now: Date) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Label {
                VStack(alignment: .leading, spacing: 2) {
                    Text("\(invitation.inviterName) shared a vault with you as \(describeVaultRole(invitation.role).lowercased())")
                    Text(invitation.invitedByEmail)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                    InvitationExpiryText(timestamp: invitation.expiresAt)
                }
            } icon: {
                Image(systemName: "person.2.fill")
                    .foregroundStyle(.tint)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("\(identifierPrefix)invitation-text")
            HStack(spacing: 10) {
                Button(action: decline) {
                    Text("Decline").frame(maxWidth: .infinity)
                }
                .applyGlassButtonStyle(isProminent: false)
                .accessibilityIdentifier("\(identifierPrefix)decline-invitation")
                Button(action: accept) {
                    Text("Accept").frame(maxWidth: .infinity)
                }
                .applyGlassButtonStyle(isProminent: true)
                .accessibilityIdentifier("\(identifierPrefix)accept-invitation")
                .disabled(InvitationLifetime(invitation.expiresAt).isExpired(at: now))
            }
        }
        .padding(.vertical, 4)
    }
}

struct SentInvitationRow: View {
    var invitation: SentShareInvitation
    var isVaultNameShown = true
    // The row's own identifier names its label, so the review button below keeps its identifier.
    var identifier: String
    var onConfirmed: () async -> Void = {}
    @EnvironmentObject private var model: AppModel
    @State private var reviewed: SentShareInvitation?
    @State private var resendError: String?

    private var stateText: String {
        if invitation.requiresConfirmation == true { return "Accepted, review recipient identity" }
        return invitation.state == SentShareInvitation.stateAccepted ? "Accepted, Hisar is finishing sharing" : "Waiting for them to accept"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label {
                VStack(alignment: .leading, spacing: 2) {
                    Text(invitation.email)
                    Text(isVaultNameShown ? "\(invitation.vaultName.isEmpty ? "A vault you own" : invitation.vaultName) · \(stateText)" : stateText)
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                    if invitation.state != SentShareInvitation.stateAccepted {
                        InvitationExpiryText(timestamp: invitation.expiresAt)
                    }
                }
            } icon: {
                Image(systemName: invitation.state == SentShareInvitation.stateAccepted ? "envelope.open" : "envelope")
            }
            .badge(Text(describeVaultRole(invitation.role)))
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier(identifier)
            if invitation.requiresConfirmation == true {
                Button("Review recipient") { reviewed = invitation }
                    .accessibilityIdentifier("review-share-\(invitation.email)")
            }
            if invitation.state != SentShareInvitation.stateAccepted && invitation.requiresConfirmation != true {
                Button("Send again") {
                    Task {
                        resendError = nil
                        if await model.shareVault(vaultID: invitation.vaultID, email: invitation.email, role: invitation.role) {
                            model.showNotice("Invitation sent to \(invitation.email).")
                            await onConfirmed()
                        } else {
                            resendError = model.errorMessage
                            model.clearError()
                        }
                    }
                }
                .disabled(model.isBusy)
                .accessibilityIdentifier("resend-invitation-\(invitation.email)")
                Text("Sending again replaces the previous invitation.")
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
            }
            if let resendError { ErrorBanner(message: resendError) }
        }
        .alert("Give this person access?", isPresented: Binding(get: { reviewed != nil }, set: { if !$0 { reviewed = nil } })) {
            Button("Cancel", role: .cancel) { reviewed = nil }
            Button("Give access") {
                guard let snapshot = reviewed else { return }
                Task {
                    if await model.confirmShareRecipient(snapshot) { await onConfirmed() }
                }
            }
            .accessibilityIdentifier("confirm-share-recipient")
        } message: {
            if let snapshot = reviewed {
                Text("\(snapshot.email)\nVault \(snapshot.vaultName) · \(describeVaultRole(snapshot.role))\nAccount \(snapshot.recipientID ?? "")\nSecurity code \(snapshot.securityCode ?? "")\nCompare this code with the recipient over a channel you trust.")
            }
        }
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
                    }
                }
                Section {
                    TextField("Email", text: $email, prompt: Text(verbatim: "them@example.com"))
                        .keyboardType(.emailAddress)
                        .textContentType(.emailAddress)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .focused($isEmailFocused)
                        .submitLabel(.send)
                        .onSubmit(invite)
                        .accessibilityIdentifier("invite-email")
                } header: {
                    Text("Who")
                } footer: {
                    Text("The server emails them a sign-up link that works once and expires after 7 days. They sign up with this email address.")
                }
            }
            .navigationTitle("Invite to Hisar")
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
        .presentationDetents([.medium, .large])
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
