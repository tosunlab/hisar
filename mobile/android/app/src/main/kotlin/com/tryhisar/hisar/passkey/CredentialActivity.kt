package com.tryhisar.hisar.passkey

import android.app.Application
import android.content.Intent
import android.os.Bundle
import android.view.WindowManager
import android.widget.Toast
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.credentials.CreatePublicKeyCredentialRequest
import androidx.credentials.CreatePublicKeyCredentialResponse
import androidx.credentials.GetCredentialResponse
import androidx.credentials.GetPublicKeyCredentialOption
import androidx.credentials.PasswordCredential
import androidx.credentials.PublicKeyCredential
import androidx.credentials.exceptions.CreateCredentialCancellationException
import androidx.credentials.exceptions.CreateCredentialUnknownException
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialUnknownException
import androidx.credentials.exceptions.domerrors.InvalidStateError
import androidx.credentials.exceptions.publickeycredential.CreatePublicKeyCredentialDomException
import androidx.credentials.provider.BeginGetCredentialResponse
import androidx.credentials.provider.BeginGetPublicKeyCredentialOption
import androidx.credentials.provider.CallingAppInfo
import androidx.credentials.provider.PendingIntentHandler
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.tryhisar.hisar.HisarApplication
import com.tryhisar.hisar.ScreenshotSetting
import com.tryhisar.hisar.autofill.AutoFillModel
import com.tryhisar.hisar.autofill.AutoFillRoot
import com.tryhisar.hisar.autofill.FillTarget
import com.tryhisar.hisar.autofill.HttpAssetLinksVerifier
import com.tryhisar.hisar.autofill.LoginMatcher
import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.PasskeyAssertionCredential
import com.tryhisar.hisar.model.PasskeyAssertionRequest
import com.tryhisar.hisar.model.PasskeyRegistrationCredential
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.io.bytestring.ByteString

/**
 * One entry of the system's sheet, from its pending intent to the result the sheet gets back. A passkey or login the
 * user picked is handed out without a screen while Hisar is unlocked; otherwise, and for the unlock action and saving a
 * passkey, [isContentShown] turns on the AutoFill screens.
 */
class CredentialViewModel(application: Application, private val intent: Intent) : AndroidViewModel(application) {
    private val hisar = application as HisarApplication
    private val pick = CredentialPick.read(intent)

    /** The intent the activity returns with RESULT_OK, carrying a credential or an exception; null while undecided. */
    val result = MutableStateFlow<Intent?>(null)
    val isContentShown = MutableStateFlow(false)

    // The client data JSON Hisar wrote for a native app; browsers write their own, so their answers carry none.
    private var clientDataJSON: ByteString? = null

    val model = AutoFillModel(
        scope = viewModelScope,
        ioDispatcher = Dispatchers.IO,
        createBackend = { hisar.backend },
        verifier = HttpAssetLinksVerifier,
        completeRequest = ::completePassword,
        completeOneTimeCodeRequest = { cancel() },
        copyOneTimeCode = ::copyOneTimeCode,
        cancelRequest = ::cancel,
        passkeyCompletion = AutoFillModel.PasskeyCompletion(::completeAssertion, ::completeRegistration, ::rejectExcludedRegistration, ::completeUnlock),
    )

    private val access by lazy { CredentialAccess(hisar.backend, hisar.relyingPartyVerifier, HttpAssetLinksVerifier) }

    init {
        viewModelScope.launch {
            try {
                start()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                refuse(error.message ?: "Hisar could not answer this request.")
            }
        }
    }

    private suspend fun start() {
        when (val current = pick) {
            null -> refuse("Hisar did not make this request.")
            CredentialPick.Unlock -> startUnlock()
            is CredentialPick.Passkey -> startPasskey(current)
            is CredentialPick.Password -> startPassword(current)
            CredentialPick.Create -> startCreate()
        }
    }

