import SwiftUI
import UIKit

struct RootView: View {
    @EnvironmentObject private var model: AppModel
    var retryStartup: () -> Void
    @Environment(\.accessibilityReduceMotion) private var isMotionReduced

    var body: some View {
        ZStack {
            switch model.screen {
            case .starting:
                ProgressView()
            case .failed(let message):
                StartupFailureView(message: message, retry: retryStartup)
            case .signedOut:
                SignInView()
            case .locked:
                LockView()
            case .unlocked:
                MainTabView()
                    .id("\(model.status.server)\n\(model.status.email)")
            case .updateRequired(let requirement):
                UpdateRequiredView(requirement: requirement, isChecking: model.isCheckingMinimumVersion) {
                    await model.checkMinimumVersion()
                }
            }
        }
        .alert("Invitation", isPresented: Binding(get: { model.inviteLinkMessage != nil }, set: { if !$0 { model.inviteLinkMessage = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(model.inviteLinkMessage ?? "")
        }
        .alert("Import from another app", isPresented: Binding(get: { model.credentialImportMessage != nil }, set: { if !$0 { model.credentialImportMessage = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(model.credentialImportMessage ?? "")
        }
        .overlay(alignment: .bottom) {
            if let noticeMessage = model.noticeMessage {
                NoticeToast(message: noticeMessage)
                    .padding(.bottom, model.screen == .unlocked ? 84 : 24)
                    .allowsHitTesting(false)
                    .transition(isMotionReduced ? .identity : .move(edge: .bottom).combined(with: .opacity))
            }
        }
        .safeAreaInset(edge: .top) {
            if !model.allPendingRecovery.isEmpty && !model.isPendingRecoveryShown && model.onboardingStep == nil {
                Button { model.openPendingRecovery() } label: {
                    HStack {
                        Image(systemName: "exclamationmark.triangle")
                        Text(model.allPendingRecovery.count == 1
                             ? model.allPendingRecovery[0].reminderLabel
                             : "\(model.allPendingRecovery.count) account setups need attention")
                            .frame(maxWidth: .infinity, alignment: .leading)
                        Image(systemName: "chevron.right")
                    }
                }
                    .accessibilityIdentifier("pending-recovery-open")
                    .accessibilityHint(model.allPendingRecovery.count == 1 ? model.allPendingRecovery[0].reminderHint : "Opens the account setup chooser")
                    .padding()
                    .frame(maxWidth: .infinity)
                    .background(.regularMaterial)
            }
        }
        .animation(isMotionReduced ? nil : .easeOut(duration: 0.2), value: model.noticeMessage)
        .sensoryFeedback(.success, trigger: model.copyCount)
        .sheet(item: Binding(get: { model.screen == .unlocked && model.onboardingStep == nil && !model.isPendingRecoveryShown ? model.presentedApproval : nil }, set: { if $0 == nil { model.closeApprovalSheet() } })) { approval in
            PhoneApprovalSheet(
                approval: approval,
                isBusy: model.isAnsweringApproval,
                isPasswordRequired: model.isApprovalPasswordRequired,
                errorMessage: model.approvalErrorMessage,
                isRemembered: Binding(get: { model.rememberChoice(for: approval) }, set: { model.setRememberChoice($0, for: approval) }),
                approve: { await model.approvePendingApproval(isRemembered: $0) },
                approveWithPassword: { await model.approvePendingApproval(password: $0, isRemembered: $1) },
                deny: { await model.denyPendingApproval() }
            )
        }
        .fullScreenCover(isPresented: Binding(get: { model.isPendingRecoveryShown || model.onboardingStep != nil }, set: {
            if !$0 && model.status.isUnlocked {
                model.closePendingRecovery()
                model.closeEmergencyKit()
            }
        })) {
            if model.isPendingRecoveryShown {
                PendingRecoveryView()
            } else if model.onboardingStep == .biometric {
                BiometricOfferView()
            } else {
                NewSecretKeyView()
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
        .safeAreaInset(edge: .top) {
            if let message = model.editorRecoveryMessage {
                VStack {
                    Text(message)
                    Button("Recover a copy") { model.recoverEditorCopy() }
                        .disabled(!model.allVault.contains(where: AppModel.isWritableVault))
                    Button("Discard draft", role: .destructive) { model.discardEditor() }
                }
                .padding()
            }
        }
        .sheet(isPresented: $model.isRestoredEditorShown) {
            if let draft = model.retainedEditor {
                ItemEditorView(vaultID: draft.sourceVaultID, originalItem: draft.originalItem, newCategory: draft.item.category) { _, _ in }
                    .environmentObject(model)
            }
        }
        .onChange(of: model.selectedTab) { _, _ in model.reportActivity() }
        // A notification tapped while Hisar was locked opens its screen once the tabs show, after unlocking.
        .onReceive(model.notifications.$openedKind) { kind in
            if let kind { model.showNotification(kind) }
        }
        .sheet(isPresented: Binding(get: { model.isCredentialImportShown }, set: { if !$0 { model.closeCredentialImport() } })) {
            CredentialImportView()
                .environmentObject(model)
        }
    }
}

struct PrivacyCover: View {
    @Environment(\.themePalette) private var palette

    var body: some View {
        ZStack {
            palette.groupedBackground.color.ignoresSafeArea()
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
    var retry: () -> Void

    var body: some View {
        ContentUnavailableView {
            Label("Hisar could not start", systemImage: "exclamationmark.triangle")
        } description: {
            Text(message)
                .accessibilityIdentifier("error-message")
                .textSelection(.enabled)
            Text("If this keeps happening, email support@tryhisar.com.")
        } actions: {
            Button("Try again", action: retry)
                .applyProminentGlassStyle()
                .accessibilityIdentifier("startup-retry")
            Button("Copy details") { UIPasteboard.general.string = message }
                .accessibilityIdentifier("startup-copy-details")
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
