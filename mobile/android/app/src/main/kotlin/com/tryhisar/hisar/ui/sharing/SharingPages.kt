package com.tryhisar.hisar.ui.sharing

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.SheetValue
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/**
 * A page of grouped sections under [bar] that scrolls above the keyboard and reloads on a pull, as a List with
 * `.refreshable` in the iPhone app. A [largeTitle] heads the sections; [bar] learns when it has scrolled away, to show
 * the title inline as iOS does. [bottomInset] keeps the last section clear of the floating tab bar.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RefreshablePage(
    bar: @Composable (isLargeTitleScrolledAway: Boolean) -> Unit,
    onRefresh: (suspend () -> Unit)?,
    largeTitle: String? = null,
    bottomInset: Dp = 0.dp,
    content: @Composable ColumnScope.() -> Unit,
) {
    val scope = rememberCoroutineScope()
    val scrollState = rememberScrollState()
    val titleHeight = with(LocalDensity.current) { 48.dp.roundToPx() }
    val isLargeTitleScrolledAway by remember { derivedStateOf { scrollState.value > titleHeight } }
    var isRefreshing by remember { mutableStateOf(false) }
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .imePadding(),
    ) {
        bar(largeTitle != null && isLargeTitleScrolledAway)
        val sections = @Composable {
            Column(
                Modifier
                    .fillMaxSize()
                    .verticalScroll(scrollState)
                    .padding(horizontal = ScreenMargin),
            ) {
                if (largeTitle != null) LargeTitle(largeTitle, topPadding = 0.dp)
                content()
                Spacer(Modifier.height(bottomInset + 32.dp))
            }
        }
        if (onRefresh == null) {
            Box(Modifier.weight(1f)) { sections() }
        } else {
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

/**
 * A form in a sheet over the screen, as the iPhone's medium-and-large sheets: [bar], then grouped sections. It stays
 * while [isBusy], as `interactiveDismissDisabled`.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FormSheet(isBusy: Boolean, onDismiss: () -> Unit, bar: @Composable () -> Unit, content: @Composable ColumnScope.() -> Unit) {
    val currentIsBusy by rememberUpdatedState(isBusy)
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true, confirmValueChange = { it != SheetValue.Hidden || !currentIsBusy }),
        containerColor = HisarTheme.colors.groupedBackground,
        dragHandle = null,
    ) {
        bar()
        Column(
            Modifier
                .weight(1f, fill = false)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = ScreenMargin)
                .padding(bottom = 32.dp),
            content = content,
        )
    }
}

/** A section card's rows clipped to its corners, so a row's swipe action never draws past them. */
@Composable
fun SwipeableRows(content: @Composable ColumnScope.() -> Unit) {
    Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(26.dp)), content = content)
}

/**
 * A Form section whose header can carry a button on its right, as a Section header with a Menu. Without [content] it
 * shows only its header and footer, as an empty Section does.
 */
@Composable
fun SectionCard(
    header: String? = null,
    headerTestTag: String? = null,
    headerAction: (@Composable () -> Unit)? = null,
    footer: (@Composable ColumnScope.() -> Unit)? = null,
    content: (@Composable ColumnScope.() -> Unit)? = null,
) {
    val colors = HisarTheme.colors
    Column(Modifier.padding(top = 20.dp)) {
        if (header != null) {
            Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, bottom = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    header,
                    style = HisarType.sectionHeader,
                    color = colors.secondaryLabel,
                    modifier = Modifier
                        .weight(1f)
                        .padding(vertical = 8.dp)
                        .then(if (headerTestTag != null) Modifier.testTag(headerTestTag) else Modifier),
                )
                headerAction?.invoke()
            }
        }
        if (content != null) {
            Column(
                Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(26.dp))
                    .background(colors.cellBackground),
                content = content,
            )
        }
        if (footer != null) {
            CompositionLocalProvider(
                LocalTextStyle provides HisarType.footnote.copy(color = colors.secondaryLabel),
                LocalContentColor provides colors.secondaryLabel,
            ) {
                Column(Modifier.padding(start = 16.dp, end = 16.dp, top = 8.dp), verticalArrangement = Arrangement.spacedBy(6.dp), content = footer)
            }
        }
    }
}

/** The line between two rows whose text starts past a symbol ([start]), as iOS insets separators. */
@Composable
fun SymbolRowDivider(start: Dp = 62.dp) {
    HorizontalDivider(Modifier.padding(start = start, end = 16.dp), thickness = 0.5.dp, color = HisarTheme.colors.separator)
}
