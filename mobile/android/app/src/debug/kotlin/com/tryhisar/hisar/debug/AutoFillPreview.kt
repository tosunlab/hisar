package com.tryhisar.hisar.debug

import android.content.Intent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import com.tryhisar.hisar.BackendProvider
import com.tryhisar.hisar.autofill.AutoFillModel
import com.tryhisar.hisar.autofill.AutoFillRoot
import com.tryhisar.hisar.autofill.FillTarget
import com.tryhisar.hisar.autofill.HttpAssetLinksVerifier
import com.tryhisar.hisar.autofill.RequestKind
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch

/**
 * The AutoFill screens inside the app for UI tests, the iPhone app's AutoFillPreviewView: `autoFillPreview <website>` and
 * `autoFillOneTimeCodePreview <website>` open the list for that website and show what would have been filled.
 */
object AutoFillPreview {
    const val PASSWORD_EXTRA = "autoFillPreview"
    const val ONE_TIME_CODE_EXTRA = "autoFillOneTimeCodePreview"

    fun read(activity: FragmentActivity, intent: Intent): (@Composable () -> Unit)? {
        val (serviceIdentifier, kind) = intent.getStringExtra(PASSWORD_EXTRA)?.let { it to RequestKind.PASSWORD }
            ?: intent.getStringExtra(ONE_TIME_CODE_EXTRA)?.let { it to RequestKind.ONE_TIME_CODE }
            ?: return null
        val provider = activity.application as BackendProvider
        val resultMessage = MutableStateFlow<String?>(null)
        var copiedOneTimeCode: String? = null
        val model = AutoFillModel(
            scope = activity.lifecycleScope,
            ioDispatcher = Dispatchers.IO,
            createBackend = { provider.backend },
            events = provider.events,
            verifier = HttpAssetLinksVerifier,
            completeRequest = { credential ->
                val copied = copiedOneTimeCode?.let { " and copied the one-time code $it" }.orEmpty()
                resultMessage.value = "Filled ${credential.username} with a password of ${credential.password.length} characters$copied"
            },
            completeOneTimeCodeRequest = { resultMessage.value = "Filled the one-time code $it" },
            copyOneTimeCode = { code ->
                copiedOneTimeCode = code
                provider.copySecret(code)
            },
            cancelRequest = { resultMessage.value = "Cancelled" },
            passkeyCompletion = AutoFillModel.PasskeyCompletion({}, {}, {}, {}),
        )
        activity.lifecycleScope.launch { model.prepareCredentialList(FillTarget.Websites(listOf(serviceIdentifier)), kind) }
        return {
            val message by resultMessage.collectAsStateWithLifecycle()
            Box(Modifier.fillMaxSize()) {
                AutoFillRoot(model)
                message?.let {
                    val theme by model.theme.collectAsStateWithLifecycle()
                    HisarTheme(theme) {
                        Text(
                            it,
                            style = HisarType.subheadline,
                            color = HisarTheme.colors.label,
                            modifier = Modifier
                                .align(Alignment.BottomCenter)
                                .windowInsetsPadding(WindowInsets.navigationBars)
                                .padding(start = 16.dp, end = 16.dp, bottom = 40.dp)
                                .background(HisarTheme.colors.cellBackground, CircleShape)
                                .padding(16.dp)
                                .testTag("autofill-result"),
                        )
                    }
                }
            }
        }
    }
}
