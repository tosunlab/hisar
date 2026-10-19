package com.tryhisar.hisar.ui.components

import android.view.inputmethod.EditorInfo
import androidx.compose.runtime.Composable
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.ContentDataType
import androidx.compose.ui.platform.InterceptPlatformTextInput
import androidx.compose.ui.platform.PlatformTextInputMethodRequest
import androidx.compose.ui.semantics.contentDataType
import androidx.compose.ui.semantics.semantics

/**
 * Wraps the text field of a secret, such as a password, the Secret Key or a card number, when [isSecret]: the keyboard
 * is asked not to learn what is typed. The field itself carries [secretInput].
 */
@OptIn(ExperimentalComposeUiApi::class)
@Composable
fun SecretInput(isSecret: Boolean, content: @Composable () -> Unit) {
    if (!isSecret) return content()
    InterceptPlatformTextInput(
        interceptor = { request, nextHandler ->
            val secretRequest = PlatformTextInputMethodRequest { outAttributes ->
                request.createInputConnection(outAttributes).also {
                    outAttributes.imeOptions = outAttributes.imeOptions or EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING
                }
            }
            nextHandler.startInputMethod(secretRequest)
        },
        content = content,
    )
}

/**
 * Marks a text field inside [SecretInput] as one autofill services cannot fill. Compose already marks typed text as
 * sensitive, which keeps its value out of the requests autofill services receive.
 */
fun Modifier.secretInput(isSecret: Boolean): Modifier = if (isSecret) semantics { contentDataType = ContentDataType.None } else this
