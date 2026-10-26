import SwiftUI

struct OneTimeCodeRow: View {
    var vaultID: String
    var itemID: String
    var copy: () -> Void
    var copyReference: () -> Void

    @EnvironmentObject private var model: AppModel
    @State private var oneTimeCode: OneTimeCode?
    @State private var isUnavailable = false

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 3) {
                    Text("One-time code")
                        .font(.footnote)
                        .foregroundStyle(.themeSecondary)
                    if let oneTimeCode {
                        Text(oneTimeCode.formattedCode)
                            .font(.title2.weight(.semibold).monospacedDigit())
                            .contentTransition(.numericText())
                            .accessibilityIdentifier("one-time-code-value")
                    } else if isUnavailable {
                        Text("The one-time password could not be read.")
                            .foregroundStyle(.themeSecondary)
                    } else {
                        ProgressView()
                    }
                }
                Spacer(minLength: 0)
                if let oneTimeCode {
                    CountdownRing(secondsLeft: oneTimeCode.countSecondsLeft(at: context.date), period: oneTimeCode.period)
                    Button(action: copy) {
                        Image(systemName: "doc.on.doc")
                            .iconButtonHitArea()
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel("Copy one-time code")
                    .accessibilityIdentifier("copy-one-time-code")
                }
            }
        }
        .contextMenu {
            Button(action: copy) {
                Label("Copy one-time code", systemImage: "doc.on.doc")
            }
            Button(action: copyReference) {
                Label("Copy Secret Reference", systemImage: "terminal")
            }
        }
        .task(id: itemID) { await refreshCode() }
    }

    private func refreshCode() async {
        while !Task.isCancelled {
            guard let code = await model.loadOneTimeCode(vaultID: vaultID, itemID: itemID) else {
                oneTimeCode = nil
                isUnavailable = true
                return
            }
            withAnimation { oneTimeCode = code }
            isUnavailable = false
            let secondsUntilExpiry = max(1, code.expiryDate?.timeIntervalSinceNow ?? 1)
            try? await Task.sleep(nanoseconds: UInt64(secondsUntilExpiry * 1_000_000_000))
        }
    }
}

private struct CountdownRing: View {
    var secondsLeft: Int
    var period: Int

    @ScaledMetric(relativeTo: .body) private var diameter: CGFloat = 30

    private var remainingFraction: Double {
        period > 0 ? Double(secondsLeft) / Double(period) : 0
    }

    var body: some View {
        ZStack {
            Circle()
                .stroke(.fill.secondary, lineWidth: 3)
            Circle()
                .trim(from: 0, to: remainingFraction)
                .stroke(secondsLeft <= 5 ? AnyShapeStyle(.themeWarning) : AnyShapeStyle(.tint), style: StrokeStyle(lineWidth: 3, lineCap: .round))
                .rotationEffect(.degrees(-90))
                .animation(.linear(duration: 0.9), value: secondsLeft)
            Text("\(secondsLeft)")
                .font(.caption2.weight(.semibold).monospacedDigit())
                .foregroundStyle(.themeSecondary)
        }
        .frame(width: diameter, height: diameter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(secondsLeft) seconds left")
        .accessibilityIdentifier("one-time-code-seconds")
    }
}
