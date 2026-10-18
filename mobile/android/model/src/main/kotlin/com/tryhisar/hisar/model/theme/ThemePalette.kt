package com.tryhisar.hisar.model.theme

/** A colour in each appearance, as opaque ARGB. ThemePalettes.kt, generated from assets/themes/themes.json, holds them. */
data class ThemeColor(val light: Long, val dark: Long) {
    fun resolve(isDark: Boolean): Long = if (isDark) dark else light
}

data class ThemePalette(
    val tint: ThemeColor,
    val onTint: ThemeColor,
    val groupedBackground: ThemeColor,
    val secondaryLabel: ThemeColor,
    val warning: ThemeColor,
)
