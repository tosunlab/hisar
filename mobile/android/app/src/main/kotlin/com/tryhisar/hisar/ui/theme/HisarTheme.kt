package com.tryhisar.hisar.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.theme.ThemePalettes

/** The palette's warning colour, which Material 3 has no role for. */
val LocalWarningColor = staticCompositionLocalOf { Color(ThemePalettes.neutral.warning.light) }

/** Hisar's palette from themes.json on Material 3's roles; never the device's dynamic colours. */
@Composable
fun HisarTheme(theme: VaultTheme = VaultTheme.standard, content: @Composable () -> Unit) {
    val isDark = when (theme.appearance) {
        ThemeAppearanceName.SYSTEM -> isSystemInDarkTheme()
        ThemeAppearanceName.LIGHT -> false
        ThemeAppearanceName.DARK -> true
    }
    val palette = ThemePalettes.find(theme.color)
    val tint = Color(palette.tint.resolve(isDark))
    val onTint = Color(palette.onTint.resolve(isDark))
    val groupedBackground = Color(palette.groupedBackground.resolve(isDark))
    val secondaryLabel = Color(palette.secondaryLabel.resolve(isDark))
    val base = if (isDark) darkColorScheme() else lightColorScheme()
    val colorScheme = base.copy(
        primary = tint,
        onPrimary = onTint,
        background = groupedBackground,
        surfaceContainer = groupedBackground,
        onSurfaceVariant = secondaryLabel,
    )
    CompositionLocalProvider(LocalWarningColor provides Color(palette.warning.resolve(isDark))) {
        MaterialTheme(colorScheme = colorScheme, content = content)
    }
}
