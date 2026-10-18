import XCTest

final class FavoritesUITests: HisarUITestCase {
    func testStarredItemsShowAtTheTopOfVaults() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_FAVORITES_INVITE_LINK")
        app.launch()
        signUp(inviteLink: inviteLink, email: "ios-favorites@example.com", name: "iOS Favorites")
        waitFor(app.staticTexts["Favorites"])
        let emptyNote = app.staticTexts["Star an item to keep it here, on all your devices. Only you see your favorites."]
        waitFor(emptyNote)
        saveScreenshot("favorites-empty")

        addLogin(title: "GitHub", username: "octocat", password: "favorite-github-password", website: "https://github.com")
        dismissSavePasswordPrompt()
        waitFor(app.buttons["add-item"])
        app.buttons["add-item"].tap()
        waitFor(app.buttons["add-login"])
        app.buttons["add-login"].tap()
        type("AWS Console", into: app.textFields["editor-title"])
        type("root", into: app.textFields["editor-username"])
        type("favorite-aws-password", into: app.secureTextFields["editor-password"])
        // A preset icon in a color, chosen before saving.
        app.buttons["editor-icon"].tap()
        waitFor(app.buttons["icon-preset-cloud"])
        app.buttons["icon-color-orange"].tap()
        app.buttons["icon-preset-cloud"].tap()
        XCTAssertTrue(app.buttons["icon-preset-cloud"].isSelected)
        saveScreenshot("item-icon-picker")
        app.buttons["icon-done"].tap()
        app.buttons["editor-save"].tap()
        dismissSavePasswordPrompt()
        waitFor(app.buttons["item-AWS Console"])

        app.buttons["item-GitHub"].tap()
        let star = app.buttons["favorite-item"]
        waitFor(star)
        XCTAssertEqual(star.label, "Add to Favorites")
        star.tap()
        waitFor(app.staticTexts["Added to Favorites."], timeout: 10)
        XCTAssertEqual(star.label, "Remove from Favorites")
        saveScreenshot("favorites-item-starred")
        goBack()

        let awsRow = app.buttons["item-AWS Console"]
        waitFor(awsRow)
        awsRow.swipeRight()
        let swipeStar = app.buttons["favorite-swipe-AWS Console"]
        waitFor(swipeStar)
        swipeStar.tap()
        goBack()

        let gitHubFavorite = app.buttons["favorite-GitHub"]
        waitFor(gitHubFavorite)
        waitFor(app.buttons["favorite-AWS Console"])
        XCTAssertFalse(emptyNote.exists)
        XCTAssertTrue(gitHubFavorite.label.contains("Personal"), gitHubFavorite.label)
        saveScreenshot("favorites-vaults")

        gitHubFavorite.swipeLeft()
        let unstar = app.buttons["unfavorite-GitHub"]
        waitFor(unstar)
        unstar.tap()
        XCTAssertTrue(gitHubFavorite.waitForNonExistence(timeout: 10))
        waitFor(app.buttons["favorite-AWS Console"])
    }
}
