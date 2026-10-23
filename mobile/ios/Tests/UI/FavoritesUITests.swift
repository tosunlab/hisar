import XCTest

final class FavoritesUITests: HisarUITestCase {
    func testStarredItemsShowAtTheTopOfVaults() throws {
        let inviteLink = try readEnvironment("HISAR_UI_TEST_FAVORITES_INVITE_LINK")
        app.launch()
        signUp(inviteLink: inviteLink, email: "ios-favorites@example.com", name: "iOS Favorites")
        let emptyNote = app.staticTexts["Star an item to keep it here, on all your devices. Only you see your favorites."]
        waitFor(emptyNote)
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
        let cloudPreset = app.buttons["icon-preset-cloud"]
        XCTAssertTrue(waitUntil(timeout: 10) { cloudPreset.isSelected }, "the cloud preset is selected")
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

        app.buttons["item-AWS Console"].swipeRight()
        app.buttons["favorite-swipe-AWS Console"].tap()
        goBack()

        let gitHubFavorite = app.buttons["favorite-GitHub"]
        waitFor(gitHubFavorite)
        waitFor(app.buttons["favorite-AWS Console"])
        XCTAssertFalse(emptyNote.exists)
        saveScreenshot("favorites-vaults")

        gitHubFavorite.swipeLeft()
        app.buttons["unfavorite-GitHub"].tap()
        XCTAssertTrue(gitHubFavorite.waitForNonExistence(timeout: 10))
        XCTAssertTrue(app.buttons["favorite-AWS Console"].exists)
    }
}