    private suspend fun startUnlock() {
        val request = PendingIntentHandler.retrieveBeginGetCredentialRequest(intent) ?: return refuse("Hisar did not make this request.")
        val caller = request.callingAppInfo?.let { readCaller(it) } ?: return refuse("Hisar does not answer this app.")
        val option = CredentialEntries.readOptions(request.beginGetCredentialOptions).firstNotNullOfOrNull { (begin, option) ->
            (option as? CredentialOption.Passkey)?.let { begin to it.request }
        }
        // The request only names the site on the lock screen; nothing is signed before the sheet asks again.
        val passkeyRequest = option?.let { (begin, webAuthn) ->
            val hash = (begin as? BeginGetPublicKeyCredentialOption)?.clientDataHash?.let { ByteString(it) } ?: ByteString()
            withContext(Dispatchers.IO) { access.resolveRelyingParty(caller, webAuthn.relyingPartyID) }?.let { PasskeyAssertionRequest(it, hash, webAuthn.allAllowedCredentialID) }
        }
        isContentShown.value = true
        model.prepareUnlock(if (passkeyRequest != null) FillTarget.Websites(emptyList()) else access.buildFillTarget(caller), passkeyRequest)
    }

    private suspend fun startPasskey(pick: CredentialPick.Passkey) {
        val request = PendingIntentHandler.retrieveProviderGetCredentialRequest(intent) ?: return refuse("Hisar did not make this request.")
        val option = request.credentialOptions.filterIsInstance<GetPublicKeyCredentialOption>().firstOrNull() ?: return refuse("The request asks for no passkey.")
        val caller = readCaller(request.callingAppInfo) ?: return refuse("Hisar does not answer this app.")
        val webAuthn = WebAuthnJson.parseGetRequest(option.requestJson) ?: return refuse("Hisar could not read the passkey request.")
        val assertionRequest = withContext(Dispatchers.IO) {
            val relyingPartyID = access.resolveRelyingParty(caller, webAuthn.relyingPartyID) ?: return@withContext null
            val clientData = access.buildClientData(caller, CLIENT_DATA_GET, webAuthn.challenge, option.clientDataHash?.let { ByteString(it) }) ?: return@withContext null
            clientDataJSON = clientData.json
            PasskeyAssertionRequest(relyingPartyID, clientData.hash, webAuthn.allAllowedCredentialID)
        } ?: return refuse("This app or site may not use these passkeys.")
        model.prepare()
        model.providePasskeyWithoutUserInteraction(assertionRequest, pick.credentialID)?.let { return completeAssertion(it) }
        isContentShown.value = true
        model.prepareInterfaceToProvidePasskey(assertionRequest, pick.credentialID)
    }

    // The login must still match the caller: while unlocked it is checked now; while locked, the list after the unlock offers
    // only what matches, and search.
    private suspend fun startPassword(pick: CredentialPick.Password) {
        val request = PendingIntentHandler.retrieveProviderGetCredentialRequest(intent) ?: return refuse("Hisar did not make this request.")
        val caller = readCaller(request.callingAppInfo) ?: return refuse("Hisar does not answer this app.")
        val target = access.buildFillTarget(caller)
        model.prepare()
        val source = withContext(Dispatchers.IO) {
            if (!hisar.backend.getStatus().isUnlocked) return@withContext null
            LoginMatcher(hisar.backend, HttpAssetLinksVerifier).listAllMatch(target).firstOrNull { it.login.id == pick.recordIdentifier }?.source
        }
        if (source != null) model.provideCredentialWithoutUserInteraction(pick.recordIdentifier, source)?.let { return completePassword(it) }
        isContentShown.value = true
        model.prepareCredentialList(target)
    }

    private suspend fun startCreate() {
        val request = PendingIntentHandler.retrieveProviderCreateCredentialRequest(intent) ?: return refuse("Hisar did not make this request.")
        val callingRequest = request.callingRequest as? CreatePublicKeyCredentialRequest ?: return refuse("Hisar saves passkeys only.")
        val caller = readCaller(request.callingAppInfo) ?: return refuse("Hisar does not answer this app.")
        val webAuthn = WebAuthnJson.parseCreateRequest(callingRequest.requestJson) ?: return refuse("Hisar could not read the passkey request.")
        val registrationRequest = withContext(Dispatchers.IO) {
            val relyingPartyID = access.resolveRelyingParty(caller, webAuthn.relyingPartyID) ?: return@withContext null
            val clientData = access.buildClientData(caller, CLIENT_DATA_CREATE, webAuthn.challenge, callingRequest.clientDataHash?.let { ByteString(it) }) ?: return@withContext null
            clientDataJSON = clientData.json
            PasskeyRegistrationRequest(relyingPartyID, webAuthn.userName, webAuthn.userHandle, clientData.hash, webAuthn.allAlgorithm, webAuthn.allExcludedCredentialID)
        } ?: return refuse("This app or site may not save a passkey here.")
        isContentShown.value = true
        model.prepareInterfaceForPasskeyRegistration(registrationRequest)
    }

