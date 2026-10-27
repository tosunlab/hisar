import SwiftUI

struct DevicesView: View {
    @EnvironmentObject private var model: AppModel
    @StateObject private var devices = DevicesLoadState()
    @State private var deviceToRevoke: Device?

    var body: some View {
        List {
            if devices.didFail {
                Section {
                    ErrorBanner(message: devices.allDevice == nil
                                ? "Couldn't load your devices."
                                : "Couldn't refresh your devices. Showing the last loaded list.")
                    Button("Retry") { Task { await load() } }
                        .accessibilityIdentifier("retry-devices")
                }
            } else if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            if let message = model.deviceOperationErrorMessage {
                Section { ErrorBanner(message: message) }
            }
            if let allDevice = devices.allDevice {
                let allCurrentDevice = allDevice.filter(\.isCurrent)
                let allOtherDevice = allDevice.filter { !$0.isCurrent }
                if !allCurrentDevice.isEmpty {
                    Section("This iPhone") {
                        ForEach(allCurrentDevice) { device in
                            DeviceRow(device: device)
                        }
                    }
                }
                Section {
                    ForEach(allOtherDevice) { device in
                        NavigationLink {
                            DeviceView(device: device) {
                                Task { await load() }
                            }
                        } label: {
                            DeviceRow(device: device)
                        }
                        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                            Button("Sign out", role: .destructive) { deviceToRevoke = device }
                        }
                        .accessibilityIdentifier("device-\(device.name)")
                    }
                } header: {
                    Text("Other devices")
                } footer: {
                    Text(allOtherDevice.isEmpty
                         ? "No other device is signed in to your account."
                         : "Sign out a device you no longer use or don't recognise. It needs your Secret Key and master password to sign in again.")
                }
            } else if !devices.didFail {
                Label {
                    Text("Loading devices…")
                } icon: {
                    ProgressView()
                }
                .foregroundStyle(.themeSecondary)
            }
        }
        .applyThemedListStyle()
        .refreshable { await load() }
        .navigationTitle("Devices")
        .confirmationDialog(
            "Sign out \(deviceToRevoke?.name ?? "this device")?",
            isPresented: Binding(get: { deviceToRevoke != nil }, set: { if !$0 { deviceToRevoke = nil } }),
            titleVisibility: .visible,
            presenting: deviceToRevoke
        ) { device in
            Button("Sign out device", role: .destructive) {
                Task {
                    if await model.revokeDevice(device) {
                        await load()
                    }
                }
            }
            .accessibilityIdentifier("confirm-revoke-device")
        } message: { _ in
            Text("It is signed out of the Hisar server now. The next time it connects, Hisar removes your account from it. Until then, it can still be unlocked while offline. To sign in again, it needs your Secret Key and master password.")
        }
        .task { await load() }
        .onAppear { model.deviceOperationErrorMessage = nil }
    }

    private func load() async {
        await devices.load(using: model)
    }
}

@MainActor
final class DevicesLoadState: ObservableObject {
    enum State {
        case loading
        case loaded([Device])
        case failed(previous: [Device]?)
    }

    @Published private(set) var state: State = .loading
    private var isLoading = false

    var allDevice: [Device]? {
        switch state {
        case .loading: nil
        case .loaded(let devices): devices
        case .failed(let previous): previous
        }
    }

    var didFail: Bool {
        if case .failed = state { return true }
        return false
    }

    func load(using model: AppModel) async {
        guard !isLoading else { return }
        isLoading = true
        defer { isLoading = false }
        model.clearError()
        if let devices = await model.loadAllDevice() {
            state = .loaded(devices)
        } else {
            state = .failed(previous: allDevice)
        }
    }
}

struct DeviceRow: View {
    var device: Device

    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text(device.name.isEmpty ? device.platformDescription : device.name)
                Text(subtitle)
                    .font(.footnote)
                    .foregroundStyle(.themeSecondary)
            }
        } icon: {
            Image(systemName: device.systemImage)
        }
        .accessibilityElement(children: .combine)
    }

    private var subtitle: String {
        if device.isCurrent {
            return "\(device.platformDescription) · Active now"
        }
        guard let lastSeenDate = device.lastSeenDate else { return device.platformDescription }
        return "\(device.platformDescription) · Last seen \(lastSeenDate.formatted(.relative(presentation: .named)))"
    }
}

struct DeviceView: View {
    var device: Device
    var finishRevoke: () -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var isRevokeConfirmationShown = false
    @State private var isRevoking = false

    var body: some View {
        List {
            Section {
                LabeledContent("Name", value: device.name)
                LabeledContent("Platform", value: device.platformDescription)
                if let lastSeenDate = device.lastSeenDate {
                    LabeledContent("Last seen", value: lastSeenDate.formatted(date: .abbreviated, time: .shortened))
                }
                if let createdDate = device.createdDate {
                    LabeledContent("Signed in", value: createdDate.formatted(date: .abbreviated, time: .shortened))
                }
            }
            Section {
                if let message = model.deviceOperationErrorMessage {
                    ErrorBanner(message: message)
                        .accessibilityIdentifier("device-operation-error")
                }
                Button(role: .destructive) { isRevokeConfirmationShown = true } label: {
                    HStack {
                        if isRevoking { ProgressView() }
                        Text(isRevoking ? "Signing out device…" : "Sign out device")
                    }
                    .frame(maxWidth: .infinity)
                }
                .disabled(isRevoking)
                    .accessibilityIdentifier("revoke-device")
            } footer: {
                Text("Signing out ends the device's session on the server, so it can no longer sync your vaults or answer for your account.")
            }
        }
        .applyThemedListStyle()
        .navigationTitle(device.name)
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { model.deviceOperationErrorMessage = nil }
        .confirmationDialog("Sign out \(device.name)?", isPresented: $isRevokeConfirmationShown, titleVisibility: .visible) {
            Button("Sign out device", role: .destructive) {
                Task {
                    isRevoking = true
                    if await model.revokeDevice(device) {
                        finishRevoke()
                        dismiss()
                    }
                    isRevoking = false
                }
            }
            .accessibilityIdentifier("confirm-revoke-device")
        } message: {
            Text("It is signed out of the Hisar server now. The next time it connects, Hisar removes your account from it. Until then, it can still be unlocked while offline. To sign in again, it needs your Secret Key and master password.")
        }
    }
}
