import SwiftUI

struct OneTimeCodeRow: View {
    var vaultID: String
    var itemID: String
    var copy: () -> Void

    @EnvironmentObject private var model: AppModel
    @State private var oneTimeCode: OneTimeCode?
    @State private var isUnavailable = false

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            HStack(alignment: .center, spacing: 12) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("One-time code")
                        .font(.caption)
                        .foregroundStyle(Theme.textMuted)
                    if let oneTimeCode {
                        Text(oneTimeCode.formattedCode)
                            .font(.system(.title3, design: .monospaced).weight(.medium))
                            .foregroundStyle(Theme.text)
                            .accessibilityIdentifier("one-time-code-value")
                    } else if isUnavailable {
                        Text("The one-time password could not be read.")
                            .foregroundStyle(Theme.textMuted)
                    } else {
                        ProgressView().tint(Theme.textMuted)
                    }
                }
                Spacer()
                if let oneTimeCode {
                    CountdownRing(secondsLeft: oneTimeCode.countSecondsLeft(at: context.date), period: oneTimeCode.period)
                    Button(action: copy) {
                        Image(systemName: "doc.on.doc")
                    }
                    .buttonStyle(.borderless)
                    .foregroundStyle(Theme.textSecondary)
                    .accessibilityLabel("Copy one-time code")
                    .accessibilityIdentifier("copy-one-time-code")
                }
            }
        }
        .padding(.vertical, 2)
        .listRowBackground(Theme.surface)
        .task(id: itemID) { await refreshCode() }
    }

    private func refreshCode() async {
        while !Task.isCancelled {
            guard let code = await model.loadOneTimeCode(vaultID: vaultID, itemID: itemID) else {
                oneTimeCode = nil
                isUnavailable = true
                return
            }
            oneTimeCode = code
            isUnavailable = false
            let secondsUntilExpiry = max(1, code.expiryDate?.timeIntervalSinceNow ?? 1)
            try? await Task.sleep(nanoseconds: UInt64(secondsUntilExpiry * 1_000_000_000))
        }
    }
}

private struct CountdownRing: View {
    var secondsLeft: Int
    var period: Int

    private var remainingFraction: Double {
        period > 0 ? Double(secondsLeft) / Double(period) : 0
    }

    var body: some View {
        ZStack {
            Circle()
                .stroke(Theme.borderStrong, lineWidth: 2.5)
            Circle()
                .trim(from: 0, to: remainingFraction)
                .stroke(secondsLeft <= 5 ? Theme.warning : Theme.textSecondary, style: StrokeStyle(lineWidth: 2.5, lineCap: .round))
                .rotationEffect(.degrees(-90))
                .animation(.linear(duration: 0.9), value: secondsLeft)
            Text("\(secondsLeft)")
                .font(.caption2.monospacedDigit())
                .foregroundStyle(Theme.textSecondary)
        }
        .frame(width: 28, height: 28)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(secondsLeft) seconds left")
        .accessibilityIdentifier("one-time-code-seconds")
    }
}
