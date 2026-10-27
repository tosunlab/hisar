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
    }
}
