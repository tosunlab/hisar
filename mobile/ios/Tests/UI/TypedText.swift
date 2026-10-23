import Foundation

/// What a field must show once everything typed into it is in, compared with the field's accessibility value.
struct TypedText {
    /// The typing whose end state the field's value cannot show.
    enum UnshownError: Error, Equatable {
        /// An empty field reports its placeholder as its value, so typing the placeholder's own text could never be told
        /// apart from typing nothing.
        case placeholder
        /// A secure field shows only dots, so the text it already holds is unknown and cannot go through the formatter
        /// with what is typed.
        case secureFormattedNotEmpty
    }

    /// The text the field holds at the end, before `normalize`.
    let expected: String
    let isSecure: Bool
    /// The field's own formatter, such as CardInput.formatExpiry. While a field has the focus it can show what was typed
    /// or what the formatter made of it ("1229" or "12/29"), so both sides go through it.
    let normalize: ((String) -> String)?

    /// The field's end state: `showing` when the typing also deletes, else what it held before followed by `typed`.
    init(typed: String, showing: String? = nil, before value: String?, placeholder: String?, isSecure: Bool, normalize: ((String) -> String)? = nil) throws {
        let entered = Self.readEntered(value, placeholder: placeholder)
        if isSecure, normalize != nil, showing == nil, !entered.isEmpty {
            throw UnshownError.secureFormattedNotEmpty
        }
        let expected = showing ?? entered + typed
        if let placeholder, !expected.isEmpty, expected == placeholder || normalize?(expected) == placeholder {
            throw UnshownError.placeholder
        }
        self.expected = expected
        self.isSecure = isSecure
        self.normalize = normalize
    }

    /// A secure field shows one dot per character it holds, the spaces of a formatted card number included; any other field
    /// shows its text.
    func isShown(byValue value: String?, placeholder: String?) -> Bool {
        let entered = Self.readEntered(value, placeholder: placeholder)
        let normalizedExpected = normalize?(expected) ?? expected
        guard isSecure else { return (normalize?(entered) ?? entered) == normalizedExpected }
        return entered.count == normalizedExpected.count && entered.allSatisfy { $0 == "•" }
    }

    static func readEntered(_ value: String?, placeholder: String?) -> String {
        guard let value, value != placeholder else { return "" }
        return value
    }
}
