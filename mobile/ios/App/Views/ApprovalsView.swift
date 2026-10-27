import SwiftUI

struct ApprovalsView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationStack {
            ApprovalList(
                allApproval: model.allPendingApproval,
                isMacApprovalAllowed: model.isMacApprovalAllowed,
                review: { model.reviewApproval($0) },
                deny: { approval in Task { await model.deny(approval) } },
                allowMacApproval: { Task { await model.setMacApprovalAllowed(true) } }
            )
            .refreshable { await model.refreshPendingApproval() }
            .navigationTitle("Approvals")
        }
    }
}

struct ApprovalList: View {
    var allApproval: [PhoneApproval]
    var isMacApprovalAllowed: Bool
    var review: (PhoneApproval) -> Void
    var deny: (PhoneApproval) -> Void
    var allowMacApproval: () -> Void

    var body: some View {
        List {
            if !allApproval.isEmpty {
                Section {
                    ForEach(allApproval) { approval in
                        Button {
                            review(approval)
                        } label: {
                            PendingApprovalSummary(approval: approval)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .swipeActions(edge: .trailing) {
                            Button("Deny", role: .destructive) { deny(approval) }
                        }
                        .accessibilityIdentifier("approval-row-\(approval.requestID)")
                    }
                } header: {
                    Text(allApproval.count == 1 ? "1 request waiting" : "\(allApproval.count) requests waiting")
                        .foregroundStyle(.themeSecondary)
                        .accessibilityIdentifier("approvals-header")
                } footer: {
                    Text("Tap a request to see what it asks for.")
                        .foregroundStyle(.themeSecondary)
                }
            }
        }
        .applyThemedListStyle()
        .overlay {
            if !isMacApprovalAllowed {
                ContentUnavailableView {
                    Label("Approving Mac requests is off", systemImage: "laptopcomputer.slash")
                        .accessibilityIdentifier("approvals-off")
                } description: {
                    Text("Your Mac decides on its own: Touch ID or the master password there.")
                } actions: {
                    Button("Approve requests from my Mac", action: allowMacApproval)
                        .buttonStyle(.glass)
                        .accessibilityIdentifier("approvals-allow")
                }
            } else if allApproval.isEmpty {
                ContentUnavailableView {
                    Label("No requests waiting", systemImage: "laptopcomputer.and.iphone")
                } description: {
                    Text("Requests from Hisar on your Mac appear here while this iPhone is unlocked.")
                }
                .accessibilityIdentifier("approvals-empty")
            }
        }
    }
}
