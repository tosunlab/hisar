import SwiftUI
import UIKit

struct PeopleView: View {
    @EnvironmentObject private var model: AppModel
    @State private var people: AppModel.PeopleSnapshot?
    private var allPerson: [Member]? { people?.allPerson }
    private var shareInvitations: ShareInvitations { people?.invitations ?? ShareInvitations() }
    @State private var isInviteShown = false
    @State private var invitationToCancel: SentShareInvitation?
    @State private var actionError: String?

    var body: some View {
        List {
            if let errorMessage = actionError ?? model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            if !shareInvitations.allReceived.isEmpty || !shareInvitations.allWaiting.isEmpty {
                Section {
                    ForEach(shareInvitations.allReceived) { invitation in
                        ReceivedInvitationRow(invitation: invitation) {
                            Task {
                                actionError = nil
                                if await model.acceptShareInvitation(invitationID: invitation.id) {
                                    model.showNotice("Accepted. The vault appears once \(invitation.inviterName)'s Hisar finishes sharing.")
                                    await load()
                                } else {
                                    actionError = model.errorMessage
                                }
                            }
                        } decline: {
                            Task {
                                actionError = nil
                                if await model.declineShareInvitation(invitationID: invitation.id) {
                                    model.showNotice("Invitation declined.")
                                    await load()
                                } else {
                                    actionError = model.errorMessage
                                }
                            }
                        }
                    }
                    ForEach(shareInvitations.allWaiting) { invitation in
                        Label {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(invitation.describe(now: Date()))
                                Text(invitation.invitedByEmail).font(.footnote)
                            }
                        } icon: {
                            Image(systemName: "clock")
                        }
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                        .accessibilityIdentifier("waiting-\(invitation.id)")
                    }
                } header: {
                    Text("Invitations for you")
                        .foregroundStyle(.themeSecondary)
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
                    .foregroundStyle(.themeSecondary)
            } footer: {
                Text("People appear here once you share a vault with them. To compare codes, ask each person for the security code Hisar shows for their own account in Settings → People, the row marked you. It must match the code you see for them.")
                    .foregroundStyle(.themeSecondary)
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
                        .foregroundStyle(.themeSecondary)
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
                        .foregroundStyle(.themeSecondary)
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
                    actionError = nil
                    if await model.cancelShareInvitation(invitationID: invitation.id) {
                        model.showNotice("Invitation to \(invitation.email) cancelled.")
                        await load()
                    } else {
                        actionError = model.errorMessage
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
        .onChange(of: model.remoteChangeCount) { _, _ in
            Task { await load() }
        }
    }

    private func load() async {
        people = await model.loadPeople(previous: people)
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
                        Text("Key changed. Hisar stopped sharing with them.")
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
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text(describeSecurityCode(person))
                        .foregroundStyle(.themeSecondary)
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
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            row(at: context.date)
        }
    }

    private func row(at now: Date) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Label {
                    Text("\(invitation.inviterName) shared a vault with you as \(describeVaultRole(invitation.role).lowercased())")
                } icon: {
                    Image(systemName: "person.2.fill")
                        .foregroundStyle(.tint)
                        .accessibilityHidden(true)
                }
                InvitationEmailText(email: invitation.invitedByEmail)
                InvitationExpiryText(timestamp: invitation.expiresAt)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("\(identifierPrefix)invitation-text")
            if dynamicTypeSize.isAccessibilitySize {
                VStack(spacing: 10) {
                    acceptButton(at: now)
                    declineButton
                }
            } else {
                HStack(spacing: 10) {
                    declineButton
                    acceptButton(at: now)
                }
            }
        }
        .padding(.vertical, 4)
    }

    private func acceptButton(at now: Date) -> some View {
        Button(action: accept) {
            Text("Accept").frame(maxWidth: .infinity)
        }
        .applyGlassButtonStyle(isProminent: true)
        .accessibilityIdentifier("\(identifierPrefix)accept-invitation")
        .disabled(InvitationLifetime(invitation.expiresAt).isExpired(at: now))
    }

