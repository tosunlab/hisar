import SwiftUI

struct ItemEditorView: View {
    var vaultID: String
    var originalItem: ItemDetail?
    /// The category of a new item; an existing item keeps its own.
    var newCategory: String = ItemDetail.categoryLogin
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
                    }
                }
                Section {
                    TextField("Title", text: $draft.title)
                        .accessibilityIdentifier("editor-title")
                }
                if draft.isSecureNote {
                    Section("Note") {
                        TextField("Recovery codes, door codes, account details…", text: $draft.notes, axis: .vertical)
                            .lineLimit(8...30)
                            .accessibilityIdentifier("editor-notes")
                    }
                    buildFieldSection()
                } else if !draft.isSSHKey {
                    Section("Login") {
                        TextField("Username or email", text: $draft.username)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-username")
                        HStack {
                            Group {
                                if isPasswordRevealed {
                                    TextField("Password", text: $draft.password)
                                        .font(.system(.body, design: .monospaced))
                                } else {
                                    SecureField("Password", text: $draft.password, prompt: Text("Password").foregroundStyle(.themeSecondary))
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
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel(isPasswordRevealed ? "Hide password" : "Reveal password")
                            Button {
                                isGeneratorShown = true
                            } label: {
                                Image(systemName: "wand.and.stars")
                            }
                            .buttonStyle(.borderless)
                            .foregroundStyle(.themeSecondary)
                            .accessibilityLabel("Generate password")
                            .accessibilityIdentifier("editor-generate-password")
                        }
                    }
                    Section {
                        TextField("Setup key or otpauth:// link", text: $draft.oneTimePassword)
                            .font(.system(.body, design: .monospaced))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("editor-one-time-password")
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
                        }
                        .onDelete { allURLText.remove(atOffsets: $0) }
                        Button {
                            allURLText.append("")
                        } label: {
                            Label("Add website", systemImage: "plus")
                        }
                    }
                    buildFieldSection()
                }
                if !draft.isSecureNote {
                    Section("Notes") {
                        TextField("Notes", text: $draft.notes, axis: .vertical)
                            .lineLimit(3...10)
                            .accessibilityIdentifier("editor-notes")
                    }
                }
                if !isNew {
                    Section {
                        Button("Delete item", role: .destructive) {
                            isDeleteConfirmationShown = true
                        }
                        .frame(maxWidth: .infinity)
                        .accessibilityIdentifier("editor-delete")
                    }
                }
            }
            .navigationTitle(isNew ? (draft.isSecureNote ? "New secure note" : "New login") : "Edit item")
            .navigationBarTitleDisplayMode(.inline)
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
                PasswordGeneratorSheet { password in
                    draft.password = password
                    isPasswordRevealed = true
                }
                .environmentObject(model)
            }
        }
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
        } else {
            draft.category = newCategory
        }
    }

    private func buildFieldSection() -> some View {
        Section("Other fields") {
            ForEach(draft.allField.indices, id: \.self) { index in
                VStack(alignment: .leading, spacing: 8) {
                    TextField("Name", text: $draft.allField[index].name)
                        .font(.caption)
                    if draft.allField[index].concealed {
                        SecureField("Value", text: $draft.allField[index].value)
                    } else {
                        TextField("Value", text: $draft.allField[index].value)
                    }
                    Toggle("Secret", isOn: $draft.allField[index].concealed)
                        .font(.caption)
                        .foregroundStyle(.themeSecondary)
                }
            }
            .onDelete { draft.allField.remove(atOffsets: $0) }
            Button {
                draft.allField.append(ItemField(name: "", value: "", concealed: false))
            } label: {
                Label("Add field", systemImage: "plus")
            }
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
