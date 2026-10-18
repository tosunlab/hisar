package com.tryhisar.hisar.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.ThemeColorName
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.theme.ThemePalette
import com.tryhisar.hisar.model.theme.ThemePalettes
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.launch

val ThemeAppearanceName.label: String
    get() = when (this) {
        ThemeAppearanceName.SYSTEM -> "System"
        ThemeAppearanceName.LIGHT -> "Light"
        ThemeAppearanceName.DARK -> "Dark"
    }

val ThemeColorName.label: String
    get() = when (this) {
        ThemeColorName.NEUTRAL -> "Neutral"
        ThemeColorName.BLUE -> "Blue"
        ThemeColorName.PURPLE -> "Purple"
        ThemeColorName.GREEN -> "Green"
    }

/** Settings → Appearance: light, dark or the device's, and one of Hisar's four palettes; a change shows at once. */
@Composable
fun AppearanceScreen(model: AppModel, onBack: () -> Unit) {
    val theme by model.theme.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val choose: (VaultTheme) -> Unit = { scope.launch { model.settings.setTheme(it) } }
    LaunchedEffect(Unit) { model.errorMessage.value = null }

    SettingsScreen("Appearance", onBack, isTitleInline = true) {
        FormSection(header = "Appearance") {
            ThemeAppearanceName.entries.forEachIndexed { index, appearance ->
                if (index > 0) RowDivider()
                CheckRow(
                    appearance.label,
                    isChosen = theme.appearance == appearance,
                    onClick = { choose(theme.copy(appearance = appearance)) },
                    testTag = "appearance-${appearance.name.lowercase()}",
                    leading = { Swatch { AppearanceSwatch(appearance) } },
                    subtitle = if (appearance == ThemeAppearanceName.SYSTEM) "Light or Dark, as set on this device" else null,
                )
            }
        }
        FormSection(
            header = "Color",
            footer = { Text("Blue, Purple and Green tint the background and the controls. Hisar on your Mac has its own theme setting.") },
        ) {
            ThemeColorName.entries.forEachIndexed { index, color ->
                if (index > 0) RowDivider()
                CheckRow(
                    color.label,
                    isChosen = theme.color == color,
                    onClick = { choose(theme.copy(color = color)) },
                    testTag = "theme-color-${color.name.lowercase()}",
                    leading = { Swatch { ColorSwatch(ThemePalettes.find(color)) } },
                )
            }
        }
        errorMessage?.let { FormSection { ErrorBanner(it) } }
    }
}

@Composable
private fun Swatch(content: @Composable () -> Unit) {
    val shape = RoundedCornerShape(8.dp)
    Box(
        Modifier
            .size(30.dp)
            .clip(shape)
            .border(0.5.dp, HisarTheme.colors.separator, shape),
    ) { content() }
}

@Composable
private fun AppearanceSwatch(appearance: ThemeAppearanceName) {
    val neutral = ThemePalettes.neutral
    Row(Modifier.fillMaxHeight()) {
        if (appearance != ThemeAppearanceName.DARK) SwatchHalf(neutral.groupedBackground.light, neutral.tint.light)
        if (appearance != ThemeAppearanceName.LIGHT) SwatchHalf(neutral.groupedBackground.dark, neutral.tint.dark)
    }
}

@Composable
private fun androidx.compose.foundation.layout.RowScope.SwatchHalf(background: Long, dot: Long) {
    Box(
        Modifier
            .weight(1f)
            .fillMaxHeight()
            .background(Color(background)),
        contentAlignment = Alignment.BottomCenter,
    ) {
        Box(
            Modifier
                .padding(bottom = 5.dp)
                .size(7.dp)
                .background(Color(dot), CircleShape),
        )
    }
}

@Composable
private fun ColorSwatch(palette: ThemePalette) {
    val isDark = HisarTheme.colors.isDark
    Box(
        Modifier
            .size(30.dp)
            .background(Color(palette.groupedBackground.resolve(isDark))),
        contentAlignment = Alignment.BottomCenter,
    ) {
        Box(
            Modifier
                .padding(bottom = 5.dp)
                .size(width = 14.dp, height = 7.dp)
                .background(Color(palette.tint.resolve(isDark)), CircleShape),
        )
    }
}
