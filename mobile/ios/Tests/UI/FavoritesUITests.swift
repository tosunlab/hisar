import XCTest

final class FavoritesUITests: HisarUITestCase {
    func testStarredItemsShowAtTheTopOfVaults() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_FAVORITES_INVITE_LINK")
        app.launch()
        signUp(inviteLink: inviteLink, email: "ios-favorites@example.com", name: "iOS Favorites")
        let favoritesHeader = app.staticTexts["favorites-header"]
        waitFor(app.buttons["vault-Personal"])
        XCTAssertFalse(favoritesHeader.exists, "Vaults shows Favorites before anything is starred")
        saveScreenshot("favorites-empty")

        addLogin(title: "GitHub", username: "octocat", password: "favorite-github-password", website: "https://github.com")
        choose("add-login", from: "add-item")
        type("AWS Console", into: app.textFields["editor-title"])
        type("root", into: app.textFields["editor-username"])
        type("favorite-aws-password", into: app.secureTextFields["editor-password"])
        // A preset icon in a color, chosen before saving.
        app.buttons["editor-icon"].tap()
        app.buttons["icon-color-orange"].tap()
        app.buttons["icon-preset-cloud"].tap()
        let isCloudSelected = XCTNSPredicateExpectation(predicate: NSPredicate(format: "isSelected == true"), object: app.buttons["icon-preset-cloud"])
        XCTAssertEqual(XCTWaiter().wait(for: [isCloudSelected], timeout: 10), .completed, "the cloud preset is selected")
        XCTAssertTrue(app.buttons["icon-color-orange"].isSelected, "the orange color stays chosen")
        saveScreenshot("item-icon-picker")
        app.buttons["icon-done"].tap()
        app.buttons["editor-save"].tap()
        waitFor(app.buttons["item-AWS Console"])

        // The editor sheet is still closing over the list.
        waitForHittable(app.buttons["item-GitHub"])
        app.buttons["item-GitHub"].tap()
        let star = app.buttons["favorite-item"]
        waitForHittable(star)
        star.tap()
        waitFor(app.staticTexts["Added to Favorites."], timeout: 10)
        saveScreenshot("favorites-item-starred")
        goBack()

        revealSwipeActions(of: app.buttons["item-AWS Console"], .right)
        let favoriteAction = app.buttons["favorite-swipe-AWS Console"]
        waitForHittable(favoriteAction)
        favoriteAction.tap()
        goBack()

        let gitHubFavorite = app.buttons["favorite-GitHub"]
        waitFor(gitHubFavorite)
        waitFor(app.buttons["favorite-AWS Console"])
        XCTAssertTrue(favoritesHeader.exists, "Vaults does not show Favorites once items are starred")
        saveScreenshot("favorites-vaults")

        revealSwipeActions(of: gitHubFavorite, .left)
        let unfavoriteAction = app.buttons["unfavorite-GitHub"]
        waitForHittable(unfavoriteAction)
        unfavoriteAction.tap()
        XCTAssertTrue(gitHubFavorite.waitForNonExistence(timeout: 10))
        XCTAssertTrue(app.buttons["favorite-AWS Console"].exists)

        app.terminate()
        app.launchArguments = ["-HisarUITest", "-HisarBrowseFailure-favorite", "-HisarBrowseFailure-rename", "-HisarBrowseFailure-refresh"]
        app.launch()
        let password = app.secureTextFields["unlock-password"]
        if password.waitForExistence(timeout: 5) {
            type(Self.masterPassword, into: password)
            app.buttons["unlock-submit"].tap()
        }
        waitFor(app.buttons["vault-Personal"])
        app.buttons["vault-Personal"].tap()
        let aws = app.buttons["item-AWS Console"]
        revealSwipeActions(of: aws, .right)
        app.buttons["unfavorite-AWS Console"].tap()
        waitFor(app.alerts["Remove from Favorites"])
        XCTAssertTrue(app.alerts.staticTexts["This action couldn't reach the Hisar server. Try again."].exists)
        app.buttons["refusal-ok"].tap()
        revealSwipeActions(of: aws, .right)
        app.buttons["unfavorite-AWS Console"].tap()
        waitFor(app.staticTexts["Removed from Favorites."])
        XCTAssertTrue(aws.exists)

        app.swipeDown()
        waitFor(app.buttons["browse-refresh-retry"])
        XCTAssertTrue(aws.exists, "a failed refresh keeps the readable list")
        choose("rename-vault", from: "vault-actions")
        let renameField = app.textFields["rename-vault-name"]
        renameField.tap()
        renameField.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: (renameField.value as? String)?.count ?? 0) + "My passwords")
        app.buttons["rename-vault-confirm"].tap()
        waitFor(app.alerts["Rename vault"])
        app.buttons["refusal-ok"].tap()
        choose("rename-vault", from: "vault-actions")
        XCTAssertEqual(app.textFields["rename-vault-name"].value as? String, "My passwords")
        app.buttons["rename-vault-confirm"].tap()
        waitFor(app.navigationBars["My passwords"])
        XCTAssertTrue(app.buttons["browse-refresh-retry"].exists, "a successful rename does not erase the refresh failure")
        app.buttons["browse-refresh-retry"].tap()
        XCTAssertTrue(app.buttons["browse-refresh-retry"].waitForNonExistence(timeout: 30))
        XCTAssertTrue(aws.exists)
    }

    func testTagsKeepNamesAndCountsReadableAtAccessibilitySizes() {
        for size in ["UICTContentSizeCategoryL", "UICTContentSizeCategoryAccessibilityXXXL"] {
            app.launchArguments = ["-HisarUITest", "-HisarVaultManagementPreview", "tags", "-UIPreferredContentSizeCategoryName", size]
            app.launch()
            for name in ["finance", "personal", "design-review"] {
                let row = app.buttons["tag-\(name)"]
                waitFor(row)
                XCTAssertTrue(row.isHittable)
                if size == "UICTContentSizeCategoryAccessibilityXXXL" {
                    let count = name == "personal" ? "1 item" : name == "finance" ? "2 items" : "3 items"
                    XCTAssertTrue(row.label.contains(count), row.label)
                }
            }
            let finance = app.staticTexts["finance"]
            XCTAssertLessThan(finance.frame.height, 80, "finance stays on one line at the largest size")
            app.buttons["tag-personal"].tap()
            waitFor(app.navigationBars["personal"])
            app.terminate()
        }
    }
}
