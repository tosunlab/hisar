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
        waitFor(app.buttons["item-GitHub"])
        XCTAssertEqual(app.buttons["item-GitHub"].value as? String, "In Favorites")
        app.buttons["item-GitHub"].press(forDuration: 1)
        app.buttons["row-copy-actions"].tap()
        waitFor(app.buttons["row-copy-password"])
        XCTAssertFalse(app.buttons["edit-item"].exists)
        app.buttons["Cancel"].tap()
        app.buttons["item-GitHub"].press(forDuration: 1)
        app.buttons["row-copy-actions"].tap()
        waitFor(app.buttons["row-copy-username"])
        app.buttons["row-copy-username"].tap()
        waitFor(app.staticTexts["Username copied. It clears in 90 seconds."])

        revealSwipeActions(of: app.buttons["item-AWS Console"], .right)
        let favoriteAction = app.buttons["favorite-swipe-AWS Console"]
        waitForHittable(favoriteAction)
        favoriteAction.tap()
        goBack()

        let gitHubFavorite = app.buttons["favorite-GitHub"]
        waitFor(gitHubFavorite)
        waitFor(app.buttons["favorite-AWS Console"])
        XCTAssertNotEqual(gitHubFavorite.value as? String, "In Favorites")
        XCTAssertTrue(favoritesHeader.exists, "Vaults does not show Favorites once items are starred")
        saveScreenshot("favorites-vaults")

        revealSwipeActions(of: gitHubFavorite, .left)
        let unfavoriteAction = app.buttons["unfavorite-GitHub"]
        waitForHittable(unfavoriteAction)
        unfavoriteAction.tap()
        XCTAssertTrue(gitHubFavorite.waitForNonExistence(timeout: 10))
        XCTAssertTrue(app.buttons["favorite-AWS Console"].exists)

        let title = "Adobe Creative Cloud personal account"
        let email = "alex.morgan+personal@example.com"
        addLogin(title: title, username: email, password: "large-type-password", website: "https://adobe.com")
        app.terminate()
        app.launchArguments = ["-HisarUITest", "-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL"]
        app.launch()
        type(Self.masterPassword, into: app.secureTextFields["unlock-password"])
        app.buttons["unlock-submit"].tap()
        waitForHittable(app.buttons["vault-Personal"])
        app.buttons["vault-Personal"].tap()
        let row = app.buttons["item-\(title)"]
        scrollTo(row)
        XCTAssertTrue(row.label.contains(title))
        XCTAssertTrue(row.label.contains(email))
        XCTAssertGreaterThan(row.frame.height, 150, "the title and email wrap at the largest accessibility text size")
        saveScreenshot("largest-text-item-row")
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
        app.alerts.buttons["refusal-ok"].firstMatch.tap()
        revealSwipeActions(of: aws, .right)
        app.buttons["unfavorite-AWS Console"].tap()
        waitFor(app.staticTexts["Removed from Favorites."])
        XCTAssertTrue(aws.exists)

        let list = app.collectionViews.firstMatch
        list.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35)).press(
            forDuration: 0.05, thenDragTo: list.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.8)),
            withVelocity: .slow, thenHoldForDuration: 1)
        waitFor(app.buttons["browse-refresh-retry"])
        XCTAssertTrue(aws.exists, "a failed refresh keeps the readable list")
        choose("rename-vault", from: "vault-actions")
        let renameField = app.alerts["Rename vault"].textFields["Vault name"]
        renameField.tap()
        renameField.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: (renameField.value as? String)?.count ?? 0) + "My passwords")
        app.alerts["Rename vault"].buttons["rename-vault-confirm"].firstMatch.tap()
        waitFor(app.alerts["Rename vault"])
        app.alerts.buttons["refusal-ok"].firstMatch.tap()
        choose("rename-vault", from: "vault-actions")
        XCTAssertEqual(renameField.value as? String, "My passwords")
        app.alerts["Rename vault"].buttons["rename-vault-confirm"].firstMatch.tap()
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
