import AuthenticationServices
import SwiftUI
import UIKit
import UserNotifications

/// AppDelegate takes what iOS hands the app delegate: the APNs token and the notifications Hisar shows or the user
/// taps. A tap that launched Hisar arrives before the model exists, so it waits for it.
final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    @MainActor weak var model: AppModel? {
        didSet {
            guard let model, let pendingUserInfo else { return }
            self.pendingUserInfo = nil
            model.notifications.open(userInfo: pendingUserInfo)
        }
    }
    @MainActor private var pendingUserInfo: [AnyHashable: Any]?

    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        Task { @MainActor in await model?.notifications.receiveDeviceToken(deviceToken) }
    }

    /// While Hisar is open, an approval already shows as a card in the app, so only the other kinds show a banner.
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification) async -> UNNotificationPresentationOptions {
        notification.request.content.userInfo["kind"] as? String == PushKind.approval.rawValue ? [] : [.banner, .sound, .list]
    }

    @MainActor
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        let userInfo = response.notification.request.content.userInfo
        if let model {
            model.notifications.open(userInfo: userInfo)
        } else {
            pendingUserInfo = userInfo
        }
    }
}

@main
struct HisarApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel(credentialIdentityStore: SystemCredentialIdentityStore())
    @Environment(\.scenePhase) private var scenePhase
    @State private var hasEnteredBackground = false
    private let lockEventRelay = LockEventRelay()
    private let configuration = HisarConfiguration.current

    init() {
        AppModel.deleteLeftoverKitFiles()
        if ProcessInfo.processInfo.arguments.contains("-HisarUITest") {
            UIView.setAnimationsEnabled(false)
        }
    }

    var body: some Scene {
        WindowGroup {
            mainView
                .environmentObject(model)
                .onAppear { startModel() }
                .onOpenURL { url in
                    if let link = SignInLink.parse(url.absoluteString) {
                        model.pendingSignInLink = link
                    } else if let link = InviteLink.parse(url.absoluteString) {
                        model.openInviteLink(link)
                    }
                }
                // Another password manager exporting to Hisar (Credential Exchange); the import waits for unlocked vaults.
                .onContinueUserActivity(ASCredentialExchangeActivity) { activity in
                    if let token = activity.userInfo?[ASCredentialImportToken] as? UUID {
                        model.receiveCredentialExchange(token: token)
                    }
                }
        }
        .onChange(of: scenePhase) { _, newPhase in
            PrivacyShield.shared.setVisible(newPhase != .active, theme: model.theme)
            switch newPhase {
            case .background:
                hasEnteredBackground = true
                model.handleDidEnterBackground()
            case .active:
                Task { await model.refreshExternalSetupStatus() }
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
        } else if let vaultManagementMode = VaultManagementPreviewView.readMode() {
            VaultManagementPreviewView(mode: vaultManagementMode)
        } else if PendingApprovalPreviewView.isRequested() {
            PendingApprovalPreviewView()
        } else if UpdateRequiredPreview.isRequested() {
            UpdateRequiredView(requirement: UpdateRequiredPreview.requirement, isChecking: false) {}
        } else {
            RootView(retryStartup: retryStartup)
        }
        #else
        RootView(retryStartup: retryStartup)
        #endif
    }

    private func retryStartup() {
        guard model.beginStartupRetry() else { return }
        Task { @MainActor in
            await Task.yield()
            startModel(isRetry: true)
        }
    }

    private func startModel(isRetry: Bool = false) {
        #if DEBUG
        guard AutoFillPreviewView.readMode() == nil, PhoneApprovalPreviewView.readApproval() == nil, VaultManagementPreviewView.readMode() == nil,
              !PendingApprovalPreviewView.isRequested(), !UpdateRequiredPreview.isRequested() else { return }
        #endif
        guard model.screen == .starting else { return }
        appDelegate.model = model
        lockEventRelay.setHandler { [weak model] in model?.handleLockedEvent() }
        lockEventRelay.setApprovalHandler { [weak model] eventName, requestID in model?.handleApprovalEvent(name: eventName, requestID: requestID) }
        lockEventRelay.setRemoteChangeHandler { [weak model] in model?.handleRemoteChange() }
        lockEventRelay.setUpdateRequiredHandler { [weak model] in model?.handleUpdateRequired() }
        let relay = lockEventRelay
        model.start {
            try BackendFactory.createBackend(configuration: configuration, isClipboardIncluded: true, eventListener: relay, isStateResetAllowed: !isRetry)
        }
    }
}
