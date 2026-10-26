import XCTest

// The open iPhone Duo is regular in both size classes, and Vaults, Search and Settings keep their list beside what it opens;
// a compact iPhone, and the closed Duo, push the opened screen over the list. The same steps run on both, and the window's
// width says which layout to expect. On the compact simulator CI uses, this covers the collapsed split views; for the open
// Duo: IOS_SIMULATOR_ID=<its UDID> IOS_UI_TESTS=HisarUITests/WideLayoutUITests make test-ios-ui
final class WideLayoutUITests: HisarUITestCase {
    func testListsStayBesideWhatTheyOpenOnAWideScreen() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_WIDE_LAYOUT_INVITE_LINK")
        app.launch()
        signUp(inviteLink: inviteLink, email: "ios-wide-layout@example.com", name: "iOS Wide Layout")
        // Every compact iPhone is under 450 points wide; the open Duo is over 600 in portrait.
        let isWide = app.windows.firstMatch.frame.width >= 500
        let layout = isWide ? "wide" : "compact"

        addLogin(title: "GitHub", username: "octocat", password: "wide-layout-password", website: "https://github.com")
        XCTAssertEqual(app.buttons["vault-Personal"].isHittable, isWide, "the vault list beside the Personal vault's items, \(layout)")
        waitForHittable(app.buttons["item-GitHub"])
        app.buttons["item-GitHub"].tap()
        waitFor(app.staticTexts["password-value"])
        XCTAssertEqual(app.buttons["vault-Personal"].isHittable, isWide, "the vault list beside an item, \(layout)")
        saveScreenshot("\(layout)-item-detail")
        if isWide {
            // Another row of the list replaces the whole detail column.
            app.buttons["category-logins"].tap()
            waitFor(app.navigationBars["Logins"])
            XCTAssertFalse(app.staticTexts["password-value"].exists, "the item left the detail column for the Logins list")
            waitForHittable(app.buttons["item-GitHub"])
        } else {
            goBack()
            goBack()
        }

        openTab("Search")
        type("git", into: app.searchFields.firstMatch)
        waitForHittable(app.buttons["item-GitHub"])
        app.buttons["item-GitHub"].tap()
        waitFor(app.staticTexts["password-value"])
        XCTAssertEqual(app.searchFields.firstMatch.isHittable, isWide, "the search field beside a result, \(layout)")
        saveScreenshot("\(layout)-search")
        if !isWide {
            goBack()
        }
        // The search stays active after the result closes; its Close (or Cancel) button comes back with the field. In the
        // Search tab the tab bar is a pill with the Vaults tab alone, so Settings is reached from Vaults.
        let closeSearchButton = app.buttons["Close"].waitForExistence(timeout: 5) ? app.buttons["Close"] : app.buttons["Cancel"]
        waitForHittable(closeSearchButton.firstMatch)
        closeSearchButton.firstMatch.tap()
        openTab("Vaults")

        openSettings("open-security")
        waitFor(app.navigationBars["Security"])
        XCTAssertEqual(app.buttons["open-account"].isHittable, isWide, "the Settings list beside Security, \(layout)")
        saveScreenshot("\(layout)-settings")
    }
}
