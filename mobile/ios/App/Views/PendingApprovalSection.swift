import SwiftUI

// The first waiting request from a Mac, at the top of Vaults; the rest are one tap away in the Approvals tab.
struct PendingApprovalCardSection: View {
    var allApproval: [PhoneApproval]
    var isBusy: Bool
    var review: (PhoneApproval) -> Void
    var approve: (PhoneApproval) -> Void
    var deny: (PhoneApproval) -> Void
    var showAll: () -> Void

    var body: some View {
        if let approval = allApproval.first {
            Section {
                PendingApprovalCard(approval: approval, isBusy: isBusy, review: review, approve: approve, deny: deny)
                    .alignmentGuide(.listRowSeparatorLeading) { _ in 0 }
                if allApproval.count > 1 {
                    Button(action: showAll) {
                        Text(allApproval.count == 2 ? "1 more request" : "\(allApproval.count - 1) more requests")
                            .frame(maxWidth: .infinity)
                    }
                    .accessibilityIdentifier("pending-approval-more")
                }
            } header: {
                Text(allApproval.count == 1 ? "Waiting for your approval" : "\(allApproval.count) requests waiting for your approval")
                    .accessibilityIdentifier("pending-approval-header")
            } footer: {
                Text("Approving answers this one request. It never unlocks Hisar on your Mac.")
            }
        }
    }
}

struct PendingApprovalCard: View {
    var approval: PhoneApproval
    var isBusy: Bool
    var review: (PhoneApproval) -> Void
    var approve: (PhoneApproval) -> Void
    var deny: (PhoneApproval) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Button {
                review(approval)
            } label: {
                PendingApprovalSummary(approval: approval)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityHint("Shows the request's details")
            .accessibilityIdentifier("pending-approval-\(approval.requestID)")
            HStack(spacing: 10) {
                Button {
                    deny(approval)
                } label: {
                    Text("Deny")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.glass)
                .accessibilityIdentifier("pending-approval-deny-\(approval.requestID)")
                Button {
                    approve(approval)
                } label: {
                    Label("Approve", systemImage: "faceid")
                        .frame(maxWidth: .infinity)
                }
                .applyProminentGlassStyle()
                .accessibilityIdentifier("pending-approval-approve-\(approval.requestID)")
            }
            .controlSize(.large)
            .disabled(isBusy)
        }
        .padding(.vertical, 6)
    }
}

struct PendingApprovalSummary: View {
    var approval: PhoneApproval

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: approval.isForwarded ? "exclamationmark.triangle.fill" : approval.systemImage)
                .font(.title3)
                .foregroundStyle(approval.isForwarded ? .red : .orange)
                .frame(width: 36, height: 36)
                .background(.fill.tertiary, in: .rect(cornerRadius: 10, style: .continuous))
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 3) {
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    Text("\(approval.macName) · \(approval.formatTimeLeft(at: context.date).replacingOccurrences(of: "Expires", with: "expires"))")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.orange)
                        .monospacedDigit()
                }
                Text(approval.summary)
                    .font(.headline)
                    .fixedSize(horizontal: false, vertical: true)
                Text(approval.detailLine)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
    }
}
