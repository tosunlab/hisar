package com.tryhisar.hisar.ui.components

import androidx.compose.material3.Snackbar
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel

/** Material owns notice duration, placement and accessibility announcements. */
@Composable
fun NoticeHost(model: AppModel) {
    val notice by model.notice.collectAsStateWithLifecycle()
    val host = remember { SnackbarHostState() }
    LaunchedEffect(notice) {
        val shown = notice ?: return@LaunchedEffect
        val result = host.showSnackbar(
            message = shown.message,
            actionLabel = shown.action?.label,
            withDismissAction = true,
            duration = if (shown.action != null) SnackbarDuration.Long else SnackbarDuration.Short,
        )
        if (result == SnackbarResult.ActionPerformed) model.performNoticeAction(shown) else model.dismissNotice(shown)
    }
    SnackbarHost(hostState = host) { data ->
        key(data) {
            val dismissState = rememberSwipeToDismissBoxState()
            LaunchedEffect(dismissState.currentValue) {
                if (dismissState.currentValue != SwipeToDismissBoxValue.Settled) data.dismiss()
            }
            SwipeToDismissBox(state = dismissState, backgroundContent = {}) {
                Snackbar(data, modifier = Modifier.testTag("notice"))
            }
        }
    }
}
