import SwiftUI

struct PhoneApprovalSheet: View {
    var approval: PhoneApproval
    var isBusy: Bool
    var isPasswordRequired: Bool
    var errorMessage: String?
    var approve: (_ isRemembered: Bool) async -> Void
    var approveWithPassword: (_ password: String, _ isRemembered: Bool) async -> Void
    var deny: () async -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var password = ""
    @State private var chosenRemember: Bool?
    @FocusState private var isPasswordFocused: Bool

    var body: some View {
        NavigationStack {
            List {
                Section {
                    header
                        .listRowBackground(Color.clear)
                }
                // Right below the request, so the field is on screen and can take focus when Face ID is not available;
                // a List creates no row below the screen.
                if let errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                    }
                }
                if isPasswordRequired {
                    Section {
                        SecureField("Master password", text: $password)
                            .textContentType(.password)
                            .submitLabel(.go)
                            .focused($isPasswordFocused)
                            .onSubmit(submitPassword)
                            .accessibilityIdentifier("phone-approval-password")
                    } header: {
                        Text("Approve with your master password")
                    } footer: {
                        Text("Face ID is not available, so Hisar checks your master password instead.")
                    }
                }
                if approval.isRememberable {
                    Section {
                        Toggle(approval.rememberLabel, isOn: Binding(get: { approval.resolveRememberChoice(chosenRemember) }, set: { chosenRemember = $0 }))
                            .disabled(isBusy)
                            .accessibilityIdentifier("phone-approval-remember")
                    } footer: {
                        Text("Off allows only this request.")
                    }
                }
                if approval.isForwarded {
                    Section {
                        Label {
                            Text("Forwarded from another computer: this request comes from \(approval.forwardedFromHost.isEmpty ? "a remote host" : approval.forwardedFromHost) through agent forwarding, not from your Mac. Approving allows it once.")
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundStyle(.red)
                        }
                        .font(.subheadline)
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("phone-approval-forwarded")
                    }
                }
                Section {
                    ForEach(approval.allDetailRow) { row in
                        LabeledContent(row.label) {
                            Text(row.value)
                                .font(row.isMonospaced ? .system(.subheadline, design: .monospaced) : .subheadline)
                                .foregroundStyle(.primary)
                                .multilineTextAlignment(.trailing)
                                .textSelection(.enabled)
                                .accessibilityIdentifier("phone-approval-row-\(row.label)")
                        }
                    }
                } footer: {
                    Label("End-to-end encrypted. Only your devices can read this request. Approving lets this one request continue; it does not unlock Hisar on your Mac.", systemImage: "lock.shield")
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("phone-approval-encryption-note")
                }
            }
            .listStyle(.insetGrouped)
            .navigationTitle("Mac request")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button {
                        dismiss()
                    } label: {
                        Label("Close", systemImage: "xmark")
                    }
                    .disabled(isBusy)
                    .accessibilityIdentifier("phone-approval-close")
                }
            }
            .safeAreaInset(edge: .bottom) {
                actions
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(isBusy)
        .accessibilityIdentifier("phone-approval")
        .onChange(of: approval.requestID) {
            chosenRemember = nil
        }
        .onChange(of: isPasswordRequired) { _, isRequired in
            isPasswordFocused = isRequired
        }
    }

    private var header: some View {
        VStack(spacing: 10) {
            if !approval.isForwarded, let appIcon = approval.appIcon.flatMap(UIImage.init(data:)) {
                Image(uiImage: appIcon)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: 64, height: 64)
                    .frame(width: 72, height: 72)
                    .accessibilityIdentifier("phone-approval-app-icon")
                    .accessibilityHidden(true)
            } else {
                Image(systemName: approval.isForwarded ? "globe" : approval.systemImage)
                    .font(.largeTitle)
                    .foregroundStyle(approval.isForwarded ? AnyShapeStyle(.red) : AnyShapeStyle(.themeWarning))
                    .frame(width: 72, height: 72)
                    .background(.fill.tertiary, in: .rect(cornerRadius: 18, style: .continuous))
                    .accessibilityHidden(true)
            }
            Text(approval.title)
                .font(.title2.weight(.bold))
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("phone-approval-title")
            Text(approval.summary)
                .font(.body)
                .foregroundStyle(.themeSecondary)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("phone-approval-summary")
            TimelineView(.periodic(from: .now, by: 1)) { context in
                Text(approval.formatTimeLeft(at: context.date))
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(.themeWarning)
                    .monospacedDigit()
                    .accessibilityIdentifier("phone-approval-expiry")
            }
        }
        .frame(maxWidth: .infinity)
    }

    private var actions: some View {
        GlassEffectContainer {
            HStack(spacing: 12) {
                Button {
                    Task { await deny() }
                } label: {
                    Text("Deny")
                        .frame(minWidth: 72)
                }
                .buttonStyle(.glass)
                .accessibilityIdentifier("phone-approval-deny")
                if isPasswordRequired {
                    Button(action: submitPassword) {
                        Group {
                            if isBusy {
                                ProgressView()
                            } else {
                                Text("Approve")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .disabled(password.isEmpty)
                    .accessibilityIdentifier("phone-approval-approve-password")
                } else {
                    Button {
                        Task { await approve(approval.resolveRememberChoice(chosenRemember)) }
                    } label: {
                        Group {
                            if isBusy {
                                ProgressView()
                            } else {
                                Label("Approve with Face ID", systemImage: "faceid")
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .applyProminentGlassStyle()
                    .accessibilityIdentifier("phone-approval-approve")
                }
            }
            .controlSize(.large)
            .disabled(isBusy)
            .padding(.horizontal, 20)
            .padding(.bottom, 8)
        }
    }

    private func submitPassword() {
        guard !password.isEmpty, !isBusy else { return }
        let enteredPassword = password
        password = ""
        Task { await approveWithPassword(enteredPassword, approval.resolveRememberChoice(chosenRemember)) }
    }
}
