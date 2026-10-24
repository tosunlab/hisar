// Reads and operates another app's windows through Accessibility and the keyboard, as a user would:
// scripts/test-agent-sign-in.sh uses it on the development Hisar's approval and Settings windows. It never reads a
// secure text field's value.
// Usage: ax tree <pid>
//        ax press|focus <pid> <selector>
//        ax type <pid> <text>
//        ax menu <pid> <selector> <offset>
// A selector is ";"-separated conditions on one element: window=, role=, name= (its title, description or value),
// after= (it comes after an element of that name) and index= (the n-th match, from 0); "~" in place of "=" means
// "contains". The tree is printed as JSON lines, one element each, depth first. Keys and text go to the app's key
// window; an open menu, such as a web view's <select>, takes them as it takes the user's.
import ApplicationServices
import AppKit
import Foundation

struct Element {
    let reference: AXUIElement
    let window: String
    let depth: Int
}

func readString(_ element: AXUIElement, _ attribute: String) -> String {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, attribute as CFString, &value) == .success, let value else {
        return ""
    }
    if let text = value as? String {
        return text
    }
    if let number = value as? NSNumber {
        return number.stringValue
    }
    return ""
}

func readBool(_ element: AXUIElement, _ attribute: String) -> Bool {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, attribute as CFString, &value) == .success else {
        return false
    }
    return (value as? Bool) ?? false
}

func readChildren(_ element: AXUIElement, _ attribute: String = kAXChildrenAttribute) -> [AXUIElement] {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, attribute as CFString, &value) == .success else {
        return []
    }
    return (value as? [AXUIElement]) ?? []
}

func isSecure(_ element: AXUIElement) -> Bool {
    readString(element, kAXSubroleAttribute) == "AXSecureTextField"
}

func getNames(_ element: AXUIElement) -> [String] {
    var names = [readString(element, kAXTitleAttribute), readString(element, kAXDescriptionAttribute)]
    if !isSecure(element) {
        names.append(readString(element, kAXValueAttribute))
    }
    return names.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
}

// walk reads every window's elements. WebKit builds a web view's elements only once something asks for them, so a
// window without its web area yet is read again, for up to two seconds.
func walk(pid: pid_t) -> [Element] {
    var elements: [Element] = []
    for _ in 0..<10 {
        elements = walkOnce(pid: pid)
        let windows = Set(elements.filter { $0.depth == 0 }.map(\.window))
        let windowsWithWebArea = Set(elements.filter { readString($0.reference, kAXRoleAttribute) == "AXWebArea" }.map(\.window))
        if windows.isSubset(of: windowsWithWebArea) {
            break
        }
        usleep(200_000)
    }
    return elements
}

func walkOnce(pid: pid_t) -> [Element] {
    let application = AXUIElementCreateApplication(pid)
    AXUIElementSetMessagingTimeout(application, 3)
    var elements: [Element] = []
    func visit(_ element: AXUIElement, window: String, depth: Int) {
        guard depth < 60, elements.count < 5000 else {
            return
        }
        elements.append(Element(reference: element, window: window, depth: depth))
        for child in readChildren(element) {
            visit(child, window: window, depth: depth + 1)
        }
    }
    for window in readChildren(application, kAXWindowsAttribute) {
        visit(window, window: readString(window, kAXTitleAttribute), depth: 0)
    }
    return elements
}

struct Condition {
    let key: String
    let wanted: String
    let isContains: Bool

    init(_ text: Substring) {
        guard let separator = text.firstIndex(where: { $0 == "=" || $0 == "~" }) else {
            fail("a condition is key=value or key~value: \(text)")
        }
        key = String(text[..<separator])
        isContains = text[separator] == "~"
        wanted = String(text[text.index(after: separator)...])
    }

    func compare(_ candidate: String) -> Bool {
        isContains ? candidate.contains(wanted) : candidate == wanted
    }

    func matches(_ element: Element) -> Bool {
        switch key {
        case "window":
            return compare(element.window)
        case "role":
            return compare(readString(element.reference, kAXRoleAttribute))
        case "name":
            return getNames(element.reference).contains(where: compare)
        case "after", "index":
            return true
        default:
            fail("unknown selector key \(key)")
        }
    }
}

