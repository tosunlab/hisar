package com.tryhisar.hisar.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned

/** The root Scaffold owns floating actions and keeps notices above every visible pinned footer. */
internal class NoticeLayout {
    var floatingAction by mutableStateOf<(@Composable () -> Unit)?>(null)
    var windowBottom by mutableFloatStateOf(0f)
    val footerTops = mutableStateMapOf<Any, Float>()
    val clearance: Float get() = footerTops.values.minOrNull()?.let { (windowBottom - it).coerceAtLeast(0f) } ?: 0f
}

internal val LocalNoticeLayout = staticCompositionLocalOf<NoticeLayout?> { null }

internal fun Modifier.noticeClearance(): Modifier = composed {
    val layout = LocalNoticeLayout.current
    val owner = remember { Any() }
    DisposableEffect(layout, owner) {
        onDispose { layout?.footerTops?.remove(owner) }
    }
    onGloballyPositioned { coordinates -> layout?.footerTops?.set(owner, coordinates.boundsInWindow().top) }
}
