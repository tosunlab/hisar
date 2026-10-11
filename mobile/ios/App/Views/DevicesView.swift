import SwiftUI

struct DevicesView: View {
    @EnvironmentObject private var model: AppModel
    @State private var allDevice: [Device]?
    @State private var deviceToRevoke: Device?

    var body: some View {
        List {
            if let errorMessage = model.errorMessage {
                Section {
                    ErrorBanner(message: errorMessage)
                }
            }
            if let allDevice {
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
                            Button("Sign Out", role: .destructive) { deviceToRevoke = device }
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
            } else {
                Label {
                    Text("Loading devices…")
                } icon: {
                    ProgressView()
                }
                .foregroundStyle(.secondary)
            }
        }
        .listStyle(.insetGrouped)
        .refreshable { await load() }
        .navigationTitle("Devices")
        .confirmationDialog(
            "Sign out \(deviceToRevoke?.name ?? "this device")?",
            isPresented: Binding(get: { deviceToRevoke != nil }, set: { if !$0 { deviceToRevoke = nil } }),
            titleVisibility: .visible,
            presenting: deviceToRevoke
        ) { device in
            Button("Sign Out Device", role: .destructive) {
                Task {
                    if await model.revokeDevice(device) {
                        await load()
                    }
                }
            }
            .accessibilityIdentifier("confirm-revoke-device")
        } message: { _ in
            Text("It loses access to your vaults at once and needs your Secret Key and master password to sign in again.")
        }
        .task { await load() }
    }

    private func load() async {
        model.clearError()
        allDevice = await model.loadAllDevice() ?? allDevice ?? []
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
                    .foregroundStyle(.secondary)
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
                Button("Sign Out Device", role: .destructive) { isRevokeConfirmationShown = true }
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("revoke-device")
            } footer: {
                Text("Signing out ends the device's session on the server, so it can no longer sync your vaults or answer for your account.")
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(device.name)
        .navigationBarTitleDisplayMode(.inline)
        .confirmationDialog("Sign out \(device.name)?", isPresented: $isRevokeConfirmationShown, titleVisibility: .visible) {
            Button("Sign Out Device", role: .destructive) {
                Task {
                    if await model.revokeDevice(device) {
                        finishRevoke()
                        dismiss()
                    }
                }
            }
            .accessibilityIdentifier("confirm-revoke-device")
        } message: {
            Text("It loses access to your vaults at once and needs your Secret Key and master password to sign in again.")
        }
    }
}
