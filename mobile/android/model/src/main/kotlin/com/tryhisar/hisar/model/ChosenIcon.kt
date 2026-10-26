package com.tryhisar.hisar.model

import kotlinx.io.bytestring.ByteString
import kotlinx.serialization.Serializable

/**
 * An item's chosen icon: a preset (core/item IconPresets) in one of the icon colors, or a PNG image, which wins. The
 * backend makes the image (prepareItemIcon) and refuses any other.
 */
@Serializable
data class ChosenIcon(
    val preset: String = "",
    val color: String = "",
    @Serializable(with = Base64ByteStringSerializer::class) val image: ByteString? = null,
) {
    /** The preset to draw, or null for an image or a preset from a newer app, which draws the category's icon. */
    val drawnPreset: String? get() = if (image == null && preset in allPreset) preset else null

    companion object {
        /** The preset icon IDs of core/item, in the picker's order. */
        val allPreset = listOf(
            "bank", "card", "wallet", "crypto", "cart", "gift", "mail", "chat", "social", "work", "home", "school",
            "cloud", "server", "database", "code", "terminal", "wifi", "globe", "game", "music", "video", "health",
            "travel", "car", "key", "lock", "shield", "star", "heart",
        )

        /** The icon color IDs of core/item; "" is the neutral tile. */
        val allColor = listOf("red", "orange", "yellow", "green", "teal", "blue", "indigo", "purple", "pink", "gray")

        /** A photo keeps its pixels; an automatic icon becomes the category's preset so its chosen color is visible. */
        fun applyColor(icon: ChosenIcon?, color: String, category: String): ChosenIcon? = when {
            icon?.image != null -> icon
            icon != null && icon.preset.isNotEmpty() -> icon.copy(color = color)
            color.isEmpty() -> icon
            else -> ChosenIcon(
                preset = when (category) {
                    ItemDetail.CATEGORY_CREDIT_CARD -> "card"
                    ItemDetail.CATEGORY_SERVER -> "server"
                    ItemDetail.CATEGORY_DATABASE -> "database"
                    ItemDetail.CATEGORY_SSH_KEY -> "terminal"
                    ItemDetail.CATEGORY_IDENTITY -> "work"
                    ItemDetail.CATEGORY_ADDRESS -> "home"
                    ItemDetail.CATEGORY_API_CREDENTIAL -> "code"
                    ItemDetail.CATEGORY_SECURE_NOTE -> "lock"
                    else -> "key"
                },
                color = color,
            )
        }
    }
}
