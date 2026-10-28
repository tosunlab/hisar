import SwiftUI
import XCTest
@testable import Hisar

@MainActor
final class ImportFileNameLayoutTests: XCTestCase {
    private let fileName = "Hisar export 2026-10-27 production credentials before Secret Key replacement.encrypted.json"

    private func checkWrapping(_ size: DynamicTypeSize) {
        let long = UIHostingController(rootView: ImportFileNameRow(fileName: fileName).dynamicTypeSize(size))
        let short = UIHostingController(rootView: ImportFileNameRow(fileName: "export.json").dynamicTypeSize(size))
        let constraint = CGSize(width: 288, height: CGFloat.greatestFiniteMagnitude)
        let longSize = long.sizeThatFits(in: constraint)
        let shortSize = short.sizeThatFits(in: constraint)
        XCTAssertGreaterThan(longSize.height, shortSize.height, "the whole dated filename wraps instead of occupying a single truncated line")
        XCTAssertLessThanOrEqual(longSize.width, constraint.width)
    }

    func testImportFileNameWrapsAtStandardTextSize() { checkWrapping(.large) }
    func testImportFileNameWrapsAtLargeTextSize() { checkWrapping(.accessibility3) }
}
