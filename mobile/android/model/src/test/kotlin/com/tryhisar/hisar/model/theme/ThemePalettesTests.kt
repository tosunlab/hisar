package com.tryhisar.hisar.model.theme

import com.tryhisar.hisar.model.ThemeColorName
import kotlin.test.Test
import kotlin.test.assertEquals

class ThemePalettesTests {
    @Test
    fun testEveryColourHasAPalette() {
        val allTint = ThemeColorName.entries.map { ThemePalettes.find(it).tint.light }
        assertEquals(listOf(0xFF1D2025, 0xFF2356C4, 0xFF6A3DCC, 0xFF1B7348), allTint)
        assertEquals(0xFF7EA6F8, ThemePalettes.blue.tint.resolve(isDark = true))
    }
}
