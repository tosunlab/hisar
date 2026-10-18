package com.tryhisar.hisar.ui.vaults

import com.tryhisar.hisar.model.ChosenIcon
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlinx.io.bytestring.ByteString

class ItemIconStyleTests {
    @Test
    fun testEveryPresetHasASymbolAndEveryColorAHue() {
        assertEquals(30, ChosenIcon.allPreset.size)
        for (preset in ChosenIcon.allPreset) {
            val drawnPreset = assertNotNull(ChosenIcon(preset = preset).drawnPreset, "preset $preset")
            assertNotNull(ItemIconStyle.getPresetIcon(drawnPreset), "preset $preset has no symbol")
        }
        assertEquals(30, ChosenIcon.allPreset.map(ItemIconStyle::getPresetIcon).distinct().size, "every preset draws its own symbol")
        assertNull(ChosenIcon(preset = "rocket").drawnPreset, "a preset from a newer app draws the category symbol")
        assertNull(ChosenIcon(preset = "bank", image = ByteString(1)).drawnPreset, "an image wins over its preset")
        for (color in ChosenIcon.allColor) {
            assertNotNull(ItemIconStyle.getColor(color), "color $color")
        }
        assertNull(ItemIconStyle.getColor(""))
    }
}
