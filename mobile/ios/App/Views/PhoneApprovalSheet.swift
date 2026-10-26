import SwiftUI

struct PhoneApprovalSheet: View {
    var approval: PhoneApproval
    var isBusy: Bool
    var isPasswordRequired: Bool
    var errorMessage: String?
    var approve: (_ isRemembered: Bool) async -> Void
    var approveWithPassword: (_ password: String, _ isRemembered: Bool) async -> Void
    var deny: () async -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var password = ""
    @State private var chosenRemember: Bool?
    @FocusState private var isPasswordFocused: Bool

    var body: some View {
        NavigationStack {
            List {
                Section {
                    header
                        .listRowBackground(Color.clear)
                }
                // Right below the request, so the field is on screen and can take focus when Face ID is not available;
                // a List creates no row below the screen.
                if let errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                if isPasswordRequired {
                    Section {
                        SecureField("Master password", text: $password)
                            .textContentType(.password)
                            .submitLabel(.go)
                            .focused($isPasswordFocused)
                            .onSubmit(submitPassword)
                            .accessibilityIdentifier("phone-approval-password")
                    } header: {
                        Text("Approve with your master password")
                    } footer: {
                        Text("\(SettingsView.biometryName) is not available, so Hisar checks your master password instead.")
                    }
                }
                if approval.isForwarded {
                    Section {
                        notice(
                            "Forwarded from another computer: this request comes from \(approval.forwardedFromHost.isEmpty ? "a remote host" : approval.forwardedFromHost) through agent forwarding, not from your Mac. Approving allows it once.",
                            style: AnyShapeStyle(.red)
                        )
                        .accessibilityIdentifier("phone-approval-forwarded")
                    }
                }
                if !approval.appWarning.isEmpty {
                    Section {
                        notice(approval.appWarning, style: AnyShapeStyle(.themeWarning))
                            .accessibilityIdentifier("phone-approval-warning")
                    }
                }
                Section {
                    ForEach(approval.allDetailRow) { row in
                        if row.isMonospaced {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(row.label)
                                Text(row.value)
                                    .font(.system(.subheadline, design: .monospaced))
                                    .foregroundStyle(.primary)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .textSelection(.enabled)
                            }
                            .accessibilityElement(children: .combine)
                            .accessibilityIdentifier("phone-approval-row-\(row.label)")
                        } else {
                            LabeledContent(row.label) {
                                Text(row.value)
                                    .font(.subheadline)
                                    .foregroundStyle(.primary)
                                    .multilineTextAlignment(.trailing)
                                    .textSelection(.enabled)
                                    .accessibilityIdentifier("phone-approval-row-\(row.label)")
                            }
                        }
                    }
                } footer: {
                    if !approval.isRememberable {
                        approvalNote
                    }
                }
                if approval.isRememberable {
                    Section {
                        Toggle(approval.rememberLabel, isOn: Binding(get: { approval.resolveRememberChoice(chosenRemember) }, set: { chosenRemember = $0 }))
                            .disabled(isBusy)
                            .accessibilityIdentifier("phone-approval-remember")
                    } footer: {
                        approvalNote
                    }
                }
            }
            .listStyle(.insetGrouped)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button {
                        dismiss()
                    } label: {
                        Label("Close", systemImage: "xmark")
                    }
                    .disabled(isBusy)
                    .accessibilityIdentifier("phone-approval-close")
                }
            }
            .safeAreaInset(edge: .bottom) {
                actions
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(isBusy)
        .accessibilityIdentifier("phone-approval")
        .onChange(of: approval.requestID) {
            chosenRemember = nil
        }
        .onChange(of: isPasswordRequired) { _, isRequired in
            isPasswordFocused = isRequired
        }
    }

    // The summary is the title: what the Mac is about to do is the question the user answers.
    private var header: some View {
        HStack(alignment: .top, spacing: 12) {
            if !approval.isForwarded, let appIcon = approval.appIcon.flatMap(UIImage.init(data:)) {
                Image(uiImage: appIcon)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: 40, height: 40)
                    .frame(width: 44, height: 44)
                    .accessibilityIdentifier("phone-approval-app-icon")
                    .accessibilityHidden(true)
            } else {
                Image(systemName: approval.isForwarded ? "globe" : approval.systemImage)
                    .font(.title2)
                    .foregroundStyle(approval.isForwarded ? AnyShapeStyle(.red) : AnyShapeStyle(.themeWarning))
                    .frame(width: 44, height: 44)
                    .background(.fill.tertiary, in: .rect(cornerRadius: 12, style: .continuous))
                    .accessibilityHidden(true)
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(approval.summary)
                    .font(.title3.weight(.bold))
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("phone-approval-summary")
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    Text("\(Text("\(approval.macName) · ").foregroundStyle(.themeSecondary))\(Text(approval.formatTimeLeft(at: context.date)).foregroundStyle(.themeWarning))")
                        .font(.subheadline.weight(.medium))
                        .monospacedDigit()
                        .accessibilityIdentifier("phone-approval-expiry")
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, 4)
    }

    private var approvalNote: some View {
        Text(approval.approvalNote)
            .accessibilityIdentifier("phone-approval-note")
    }

    private func notice(_ text: String, style: AnyShapeStyle) -> some View {
        Label {
            Text(text)
                .frame(maxWidth: .infinity, alignment: .leading)
        } icon: {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(style)
        }
        .font(.subheadline)
        .accessibilityElement(children: .combine)
    }

    private var actions: some View {
        GlassEffectContainer {
            HStack(spacing: 12) {
                Button {
                    Task { await deny() }
                } label: {
                    Text("Deny")
                        .frame(minWidth: 72)
                }
                .buttonStyle(.glass)
                .accessibilityIdentifier("phone-approval-deny")
                if isPasswordRequired {
                    Button(action: submitPassword) {
                        Group {
                            if isBusy {
                                ProgressView()
                            } else {
                                Text("Approve")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .disabled(password.isEmpty)
                    .accessibilityIdentifier("phone-approval-approve-password")
                } else {
                    Button {
                        Task { await approve(approval.resolveRememberChoice(chosenRemember)) }
                    } label: {
                        Group {
                            if isBusy {
                                ProgressView()
                            } else {
                                Label("Approve with \(SettingsView.biometryName)", systemImage: SettingsView.biometrySystemImage)
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .accessibilityIdentifier("phone-approval-approve")
                }
            }
            .controlSize(.large)
            .disabled(isBusy)
            .padding(.horizontal, 20)
            .padding(.bottom, 8)
        }
    }

    private func submitPassword() {
        guard !password.isEmpty, !isBusy else { return }
        let enteredPassword = password
        password = ""
        Task { await approveWithPassword(enteredPassword, approval.resolveRememberChoice(chosenRemember)) }
    }
}
