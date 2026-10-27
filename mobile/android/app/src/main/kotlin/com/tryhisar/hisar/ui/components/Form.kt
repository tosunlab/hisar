package com.tryhisar.hisar.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableStateOf
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.IconButton
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

/** The iPhone app's insets: 16 dp from the screen to a section's card, 16 dp from the card to its text. */
val ScreenMargin = 16.dp
private val RowInset = 16.dp
private val CellShape = RoundedCornerShape(26.dp)
private val RowMinHeight = 52.dp

/**
 * A full screen of grouped sections on the theme's grouped background, the iPhone app's inset grouped Form with a large
 * title. [bottomBar] stays above the keyboard, as SwiftUI's bottom safe-area inset does.
 */
@Composable
fun GroupedForm(
    title: String?,
    modifier: Modifier = Modifier,
    bottomBar: (@Composable ColumnScope.() -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .imePadding(),
    ) {
        Column(
            Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState())
                .windowInsetsPadding(WindowInsets.statusBars)
                .padding(horizontal = ScreenMargin),
        ) {
            if (title != null) LargeTitle(title)
            content()
            Spacer(Modifier.height(24.dp))
        }
        if (bottomBar != null) {
            Column(
                Modifier
                    .fillMaxWidth()
                    .noticeClearance()
                    .windowInsetsPadding(WindowInsets.navigationBars)
                    .padding(horizontal = 20.dp)
                    .padding(top = 8.dp, bottom = 8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                content = bottomBar,
            )
        } else {
            Spacer(Modifier.windowInsetsPadding(WindowInsets.navigationBars))
        }
    }
}

@Composable
fun LargeTitle(title: String, topPadding: Dp = 44.dp) {
    Text(
        title,
        style = HisarType.largeTitle,
        color = HisarTheme.colors.label,
        modifier = Modifier.padding(top = topPadding, bottom = 8.dp).then(if (title.isNotBlank()) Modifier.semantics { heading() } else Modifier),
    )
}

/** A Form section: an optional header, its rows on one rounded card, and an optional footer under it. */
@Composable
fun FormSection(
    modifier: Modifier = Modifier,
    header: String? = null,
    footer: (@Composable ColumnScope.() -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(modifier.padding(top = 20.dp)) {
        if (!header.isNullOrBlank()) {
            Text(
                header,
                style = HisarType.sectionHeader,
                color = HisarTheme.colors.secondaryLabel,
                modifier = Modifier.padding(start = RowInset, end = RowInset, bottom = 10.dp).semantics { heading() },
            )
        }
        Column(
            Modifier
                .fillMaxWidth()
                .background(HisarTheme.colors.cellBackground, CellShape),
            content = content,
        )
        if (footer != null) {
            Column(Modifier.padding(start = RowInset, end = RowInset, top = 8.dp)) {
                ProvideFootnote(footer)
            }
        }
    }
}

/** Content on the grouped background with no card, as a section whose rows have a clear background. */
@Composable
fun ClearSection(modifier: Modifier = Modifier, footer: (@Composable ColumnScope.() -> Unit)? = null, content: @Composable ColumnScope.() -> Unit) {
    Column(modifier.padding(top = 20.dp)) {
        content()
        if (footer != null) {
            Column(Modifier.padding(start = RowInset, end = RowInset, top = 8.dp)) {
                ProvideFootnote(footer)
            }
        }
    }
}

@Composable
private fun ProvideFootnote(content: @Composable ColumnScope.() -> Unit) {
    CompositionLocalProvider(
        LocalTextStyle provides HisarType.footnote.copy(color = HisarTheme.colors.secondaryLabel),
        LocalContentColor provides HisarTheme.colors.secondaryLabel,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) { content() }
    }
}

/** The line between two rows of a card, inset like iOS's. */
@Composable
fun RowDivider() {
    HorizontalDivider(Modifier.padding(horizontal = RowInset), thickness = 0.5.dp, color = HisarTheme.colors.separator)
}

/**
 * A text field filling one row, with a grey placeholder, as SwiftUI's TextField and SecureField in a Form. A secure
 * field hides its text; it and any other [isSecret] field keep their text from the keyboard's learning and autofill.
 */
@Composable
fun FormTextField(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    testTag: String,
    modifier: Modifier = Modifier,
    isSecure: Boolean = false,
    isSecret: Boolean = isSecure,
    revealName: String? = null,
    concealWhen: Boolean = false,
    textStyle: TextStyle = HisarType.body,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    keyboardActions: KeyboardActions = KeyboardActions.Default,
) {
    val color = HisarTheme.colors.label
    var isRevealed by remember(testTag, concealWhen) { mutableStateOf(false) }
    LaunchedEffect(value.isEmpty()) { if (value.isEmpty()) isRevealed = false }
    Row(verticalAlignment = Alignment.CenterVertically) {
        SecretInput(isSecret) {
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = true,
                textStyle = textStyle.copy(color = color),
                cursorBrush = SolidColor(HisarTheme.colors.tint),
                visualTransformation = if (isSecure && !isRevealed) PasswordVisualTransformation() else VisualTransformation.None,
                keyboardOptions = keyboardOptions,
                keyboardActions = keyboardActions,
                modifier = modifier
                    .weight(1f)
                    .heightIn(min = RowMinHeight)
                    .secretInput(isSecret)
                    .testTag(testTag),
                decorationBox = { innerTextField ->
                    Box(Modifier.padding(horizontal = RowInset, vertical = 14.dp), contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty()) Text(placeholder, style = textStyle, color = HisarTheme.colors.placeholder)
                        innerTextField()
                    }
                },
            )
        }
        if (isSecure && revealName != null) {
            IconButton(onClick = { isRevealed = !isRevealed }, enabled = !concealWhen, modifier = Modifier.size(48.dp).testTag("$testTag-reveal")) {
                Icon(
                    if (isRevealed) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                    contentDescription = "${if (isRevealed) "Hide" else "Show"} $revealName",
                    tint = HisarTheme.colors.secondaryLabel,
                    modifier = Modifier.size(22.dp),
                )
            }
        }
    }
}

/** A tappable row, with an optional leading symbol, as a Button or NavigationLink row of a Form. */
@Composable
fun FormButtonRow(
    text: String,
    onClick: () -> Unit,
    testTag: String,
    icon: ImageVector? = null,
    color: Color = HisarTheme.colors.label,
    enabled: Boolean = true,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .clickable(enabled = enabled, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (icon != null) {
            Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(24.dp))
            Spacer(Modifier.size(14.dp))
        }
        Text(text, style = HisarType.body, color = if (enabled) color else HisarTheme.colors.secondaryLabel)
    }
}

/** A row of plain content, padded as a Form row. */
@Composable
fun FormRow(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Box(
        modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .padding(horizontal = RowInset, vertical = 14.dp),
        contentAlignment = Alignment.CenterStart,
    ) { content() }
}

/** A label and its value on one row, as LabeledContent. */
@Composable
fun LabeledRow(label: String, value: String) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .padding(horizontal = RowInset, vertical = 14.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, style = HisarType.body, color = HisarTheme.colors.label)
        SelectionContainer(Modifier.weight(1f)) {
            Text(value, style = HisarType.body, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.End, modifier = Modifier.fillMaxWidth())
        }
    }
}
