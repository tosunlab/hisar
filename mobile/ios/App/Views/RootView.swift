import SwiftUI

struct RootView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ZStack {
            Theme.background.ignoresSafeArea()
            switch model.screen {
            case .starting:
                ProgressView().tint(Theme.textMuted)
            case .failed(let message):
                StartupFailureView(message: message)
            case .signedOut:
                SignInView()
            case .locked:
                LockView()
            case .unlocked:
                HomeView()
            }
        }
        .overlay(alignment: .bottom) {
            if let noticeMessage = model.noticeMessage {
                NoticeToast(message: noticeMessage)
                    .padding(.bottom, 24)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeOut(duration: 0.2), value: model.noticeMessage)
        .fullScreenCover(isPresented: Binding(get: { model.newSecretKey != nil }, set: { if !$0 { model.dismissNewSecretKey() } })) {
            NewSecretKeyView(secretKey: model.newSecretKey ?? "", email: model.status.email, server: model.status.server) {
                model.dismissNewSecretKey()
            }
            .preferredColorScheme(.dark)
        }
    }
}

struct PrivacyCover: View {
    var body: some View {
        ZStack {
            Theme.background.ignoresSafeArea()
            LogoMark(size: 88)
        }
        .accessibilityIdentifier("privacy-cover")
    }
}

struct NoticeToast: View {
    var message: String

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "checkmark.circle.fill")
                .foregroundStyle(Theme.success)
            Text(message)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(Theme.text)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Theme.surfaceRaised)
        .overlay(Capsule().stroke(Theme.borderStrong, lineWidth: 1))
        .clipShape(Capsule())
        .shadow(color: .black.opacity(0.4), radius: 12, y: 4)
        .accessibilityIdentifier("notice")
    }
}

struct StartupFailureView: View {
    var message: String

    var body: some View {
        VStack(spacing: 16) {
            LogoMark(size: 64)
            Text("Hisar could not start")
                .font(.title3.weight(.semibold))
                .foregroundStyle(Theme.text)
            ErrorBanner(message: message)
        }
        .padding(24)
    }
}

struct OfflineBanner: View {
    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "wifi.slash")
            Text("Offline. Showing items saved on this iPhone; changes need a connection.")
                .font(.footnote)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .foregroundStyle(Theme.warning)
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Theme.warning.opacity(0.12))
        .accessibilityIdentifier("offline-banner")
    }
}
