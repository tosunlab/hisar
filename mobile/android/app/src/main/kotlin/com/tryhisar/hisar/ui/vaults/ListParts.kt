package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.AnchoredDraggableState
import androidx.compose.foundation.gestures.DraggableAnchors
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.anchoredDraggable
import androidx.compose.foundation.gestures.animateTo
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.WifiOff
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MenuDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlin.math.roundToInt
import kotlinx.coroutines.launch

private val CardRadius = 26.dp
private val RowInset = 16.dp

/** Where a row's text starts in its card: past a 24 dp symbol, a 32 dp item tile, a 20 dp note symbol, an invitation's icon. */
val SymbolRowTextStart = 52.dp
val ItemRowTextStart = 60.dp
val NoteRowTextStart = 48.dp
val InvitationRowTextStart = 50.dp

/**
 * One inset grouped section of a lazy list: an optional header, [allRow] on a rounded card with lines between them, and an
 * optional footer, as a Section of the iPhone app's lists. A line starts where the text of the row under it starts
 * ([dividerStart]), as iOS insets separators past a row's icon.
 */
fun <Row> LazyListScope.cardSection(
    key: String,
    allRow: List<Row>,
    rowKey: (Row) -> Any,
    header: String? = null,
    headerTestTag: String? = null,
    footer: String? = null,
    dividerStart: (Row) -> Dp = { RowInset },
    row: @Composable (Row) -> Unit,
) {
    if (allRow.isEmpty()) return
    if (header != null) {
        item(key = "$key-header") {
            Text(
                header,
                style = HisarType.sectionHeader,
                color = HisarTheme.colors.secondaryLabel,
                modifier = Modifier
                    .padding(start = ScreenMargin + RowInset, end = ScreenMargin + RowInset, top = 20.dp, bottom = 10.dp)
                    .then(if (headerTestTag != null) Modifier.testTag(headerTestTag) else Modifier),
            )
        }
    } else {
        item(key = "$key-space") { Spacer(Modifier.height(20.dp)) }
    }
    allRow.forEachIndexed { index, value ->
        item(key = "$key-${rowKey(value)}") {
            val top = if (index == 0) CardRadius else 0.dp
            val bottom = if (index == allRow.lastIndex) CardRadius else 0.dp
            val shape = RoundedCornerShape(topStart = top, topEnd = top, bottomStart = bottom, bottomEnd = bottom)
            Column(
                Modifier
                    .padding(horizontal = ScreenMargin)
                    .fillMaxWidth()
                    .clip(shape)
                    .background(HisarTheme.colors.cellBackground),
            ) {
                if (index > 0) {
                    HorizontalDivider(Modifier.padding(start = dividerStart(value), end = RowInset), thickness = 0.5.dp, color = HisarTheme.colors.separator)
                }
                row(value)
            }
        }
    }
    if (footer != null) {
        item(key = "$key-footer") {
            Text(
                footer,
                style = HisarType.footnote,
                color = HisarTheme.colors.secondaryLabel,
                modifier = Modifier.padding(start = ScreenMargin + RowInset, end = ScreenMargin + RowInset, top = 8.dp),
            )
        }
    }
}

/** A row that opens a screen: [leading], its text, then its count unless 0 and a chevron, as a NavigationLink with a badge. */
@Composable
fun NavigationRow(
    testTag: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    badge: Int = 0,
    leading: (@Composable () -> Unit)? = null,
    content: @Composable () -> Unit,
) {
    Row(
        modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .background(HisarTheme.colors.cellBackground)
            .clickable(role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (leading != null) leading()
        Box(Modifier.weight(1f)) { content() }
        if (badge != 0) Text("$badge", style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = HisarTheme.colors.secondaryLabel.copy(alpha = 0.6f), modifier = Modifier.size(20.dp))
    }
}

/** A row's symbol, as a List Label's icon. */
@Composable
fun RowSymbol(icon: ImageVector, tint: Color = HisarTheme.colors.label) {
    Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(24.dp))
}