    private var declineButton: some View {
        Button(action: decline) {
            Text("Decline").frame(maxWidth: .infinity)
        }
        .applyGlassButtonStyle(isProminent: false)
        .accessibilityIdentifier("\(identifierPrefix)decline-invitation")
    }
}

struct InvitationEmailText: UIViewRepresentable {
    var email: String
    @Environment(\.themePalette) private var palette
    @ScaledMetric(relativeTo: .footnote) private var fontSize = 13.0

    func makeUIView(context: Context) -> UITextView {
        let view = UITextView()
        view.isEditable = false
        view.isSelectable = true
        view.isScrollEnabled = false
        view.backgroundColor = .clear
        view.textContainerInset = .zero
        view.textContainer.lineFragmentPadding = 0
        view.textContainer.lineBreakMode = .byCharWrapping
        view.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        return view
    }

    func updateUIView(_ view: UITextView, context: Context) {
        let paragraph = NSMutableParagraphStyle()
        paragraph.lineBreakMode = .byCharWrapping
        paragraph.hyphenationFactor = 0
        view.attributedText = NSAttributedString(string: email, attributes: [
            .font: UIFont.systemFont(ofSize: fontSize),
            .foregroundColor: palette.secondaryLabel.uiColor,
            .paragraphStyle: paragraph,
        ])
        view.accessibilityLabel = email
    }

    func sizeThatFits(_ proposal: ProposedViewSize, uiView: UITextView, context: Context) -> CGSize? {
        guard let width = proposal.width else { return nil }
        return uiView.sizeThatFits(CGSize(width: width, height: .greatestFiniteMagnitude))
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
        .sheet(item: $reviewed) { snapshot in
            ReviewRecipientSheet(invitation: snapshot, onChanged: onConfirmed)
                .environmentObject(model)
                }
            }
}

struct ReviewRecipientSheet: View {
    var invitation: SentShareInvitation
    var onChanged: () async -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var isWorking = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    LabeledContent("Name", value: invitation.recipientName.flatMap { $0.isEmpty ? nil : $0 } ?? invitation.email)
                    LabeledContent("Email", value: invitation.email)
                    LabeledContent("Vault", value: invitation.vaultName)
                    LabeledContent("Role", value: describeVaultRole(invitation.role))
                    LabeledContent("Account ID", value: invitation.recipientID ?? "")
                }
                .textSelection(.enabled)
                Section {
                    Text(invitation.securityCode ?? "")
                        .font(.title3.monospaced().weight(.medium))
                        .frame(maxWidth: .infinity)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("review-security-code")
                } header: {
                    Text("Security code")
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text("To compare codes, ask this person for the security code Hisar shows for their own account in Settings → People, the row marked you. It must match this code.")
                        .foregroundStyle(.themeSecondary)
                }
                if let errorMessage {
                    Section { ErrorBanner(message: errorMessage) }
                }
                Section {
                    Button("Give access") { complete(isMismatch: false) }
            .accessibilityIdentifier("confirm-share-recipient")
                    Button("Codes don't match", role: .destructive) { complete(isMismatch: true) }
                        .accessibilityIdentifier("share-recipient-mismatch")
            }
                .disabled(isWorking)
            }
            .navigationTitle("Review recipient")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(isWorking)
                }
                if isWorking {
                    ToolbarItem(placement: .confirmationAction) { ProgressView() }
                }
            }
        }
        .interactiveDismissDisabled(isWorking)
    }

    private func complete(isMismatch: Bool) {
        guard !isWorking else { return }
        isWorking = true
        errorMessage = nil
        Task {
            let succeeded = isMismatch
                ? await model.cancelShareInvitation(invitationID: invitation.id)
                : await model.confirmShareRecipient(invitation)
            if succeeded {
                if isMismatch { model.showNotice("Cancelled the invitation to \(invitation.email).") }
                await onChanged()
                dismiss()
            } else {
                errorMessage = model.errorMessage
            }
            isWorking = false
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
                        .foregroundStyle(.themeSecondary)
                } footer: {
                    Text("The server emails them a sign-up link that works once and expires after 7 days. They sign up with this email address.")
                        .foregroundStyle(.themeSecondary)
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
