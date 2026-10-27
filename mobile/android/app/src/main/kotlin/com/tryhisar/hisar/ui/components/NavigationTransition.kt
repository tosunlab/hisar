package com.tryhisar.hisar.ui.components

import androidx.activity.compose.PredictiveBackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.SeekableTransitionState
import androidx.compose.animation.core.rememberTransition
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import kotlinx.coroutines.CancellationException

/**
 * A tab's stack of screens: a push slides the new screen in from the right over the one under it, a pop slides it back
 * out, and the back gesture drags the top screen out as it uncovers the one under it, popping or settling back when the
 * finger lifts.
 */
@Composable
fun <Destination> NavigationTransition(
    path: List<Destination>,
    onPop: () -> Unit,
    isBackEnabled: Boolean = path.isNotEmpty(),
    contentKey: (List<Destination>) -> Any?,
    content: @Composable (List<Destination>) -> Unit,
) {
    val transitionState = remember { SeekableTransitionState(path) }
    val transition = rememberTransition(transitionState, label = "navigation stack")
    LaunchedEffect(path) { transitionState.animateTo(path) }
    PredictiveBackHandler(enabled = isBackEnabled) { progress ->
        val popped = path.dropLast(1)
        try {
            progress.collect { transitionState.seekTo(it.progress, popped) }
            onPop()
        } catch (_: CancellationException) {
            transitionState.animateTo(path)
        }
    }
    transition.AnimatedContent(
        transitionSpec = {
            if (targetState.size >= initialState.size) {
                slideInHorizontally { it } togetherWith slideOutHorizontally { -it / 3 }
            } else {
                slideInHorizontally { -it / 3 } togetherWith slideOutHorizontally { it }
            }
        },
        contentKey = contentKey,
    ) { shownPath -> content(shownPath) }
}
