package com.tryhisar.hisar.autofill

import android.app.Application
import android.content.Intent
import android.os.Bundle
import android.view.WindowManager
import android.view.autofill.AutofillId
import android.view.autofill.AutofillManager
import android.widget.Toast
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.tryhisar.hisar.BackendProvider
import com.tryhisar.hisar.ScreenshotSetting
import com.tryhisar.hisar.model.LoginCredential
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/** How the AutoFill screens end: the values of the login the user picked, or nothing. */
sealed interface AutofillResult {
    data class Filled(val allValue: List<Pair<AutofillId, String>>, val title: String) : AutofillResult

    data object Cancelled : AutofillResult
}

/**
 * The AutoFill model of one request. It outlives a rotation, and finishes by publishing [result]. A suggestion for one
 * login fills without showing anything while Hisar is unlocked; otherwise [isContentShown] turns on the screens.
 */
class AutofillViewModel(application: Application, val request: AutofillRequest, private val provider: BackendProvider) : AndroidViewModel(application) {
    val result = MutableStateFlow<AutofillResult?>(null)
    val isContentShown = MutableStateFlow(false)
    private val filledTitle = request.target.name

    val model = AutoFillModel(
        scope = viewModelScope,
        ioDispatcher = Dispatchers.IO,
        createBackend = { provider.backend },
        verifier = HttpAssetLinksVerifier,
        completeRequest = ::completeLogin,
        completeOneTimeCodeRequest = ::fillOneTimeCode,
        copyOneTimeCode = ::copyOneTimeCode,
        cancelRequest = ::cancel,
        // AutoFill's datasets carry no passkeys; Credential Manager brings those to CredentialActivity.
        passkeyCompletion = AutoFillModel.PasskeyCompletion({ cancel() }, { cancel() }, ::cancel, ::cancel),
    )

    init {
        viewModelScope.launch { start() }
    }

    private suspend fun start() {
        val pick = request.pick
        if (pick != null) {
            model.prepare()
            val isFilled = if (request.kind == RequestKind.ONE_TIME_CODE) {
                model.provideOneTimeCodeWithoutUserInteraction(pick.recordIdentifier, pick.source)?.let(::fillOneTimeCode) != null
            } else {
                model.provideCredentialWithoutUserInteraction(pick.recordIdentifier, pick.source)?.let(::completeLogin) != null
            }
            if (isFilled) return
        }
        isContentShown.value = true
        if (pick != null) {
            model.prepareInterfaceToProvideCredential(pick.recordIdentifier, request.target, pick.source, request.kind)
        } else {
            model.prepareCredentialList(request.target, request.kind)
        }
    }

    private fun fillOneTimeCode(code: String) {
        request.oneTimeCode?.let { finish(listOf(it to code)) } ?: cancel()
    }

    private fun completeLogin(credential: LoginCredential) {
        val allValue = listOfNotNull(
            request.username?.takeIf { credential.username.isNotEmpty() }?.let { it to credential.username },
            request.password?.let { it to credential.password },
        )
        if (allValue.isEmpty()) cancel() else finish(allValue)
    }

    private fun finish(allValue: List<Pair<AutofillId, String>>) {
        result.value = AutofillResult.Filled(allValue, filledTitle)
    }

    private fun cancel() {
        result.value = AutofillResult.Cancelled
    }

    private fun copyOneTimeCode(code: String) {
        if (provider.copySecret(code)) {
            Toast.makeText(getApplication(), "One-time code copied. It clears in about 2 minutes.", Toast.LENGTH_LONG).show()
        }
    }
}

/**
 * Opened by an AutoFill suggestion: unlocks Hisar when it is locked, lets the user pick a login, and gives the system the
 * values to fill.
 */
class AutofillActivity : FragmentActivity() {
    private val viewModel: AutofillViewModel by viewModels {
        viewModelFactory {
            initializer { AutofillViewModel(application, checkNotNull(AutofillRequest.read(intent)), application as BackendProvider) }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        if (AutofillRequest.read(intent) == null) {
            setResult(RESULT_CANCELED)
            finish()
            return
        }
        if (!ScreenshotSetting(this).isAllowed) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        lifecycleScope.launch {
            viewModel.isContentShown.first { it }
            setContent { AutoFillRoot(viewModel.model) }
        }
        lifecycleScope.launch {
            when (val result = viewModel.result.filterNotNull().first()) {
                is AutofillResult.Filled -> setResult(
                    RESULT_OK,
                    Intent().putExtra(AutofillManager.EXTRA_AUTHENTICATION_RESULT, Datasets.buildFilled(this@AutofillActivity, result.allValue, result.title)),
                )
                AutofillResult.Cancelled -> setResult(RESULT_CANCELED)
            }
            finish()
        }
    }
}
