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

        /**
         * The icon once [color] is chosen in the picker: the chosen preset in that color, or the first preset when the icon
         * is automatic, so the color shows. A photo has no color, and "no color" leaves an automatic icon automatic.
         */
        fun applyColor(icon: ChosenIcon?, color: String): ChosenIcon? = when {
            icon?.image != null -> icon
            icon != null && icon.preset.isNotEmpty() -> icon.copy(color = color)
            color.isEmpty() -> icon
            else -> ChosenIcon(preset = allPreset.first(), color = color)
        }
    }
}