/** A title with an optional footnote under it. */
@Composable
fun TitleAndSubtitle(title: String, subtitle: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(title, style = HisarType.body, color = HisarTheme.colors.label, maxLines = 1, overflow = TextOverflow.Ellipsis)
        if (subtitle.isNotEmpty()) {
            Text(subtitle, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}

/** A line of grey text with a symbol, as a Label row in a footnote style. */
@Composable
fun NoteRow(text: String, icon: ImageVector, modifier: Modifier = Modifier, iconTint: Color = HisarTheme.colors.secondaryLabel) {
    Row(
        modifier
            .fillMaxWidth()
            .heightIn(min = 44.dp)
            .padding(horizontal = RowInset, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(icon, contentDescription = null, tint = iconTint, modifier = Modifier.size(20.dp))
        Text(text, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel)
    }
}

/** The offline line at the top of the Vaults and vault screens. */
@Composable
fun OfflineNote() {
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = RowInset, vertical = 12.dp)
            .testTag("offline-banner"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Icon(Icons.Outlined.WifiOff, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(18.dp))
        Text("Offline. Showing items saved on this device; changes need a connection.", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
    }
}

/** The search field under a list's large title, as SwiftUI's searchable. */
@Composable
fun SearchField(value: String, onValueChange: (String) -> Unit, placeholder: String, testTag: String) {
    val colors = HisarTheme.colors
    BasicTextField(
        value = value,
        onValueChange = onValueChange,
        singleLine = true,
        textStyle = HisarType.body.copy(color = colors.label),
        cursorBrush = SolidColor(colors.tint),
        modifier = Modifier
            .padding(horizontal = ScreenMargin)
            .fillMaxWidth()
            .testTag(testTag),
        decorationBox = { innerTextField ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .height(40.dp)
                    .background(colors.fill, RoundedCornerShape(20.dp))
                    .padding(horizontal = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Outlined.Search, contentDescription = null, tint = colors.secondaryLabel, modifier = Modifier.size(20.dp))
                Spacer(Modifier.width(6.dp))
                Box(Modifier.weight(1f)) {
                    if (value.isEmpty()) Text(placeholder, style = HisarType.body, color = colors.secondaryLabel, maxLines = 1)
                    innerTextField()
                }
                if (value.isNotEmpty()) {
                    Icon(
                        Icons.Filled.Cancel,
                        contentDescription = "Clear",
                        tint = colors.secondaryLabel,
                        modifier = Modifier
                            .size(18.dp)
                            .clickable(role = Role.Button) { onValueChange("") },
                    )
                }
            }
        },
    )
}

private enum class SwipeState { CLOSED, OPEN }

/**
 * A row with one swipe action, as a List row's swipeActions: dragging it from [isLeading]'s edge uncovers a button that
 * runs [onAction]. TalkBack offers the action on the row itself.
 */
@Composable
fun SwipeActionRow(
    actionLabel: String,
    actionIcon: ImageVector,
    actionTestTag: String,
    isLeading: Boolean,
    onAction: () -> Unit,
    content: @Composable () -> Unit,
) {
    val actionWidth = 96.dp
    val actionWidthPixels = with(LocalDensity.current) { actionWidth.toPx() }
    val openOffset = if (isLeading) actionWidthPixels else -actionWidthPixels
    val state = remember(openOffset) {
        AnchoredDraggableState(SwipeState.CLOSED, DraggableAnchors { SwipeState.CLOSED at 0f; SwipeState.OPEN at openOffset })
    }
    val scope = rememberCoroutineScope()
    fun runAction() {
        scope.launch { state.animateTo(SwipeState.CLOSED) }
        onAction()
    }
    Box(
        Modifier
            .fillMaxWidth()
            .semantics { customActions = listOf(CustomAccessibilityAction(actionLabel) { runAction(); true }) },
    ) {
        Box(Modifier.matchParentSize(), contentAlignment = if (isLeading) Alignment.CenterStart else Alignment.CenterEnd) {
            Column(
                Modifier
                    .width(actionWidth)
                    .fillMaxHeight()
                    .background(SwipeActionColor)
                    .clickable(role = Role.Button, onClickLabel = actionLabel) { runAction() }
                    .testTag(actionTestTag),
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Icon(actionIcon, contentDescription = null, tint = Color.White, modifier = Modifier.size(20.dp))
                Text(actionLabel, style = HisarType.footnote, fontWeight = FontWeight.Medium, color = Color.White, textAlign = TextAlign.Center, maxLines = 2)
            }
        }
        Box(
            Modifier
                .offset { IntOffset(state.offset.takeUnless { it.isNaN() }?.roundToInt() ?: 0, 0) }
                .anchoredDraggable(state, Orientation.Horizontal),
        ) { content() }
    }
}

private val SwipeActionColor = Color(0xFFFF9500)

/** A menu in Hisar's colors, opened from a toolbar button, as a SwiftUI Menu. */
@Composable
fun HisarMenu(isExpanded: Boolean, onDismiss: () -> Unit, content: @Composable () -> Unit) {
    DropdownMenu(
        expanded = isExpanded,
        onDismissRequest = onDismiss,
        shape = RoundedCornerShape(14.dp),
        containerColor = HisarTheme.colors.cellBackground,
    ) { content() }
}

@Composable
fun HisarMenuItem(
    text: String,
    testTag: String,
    onClick: () -> Unit,
    icon: ImageVector? = null,
    enabled: Boolean = true,
    isDestructive: Boolean = false,
) {
    val color = if (isDestructive) HisarTheme.colors.destructive else HisarTheme.colors.label
    DropdownMenuItem(
        text = { Text(text, style = HisarType.body) },
        onClick = onClick,
        enabled = enabled,
        leadingIcon = icon?.let { { Icon(it, contentDescription = null) } },
        colors = MenuDefaults.itemColors(
            textColor = color,
            leadingIconColor = color,
            disabledTextColor = HisarTheme.colors.secondaryLabel,
            disabledLeadingIconColor = HisarTheme.colors.secondaryLabel,
        ),
        modifier = Modifier.testTag(testTag),
    )
}

/** The top bar of a screen shown as a sheet on the iPhone: Cancel on the left, the inline title, an action on the right. */
@Composable
fun SheetTopBar(title: String, onCancel: () -> Unit, cancelTestTag: String, action: @Composable RowScope.() -> Unit = {}) {
    Box(
        Modifier
            .fillMaxWidth()
            .windowInsetsPadding(WindowInsets.statusBars)
            .height(64.dp)
            .padding(horizontal = ScreenMargin),
    ) {
        Box(Modifier.align(Alignment.CenterStart)) {
            BarCapsuleButton("Cancel", onCancel, testTag = cancelTestTag)
        }
        Text(
            title,
            style = HisarType.headline,
            color = HisarTheme.colors.label,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center,
            modifier = Modifier
                .align(Alignment.Center)
                .padding(horizontal = 104.dp),
        )
        Row(Modifier.align(Alignment.CenterEnd), verticalAlignment = Alignment.CenterVertically, content = action)
    }
}

/** A text button in a sheet's bar: a raised capsule, as the iPhone's glass toolbar buttons. */
@Composable
fun BarCapsuleButton(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean = true, isBold: Boolean = false) {
    val colors = HisarTheme.colors
    Box(
        Modifier
            .height(44.dp)
            .shadow(6.dp, CircleShape)
            .background(colors.cellBackground, CircleShape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 18.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text,
            style = HisarType.body,
            fontWeight = if (isBold) FontWeight.SemiBold else FontWeight.Normal,
            color = if (enabled) colors.label else colors.secondaryLabel,
        )
    }
}
