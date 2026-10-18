package com.tryhisar.hisar.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.theme.ThemePalettes

/**
 * The colours the iPhone app draws with: the theme's palette from themes.json, plus the system colours iOS gives its
 * grouped lists. Screens read them from [HisarTheme.colors].
 */
@Immutable
data class HisarColors(
    val tint: Color,
    val onTint: Color,
    val groupedBackground: Color,
    val cellBackground: Color,
    val label: Color,
    val secondaryLabel: Color,
    val placeholder: Color,
    val separator: Color,
    val fill: Color,
    val warning: Color,
    /** iOS's system orange, which the iPhone app gives a field's warning, such as a port out of range. */
    val orange: Color,
    val destructive: Color,
    val systemBackground: Color,
    val isDark: Boolean,
)

/** iOS's Dynamic Type sizes at the default setting, which the iPhone app's screens use. */
object HisarType {
    val largeTitle = TextStyle(fontSize = 34.sp, lineHeight = 41.sp, fontWeight = FontWeight.Bold)
    val title2 = TextStyle(fontSize = 22.sp, lineHeight = 28.sp, fontWeight = FontWeight.Bold)
    val title3Monospaced = TextStyle(fontSize = 20.sp, lineHeight = 25.sp, fontWeight = FontWeight.Medium, fontFamily = FontFamily.Monospace)
    val headline = TextStyle(fontSize = 17.sp, lineHeight = 22.sp, fontWeight = FontWeight.SemiBold)
    val body = TextStyle(fontSize = 17.sp, lineHeight = 22.sp)
    val bodyMonospaced = body.copy(fontFamily = FontFamily.Monospace)
    val subheadline = TextStyle(fontSize = 15.sp, lineHeight = 20.sp)
    val footnote = TextStyle(fontSize = 13.sp, lineHeight = 18.sp)
    val sectionHeader = TextStyle(fontSize = 17.sp, lineHeight = 22.sp, fontWeight = FontWeight.SemiBold)
    val tabLabel = TextStyle(fontSize = 10.sp, lineHeight = 12.sp, fontWeight = FontWeight.Medium)
}

private val LocalHisarColors = staticCompositionLocalOf { buildColors(VaultTheme.standard, isDark = false) }

object HisarTheme {
    val colors: HisarColors @Composable get() = LocalHisarColors.current
}

private fun buildColors(theme: VaultTheme, isDark: Boolean): HisarColors {
    val palette = ThemePalettes.find(theme.color)
    fun pick(light: Long, dark: Long) = Color(if (isDark) dark else light)
    return HisarColors(
        tint = Color(palette.tint.resolve(isDark)),
        onTint = Color(palette.onTint.resolve(isDark)),
        groupedBackground = Color(palette.groupedBackground.resolve(isDark)),
        cellBackground = pick(0xFFFFFFFF, 0xFF1C1C1E),
        label = pick(0xFF000000, 0xFFFFFFFF),
        secondaryLabel = Color(palette.secondaryLabel.resolve(isDark)),
        placeholder = pick(0x4D3C3C43, 0x4DEBEBF5),
        separator = pick(0xFFC6C6C8, 0xFF38383A),
        fill = pick(0x1F767680, 0x3D767680),
        warning = Color(palette.warning.resolve(isDark)),
        orange = pick(0xFFFF9500, 0xFFFF9F0A),
        destructive = pick(0xFFFF3B30, 0xFFFF453A),
        systemBackground = pick(0xFFFFFFFF, 0xFF000000),
        isDark = isDark,
    )
}

/** Hisar's palette from themes.json on Material 3's roles; never the device's dynamic colours. */
@Composable
fun HisarTheme(theme: VaultTheme = VaultTheme.standard, content: @Composable () -> Unit) {
    val isDark = when (theme.appearance) {
        ThemeAppearanceName.SYSTEM -> isSystemInDarkTheme()
        ThemeAppearanceName.LIGHT -> false
        ThemeAppearanceName.DARK -> true
    }
    val colors = buildColors(theme, isDark)
    val base = if (isDark) darkColorScheme() else lightColorScheme()
    val colorScheme = base.copy(
        primary = colors.tint,
        onPrimary = colors.onTint,
        background = colors.groupedBackground,
        onBackground = colors.label,
        surface = colors.cellBackground,
        onSurface = colors.label,
        surfaceContainer = colors.groupedBackground,
        surfaceContainerHigh = colors.cellBackground,
        surfaceContainerHighest = colors.cellBackground,
        onSurfaceVariant = colors.secondaryLabel,
        outlineVariant = colors.separator,
        error = colors.destructive,
    )
    val typography = Typography(bodyLarge = HisarType.body, bodyMedium = HisarType.subheadline, bodySmall = HisarType.footnote)
    CompositionLocalProvider(LocalHisarColors provides colors) {
        MaterialTheme(colorScheme = colorScheme, typography = typography, content = content)
    }
}
