package com.tryhisar.hisar.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBackIos
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

private val TitleGap = 8.dp

/**
 * The navigation bar of a pushed screen, as the iPhone app's inline title: a back button, the title centred on the screen,
 * then [actions]. The title takes all the width the buttons leave it on the wider side.
 */
@Composable
fun PushedTopBar(title: String, onBack: () -> Unit, actions: @Composable RowScope.() -> Unit = {}) {
    Layout(
        content = {
            ToolbarIconButton(Icons.AutoMirrored.Filled.ArrowBackIos, "Back", testTag = "back", onClick = onBack)
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically, content = actions)
            Text(title, style = HisarType.headline, color = HisarTheme.colors.label, textAlign = TextAlign.Center, maxLines = 1, overflow = TextOverflow.Ellipsis)
        },
        modifier = Modifier
            .fillMaxWidth()
            .windowInsetsPadding(WindowInsets.statusBars)
            .heightIn(min = 52.dp)
            .padding(horizontal = ScreenMargin),
    ) { measurables, constraints ->
        val looseConstraints = constraints.copy(minWidth = 0, minHeight = 0)
        val back = measurables[0].measure(looseConstraints)
        val actionRow = measurables[1].measure(looseConstraints)
        val side = maxOf(back.width, actionRow.width) + TitleGap.roundToPx()
        val titleText = measurables[2].measure(looseConstraints.copy(maxWidth = (constraints.maxWidth - 2 * side).coerceAtLeast(0)))
        // The bar is at least its minimum and grows with a large-font title or button.
        val height = maxOf(constraints.minHeight, back.height, actionRow.height, titleText.height)
        layout(constraints.maxWidth, height) {
            back.placeRelative(0, (height - back.height) / 2)
            actionRow.placeRelative(constraints.maxWidth - actionRow.width, (height - actionRow.height) / 2)
            titleText.placeRelative((constraints.maxWidth - titleText.width) / 2, (height - titleText.height) / 2)
        }
    }
}

/** A pushed screen of grouped sections under a [PushedTopBar], scrolling clear of the floating tab bar ([bottomInset]). */
@Composable
fun PushedScaffold(
    title: String,
    onBack: () -> Unit,
    bottomInset: Dp,
    actions: @Composable RowScope.() -> Unit = {},
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground),
    ) {
        PushedTopBar(title, onBack, actions)
        Column(
            Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = ScreenMargin),
        ) {
            content()
            Spacer(Modifier.height(bottomInset))
        }
    }
}
