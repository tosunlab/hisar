// Rewrites a PNG without an alpha channel (iOS app icons must be opaque).
// Usage: swift scripts/flatten-png.swift <input.png> <output.png>
import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let allArgument = CommandLine.arguments
guard allArgument.count == 3,
      let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: allArgument[1]) as CFURL, nil),
      let image = CGImageSourceCreateImageAtIndex(source, 0, nil),
      let context = CGContext(data: nil, width: image.width, height: image.height, bitsPerComponent: 8, bytesPerRow: 0,
                              space: CGColorSpace(name: CGColorSpace.sRGB)!,
                              bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
else {
    FileHandle.standardError.write("usage: flatten-png.swift <input.png> <output.png>\n".data(using: .utf8)!)
    exit(1)
}
context.draw(image, in: CGRect(x: 0, y: 0, width: image.width, height: image.height))
guard let flattenedImage = context.makeImage(),
      let destination = CGImageDestinationCreateWithURL(URL(fileURLWithPath: allArgument[2]) as CFURL, UTType.png.identifier as CFString, 1, nil)
else {
    exit(1)
}
CGImageDestinationAddImage(destination, flattenedImage, nil)
exit(CGImageDestinationFinalize(destination) ? 0 : 1)
