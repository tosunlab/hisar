import SwiftUI

@MainActor
final class DeferredSecretKeyDelivery: ObservableObject {
    enum State {
        case password
        case preparing
        case ready(PendingRecoveryExport, password: String, hasCompletedHandoff: Bool)
    }

    let operationID: String
    @Published var password = ""
    @Published private(set) var state = State.password
    private var generation = 0

    init(operationID: String) {
        self.operationID = operationID
    }

    var export: PendingRecoveryExport? {
        if case .ready(let export, _, _) = state { return export }
        return nil
    }

    var canAcknowledge: Bool {
        if case .ready(_, _, let hasCompletedHandoff) = state { return hasCompletedHandoff }
        return false
    }

    func prepare(using model: AppModel) async {
        guard case .password = state, !password.isEmpty, !model.isBusy else { return }
        let enteredPassword = password
        password = ""
        state = .preparing
        let requestedGeneration = generation
        await model.exportPendingRecoveryKit(operationID: operationID, password: enteredPassword)
        guard requestedGeneration == generation else { return }
        if let export = model.pendingRecoveryExport {
            state = .ready(export, password: enteredPassword, hasCompletedHandoff: false)
        } else {
            state = .password
        }
    }

    func completedHandoff(path: String) {
        guard case .ready(let export, let password, _) = state, export.path == path else { return }
        state = .ready(export, password: password, hasCompletedHandoff: true)
    }

    func acknowledge(using model: AppModel) async {
        guard case .ready(_, let password, true) = state, !model.isBusy else { return }
        await model.acknowledgePendingRecovery(operationID: operationID, password: password)
        if !model.allPendingRecovery.contains(where: { $0.operationId == operationID }) {
            clear(using: model)
            model.closePendingRecovery()
        }
    }

    func clear(using model: AppModel) {
        generation += 1
        password = ""
        state = .password
        model.clearPendingRecoveryExport()
    }
}

struct DeferredSecretKeyView: View {
    @EnvironmentObject private var model: AppModel
    var recovery: PendingRecovery
    var chooseAnotherAccount: () -> Void
    @StateObject private var delivery: DeferredSecretKeyDelivery

    init(recovery: PendingRecovery, chooseAnotherAccount: @escaping () -> Void) {
        self.recovery = recovery
        self.chooseAnotherAccount = chooseAnotherAccount
        _delivery = StateObject(wrappedValue: DeferredSecretKeyDelivery(operationID: recovery.operationId))
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(verbatim: recovery.email)
                    Text(verbatim: recovery.origin)
                        .font(.subheadline)
                        .foregroundStyle(.themeSecondary)
                }
                if let export = delivery.export {
                    SecretKeySection(
                        secretKey: export.secretKey,
                        valueIdentifier: "pending-recovery-secret-key",
                        copyIdentifier: "pending-recovery-copy-secret-key"
                    )
                    Section {
                        EmergencyKitShareButton(
                            url: URL(fileURLWithPath: export.path),
                            label: "Save or print Emergency Kit",
                            onPresent: model.reportActivity,
                            onCompleted: { delivery.completedHandoff(path: export.path) }
                        )
                        .applyProminentGlassStyle()
                        .controlSize(.large)
                        .accessibilityIdentifier("pending-recovery-share-kit")
                    } header: {
                        Text("Emergency Kit")
                            .foregroundStyle(.themeSecondary)
                    } footer: {
                        Text("The Emergency Kit holds your Secret Key and has a space to write your master password. Keep it away from your devices.")
                            .foregroundStyle(.themeSecondary)
                    }
                    Section {
                        Button("I saved my Emergency Kit") {
                            Task { await delivery.acknowledge(using: model) }
                        }
                        .disabled(!delivery.canAcknowledge || model.isBusy)
                        .accessibilityIdentifier("pending-recovery-acknowledge")
                    } footer: {
                        if !delivery.canAcknowledge {
                            Text("Save or print the Emergency Kit to continue.")
                                .foregroundStyle(.themeSecondary)
                        }
                    }
                } else if case .preparing = delivery.state {
                    Section { ProgressView("Preparing Emergency Kit…") }
                } else {
                    Section {
                        SecureField("Master password", text: $delivery.password)
                            .textContentType(.password)
                            .submitLabel(.go)
                            .onSubmit { Task { await delivery.prepare(using: model) } }
                            .accessibilityIdentifier("pending-recovery-password")
                        Button("Continue") {
                            Task { await delivery.prepare(using: model) }
                        }
                        .disabled(delivery.password.isEmpty || model.isBusy)
                        .accessibilityIdentifier("pending-recovery-export")
                    } footer: {
                        Text("Enter the master password you chose for this account to prepare its Emergency Kit. Your current account stays unchanged.")
                            .foregroundStyle(.themeSecondary)
                    }
                }
                if let errorMessage = model.errorMessage {
                    Section { ErrorBanner(message: errorMessage) }
                }
                Section {
                    Button("Choose another account") {
                        delivery.clear(using: model)
                        chooseAnotherAccount()
                    }
                    Button("Finish later") {
                        delivery.clear(using: model)
                        model.closePendingRecovery()
                    }
                    .accessibilityIdentifier("pending-recovery-later")
                }
                .disabled(model.isBusy)
            }
            .applyThemedListStyle()
            .navigationTitle("Save your Secret Key")
        }
        .onChange(of: [model.status.state, model.status.email, model.status.server]) { _, _ in
            delivery.clear(using: model)
            chooseAnotherAccount()
        }
        .onChange(of: model.allPendingRecovery) { _, records in
            guard let current = records.first(where: { $0.operationId == recovery.operationId }),
                  AccountLifecycleLogic.canDeliverDeferredSignUp(current),
                  current.accountId == recovery.accountId, current.origin == recovery.origin,
                  current.email == recovery.email else {
                delivery.clear(using: model)
                chooseAnotherAccount()
                return
            }
        }
        .onDisappear { delivery.clear(using: model) }
    }
}
