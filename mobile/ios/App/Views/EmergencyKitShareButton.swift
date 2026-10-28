import SwiftUI
import UIKit

struct EmergencyKitShareButton: View {
    var url: URL
    var label: String
    var onPresent: () -> Void = {}
    var onCompleted: (String?) -> Void
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
            EmergencyKitActivityView(url: url) { method in
                isPresented = false
                onCompleted(method)
            }
        }
        .onDisappear { isPresented = false }
    }

    static func deliveryMethod(activity: UIActivity.ActivityType?, completed: Bool, error: Error?) -> String? {
        guard completed, error == nil, let activity, activity != .copyToPasteboard else { return nil }
        if activity == .print { return "print" }
        if activity.rawValue == "com.apple.DocumentManagerUICore.SaveToFiles" { return "file" }
        return "share"
    }
}

private struct EmergencyKitActivityView: UIViewControllerRepresentable {
    var url: URL
    var onDismiss: (String?) -> Void

    func makeUIViewController(context: Context) -> UIActivityViewController {
        let controller = UIActivityViewController(activityItems: [url], applicationActivities: nil)
        controller.completionWithItemsHandler = { activity, completed, _, error in
            onDismiss(EmergencyKitShareButton.deliveryMethod(activity: activity, completed: completed, error: error))
        }
        return controller
    }

    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}
