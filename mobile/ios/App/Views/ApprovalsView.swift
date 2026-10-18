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
                        .accessibilityIdentifier("approvals-header")
                } footer: {
                    Text("Tap a request to see what it asks for. Approving answers that one request; it never unlocks Hisar on your Mac.")
                }
            }
        }
        .applyThemedListStyle()
        .overlay {
            if !isMacApprovalAllowed {
                ContentUnavailableView {
                    Label("Approving Mac requests is off", systemImage: "laptopcomputer.slash")
                } description: {
                    Text("Your Mac decides on its own: Touch ID or the master password there.")
                } actions: {
                    Button("Allow Approving Mac Requests", action: allowMacApproval)
                        .buttonStyle(.glass)
                        .accessibilityIdentifier("approvals-allow")
                }
                .accessibilityIdentifier("approvals-off")
            } else if allApproval.isEmpty {
                ContentUnavailableView {
                    Label("No requests waiting", systemImage: "laptopcomputer.and.iphone")
                } description: {
                    Text("When Hisar on your Mac asks to use an SSH key or the command-line tool, or to let an app like Raycast use Hisar, the request appears here while Hisar is open and unlocked on this iPhone.")
                }
                .accessibilityIdentifier("approvals-empty")
            }
        }
    }
}
