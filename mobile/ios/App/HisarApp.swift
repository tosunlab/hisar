import SwiftUI
import UIKit

@main
struct HisarApp: App {
    @StateObject private var model = AppModel(credentialIdentityStore: SystemCredentialIdentityStore())
    @Environment(\.scenePhase) private var scenePhase
    @State private var hasEnteredBackground = false
    private let lockEventRelay = LockEventRelay()

    init() {
        if ProcessInfo.processInfo.arguments.contains("-HisarUITest") {
            UIView.setAnimationsEnabled(false)
        }
    }

    var body: some Scene {
        WindowGroup {
            mainView
                .environmentObject(model)
                .onAppear(perform: startModel)
                .onOpenURL { url in
                    if let link = SignInLink.parse(url.absoluteString) {
                        model.pendingSignInLink = link
                    } else if let link = InviteLink.parse(url.absoluteString) {
                        model.openInviteLink(link)
                    }
                }
        }
        .onChange(of: scenePhase) { _, newPhase in
            PrivacyShield.shared.setVisible(newPhase != .active)
            switch newPhase {
            case .background:
                hasEnteredBackground = true
                model.handleDidEnterBackground()
            case .active:
                if hasEnteredBackground {
                    hasEnteredBackground = false
                    model.handleWillEnterForeground()
                }
                model.reportActivity()
            default:
                break
            }
        }
    }

    @ViewBuilder
    private var mainView: some View {
        #if DEBUG
        if let mode = AutoFillPreviewView.readMode() {
            AutoFillPreviewView(mode: mode)
        } else if let approval = PhoneApprovalPreviewView.readApproval() {
            PhoneApprovalPreviewView(approval: approval)
        } else if PendingApprovalPreviewView.isRequested() {
            PendingApprovalPreviewView()
        } else if UpdateRequiredPreview.isRequested() {
            UpdateRequiredView(requirement: UpdateRequiredPreview.requirement, isChecking: false) {}
        } else {
            RootView()
        }
        #else
        RootView()
        #endif
    }

    private func startModel() {
        #if DEBUG
        guard AutoFillPreviewView.readMode() == nil, PhoneApprovalPreviewView.readApproval() == nil, !PendingApprovalPreviewView.isRequested(), !UpdateRequiredPreview.isRequested() else { return }
        #endif
        guard model.screen == .starting else { return }
        lockEventRelay.setHandler { [weak model] in model?.handleLockedEvent() }
        lockEventRelay.setApprovalHandler { [weak model] eventName, requestID in model?.handleApprovalEvent(name: eventName, requestID: requestID) }
        lockEventRelay.setRemoteChangeHandler { [weak model] in model?.handleRemoteChange() }
        lockEventRelay.setUpdateRequiredHandler { [weak model] in model?.handleUpdateRequired() }
        let relay = lockEventRelay
        model.start {
            try BackendFactory.createBackend(configuration: .current, isClipboardIncluded: true, eventListener: relay)
        }
    }
}
