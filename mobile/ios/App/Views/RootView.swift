import SwiftUI
import UIKit

struct RootView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ZStack {
            switch model.screen {
            case .starting:
                ProgressView()
            case .failed(let message):
                StartupFailureView(message: message)
            case .signedOut:
                SignInView()
            case .locked:
                LockView()
            case .unlocked:
                MainTabView()
            }
        }
        .overlay(alignment: .bottom) {
            if let noticeMessage = model.noticeMessage {
                NoticeToast(message: noticeMessage)
                    .padding(.bottom, model.screen == .unlocked ? 84 : 24)
                    .allowsHitTesting(false)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeOut(duration: 0.2), value: model.noticeMessage)
        .sensoryFeedback(.success, trigger: model.copyCount)
        .sheet(item: Binding(get: { model.screen == .unlocked && model.newSecretKey == nil ? model.presentedApproval : nil }, set: { if $0 == nil { model.closeApprovalSheet() } })) { approval in
            PhoneApprovalSheet(
                approval: approval,
                isBusy: model.isAnsweringApproval,
                isPasswordRequired: model.isApprovalPasswordRequired,
                errorMessage: model.approvalErrorMessage,
                approve: { await model.approvePendingApproval(isRemembered: $0) },
                approveWithPassword: { await model.approvePendingApproval(password: $0, isRemembered: $1) },
                deny: { await model.denyPendingApproval() }
            )
        }
        .fullScreenCover(isPresented: Binding(get: { model.newSecretKey != nil }, set: { if !$0 { model.dismissNewSecretKey() } })) {
            if model.isBiometricOfferShown {
                BiometricOfferView()
            } else {
                NewSecretKeyView(secretKey: model.newSecretKey ?? "", email: model.status.email, server: model.status.server) {
                    model.finishNewSecretKey()
                }
            }
        }
        .applyTheme(model.theme)
        .onChange(of: model.theme, initial: true) { _, theme in applyThemeToAllWindow(theme) }
    }

    // Alerts, the privacy cover and other UIKit windows follow the theme too.
    private func applyThemeToAllWindow(_ theme: VaultTheme) {
        let allWindow = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.flatMap(\.windows)
        for window in allWindow {
            window.overrideUserInterfaceStyle = theme.userInterfaceStyle
            window.tintColor = theme.palette.tint.uiColor
        }
    }
}

struct MainTabView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        TabView(selection: $model.selectedTab) {
            Tab("Vaults", systemImage: "lock.rectangle.stack", value: AppModel.Tab.vaults) {
                VaultsView()
            }
            .accessibilityIdentifier("tab-vaults")
            Tab("Approvals", systemImage: "laptopcomputer.and.iphone", value: AppModel.Tab.approvals) {
                ApprovalsView()
            }
            .badge(model.allPendingApproval.count)
            .accessibilityIdentifier("tab-approvals")
            Tab("Settings", systemImage: "gearshape", value: AppModel.Tab.settings) {
                SettingsView()
            }
            .accessibilityIdentifier("tab-settings")
            Tab(value: AppModel.Tab.search, role: .search) {
                SearchView()
            }
        }
        .onChange(of: model.selectedTab) { _, _ in model.reportActivity() }
    }
}

struct PrivacyCover: View {
    var body: some View {
        ZStack {
            Color(.systemBackground).ignoresSafeArea()
            LogoMark(size: 88)
        }
        .accessibilityIdentifier("privacy-cover")
    }
}

// A floating confirmation; SwiftUI has no system toast, so it sits on Liquid Glass like other floating controls.
struct NoticeToast: View {
    var message: String

    var body: some View {
        Label(message, systemImage: "checkmark.circle.fill")
            .font(.subheadline.weight(.medium))
            .symbolRenderingMode(.multicolor)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .glassEffect(.regular, in: .capsule)
            .padding(.horizontal, 16)
            .accessibilityIdentifier("notice")
    }
}

struct StartupFailureView: View {
    var message: String

    var body: some View {
        ContentUnavailableView {
            Label("Hisar could not start", systemImage: "exclamationmark.triangle")
        } description: {
            Text(message)
                .accessibilityIdentifier("error-message")
        }
    }
}

struct OfflineNote: View {
    var body: some View {
        Label("Offline. Showing items saved on this iPhone; changes need a connection.", systemImage: "wifi.slash")
            .font(.footnote)
            .foregroundStyle(.themeSecondary)
            .accessibilityIdentifier("offline-banner")
    }
}
