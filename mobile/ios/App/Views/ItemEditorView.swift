import SwiftUI

struct ItemEditorView: View {
    var vaultID: String
    var originalItem: ItemDetail?
    var finish: (ItemDetail?) -> Void

    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var draft = ItemDetail()
    @State private var allURLText: [String] = [""]
    @State private var isPasswordRevealed = false
    @State private var isGeneratorShown = false
    @State private var isDeleteConfirmationShown = false
    @State private var hasLoadedDraft = false

    private var isNew: Bool { originalItem == nil }

    var body: some View {
        NavigationStack {
            Form {
                if let errorMessage = model.errorMessage {
                    Section {
                        ErrorBanner(message: errorMessage)
                            .listRowInsets(EdgeInsets())
                            .listRowBackground(Color.clear)
                    }
                }
                Section {
                    TextField("Title", text: $draft.title)
                        .accessibilityIdentifier("editor-title")
                        .listRowBackground(Theme.surface)
                }
                if !draft.isSSHKey {
                    Section("Login") {
                        TextField("Username or email", text: $draft.username)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-username")
                            .listRowBackground(Theme.surface)
                        HStack {
                            Group {
                                if isPasswordRevealed {
                                    TextField("Password", text: $draft.password)
                                        .font(.system(.body, design: .monospaced))
                                } else {
                                    SecureField("Password", text: $draft.password, prompt: Text("Password").foregroundStyle(Theme.textMuted))
                                }
                            }
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-password")
                            Button {
                                isPasswordRevealed.toggle()
                            } label: {
                                Image(systemName: isPasswordRevealed ? "eye.slash" : "eye")
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(Theme.textSecondary)
                            .accessibilityLabel(isPasswordRevealed ? "Hide password" : "Reveal password")
                            Button {
                                isGeneratorShown = true
                            } label: {
                                Image(systemName: "wand.and.stars")
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(Theme.textSecondary)
                            .accessibilityLabel("Generate password")
                            .accessibilityIdentifier("editor-generate-password")
                        }
                        .listRowBackground(Theme.surface)
                    }
                    Section {
                        TextField("Setup key or otpauth:// link", text: $draft.oneTimePassword)
                            .font(.system(.body, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-one-time-password")
                            .listRowBackground(Theme.surface)
                    } header: {
                        Text("One-time password")
                    } footer: {
                        Text("The setup key or link from the site's two-factor setup. Hisar then shows the current code.")
                    }
                    Section("Websites") {
                        ForEach(allURLText.indices, id: \.self) { index in
                            TextField("Website", text: $allURLText[index], prompt: Text(verbatim: "https://example.com"))
                                .keyboardType(.URL)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .accessibilityIdentifier("editor-website-\(index)")
                                .listRowBackground(Theme.surface)
                        }
                        .onDelete { allURLText.remove(atOffsets: $0) }
                        Button {
                            allURLText.append("")
                        } label: {
                            Label("Add website", systemImage: "plus")
                        }
                        .listRowBackground(Theme.surface)
                    }
                    Section("Fields") {
                        ForEach(draft.allField.indices, id: \.self) { index in
                            VStack(alignment: .leading, spacing: 8) {
                                TextField("Name", text: $draft.allField[index].name)
                                    .font(.caption)
                                if draft.allField[index].concealed {
                                    SecureField("Value", text: $draft.allField[index].value)
                                } else {
                                    TextField("Value", text: $draft.allField[index].value)
                                }
                                Toggle("Concealed", isOn: $draft.allField[index].concealed)
                                    .tint(Theme.success)
                                    .font(.caption)
                                    .foregroundStyle(Theme.textMuted)
                            }
                            .listRowBackground(Theme.surface)
                        }
                        .onDelete { draft.allField.remove(atOffsets: $0) }
                        Button {
                            draft.allField.append(ItemField(name: "", value: "", concealed: false))
                        } label: {
                            Label("Add field", systemImage: "plus")
                        }
                        .listRowBackground(Theme.surface)
                    }
                }
                Section("Notes") {
                    TextField("Notes", text: $draft.notes, axis: .vertical)
                        .lineLimit(3...10)
                        .accessibilityIdentifier("editor-notes")
                        .listRowBackground(Theme.surface)
                }
                if !isNew {
                    Section {
                        Button("Delete item", role: .destructive) {
                            isDeleteConfirmationShown = true
                        }
                        .frame(maxWidth: .infinity)
                        .listRowBackground(Theme.surface)
                        .accessibilityIdentifier("editor-delete")
                    }
                }
            }
            .applyListTheme()
            .navigationTitle(isNew ? "New login" : "Edit item")
            .navigationBarTitleDisplayMode(.inline)
            .applyNavigationBarTheme()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        model.clearError()
                        dismiss()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.isBusy {
                        ProgressView()
                    } else {
                        Button("Save", action: save)
                            .disabled(draft.title.trimmingCharacters(in: .whitespaces).isEmpty)
                            .accessibilityIdentifier("editor-save")
                    }
                }
            }
            .confirmationDialog("Delete “\(draft.title)”?", isPresented: $isDeleteConfirmationShown, titleVisibility: .visible) {
                Button("Delete", role: .destructive, action: delete)
            } message: {
                Text("It is deleted on every device that can see this vault.")
            }
            .sheet(isPresented: $isGeneratorShown) {
                PasswordGeneratorView(isUseShown: true) { password in
                    draft.password = password
                    isPasswordRevealed = true
                }
                .environmentObject(model)
            }
        }
        .preferredColorScheme(.dark)
        .interactiveDismissDisabled(model.isBusy)
        .onAppear(perform: loadDraft)
    }

    private func loadDraft() {
        guard !hasLoadedDraft else { return }
        hasLoadedDraft = true
        model.clearError()
        if let originalItem {
            draft = originalItem
            allURLText = originalItem.allURL.isEmpty ? [""] : originalItem.allURL
        }
    }

    private func save() {
        var item = draft
        item.allURL = allURLText.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        Task {
            if let savedItem = await model.saveItem(vaultID: vaultID, item: item) {
                finish(savedItem)
                dismiss()
            }
        }
    }

    private func delete() {
        guard let originalItem else { return }
        Task {
            if await model.deleteItem(vaultID: vaultID, itemID: originalItem.id) {
                dismiss()
                finish(nil)
            }
        }
    }
}
