import SwiftUI
import UIKit

struct EmergencyKitShareButton: View {
    var url: URL
    var label: String
    var onPresent: () -> Void = {}
    var onCompleted: () -> Void
    @State private var isPresented = false

    var body: some View {
        Button {
            onPresent()
            isPresented = true
        } label: {
            Label(label, systemImage: "square.and.arrow.up")
                .frame(maxWidth: .infinity)
        }
        .sheet(isPresented: $isPresented) {
            EmergencyKitActivityView(url: url) { didComplete in
                isPresented = false
                if didComplete { onCompleted() }
            }
        }
    }
}

private struct EmergencyKitActivityView: UIViewControllerRepresentable {
    var url: URL
    var onDismiss: (Bool) -> Void

    func makeUIViewController(context: Context) -> UIActivityViewController {
        let controller = UIActivityViewController(activityItems: [url], applicationActivities: nil)
        controller.completionWithItemsHandler = { activity, completed, _, error in
            onDismiss(completed && error == nil && activity != .copyToPasteboard)
        }
        return controller
    }

    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}
