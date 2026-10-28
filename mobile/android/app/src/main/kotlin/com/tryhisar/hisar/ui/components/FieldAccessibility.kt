package com.tryhisar.hisar.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription

@Composable
fun fieldAccessibility(label: String, errorMessage: String?, announceErrorOnBlur: Boolean): Modifier {
    val view = LocalView.current
    var wasFocused by remember { mutableStateOf(false) }
    var announcedMessage by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(errorMessage) {
        if (errorMessage == null) announcedMessage = null
    }
    return Modifier
        .semantics {
            stateDescription = label
            errorMessage?.let { error(it) }
        }
        .onFocusChanged {
            if (wasFocused && !it.isFocused && announceErrorOnBlur && errorMessage != null && errorMessage != announcedMessage) {
                view.announceForAccessibility(errorMessage)
                announcedMessage = errorMessage
            }
            wasFocused = it.isFocused
        }
}
