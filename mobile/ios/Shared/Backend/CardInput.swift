import Foundation

/// How the editor formats a card's number and expiry as they are typed. It needs only Foundation, so the UI tests compile
/// it too and wait for exactly what the editor shows.
enum CardInput {
    /// Groups a card number as it is typed, the way core/item saves it: 4-6-5 for American Express, 4-6-4 for a 14-digit
    /// Diners Club number and fours otherwise. Text with other characters is left as typed.
    static func formatNumber(_ value: String) -> String {
        guard value.allSatisfy({ $0.isASCII && ($0.isNumber || $0 == " " || $0 == "-") }) else { return value }
        let digits = Array(value.filter(\.isNumber))
        var allGroupSize = [4]
        if digits.starts(with: ["3", "4"]) || digits.starts(with: ["3", "7"]), digits.count <= 15 {
            allGroupSize = [4, 6, 5]
        } else if String(digits).range(of: "^3(0[0-5]|[689])", options: .regularExpression) != nil, digits.count <= 14 {
            allGroupSize = [4, 6, 4]
        }
        var allGroup: [String] = []
        var start = 0
        while start < digits.count {
            let size = allGroupSize[min(allGroup.count, allGroupSize.count - 1)]
            allGroup.append(String(digits[start..<min(start + size, digits.count)]))
            start += size
        }
        return allGroup.joined(separator: " ")
    }

    /// Writes an expiry as MM/YY while it is typed: "1230" becomes "12/30", "3" becomes "03" and "1/" becomes "01/". A pasted
    /// 12/2030 or 2030-12 becomes 12/30, and text with other characters is left as typed.
    static func formatExpiry(_ value: String) -> String {
        guard value.allSatisfy({ $0.isASCII && ($0.isNumber || " /.-".contains($0)) }) else { return value }
        let trimmed = value.trimmingCharacters(in: .whitespaces)
        if let match = trimmed.wholeMatch(of: /(\d{4})\s*[\/.\-]\s*(\d{1,2})/) {
            return "\(pad(match.2))/\(match.1.suffix(2))"
        }
        if let match = trimmed.wholeMatch(of: /(\d{1,2})\s*[\/.\-]\s*(\d*)/) {
            let year = match.2.count == 4 ? match.2.suffix(2) : match.2.prefix(2)
            return "\(pad(match.1))/\(year)"
        }
        var digits = value.filter(\.isNumber)
        if digits.range(of: "^([2-9]|1[3-9])", options: .regularExpression) != nil {
            digits = "0" + digits
        }
        guard digits.count > 2 else { return digits }
        let year = digits.count == 6 ? digits.suffix(2) : digits.dropFirst(2).prefix(2)
        return "\(digits.prefix(2))/\(year)"
    }

    private static func pad(_ month: Substring) -> String {
        month.count == 1 ? "0" + month : String(month)
    }
}
