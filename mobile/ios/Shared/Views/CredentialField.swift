import SwiftUI
import UIKit

struct CredentialField: View {
    private var title: String
    @Binding private var text: String
    private var prompt: String?
    private var secretName: String
    private var contentType: UITextContentType = .password
    private var returnKey: UIReturnKeyType = .default
    private var fieldID = ""
    private var focus: FocusState<Bool>.Binding?
    private var submit: (() -> Void)?
    private var shouldConceal = false
    @Environment(\.scenePhase) private var scenePhase
    @State private var isRevealed = false

    init(_ title: String, text: Binding<String>, prompt: String? = nil, secretName: String = "master password") {
        self.title = title
        _text = text
        self.prompt = prompt
        self.secretName = secretName
    }

    var body: some View {
        HStack(spacing: 0) {
            CredentialTextInput(title: title, prompt: prompt, text: $text, isRevealed: isRevealed, contentType: contentType, returnKey: returnKey, fieldID: fieldID, focus: focus) {
                isRevealed = false
                submit?()
            }
            Button {
                isRevealed.toggle()
            } label: {
                Image(systemName: isRevealed ? "eye.slash" : "eye")
                    .frame(minWidth: 44, minHeight: 44)
            }
            .buttonStyle(.borderless)
            .foregroundStyle(.themeSecondary)
            .accessibilityLabel("\(isRevealed ? "Hide" : "Show") \(secretName)")
            .accessibilityIdentifier("\(fieldID)-reveal")
            .disabled(shouldConceal)
        }
        .onChange(of: shouldConceal) { _, _ in isRevealed = false }
        .onChange(of: text.isEmpty) { _, isEmpty in if isEmpty { isRevealed = false } }
        .onChange(of: scenePhase) { _, phase in if phase != .active { isRevealed = false } }
        .onDisappear { isRevealed = false }
    }

    func textContentType(_ type: UITextContentType) -> Self {
        var field = self
        field.contentType = type
        return field
    }

    func submitLabel(_ label: UIReturnKeyType) -> Self {
        var field = self
        field.returnKey = label
        return field
    }

    func focused(_ binding: FocusState<Bool>.Binding) -> Self {
        var field = self
        field.focus = binding
        return field
    }

    func onSubmit(_ action: @escaping () -> Void) -> Self {
        var field = self
        field.submit = action
        return field
    }

    func accessibilityIdentifier(_ identifier: String) -> Self {
        var field = self
        field.fieldID = identifier
        return field
    }

    func concealWhen(_ condition: Bool) -> Self {
        var field = self
        field.shouldConceal = condition
        return field
    }

    static func setRevealed(_ isRevealed: Bool, on field: UITextField) {
        guard field.isSecureTextEntry == isRevealed else { return }
        let text = field.text
        let selection = field.selectedTextRange.map {
            (field.offset(from: field.beginningOfDocument, to: $0.start), field.offset(from: field.beginningOfDocument, to: $0.end))
        }
        field.isSecureTextEntry = !isRevealed
        field.text = text
        if let selection,
           let start = field.position(from: field.beginningOfDocument, offset: selection.0),
           let end = field.position(from: field.beginningOfDocument, offset: selection.1) {
            field.selectedTextRange = field.textRange(from: start, to: end)
        }
    }
}

private struct CredentialTextInput: UIViewRepresentable {
    var title: String
    var prompt: String?
    @Binding var text: String
    var isRevealed: Bool
    var contentType: UITextContentType
    var returnKey: UIReturnKeyType
    var fieldID: String
    var focus: FocusState<Bool>.Binding?
    var submit: () -> Void

    func makeUIView(context: Context) -> UITextField {
        let field = UITextField()
        field.delegate = context.coordinator
        field.addTarget(context.coordinator, action: #selector(Coordinator.changed), for: .editingChanged)
        field.autocapitalizationType = .none
        field.autocorrectionType = .no
        field.spellCheckingType = .no
        field.smartQuotesType = .no
        field.smartDashesType = .no
        field.font = .preferredFont(forTextStyle: .body)
        field.adjustsFontForContentSizeCategory = true
        field.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        return field
    }

    func updateUIView(_ field: UITextField, context: Context) {
        context.coordinator.input = self
        if field.text != text { field.text = text }
        CredentialField.setRevealed(isRevealed, on: field)
        field.placeholder = prompt ?? title
        field.accessibilityLabel = title
        field.accessibilityIdentifier = fieldID
        field.textContentType = contentType
        field.returnKeyType = returnKey
        if let focus, focus.wrappedValue != field.isFirstResponder {
            if focus.wrappedValue {
                field.becomeFirstResponder()
            } else {
                field.resignFirstResponder()
            }
        }
    }

    func makeCoordinator() -> Coordinator { Coordinator(input: self) }

    final class Coordinator: NSObject, UITextFieldDelegate {
        var input: CredentialTextInput
        init(input: CredentialTextInput) { self.input = input }
        @objc func changed(_ field: UITextField) { input.text = field.text ?? "" }
        func textFieldDidBeginEditing(_ textField: UITextField) { input.focus?.wrappedValue = true }
        func textFieldDidEndEditing(_ textField: UITextField) { input.focus?.wrappedValue = false }
        func textFieldShouldReturn(_ textField: UITextField) -> Bool {
            input.submit()
            return false
        }
    }
}
