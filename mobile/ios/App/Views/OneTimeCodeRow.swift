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
            HStack(spacing: 0) {
                if let oneTimeCode {
                    let secondsLeft = oneTimeCode.countSecondsLeft(at: context.date)
                    CopyFieldButton(label: "one-time code", value: "\(oneTimeCode.formattedCode), \(secondsLeft) seconds left", identifier: "copy-one-time-code", copy: copy) {
                        HStack {
                            VStack(alignment: .leading, spacing: 3) {
                                title
                                Text(oneTimeCode.formattedCode)
                                    .font(.title2.weight(.semibold).monospacedDigit())
                                    .contentTransition(.numericText())
                            }
                            Spacer(minLength: 8)
                            CountdownRing(secondsLeft: secondsLeft, period: oneTimeCode.period)
                                .accessibilityHidden(true)
                        }
                    }
                    CopyIconButton(copy: copy)
                } else {
                    VStack(alignment: .leading, spacing: 3) {
                        title
                        if isUnavailable {
                            Text("The one-time code could not be read.")
                                .foregroundStyle(.themeSecondary)
                        } else {
                            ProgressView()
                        }
                    }
                    .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                }
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .listRowInsets(EdgeInsets())
        .contextMenu {
            Button(action: copy) {
                Label("Copy one-time code", systemImage: "doc.on.doc")
            }
            Button(action: copyReference) {
                Label("Copy secret reference", systemImage: "terminal")
            }
        }
        .task(id: itemID) { await refreshCode() }
    }

    private var title: some View {
        Text("One-time code")
            .font(.footnote)
            .foregroundStyle(.themeSecondary)
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
