import SwiftUI

struct FamilyView: View {
    @EnvironmentObject private var model: AppModel
    @State private var allMember: [Member]?
    @State private var isInviteShown = false

    private var isAdmin: Bool { allMember?.contains { $0.isCurrentUser && $0.role == Member.roleAdmin } ?? false }

    var body: some View {
        List {
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                        .listRowInsets(EdgeInsets())
                        .listRowBackground(Color.clear)
                }
            }
            Section {
                if let allMember {
                    ForEach(allMember) { member in
                        MemberRow(member: member, roleText: member.role == Member.roleAdmin ? "Admin" : "Member")
                            .listRowBackground(Theme.surface)
                            .accessibilityIdentifier("family-member-\(member.email)")
                    }
                } else {
                    HStack {
                        ProgressView().tint(Theme.textMuted)
                        Text("Loading your family…")
                    }
                    .foregroundStyle(Theme.textMuted)
                    .listRowBackground(Theme.surface)
                }
            } header: {
                Text("Everyone on \(model.status.server)")
                    .textCase(nil)
                    .foregroundStyle(Theme.textMuted)
            } footer: {
                Text("Compare security codes in person before sharing a vault with someone for the first time.")
            }
            if allMember != nil {
                Section {
                    if isAdmin {
                        Button {
                            isInviteShown = true
                        } label: {
                            Label("Invite someone", systemImage: "envelope.badge.person.crop")
                                .foregroundStyle(Theme.text)
                        }
                        .listRowBackground(Theme.surface)
                        .accessibilityIdentifier("invite-member")
                    } else {
                        Label("Only family admins can invite people", systemImage: "envelope.badge.person.crop")
                            .foregroundStyle(Theme.textMuted)
                            .listRowBackground(Theme.surface)
                    }
                } footer: {
                    if isAdmin {
                        Text("An invite is a sign-up link for one person. It is valid for 7 days and works once.")
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .applyListTheme()
        .refreshable { await load() }
        .navigationTitle("Family")
        .navigationBarTitleDisplayMode(.inline)
        .applyNavigationBarTheme()
        .sheet(isPresented: $isInviteShown) {
            InviteMemberView()
                .environmentObject(model)
        }
        .task { await load() }
    }

    private func load() async {
        model.clearError()
        allMember = await model.loadAllMember() ?? allMember ?? []
    }
}

struct InviteMemberView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var email = ""
    @State private var role = Member.roleMember
    @State private var inviteLink: String?
    @State private var isCopied = false
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
                if let inviteLink {
                    inviteLinkSection(inviteLink)
                } else {
                    inviteFormSection
                }
            }
            .applyListTheme()
            .navigationTitle("Invite someone")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                if inviteLink == nil {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Cancel") {
                            model.clearError()
                            dismiss()
                        }
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.isBusy {
                        ProgressView()
                    } else if inviteLink != nil {
                        Button("Done") { dismiss() }
                            .accessibilityIdentifier("invite-done")
                    } else {
                        Button("Invite", action: invite)
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

    @ViewBuilder
    private var inviteFormSection: some View {
        Section {
            TextField("Email", text: $email, prompt: Text(verbatim: "them@example.com").foregroundStyle(Theme.textMuted))
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .focused($isEmailFocused)
                .submitLabel(.done)
                .onSubmit(invite)
                .accessibilityIdentifier("invite-email")
                .listRowBackground(Theme.surface)
        } header: {
            Text("Who")
        } footer: {
            Text("They sign up with this email address; the link does not work with another one.")
        }
        Section {
            Picker("Role", selection: $role) {
                Text("Member").tag(Member.roleMember)
                Text("Admin").tag(Member.roleAdmin)
            }
            .pickerStyle(.segmented)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
            .accessibilityIdentifier("invite-role")
        } header: {
            Text("Role")
        } footer: {
            Text("Admins can invite more people and help with account recovery. Members use and share vaults.")
        }
    }

    private func inviteLinkSection(_ inviteLink: String) -> some View {
        Section {
            VStack(alignment: .leading, spacing: 12) {
                Text(inviteLink)
                    .font(.system(.footnote, design: .monospaced))
                    .foregroundStyle(Theme.text)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("invite-link")
                HStack(spacing: 10) {
                    Button {
                        _ = PasteboardClipboard().setText(inviteLink)
                        isCopied = true
                    } label: {
                        Label(isCopied ? "Copied" : "Copy", systemImage: isCopied ? "checkmark" : "doc.on.doc")
                    }
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityIdentifier("invite-copy")
                    ShareLink(item: inviteLink, subject: Text("Join Hisar"), message: Text("Use this link to create your Hisar account. It works once and is valid for 7 days.")) {
                        Label("Send", systemImage: "square.and.arrow.up")
                    }
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityIdentifier("invite-share")
                }
            }
            .padding(.vertical, 6)
            .listRowBackground(Theme.surface)
        } header: {
            Text("Invite link for \(email)")
                .textCase(nil)
        } footer: {
            Text("Send this link to \(email). It is valid for 7 days and works once. Hisar does not show it again, so copy it now.")
        }
    }

    private func invite() {
        guard canInvite, !model.isBusy else { return }
        Task {
            inviteLink = await model.inviteMember(email: email, role: role)
        }
    }
}
