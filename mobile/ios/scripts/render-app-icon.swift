// Renders the iOS app icon (full-bleed #f2f3f5 square with the tower of assets/logo/hisar-mark.svg) to the path given as the first argument.
import AppKit
import CoreGraphics

let size = 1024
let scale = CGFloat(size) / 32
let context = CGContext(data: nil, width: size, height: size, bitsPerComponent: 8, bytesPerRow: 0, space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
context.setFillColor(CGColor(srgbRed: 0xF2 / 255, green: 0xF3 / 255, blue: 0xF5 / 255, alpha: 1))
context.fill(CGRect(x: 0, y: 0, width: size, height: size))
context.translateBy(x: 0, y: CGFloat(size))
context.scaleBy(x: scale, y: -scale)
let path = CGMutablePath()
path.move(to: CGPoint(x: 8, y: 26))
for point in [(8.0, 12.0), (11, 12), (11, 9), (13.5, 9), (13.5, 12), (18.5, 12), (18.5, 9), (21, 9), (21, 12), (24, 12), (24, 26), (18, 26), (18, 21)] {
    path.addLine(to: CGPoint(x: point.0, y: point.1))
}
path.addArc(center: CGPoint(x: 16, y: 21), radius: 2, startAngle: 0, endAngle: .pi, clockwise: true)
path.addLine(to: CGPoint(x: 14, y: 26))
path.closeSubpath()
context.addPath(path)
context.setFillColor(CGColor(srgbRed: 0x0B / 255, green: 0x0C / 255, blue: 0x0E / 255, alpha: 1))
context.fillPath()
let image = context.makeImage()!
let data = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:])!
try! data.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
