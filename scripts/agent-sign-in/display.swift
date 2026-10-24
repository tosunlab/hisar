import CoreGraphics
import Foundation

struct Frame: Codable {
    let x: Double
    let y: Double
    let w: Double
    let h: Double
}
struct Display: Decodable {
    let id: UInt32
    let index: Int
    let frame: Frame
}
struct Space: Decodable {
    let index: Int
    let display: Int
    let visible: Bool
    let fullscreen: Bool
    enum CodingKeys: String, CodingKey {
        case index, display
        case visible = "is-visible"
        case fullscreen = "is-native-fullscreen"
    }
}
struct Layout: Codable {
    let display: Int
    let visibleSpace: Int
    let otherSpace: Int
    let builtInSpaces: [Int]
    let frame: Frame
}
func fail(_ message: String) -> Never {
    fputs("DISPLAY PREFLIGHT FAIL \(message)\n", stderr)
    exit(1)
}
func query(_ executable: String, _ arguments: [String]) throws -> Data {
    let process = Process()
    process.executableURL = URL(fileURLWithPath: executable)
    process.arguments = arguments
    let pipe = Pipe()
    process.standardOutput = pipe
    process.standardError = FileHandle.standardError
    try process.run()
    let deadline = DispatchWorkItem {
        if process.isRunning {
            process.terminate()
            kill(process.processIdentifier, SIGKILL)
        }
    }
    DispatchQueue.global().asyncAfter(deadline: .now() + 2, execute: deadline)
    defer { deadline.cancel() }
    let output = pipe.fileHandleForReading.readDataToEndOfFile()
    process.waitUntilExit()
    guard process.terminationStatus == 0 else { throw NSError(domain: executable, code: Int(process.terminationStatus)) }
    return output
}
let arguments = CommandLine.arguments
let yabai = ProcessInfo.processInfo.environment["HISAR_TEST_YABAI"] ?? "/opt/homebrew/bin/yabai"
guard arguments.count == 2, arguments[1] == "layout" else { fail("usage: display layout") }
do {
    let displays = try JSONDecoder().decode([Display].self, from: query(yabai, ["-m", "query", "--displays"]))
    let spaces = try JSONDecoder().decode([Space].self, from: query(yabai, ["-m", "query", "--spaces"]))
    guard displays.count == 1, let builtIn = displays.first, CGDisplayIsBuiltin(builtIn.id) != 0,
          let visible = spaces.first(where: { $0.display == builtIn.index && $0.visible && !$0.fullscreen }),
          let other = spaces.first(where: { $0.display == builtIn.index && $0.index != visible.index && !$0.fullscreen }) else {
        fail("run on the built-in display alone, with a visible Space and another non-fullscreen Space")
    }
    let layout = Layout(display: builtIn.index, visibleSpace: visible.index, otherSpace: other.index,
                        builtInSpaces: spaces.filter { $0.display == builtIn.index }.map(\.index), frame: builtIn.frame)
    let data = try JSONEncoder().encode(layout)
    print(String(data: data, encoding: .utf8)!)
} catch { fail("\(error)") }