    private suspend fun readCaller(callingAppInfo: CallingAppInfo): CredentialCaller? =
        withContext(Dispatchers.IO) { CredentialCallers.read(getApplication(), callingAppInfo) }

    private fun completeAssertion(credential: PasskeyAssertionCredential) {
        val json = WebAuthnJson.buildAuthenticationResponse(credential, clientDataJSON)
        finish { PendingIntentHandler.setGetCredentialResponse(it, GetCredentialResponse(PublicKeyCredential(json))) }
    }

    private fun completeRegistration(credential: PasskeyRegistrationCredential) {
        val json = WebAuthnJson.buildRegistrationResponse(credential, clientDataJSON) ?: return refuse("Hisar could not read the new passkey.")
        finish { PendingIntentHandler.setCreateCredentialResponse(it, CreatePublicKeyCredentialResponse(json)) }
    }

    private fun rejectExcludedRegistration() {
        finish {
            PendingIntentHandler.setCreateCredentialException(
                it, CreatePublicKeyCredentialDomException(InvalidStateError(), "Hisar already has a passkey for this account. Sign in with it instead."),
            )
        }
    }

    private fun completeUnlock() {
        viewModelScope.launch {
            val request = PendingIntentHandler.retrieveBeginGetCredentialRequest(intent) ?: return@launch refuse("Hisar did not make this request.")
            val response = withContext(Dispatchers.IO) {
                val caller = request.callingAppInfo?.let { CredentialCallers.read(getApplication(), it) }
                val allOption = CredentialEntries.readOptions(request.beginGetCredentialOptions)
                val offer = if (caller == null) CredentialOffer.Nothing else access.offer(caller, allOption.map { it.second })
                CredentialEntries.buildResponse(getApplication(), offer, allOption)
            }
            finish { PendingIntentHandler.setBeginGetCredentialResponse(it, response) }
        }
    }

    private fun completePassword(credential: LoginCredential) {
        if (credential.password.isEmpty()) return refuse("This login has no password.")
        finish { PendingIntentHandler.setGetCredentialResponse(it, GetCredentialResponse(PasswordCredential(credential.username, credential.password))) }
    }

    private fun copyOneTimeCode(code: String) {
        if (hisar.copySecret(code)) {
            Toast.makeText(getApplication(), "One-time code copied. It clears in about 2 minutes.", Toast.LENGTH_LONG).show()
        }
    }

    private fun cancel() {
        finish {
            when (pick) {
                CredentialPick.Create -> PendingIntentHandler.setCreateCredentialException(it, CreateCredentialCancellationException())
                CredentialPick.Unlock, null -> Unit
                else -> PendingIntentHandler.setGetCredentialException(it, GetCredentialCancellationException())
            }
        }
    }

    private fun refuse(message: String) {
        finish {
            when (pick) {
                CredentialPick.Create -> PendingIntentHandler.setCreateCredentialException(it, CreateCredentialUnknownException(message))
                CredentialPick.Unlock, null -> PendingIntentHandler.setBeginGetCredentialResponse(it, BeginGetCredentialResponse())
                else -> PendingIntentHandler.setGetCredentialException(it, GetCredentialUnknownException(message))
            }
        }
    }

    private fun finish(write: (Intent) -> Unit) {
        if (result.value == null) result.value = Intent().also(write)
    }

    private companion object {
        const val CLIENT_DATA_GET = "webauthn.get"
        const val CLIENT_DATA_CREATE = "webauthn.create"
    }
}

/** Opened by an entry of Credential Manager's sheet: unlocks Hisar, saves or signs with a passkey, or hands out a password. */
class CredentialActivity : FragmentActivity() {
    private val viewModel: CredentialViewModel by viewModels {
        viewModelFactory { initializer { CredentialViewModel(application, intent) } }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        if (!ScreenshotSetting(this).isAllowed) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        lifecycleScope.launch {
            viewModel.isContentShown.first { it }
            setContent { AutoFillRoot(viewModel.model) }
        }
        lifecycleScope.launch {
            val result = viewModel.result.filterNotNull().first()
            setResult(if (result.extras == null) RESULT_CANCELED else RESULT_OK, result)
            finish()
        }
    }
}
