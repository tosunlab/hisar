package com.tryhisar.hisar.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

private val ButtonHeight = 52.dp

/**
 * The iPhone app's main action, a full-width capsule in the theme's tint. Disabled, it greys out; busy, it shows a
 * spinner in place of its title.
 */
@Composable
fun ProminentButton(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean = true, isBusy: Boolean = false, icon: ImageVector? = null) {
    val colors = HisarTheme.colors
    val isActive = enabled && !isBusy
    CapsuleButton(
        onClick = onClick,
        testTag = testTag,
        enabled = isActive,
        background = if (isActive) colors.tint else colors.secondaryLabel.copy(alpha = 0.55f),
        content = colors.onTint.takeIf { isActive } ?: colors.label.copy(alpha = 0.6f),
        text = text,
        isBusy = isBusy,
        icon = icon,
    )
}

/** A secondary full-width capsule on a grey fill, as the iPhone app's glass buttons. */
@Composable
fun GlassButton(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean = true, isBusy: Boolean = false) {
    val colors = HisarTheme.colors
    CapsuleButton(
        onClick = onClick,
        testTag = testTag,
        enabled = enabled && !isBusy,
        background = colors.fill,
        content = if (enabled) colors.label else colors.secondaryLabel,
        text = text,
        isBusy = isBusy,
        icon = null,
    )
}

@Composable
private fun CapsuleButton(onClick: () -> Unit, testTag: String, enabled: Boolean, background: Color, content: Color, text: String, isBusy: Boolean, icon: ImageVector?) {
    Row(
        Modifier
            .fillMaxWidth()
            .height(ButtonHeight)
            .background(background, CircleShape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (isBusy) {
            CircularProgressIndicator(Modifier.size(22.dp), color = content, strokeWidth = 2.dp)
        } else {
            if (icon != null) {
                Icon(icon, contentDescription = null, tint = content, modifier = Modifier.size(22.dp))
                Spacer(Modifier.size(8.dp))
            }
            Text(text, style = HisarType.body, color = content)
        }
    }
}

/** A plain text button, as a borderless SwiftUI Button. */
@Composable
fun PlainButton(
    text: String,
    onClick: () -> Unit,
    testTag: String,
    color: Color = HisarTheme.colors.tint,
    enabled: Boolean = true,
    textStyle: TextStyle = HisarType.body,
) {
    Box(
        Modifier
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Text(text, style = textStyle, color = if (enabled) color else HisarTheme.colors.secondaryLabel)
    }
}

/** A segmented Picker: options side by side in a grey capsule, the chosen one on a raised white capsule. */
@Composable
fun <Option> SegmentedControl(options: List<Option>, selected: Option, label: (Option) -> String, onSelect: (Option) -> Unit, testTag: String) {
    val colors = HisarTheme.colors
    Row(
        Modifier
            .fillMaxWidth()
            .height(36.dp)
            .background(colors.fill, CircleShape)
            .padding(2.dp)
            .selectableGroup()
            .testTag(testTag),
    ) {
        for (option in options) {
            val isSelected = option == selected
            Box(
                Modifier
                    .weight(1f)
                    .fillMaxSize()
                    .then(if (isSelected) Modifier.shadow(2.dp, CircleShape).background(if (colors.isDark) Color(0xFF636366) else Color.White, CircleShape) else Modifier)
                    .selectable(selected = isSelected, role = Role.Tab, onClick = { onSelect(option) }),
                contentAlignment = Alignment.Center,
            ) {
                Text(label(option), style = HisarType.subheadline, fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal, color = colors.label)
            }
        }
    }
}

/** A red triangle and a sentence at the top of a form, the iPhone app's ErrorBanner. */
@Composable
fun ErrorBanner(message: String) {
    FormRow(Modifier.testTag("error-message")) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Filled.Warning, contentDescription = null, tint = HisarTheme.colors.destructive, modifier = Modifier.size(20.dp))
            Spacer(Modifier.size(12.dp))
            Text(message, style = HisarType.subheadline, color = HisarTheme.colors.label)
        }
    }
}

/** The floating confirmation at the bottom of the screen. */
@Composable
fun NoticeToast(message: String, modifier: Modifier = Modifier) {
    Row(
        modifier
            .padding(horizontal = 16.dp)
            .shadow(8.dp, CircleShape)
            .background(HisarTheme.colors.cellBackground, CircleShape)
            .padding(horizontal = 16.dp, vertical = 12.dp)
            .testTag("notice"),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Filled.CheckCircle, contentDescription = null, tint = Color(0xFF34C759), modifier = Modifier.size(20.dp))
        Spacer(Modifier.size(8.dp))
        Text(message, style = HisarType.subheadline, fontWeight = FontWeight.Medium, color = HisarTheme.colors.label)
    }
}

/** A centered symbol, title and description for an empty or blocked screen, as ContentUnavailableView. */
@Composable
fun ContentUnavailable(
    title: String,
    icon: ImageVector,
    modifier: Modifier = Modifier,
    description: (@Composable () -> Unit)? = null,
    actions: (@Composable () -> Unit)? = null,
) {
    Column(
        modifier
            .fillMaxSize()
            .padding(horizontal = 32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(icon, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(48.dp))
        Spacer(Modifier.height(12.dp))
        Text(title, style = HisarType.title2, color = HisarTheme.colors.label, textAlign = TextAlign.Center)
        if (description != null) {
            Spacer(Modifier.height(8.dp))
            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) { description() }
        }
        if (actions != null) {
            Spacer(Modifier.height(20.dp))
            actions()
        }
    }
}

/** Makes every testTag below it a resource ID, so UiAutomator finds controls by the iPhone app's identifiers. */
fun Modifier.exposeTestTags(): Modifier = semantics { testTagsAsResourceId = true }
