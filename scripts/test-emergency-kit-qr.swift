import AppKit
import Foundation
import PDFKit
import Vision

let pdfURL = URL(fileURLWithPath: CommandLine.arguments[1])
guard let document = PDFDocument(url: pdfURL), document.pageCount == 1, let page = document.page(at: 0) else {
    fatalError("Emergency Kit is not a single-page PDF")
}
let image = page.thumbnail(of: NSSize(width: 1786, height: 2526), for: .mediaBox)
guard let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    fatalError("PDF did not render")
}
let request = VNDetectBarcodesRequest()
request.symbologies = [.qr]
try VNImageRequestHandler(cgImage: cgImage).perform([request])
let results = request.results ?? []
guard results.count == 1, let payload = results.first?.payloadStringValue, payload.hasPrefix("HISARKIT1 ") else {
    fatalError("PDF did not contain exactly one plain-text Emergency Kit QR code")
}
let encoded = String(payload.dropFirst("HISARKIT1 ".count))
precondition(encoded.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil)
let base64 = encoded.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
let data = Data(base64Encoded: base64 + String(repeating: "=", count: (4 - base64.count % 4) % 4))!
let details = try JSONSerialization.jsonObject(with: data) as! [String: String]
precondition(details == ["server": "https://tryhisar.com", "email": "alper@tryhisar.com", "secretKey": "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"])
print("PDF render and Vision QR decode preserved all sign-in bytes. The payload has no URI scheme.")
