import SwiftUI

struct NewSecretKeyView: View {
    var secretKey: String
    var email: String
    var server: String
    var finish: () -> Void

    @State private var isCopied = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                LogoMark(size: 56)
                    .padding(.top, 32)
                Text("Save your Secret Key")
                    .font(.largeTitle.weight(.bold))
                    .foregroundStyle(Theme.text)
                Text("Your account is ready. To sign in on another device you need two secrets: your master password and this Secret Key. Hisar keeps the Secret Key on this iPhone, but nobody, not even your Hisar admin, can show it to you again.")
                    .font(.body)
                    .foregroundStyle(Theme.textSecondary)
                VStack(alignment: .leading, spacing: 10) {
                    Text("Secret Key")
                        .font(.footnote.weight(.medium))
                        .foregroundStyle(Theme.textMuted)
                    Text(secretKey)
                        .font(.system(.title3, design: .monospaced).weight(.medium))
                        .foregroundStyle(Theme.text)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("new-secret-key")
                    Button {
                        _ = PasteboardClipboard().setText(secretKey)
                        isCopied = true
                    } label: {
                        Label(isCopied ? "Copied" : "Copy Secret Key", systemImage: isCopied ? "checkmark" : "doc.on.doc")
                    }
                    .buttonStyle(SecondaryButtonStyle())
                }
                .padding(16)
                .background(Theme.surface)
                .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(Theme.border, lineWidth: 1))
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                VStack(alignment: .leading, spacing: 12) {
                    Label("Make an Emergency Kit", systemImage: "doc.text")
                        .font(.headline)
                        .foregroundStyle(Theme.text)
                    Text("The Emergency Kit is a page with your server, email and Secret Key, and a space to write your master password. Print it from the Hisar desktop app (Settings, Emergency Kit), or write these down now and keep them somewhere safe, away from your devices:")
                        .font(.subheadline)
                        .foregroundStyle(Theme.textSecondary)
                    buildDetailRow(label: "Server", value: server)
                    buildDetailRow(label: "Email", value: email)
                }
                Button("I saved my Secret Key", action: finish)
                    .buttonStyle(PrimaryButtonStyle())
                    .accessibilityIdentifier("new-secret-key-done")
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 32)
        }
        .background(Theme.background.ignoresSafeArea())
        .coverStatusBar()
    }

    private func buildDetailRow(label: String, value: String) -> some View {
        HStack {
            Text(label)
                .foregroundStyle(Theme.textMuted)
            Spacer()
            Text(value)
                .foregroundStyle(Theme.text)
                .textSelection(.enabled)
        }
        .font(.subheadline)
    }
}
