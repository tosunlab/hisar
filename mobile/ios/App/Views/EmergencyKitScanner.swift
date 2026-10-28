import AVFoundation
import SwiftUI
import UIKit

enum EmergencyKitCameraState: Equatable {
    case unavailable, requesting, authorized, denied, restricted

    init(hasCamera: Bool, authorization: AVAuthorizationStatus) {
        guard hasCamera else { self = .unavailable; return }
        switch authorization {
        case .notDetermined: self = .requesting
        case .authorized: self = .authorized
        case .denied: self = .denied
        case .restricted: self = .restricted
        @unknown default: self = .restricted
        }
    }

    var explanation: String {
        switch self {
        case .denied:
            return "Camera access is off. Allow camera access in Settings to scan the QR code, or type the details from your Emergency Kit."
        case .restricted:
            return "Camera access is restricted on this iPhone. Type the details from your Emergency Kit instead."
        default:
            return "No camera is available. Type the details from your Emergency Kit instead."
        }
    }
}

struct EmergencyKitScannerSheet: View {
    var handleScannedKit: (SignInLink) -> Void
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.openURL) private var openURL
    @State private var cameraState = EmergencyKitCameraState(
        hasCamera: AVCaptureDevice.default(for: .video) != nil,
        authorization: AVCaptureDevice.authorizationStatus(for: .video)
    )
    @State private var scannerMessage: String?
    @State private var scanAttempt = 0

    var body: some View {
        NavigationStack {
            ZStack {
                Color(.systemGroupedBackground).ignoresSafeArea()
                if cameraState == .authorized && scenePhase == .active {
                    QRCodeCameraView { text in
                        if let link = SignInLink.parse(text) {
                            handleScannedKit(link)
                        } else {
                            let message = "This QR code is not from a Hisar Emergency Kit."
                            scannerMessage = message
                            UIAccessibility.post(notification: .announcement, argument: message)
                        }
                    }
                        .id(scanAttempt)
                        .ignoresSafeArea(edges: .bottom)
                    VStack {
                        Spacer()
                        VStack(spacing: 12) {
                            Text(scannerMessage ?? "Point the camera at the QR code on your Emergency Kit.")
                                .font(.subheadline)
                                .foregroundStyle(.primary)
                                .accessibilityIdentifier("scanner-note")
                            if scannerMessage != nil {
                                Button("Try again") {
                                    scannerMessage = nil
                                    scanAttempt += 1
                                }
                                .accessibilityIdentifier("scanner-try-again")
                            }
                        }
                        .padding(14)
                        .glassEffect(.regular, in: .rect(cornerRadius: 16))
                        .padding(24)
                    }
                } else if cameraState == .requesting {
                    ProgressView("Requesting camera access…")
                } else {
                    VStack(spacing: 12) {
                        Image(systemName: "camera.fill")
                            .font(.largeTitle)
                            .foregroundStyle(.themeSecondary)
                        Text(cameraState.explanation)
                            .multilineTextAlignment(.center)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityIdentifier("scanner-camera-message")
                        if cameraState == .denied, let settingsURL = URL(string: UIApplication.openSettingsURLString) {
                            Button("Open Settings") { openURL(settingsURL) }
                                .accessibilityIdentifier("scanner-open-settings")
                        }
                    }
                    .padding(32)
                }
            }
            .navigationTitle("Scan Emergency Kit")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .task(id: scenePhase) {
            guard scenePhase == .active else { return }
            let hasCamera = AVCaptureDevice.default(for: .video) != nil
            let authorization = AVCaptureDevice.authorizationStatus(for: .video)
            cameraState = EmergencyKitCameraState(hasCamera: hasCamera, authorization: authorization)
            if hasCamera && authorization == .notDetermined {
                let isGranted = await AVCaptureDevice.requestAccess(for: .video)
                guard !Task.isCancelled else { return }
                cameraState = isGranted ? .authorized : .denied
            }
        }
    }
}

struct QRCodeCameraView: UIViewControllerRepresentable {
    var handleScannedText: (String) -> Void

    func makeUIViewController(context: Context) -> QRCodeCameraController {
        let controller = QRCodeCameraController()
        controller.handleScannedText = handleScannedText
        return controller
    }

    func updateUIViewController(_ controller: QRCodeCameraController, context: Context) {}
}

final class QRCodeCameraController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
    var handleScannedText: ((String) -> Void)?
    private let session = AVCaptureSession()
    private var previewLayer: AVCaptureVideoPreviewLayer?
    private var hasScanned = false

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        guard AVCaptureDevice.authorizationStatus(for: .video) == .authorized else { return }
        configureSession()
    }

    private func configureSession() {
        guard let device = AVCaptureDevice.default(for: .video),
              let input = try? AVCaptureDeviceInput(device: device),
              session.canAddInput(input) else { return }
        session.addInput(input)
        let output = AVCaptureMetadataOutput()
        guard session.canAddOutput(output) else { return }
        session.addOutput(output)
        output.setMetadataObjectsDelegate(self, queue: .main)
        output.metadataObjectTypes = [.qr]
        let previewLayer = AVCaptureVideoPreviewLayer(session: session)
        previewLayer.videoGravity = .resizeAspectFill
        previewLayer.frame = view.bounds
        view.layer.addSublayer(previewLayer)
        self.previewLayer = previewLayer
        let session = session
        DispatchQueue.global(qos: .userInitiated).async { session.startRunning() }
    }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        previewLayer?.frame = view.bounds
    }

    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        let session = session
        DispatchQueue.global(qos: .userInitiated).async { session.stopRunning() }
    }

    func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput allMetadataObject: [AVMetadataObject], from connection: AVCaptureConnection) {
        guard !hasScanned,
              let code = allMetadataObject.compactMap({ $0 as? AVMetadataMachineReadableCodeObject }).first?.stringValue else { return }
        hasScanned = true
        handleScannedText?(code)
    }
}
