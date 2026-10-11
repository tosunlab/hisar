import SwiftUI

// The Mac's open requests at the top of the vault list, so they stay reachable when the sheet is closed or was never shown.
struct PendingApprovalSection: View {
    var allApproval: [PhoneApproval]
    var review: (PhoneApproval) -> Void

    var body: some View {
        Section {
            ForEach(allApproval) { approval in
                Button {
                    review(approval)
                } label: {
                    PendingApprovalRow(approval: approval)
                }
                .buttonStyle(.plain)
                .listRowBackground(Theme.surfaceRaised)
                .accessibilityIdentifier("pending-approval-\(approval.requestID)")
            }
        } header: {
            Text(allApproval.count == 1 ? "Waiting for your approval" : "\(allApproval.count) requests waiting for your approval")
                .accessibilityIdentifier("pending-approval-header")
        }
    }
}

private struct PendingApprovalRow: View {
    var approval: PhoneApproval

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Image(systemName: approval.isForwarded ? "globe" : approval.systemImage)
                .font(.title3)
                .foregroundStyle(approval.isForwarded ? Theme.danger : Theme.warning)
                .frame(width: 32)
            VStack(alignment: .leading, spacing: 3) {
                Text(approval.bannerTitle)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Theme.text)
                    .fixedSize(horizontal: false, vertical: true)
                Text(approval.summary)
                    .font(.footnote)
                    .foregroundStyle(Theme.textSecondary)
                    .lineLimit(2)
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    Text(approval.formatTimeLeft(at: context.date))
                        .font(.caption)
                        .foregroundStyle(Theme.textMuted)
                        .monospacedDigit()
                }
            }
            Spacer(minLength: 8)
            Text("Review")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(Theme.primaryText)
                .padding(.horizontal, 12)
                .padding(.vertical, 7)
                .background(Theme.primary)
                .clipShape(Capsule())
        }
        .padding(.vertical, 4)
        .contentShape(Rectangle())
    }
}
