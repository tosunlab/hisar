// Prints one JSON line per image with its path and Vision-recognized text.
// Usage: ocr <image> ...
import Foundation
import Vision

func recognize(_ path: String) -> String {
    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    request.usesLanguageCorrection = false
    let handler = VNImageRequestHandler(url: URL(fileURLWithPath: path))
    do {
        try handler.perform([request])
    } catch {
        FileHandle.standardError.write("\(path): \(error)\n".data(using: .utf8)!)
        exit(1)
    }
    return (request.results ?? []).compactMap { $0.topCandidates(1).first?.string }.joined(separator: "\n")
}

for path in CommandLine.arguments.dropFirst() {
    let line = try! JSONSerialization.data(withJSONObject: ["path": path, "text": recognize(path)], options: [.sortedKeys])
    print(String(data: line, encoding: .utf8)!)
}
