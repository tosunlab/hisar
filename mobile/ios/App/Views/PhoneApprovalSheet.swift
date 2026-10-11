import SwiftUI

struct PhoneApprovalSheet: View {
    var approval: PhoneApproval
    var isBusy: Bool
    var isPasswordRequired: Bool
    var errorMessage: String?
    var approve: () async -> Void
    var approveWithPassword: (String) async -> Void
    var deny: () async -> Void

    @State private var password = ""
    @FocusState private var isPasswordFocused: Bool

    var body: some View {
        ScrollView {
            VStack(spacing: 18) {
                header
                if approval.isForwarded {
                    forwardedBanner
                }
                detailBox
                encryptionNote
                if let errorMessage {
                    ErrorBanner(message: errorMessage)
                }
                actions
                ExpiryText(approval: approval)
            }
            .padding(.horizontal, 22)
            .padding(.top, 28)
            .padding(.bottom, 20)
        }
        .scrollBounceBehavior(.basedOnSize)
        .background(Theme.panel.ignoresSafeArea())
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled()
        .accessibilityIdentifier("phone-approval")
        .onChange(of: isPasswordRequired) { _, isRequired in
            isPasswordFocused = isRequired
        }
    }

    private var header: some View {
        VStack(spacing: 12) {
            ZStack(alignment: .bottomTrailing) {
                RoundedRectangle(cornerRadius: 16, style: .continuous)
                    .fill(Theme.surfaceRaised)
                    .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).stroke(Theme.borderStrong, lineWidth: 1))
                    .frame(width: 64, height: 64)
                    .overlay(
                        Image(systemName: approval.isForwarded ? "globe" : approval.systemImage)
                            .font(.system(size: 26, weight: .medium))
                            .foregroundStyle(Theme.textSecondary)
                    )
                LogoMark(size: 24)
                    .padding(2)
                    .background(Theme.panel)
                    .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
                    .offset(x: 6, y: 6)
            }
            .accessibilityHidden(true)
            Text(approval.title)
                .font(.title3.weight(.semibold))
                .foregroundStyle(Theme.text)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("phone-approval-title")
            Text(approval.summary)
                .font(.subheadline)
                .foregroundStyle(Theme.textSecondary)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("phone-approval-summary")
        }
    }

    private var forwardedBanner: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(Theme.danger)
            Text("Forwarded from another computer: this request comes from \(approval.forwardedFromHost.isEmpty ? "a remote host" : approval.forwardedFromHost) through agent forwarding, not from your Mac. Approving allows it once.")
                .font(.footnote)
                .foregroundStyle(Theme.text)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(12)
        .background(Theme.dangerBackground)
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(Theme.danger.opacity(0.5), lineWidth: 1))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("phone-approval-forwarded")
    }

    private var detailBox: some View {
        VStack(spacing: 0) {
            ForEach(Array(approval.allDetailRow.enumerated()), id: \.element.id) { index, row in
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    Text(row.label)
                        .font(.footnote)
                        .foregroundStyle(Theme.textSecondary)
                        .frame(width: 96, alignment: .leading)
                    Text(row.value)
                        .font(row.isMonospaced ? .system(.footnote, design: .monospaced) : .footnote)
                        .foregroundStyle(Theme.text)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("phone-approval-row-\(row.label)")
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 11)
                if index < approval.allDetailRow.count - 1 {
                    Divider().overlay(Theme.border)
                }
            }
        }
        .background(Theme.surface)
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(Theme.border, lineWidth: 1))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var encryptionNote: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "lock.shield")
                .foregroundStyle(Theme.success)
            Text("End-to-end encrypted. Only your devices can read this request. Approving lets this one request continue; it does not unlock Hisar on your Mac.")
                .font(.footnote)
                .foregroundStyle(Theme.textSecondary)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("phone-approval-encryption-note")
    }

    @ViewBuilder
    private var actions: some View {
        VStack(spacing: 10) {
            if isPasswordRequired {
                LabeledInput(label: "Master password", systemImage: "lock") {
                    SecureField("Master password", text: $password)
                        .submitLabel(.go)
                        .focused($isPasswordFocused)
                        .onSubmit(submitPassword)
                        .accessibilityIdentifier("phone-approval-password")
                }
                Button(action: submitPassword) {
                    if isBusy {
                        ProgressView().tint(Theme.primaryText)
                    } else {
                        Text("Approve")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(isBusy || password.isEmpty)
                .accessibilityIdentifier("phone-approval-approve-password")
            } else {
                Button {
                    Task { await approve() }
                } label: {
                    if isBusy {
                        ProgressView().tint(Theme.primaryText)
                    } else {
                        Label("Approve with Face ID", systemImage: "faceid")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(isBusy)
                .accessibilityIdentifier("phone-approval-approve")
            }
            Button {
                Task { await deny() }
            } label: {
                Text("Deny")
            }
            .buttonStyle(SecondaryButtonStyle())
            .disabled(isBusy)
            .accessibilityIdentifier("phone-approval-deny")
        }
    }

    private func submitPassword() {
        guard !password.isEmpty, !isBusy else { return }
        let enteredPassword = password
        password = ""
        Task { await approveWithPassword(enteredPassword) }
    }
}

private struct ExpiryText: View {
    var approval: PhoneApproval

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            let secondCount = max(0, Int((approval.expiryDate ?? context.date).timeIntervalSince(context.date).rounded(.up)))
            Text(secondCount == 1 ? "Expires in 1 second" : "Expires in \(secondCount) seconds")
                .font(.caption)
                .foregroundStyle(Theme.textMuted)
                .monospacedDigit()
        }
    }
}
