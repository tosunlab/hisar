package com.tryhisar.hisar.ui.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

private val RowInset = 16.dp
private val RowMinHeight = 52.dp

/**
 * A screen pushed inside the Settings tab: the shared [PushedTopBar], the title inline in it or large under it as the
 * iPhone screen has it, and grouped sections that scroll clear of the floating tab bar. [leading] takes the back
 * button's place for a screen with its own way out, such as Cancel or Done; with neither, the bar is empty. With
 * [onRefresh], a pull reloads the sections, as `.refreshable`.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    title: String,
    onBack: (() -> Unit)?,
    isTitleInline: Boolean = false,
    leading: (@Composable () -> Unit)? = null,
    bottomInset: Dp = TabBarHeight,
    onRefresh: (suspend () -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    val barTitle = if (isTitleInline) title else ""
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .imePadding(),
    ) {
        if (onBack != null && leading == null) {
            PushedTopBar(barTitle, onBack)
        } else {
            Box(
                Modifier
                    .fillMaxWidth()
                    .windowInsetsPadding(WindowInsets.statusBars)
                    .heightIn(min = 52.dp)
                    .padding(horizontal = ScreenMargin),
            ) {
                Box(Modifier.align(Alignment.CenterStart)) { leading?.invoke() }
                Text(
                    barTitle,
                    style = HisarType.headline,
                    color = HisarTheme.colors.label,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.align(Alignment.Center).padding(horizontal = 112.dp),
                )
            }
        }
        val sections = @Composable {
            Column(
                Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = ScreenMargin),
            ) {
                if (!isTitleInline) LargeTitle(title, topPadding = 0.dp)
                content()
                Spacer(Modifier.height(bottomInset + 24.dp))
            }
        }
        if (onRefresh == null) {
            Box(Modifier.weight(1f)) { sections() }
        } else {
            val scope = rememberCoroutineScope()
            var isRefreshing by remember { mutableStateOf(false) }
            PullToRefreshBox(
                isRefreshing = isRefreshing,
                onRefresh = {
                    scope.launch {
                        isRefreshing = true
                        onRefresh()
                        isRefreshing = false
                    }
                },
                modifier = Modifier.weight(1f),
            ) { sections() }
        }
    }
}

/** A row that opens another screen, with its symbol, an optional value or badge, and the chevron, as a NavigationLink. */
@Composable
fun NavigationRow(
    text: String,
    testTag: String,
    onClick: () -> Unit,
    icon: ImageVector? = null,
    value: String? = null,
    color: Color = HisarTheme.colors.label,
    enabled: Boolean = true,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val textColor = if (enabled) color else HisarTheme.colors.secondaryLabel
        if (icon != null) RowIcon(icon, if (enabled && color == HisarTheme.colors.label) HisarTheme.colors.tint else textColor)
        Text(text, style = HisarType.body, color = textColor, modifier = Modifier.weight(1f))
        if (value != null) {
            Text(value, style = HisarType.body, color = HisarTheme.colors.secondaryLabel, maxLines = 1, modifier = Modifier.padding(start = 8.dp))
        }
        Icon(
            Icons.AutoMirrored.Filled.KeyboardArrowRight,
            contentDescription = null,
            tint = HisarTheme.colors.secondaryLabel.copy(alpha = 0.6f),
            modifier = Modifier.padding(start = 6.dp).size(22.dp),
        )
    }
}

@Composable
fun RowScope.RowIcon(icon: ImageVector, tint: Color = HisarTheme.colors.tint) {
    Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(24.dp))
    Spacer(Modifier.size(14.dp))
}

/** A row with a switch, as a Toggle in a Form; the switch takes the theme's tint as SwiftUI's does. */
@Composable
fun ToggleRow(
    text: String,
    isOn: Boolean,
    onChange: (Boolean) -> Unit,
    testTag: String,
    icon: ImageVector? = null,
    enabled: Boolean = true,
    subtitle: String? = null,
) {
    val colors = HisarTheme.colors
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .toggleable(value = isOn, enabled = enabled, role = Role.Switch, onValueChange = onChange)
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (icon != null) RowIcon(icon, if (enabled) colors.tint else colors.secondaryLabel)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(text, style = HisarType.body, color = if (enabled) colors.label else colors.secondaryLabel)
            if (subtitle != null) Text(subtitle, style = HisarType.footnote, color = colors.secondaryLabel)
        }
        Switch(
            checked = isOn,
            onCheckedChange = null,
            enabled = enabled,
            colors = SwitchDefaults.colors(
                checkedThumbColor = Color.White,
                checkedTrackColor = colors.tint,
                checkedBorderColor = Color.Transparent,
                uncheckedThumbColor = Color.White,
                uncheckedTrackColor = colors.fill,
                uncheckedBorderColor = Color.Transparent,
                disabledCheckedTrackColor = colors.tint.copy(alpha = 0.4f),
                disabledCheckedThumbColor = Color.White,
                disabledUncheckedTrackColor = colors.fill,
                disabledUncheckedThumbColor = Color.White,
                disabledUncheckedBorderColor = Color.Transparent,
            ),
        )
    }
}

/** One choice of an inline Picker: its title, and a checkmark in the tint when chosen. */
@Composable
fun CheckRow(text: String, isChosen: Boolean, onClick: () -> Unit, testTag: String, leading: (@Composable () -> Unit)? = null, subtitle: String? = null) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .clickable(role = Role.RadioButton, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            leading()
            Spacer(Modifier.size(14.dp))
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(text, style = HisarType.body, color = HisarTheme.colors.label)
            if (subtitle != null) Text(subtitle, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
        }
        if (isChosen) Icon(Icons.Filled.Check, contentDescription = "Chosen", tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
    }
}

/** A centered destructive button row, as `Button(role: .destructive)` in a list. */
@Composable
fun DestructiveRow(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean = true) {
    Box(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, style = HisarType.body, color = if (enabled) HisarTheme.colors.destructive else HisarTheme.colors.secondaryLabel)
    }
}

/**
 * A confirmationDialog: a title, an optional message, the action (red when [isDestructive]) and [dismissLabel].
 * [confirmTestTag] is the iPhone's identifier for the action.
 */
@Composable
fun ConfirmationDialog(
    title: String,
    message: String?,
    confirmLabel: String,
    confirmTestTag: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    isDestructive: Boolean = true,
    dismissLabel: String = "Cancel",
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = message?.let { { Text(it) } },
        confirmButton = {
            PlainButton(
                confirmLabel,
                {
                    onDismiss()
                    onConfirm()
                },
                testTag = confirmTestTag,
                color = if (isDestructive) HisarTheme.colors.destructive else HisarTheme.colors.tint,
            )
        },
        dismissButton = { PlainButton(dismissLabel, onDismiss, testTag = "cancel-confirmation") },
        containerColor = HisarTheme.colors.cellBackground,
    )
}

/** A round initials badge, as the account card's. */
@Composable
fun InitialsBadge(initials: String) {
    Box(
        Modifier
            .size(48.dp)
            .background(HisarTheme.colors.fill, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Text(initials, style = HisarType.headline, color = HisarTheme.colors.secondaryLabel)
    }
}
