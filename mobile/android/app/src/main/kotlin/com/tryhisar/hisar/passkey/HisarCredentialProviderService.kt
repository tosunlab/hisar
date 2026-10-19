package com.tryhisar.hisar.passkey

import android.os.Build
import android.os.CancellationSignal
import android.os.OutcomeReceiver
import android.util.Log
import androidx.annotation.RequiresApi
import androidx.credentials.exceptions.ClearCredentialException
import androidx.credentials.exceptions.CreateCredentialException
import androidx.credentials.exceptions.CreateCredentialNoCreateOptionException
import androidx.credentials.exceptions.GetCredentialException
import androidx.credentials.exceptions.GetCredentialUnknownException
import androidx.credentials.provider.BeginCreateCredentialRequest
import androidx.credentials.provider.BeginCreateCredentialResponse
import androidx.credentials.provider.BeginCreatePublicKeyCredentialRequest
import androidx.credentials.provider.BeginGetCredentialRequest
import androidx.credentials.provider.BeginGetCredentialResponse
import androidx.credentials.provider.CredentialProviderService
import androidx.credentials.provider.ProviderClearCredentialStateRequest
import com.tryhisar.hisar.HisarApplication
import com.tryhisar.hisar.autofill.HttpAssetLinksVerifier
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import com.tryhisar.hisar.model.VaultStatus
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * Hisar's Credential Manager provider (API 34 and later), in the app's process and on its one backend. The begin calls
 * answer from the vault without any screen: the passkeys and logins the caller may use, the "Unlock Hisar" action while
 * locked, or Hisar as a place to save a new passkey. Every entry opens [CredentialActivity].
 */
@RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
class HisarCredentialProviderService : CredentialProviderService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val hisar get() = application as HisarApplication

    private fun createAccess() = CredentialAccess(hisar.backend, hisar.relyingPartyVerifier, HttpAssetLinksVerifier)

    override fun onBeginGetCredentialRequest(
        request: BeginGetCredentialRequest,
        cancellationSignal: CancellationSignal,
        callback: OutcomeReceiver<BeginGetCredentialResponse, GetCredentialException>,
    ) {
        val job = scope.launch {
            val response = try {
                val caller = request.callingAppInfo?.let { CredentialCallers.read(this@HisarCredentialProviderService, it) }
                val allOption = CredentialEntries.readOptions(request.beginGetCredentialOptions)
                val offer = if (caller == null) CredentialOffer.Nothing else createAccess().offer(caller, allOption.map { it.second })
                CredentialEntries.buildResponse(this@HisarCredentialProviderService, offer, allOption, isUserVerified = false)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                Log.e("Hisar", "Credential Manager get request failed", error)
                return@launch callback.onError(GetCredentialUnknownException(error.message))
            }
            callback.onResult(response)
        }
        cancellationSignal.setOnCancelListener { job.cancel() }
    }

    override fun onBeginCreateCredentialRequest(
        request: BeginCreateCredentialRequest,
        cancellationSignal: CancellationSignal,
        callback: OutcomeReceiver<BeginCreateCredentialResponse, CreateCredentialException>,
    ) {
        if (request !is BeginCreatePublicKeyCredentialRequest) {
            return callback.onError(CreateCredentialNoCreateOptionException("Hisar saves passkeys; save passwords in the Hisar app."))
        }
        val job = scope.launch {
            val accountName = try {
                findAccountName(request)
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                Log.e("Hisar", "Credential Manager create request failed", error)
                null
            }
            if (accountName == null) {
                callback.onError(CreateCredentialNoCreateOptionException("Hisar cannot save this passkey."))
            } else {
                callback.onResult(BeginCreateCredentialResponse(createEntries = listOf(CredentialEntries.buildCreateEntry(this@HisarCredentialProviderService, accountName))))
            }
        }
        cancellationSignal.setOnCancelListener { job.cancel() }
    }

    // The account the passkey would be saved to, or null when Hisar must not offer to save it.
    private fun findAccountName(request: BeginCreatePublicKeyCredentialRequest): String? {
        val caller = request.callingAppInfo?.let { CredentialCallers.read(this, it) } ?: return null
        val webAuthn = WebAuthnJson.parseCreateRequest(request.requestJson) ?: return null
        if (!PasskeyRegistrationRequest.isAlgorithmSupported(webAuthn.allAlgorithm)) return null
        createAccess().resolveRelyingParty(caller, webAuthn.relyingPartyID) ?: return null
        val status = hisar.backend.getStatus()
        if (status.updateRequired != null || status.state == VaultStatus.STATE_SIGNED_OUT) return null
        return status.email.ifEmpty { "Hisar" }
    }

    override fun onClearCredentialStateRequest(
        request: ProviderClearCredentialStateRequest,
        cancellationSignal: CancellationSignal,
        callback: OutcomeReceiver<Void?, ClearCredentialException>,
    ) {
        callback.onResult(null)
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }
}
