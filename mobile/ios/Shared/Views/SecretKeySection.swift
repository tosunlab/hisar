import SwiftUI

struct SecretKeySection: View {
    var secretKey: String
    var valueIdentifier: String
    var copyIdentifier: String
    var footer: String? = nil
    var onCopy: () -> Void = {}
    @State private var isCopied = false

    var body: some View {
        Section {
            Text(secretKey)
                .font(.title3.monospaced().weight(.medium))
                .textSelection(.enabled)
                .accessibilityIdentifier(valueIdentifier)
            Button {
                isCopied = PasteboardClipboard().setText(secretKey)
                onCopy()
            } label: {
                Label(isCopied ? "Copied · clears in 90 seconds" : "Copy Secret Key", systemImage: isCopied ? "checkmark" : "doc.on.doc")
                    .contentTransition(.symbolEffect(.replace))
            }
            .sensoryFeedback(.success, trigger: isCopied)
            .accessibilityIdentifier(copyIdentifier)
        } header: {
            Text("Secret Key")
        } footer: {
            if let footer { Text(footer) }
        }
        .onChange(of: secretKey) { _, _ in isCopied = false }
    }
}
