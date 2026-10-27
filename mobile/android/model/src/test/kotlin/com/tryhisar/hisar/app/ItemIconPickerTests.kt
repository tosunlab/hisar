package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ChosenIcon
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlinx.coroutines.test.runTest
import kotlinx.io.bytestring.ByteString
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** The icon an item keeps: its JSON, and photos turned into icons by the backend. */
class ItemIconPickerTests {
    @Test
    fun testIconsRoundTripThroughTheItemJSON() {
        val decoded = backendJson.decodeFromString<ItemDetail>("""{"title":"Bank","icon":{"preset":"bank","color":"green"}}""")
        assertEquals(ChosenIcon(preset = "bank", color = "green"), decoded.icon)
        val withImage = ItemDetail(title = "GitHub", icon = ChosenIcon(image = ByteString(1, 2, 3)))
        val encoded = backendJson.encodeToJsonElement(ItemDetail.serializer(), withImage).jsonObject
        assertEquals("AQID", encoded.getValue("icon").jsonObject.getValue("image").jsonPrimitive.content, "images cross as base64, as Go's []byte")
        assertNull(backendJson.decodeFromString<ItemDetail>("""{"title":"Old"}""").icon)
    }

    @Test
    fun testChoosingAColorShowsOnTheIcon() {
        assertEquals(ChosenIcon(preset = "key", color = "blue"), ChosenIcon.applyColor(null, "blue", ItemDetail.CATEGORY_LOGIN))
        assertNull(ChosenIcon.applyColor(null, "", ItemDetail.CATEGORY_LOGIN), "no color keeps the icon automatic")
        assertEquals(ChosenIcon(preset = "code", color = "red"), ChosenIcon.applyColor(ChosenIcon(preset = "code", color = "blue"), "red", ItemDetail.CATEGORY_LOGIN))
        assertEquals(ChosenIcon(preset = "code"), ChosenIcon.applyColor(ChosenIcon(preset = "code", color = "blue"), "", ItemDetail.CATEGORY_LOGIN))
        assertEquals(ChosenIcon(preset = "rocket", color = "red"), ChosenIcon.applyColor(ChosenIcon(preset = "rocket"), "red", ItemDetail.CATEGORY_LOGIN), "a newer app's preset keeps its ID")
        val photo = ChosenIcon(image = ByteString(1, 2, 3))
        assertEquals(photo, ChosenIcon.applyColor(photo, "red", ItemDetail.CATEGORY_LOGIN), "a photo has no color")
        for ((category, preset) in listOf(
            ItemDetail.CATEGORY_SERVER to "server",
            ItemDetail.CATEGORY_DATABASE to "database",
            ItemDetail.CATEGORY_SSH_KEY to "terminal",
            ItemDetail.CATEGORY_CREDIT_CARD to "card",
        )) {
            assertEquals(ChosenIcon(preset = preset, color = "blue"), ChosenIcon.applyColor(null, "blue", category))
        }
    }

    @Test
    fun testPreparingAPhotoShowsWhyItWasRefused() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val photo = ByteString(7, 7, 7)
        // The device decodes photos; here only that one is readable, and it converts to itself.
        val convertPhoto = { data: ByteString -> data.takeIf { it == photo } }

        val unreadable = model.itemEditor.prepareItemIcon(ByteString(1), convertPhoto)
        assertNull(unreadable)
        assertEquals(ItemEditorModel.UNREADABLE_PHOTO_MESSAGE, model.errorMessage.value)
        assertFalse("prepareItemIcon" in harness.backend.allCallName, "nothing the device can't read reaches the backend")

        val refused = model.itemEditor.prepareItemIcon(photo, convertPhoto)
        assertNull(refused)
        assertEquals("choose a PNG, JPEG, GIF or WebP image", model.errorMessage.value)
        harness.backend.preparedIcon = ByteString(4, 5)
        val prepared = model.itemEditor.prepareItemIcon(photo, convertPhoto)
        assertEquals(ByteString(4, 5), prepared)
    }
}