func find(pid: pid_t, selector: String) -> AXUIElement {
    let conditions = selector.split(separator: ";").map(Condition.init)
    var elements = walk(pid: pid)
    if let after = conditions.first(where: { $0.key == "after" }) {
        guard let anchor = elements.firstIndex(where: { getNames($0.reference).contains(where: after.compare) }) else {
            fail("no element is named \(after.wanted)")
        }
        elements = Array(elements[(anchor + 1)...])
    }
    let found = elements.filter { element in conditions.allSatisfy { $0.matches(element) } }
    let index = conditions.first(where: { $0.key == "index" }).flatMap { Int($0.wanted) } ?? 0
    guard index < found.count else {
        fail("no element matches \(selector) (\(found.count) found)")
    }
    return found[index].reference
}

func describe(_ element: Element) -> [String: Any] {
    let reference = element.reference
    return [
        "role": readString(reference, kAXRoleAttribute),
        "subrole": readString(reference, kAXSubroleAttribute),
        "title": readString(reference, kAXTitleAttribute),
        "description": readString(reference, kAXDescriptionAttribute),
        "value": isSecure(reference) ? "" : readString(reference, kAXValueAttribute),
        "enabled": readBool(reference, kAXEnabledAttribute),
        "focused": readBool(reference, kAXFocusedAttribute),
        "window": element.window,
        "depth": element.depth,
    ]
}

func fail(_ message: String) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(1)
}

func check(_ result: AXError, _ action: String) {
    if result != .success {
        fail("\(action) failed: AXError \(result.rawValue)")
    }
}

let keySource = CGEventSource(stateID: .hidSystemState)

func post(pid: pid_t, keyCode: CGKeyCode, text: String? = nil) {
    for isDown in [true, false] {
        guard let event = CGEvent(keyboardEventSource: keySource, virtualKey: keyCode, keyDown: isDown) else {
            fail("could not create a key event")
        }
        if let text {
            let units = Array(text.utf16)
            event.keyboardSetUnicodeString(stringLength: units.count, unicodeString: units)
        }
        event.postToPid(pid)
        usleep(8000)
    }
}

func focus(pid: pid_t, selector: String) {
    let application = AXUIElementCreateApplication(pid)
    check(AXUIElementSetAttributeValue(application, kAXFrontmostAttribute as CFString, kCFBooleanTrue), "front owned development app")
    let deadline = Date().addingTimeInterval(2)
    while NSWorkspace.shared.frontmostApplication?.processIdentifier != pid {
        guard Date() < deadline else {
            let front = NSWorkspace.shared.frontmostApplication
            fail("owned development app did not become foreground; target=\(pid) front=\(front?.processIdentifier ?? 0) bundle=\(front?.bundleIdentifier ?? "")")
        }
        RunLoop.current.run(until: Date().addingTimeInterval(0.02))
    }
    usleep(200_000)
    let element = find(pid: pid, selector: selector)
    check(AXUIElementSetAttributeValue(element, kAXFocusedAttribute as CFString, kCFBooleanTrue), "focus element")
    usleep(200_000)
}

let allArgument = CommandLine.arguments
let usage = "usage: ax tree <pid> | press|focus <pid> <selector> | menu <pid> <selector> <offset> | type <pid> <text>"
guard allArgument.count >= 3, let pid = pid_t(allArgument[2]) else {
    fail(usage)
}
guard AXIsProcessTrusted() else {
    fail("this process has no Accessibility permission")
}
switch (allArgument[1], allArgument.count) {
case ("tree", 3):
    for element in walk(pid: pid) {
        let data = try! JSONSerialization.data(withJSONObject: describe(element), options: [.sortedKeys])
        print(String(data: data, encoding: .utf8)!)
    }
case ("press", 4):
    check(AXUIElementPerformAction(find(pid: pid, selector: allArgument[3]), kAXPressAction as CFString), "AXPress")
case ("menu", 5):
    guard let offset = Int(allArgument[4]), (-99...99).contains(offset) else {
        fail("menu offset must be between -99 and 99")
    }
    focus(pid: pid, selector: allArgument[3])
    post(pid: pid, keyCode: 125)
    usleep(300_000)
    for _ in 0..<abs(offset) {
        post(pid: pid, keyCode: offset > 0 ? 125 : 126)
        usleep(150_000)
    }
    post(pid: pid, keyCode: 36)
    usleep(300_000)
    print(readString(find(pid: pid, selector: allArgument[3]), kAXValueAttribute))
case ("focus", 4):
    focus(pid: pid, selector: allArgument[3])
case ("type", 4):
    for character in allArgument[3] {
        post(pid: pid, keyCode: 0, text: String(character))
    }
default:
    fail(usage)
}
