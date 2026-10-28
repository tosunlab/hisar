import UIKit
import XCTest
@testable import Hisar

@MainActor
final class CredentialFieldTests: XCTestCase {
    func testCredentialInputPreservesSelectionAndFocus() {
        let window = UIWindow(frame: UIScreen.main.bounds)
        let controller = UIViewController()
        window.rootViewController = controller
        let field = UITextField(frame: CGRect(x: 10, y: 10, width: 300, height: 44))
        controller.view.addSubview(field)
        window.makeKeyAndVisible()
        field.isSecureTextEntry = true
        field.text = "long master password"
        field.textContentType = .password
        field.returnKeyType = .go
        field.becomeFirstResponder()
        field.selectedTextRange = field.textRange(from: field.position(from: field.beginningOfDocument, offset: 2)!, to: field.position(from: field.beginningOfDocument, offset: 7)!)
        for isRevealed in [true, false] {
            CredentialField.setRevealed(isRevealed, on: field)
            XCTAssertEqual(field.isSecureTextEntry, !isRevealed)
            XCTAssertEqual(field.text, "long master password")
            XCTAssertTrue(field.isFirstResponder)
            XCTAssertEqual(field.offset(from: field.beginningOfDocument, to: field.selectedTextRange!.start), 2)
            XCTAssertEqual(field.offset(from: field.beginningOfDocument, to: field.selectedTextRange!.end), 7)
            XCTAssertEqual(field.textContentType, .password)
            XCTAssertEqual(field.returnKeyType, .go)
        }
        field.resignFirstResponder()
        window.isHidden = true
    }
}
