package com.tryhisar.hisar.ui.item

import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.indication
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.PressInteraction
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.TextButton
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.ripple
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

/** One entry of a row's touch-and-hold menu, as a SwiftUI context menu button. */
internal class MenuAction(val title: String, val icon: ImageVector, val perform: () -> Unit)

/** A copyable field merges its text into one button; inner buttons retain their own actions. */
@Composable
internal fun MenuRow(
    allAction: List<MenuAction>,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
    onClickLabel: String? = null,
    content: @Composable () -> Unit,
) {
    var isMenuShown by remember { mutableStateOf(false) }
    val interactionSource = remember { MutableInteractionSource() }
    val hasMenu = allAction.isNotEmpty()
    val currentOnClick by rememberUpdatedState(onClick)
    Box(
        modifier
            .fillMaxWidth()
            .heightIn(min = 48.dp)
            .indication(interactionSource, ripple())
            .pointerInput(onClick != null, hasMenu) {
                detectTapGestures(
                    onPress = { offset ->
                        val press = PressInteraction.Press(offset)
                        interactionSource.emit(press)
                        interactionSource.emit(if (tryAwaitRelease()) PressInteraction.Release(press) else PressInteraction.Cancel(press))
                    },
                    onTap = if (onClick != null) ({ _: Offset -> currentOnClick?.invoke() }) else null,
                    onLongPress = if (hasMenu) ({ isMenuShown = true }) else null,
                )
            }
            .semantics(mergeDescendants = onClick != null) {
                if (onClick != null) {
                    role = Role.Button
                    onClickLabel?.let { contentDescription = it }
                    onClick(onClickLabel) {
                        currentOnClick?.invoke()
                        true
                    }
                }
                if (hasMenu) {
                    onLongClick("Show actions") {
                        isMenuShown = true
                        true
                    }
                }
            },
    ) {
        content()
        DropdownMenu(expanded = isMenuShown, onDismissRequest = { isMenuShown = false }) {
            for (action in allAction) {
                DropdownMenuItem(
                    text = { Text(action.title, style = HisarType.body, color = HisarTheme.colors.label) },
                    trailingIcon = { Icon(action.icon, contentDescription = null, tint = HisarTheme.colors.label) },
                    modifier = Modifier.testTag("field-action-${action.title}"),
                    onClick = {
                        isMenuShown = false
                        action.perform()
                    },
                )
            }
        }
    }
}

/**
 * A field of an item: its label, its value (twelve dots while a concealed one is hidden), a reveal button for a concealed
 * one, and a copy button; touch and hold offers copy, copy secret reference and reveal. The tags follow the iPhone app's
 * identifiers, made from the label: "password-value", "reveal-password", "copy-password".
 */
@Composable
internal fun SecretFieldRow(
    label: String,
    value: String,
    isConcealed: Boolean,
    isRevealed: Boolean,
    toggleReveal: (() -> Unit)?,
    copy: () -> Unit,
    copyReference: () -> Unit,
    warning: String = "",
    actionLabel: String = label,
) {
    val isHidden = isConcealed && !isRevealed
    val tagName = label.lowercase()
    var hasOverflow by remember(value) { mutableStateOf(false) }
    var isFullValueShown by remember(value) { mutableStateOf(false) }
    LaunchedEffect(isRevealed) {
        if (!isRevealed) isFullValueShown = false
    }
    val canViewFullValue = isConcealed && isRevealed && hasOverflow
    val allAction = buildList {
        add(MenuAction("Copy $actionLabel", Icons.Outlined.ContentCopy, copy))
        add(MenuAction("Copy secret reference", Icons.Outlined.Terminal, copyReference))
        if (toggleReveal != null) {
            add(MenuAction(if (isRevealed) "Hide" else "Reveal", if (isRevealed) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility, toggleReveal))
        }
        if (canViewFullValue) {
            add(MenuAction("View full value", Icons.Outlined.Visibility, { isFullValueShown = true }))
        }
    }
    MenuRow(allAction, onClick = copy, onClickLabel = "Copy $actionLabel") {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 11.dp).heightIn(min = 30.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                    Text(label, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                    Text(
                        if (isHidden) HIDDEN_VALUE else value,
                        style = when {
                            // SF Mono's dot is a heavy one; the Android monospace font's is small, so the dots use the body font.
                            isHidden -> HiddenValueStyle
                            isConcealed -> HisarType.bodyMonospaced
                            else -> HisarType.body
                        },
                        color = HisarTheme.colors.label,
                        maxLines = if (isConcealed) 3 else 5,
                        overflow = TextOverflow.Ellipsis,
                        onTextLayout = { hasOverflow = isRevealed && it.hasVisualOverflow },
                        modifier = Modifier
                            .testTag("$tagName-value")
                            .semantics { if (isHidden) contentDescription = "Hidden" },
                    )
                }
                if (toggleReveal != null) {
                    FieldIconButton(
                        if (isRevealed) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                        if (isRevealed) "Hide $actionLabel" else "Reveal $actionLabel",
                        "reveal-$tagName",
                        toggleReveal,
                        tint = HisarTheme.colors.label,
                    )
                }
                FieldIconButton(Icons.Outlined.ContentCopy, "Copy $actionLabel", "copy-$tagName", copy, tint = HisarTheme.colors.label)
            }
            if (canViewFullValue) {
                TextButton(
                    onClick = { isFullValueShown = true },
                    modifier = Modifier.testTag("view-full-$tagName").semantics { contentDescription = "View full value for $actionLabel" },
                ) { Text("View full value") }
            }
            if (warning.isNotEmpty()) {
                Spacer(Modifier.size(4.dp))
                Text(warning, style = HisarType.footnote, color = HisarTheme.colors.orange, modifier = Modifier.testTag("card-warning-$label"))
            }
        }
    }
    if (isFullValueShown && isRevealed) {
        AlertDialog(
            onDismissRequest = { isFullValueShown = false },
            title = { Text(label, modifier = Modifier.semantics { heading() }) },
            text = {
                Text(
                    value,
                    style = HisarType.bodyMonospaced,
                    modifier = Modifier.heightIn(max = 360.dp).verticalScroll(rememberScrollState()).testTag("full-value-text"),
                )
            },
            confirmButton = {
                TextButton(onClick = { isFullValueShown = false }, modifier = Modifier.testTag("full-value-close")) { Text("Close") }
            },
            dismissButton = {
                Row {
                    TextButton(onClick = copy, modifier = Modifier.testTag("full-value-copy")) { Text("Copy") }
                    TextButton(onClick = { isFullValueShown = false; toggleReveal?.invoke() }, modifier = Modifier.testTag("full-value-hide")) { Text("Hide") }
                }
            },
        )
    }
}

/** A borderless symbol button inside a row, tinted with the theme; it answers touches on the 48 dp around it. */
@Composable
internal fun FieldIconButton(icon: ImageVector, contentDescription: String, testTag: String, onClick: () -> Unit, tint: Color = HisarTheme.colors.tint) {
    Box(
        Modifier
            .size(48.dp)
            .clickable(role = Role.Button, onClick = onClick)
            .testTag(testTag),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = contentDescription, tint = tint, modifier = Modifier.size(22.dp))
    }
}

private const val HIDDEN_VALUE = "••••••••••••"
private val HiddenValueStyle = HisarType.body.copy(fontSize = 20.sp, letterSpacing = 1.sp)
