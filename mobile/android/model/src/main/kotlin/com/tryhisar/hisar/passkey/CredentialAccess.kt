package com.tryhisar.hisar.passkey

import com.tryhisar.hisar.autofill.AndroidApp
import com.tryhisar.hisar.autofill.AppWebsiteVerifier
import com.tryhisar.hisar.autofill.FillTarget
import com.tryhisar.hisar.autofill.LoginMatcher
import com.tryhisar.hisar.autofill.MatchedLogin
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.StoredPasskey
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultStatus
import kotlinx.io.bytestring.ByteString

/** Who asks Credential Manager for a credential, as the platform names it. */
sealed interface CredentialCaller {
    /** A browser on the privileged list, asking for a page of [origin]. It writes the client data and sends its hash. */
    data class Browser(val packageName: String, val origin: String) : CredentialCaller

    /** A native app with the SHA-256 of its signing certificate. Hisar writes its client data. */
    data class App(val app: AndroidApp, val label: String) : CredentialCaller
}

/** Answers whether a relying party's Digital Asset Links let an app use its credentials. */
fun interface RelyingPartyVerifier {
    /** Blocks on the network; a site that cannot be asked does not vouch. */
    fun isVouchingFor(relyingPartyID: String, app: AndroidApp): Boolean
}

/** A credential the caller asked for, in the order of its options. */
sealed interface CredentialOption {
    data class Passkey(val request: WebAuthnGetRequest) : CredentialOption

    data object Password : CredentialOption
}

/** The client data hash a passkey signs, with the client data JSON when Hisar wrote it for a native app. */
data class AnswerClientData(val hash: ByteString, val json: ByteString?)

/** What Hisar offers in the system's sheet for a get request. */
sealed interface CredentialOffer {
    /** Nothing: the caller may not use these credentials, or Hisar is signed out or needs an update. */
    data object Nothing : CredentialOffer

    /** One "Unlock Hisar" action; the offer is made again after the unlock. */
    data object Unlock : CredentialOffer

    data class Entries(val allPasskey: List<Offered<StoredPasskey>>, val allLogin: List<Offered<MatchedLogin>>) : CredentialOffer
}

/** A credential and the index of the request option it answers. */
data class Offered<Value>(val optionIndex: Int, val value: Value)

/**
 * The rules for Credential Manager callers. A browser on the privileged list may use the relying parties of its page's
 * origin. A native app may use a relying party only when that site's Digital Asset Links name the app's package and
 * certificate, because the platform offers every passkey to any app that asks. Every call blocks on the backend or the
 * network.
 */
class CredentialAccess(
    private val backend: VaultBackend,
    private val relyingPartyVerifier: RelyingPartyVerifier,
    private val appWebsiteVerifier: AppWebsiteVerifier,
) {
    /** The relying party ID the caller may use for the requested one (empty: the browser page's own), or null. */
    fun resolveRelyingParty(caller: CredentialCaller, requestedRelyingPartyID: String): String? = when (caller) {
        is CredentialCaller.Browser -> try {
            backend.resolvePasskeyRelyingPartyID(caller.origin, requestedRelyingPartyID)
        } catch (_: BackendError) {
            null
        }
        is CredentialCaller.App -> requestedRelyingPartyID.lowercase().takeIf { it.isNotEmpty() && relyingPartyVerifier.isVouchingFor(it, caller.app) }
    }

    /** A browser's own hash, or client data Hisar writes for an app; null when a browser sent no hash. */
    fun buildClientData(caller: CredentialCaller, clientDataType: String, challenge: String, browserHash: ByteString?): AnswerClientData? =
        when (caller) {
            is CredentialCaller.Browser -> browserHash?.takeIf { it.size == CLIENT_DATA_HASH_SIZE }?.let { AnswerClientData(it, null) }
            is CredentialCaller.App -> {
                val origin = backend.buildAndroidAppOrigin(ByteString(parseFingerprint(caller.app.certificateSHA256)))
                backend.buildPasskeyClientData(clientDataType, challenge, origin, caller.app.packageName).let { AnswerClientData(it.hash, it.json) }
            }
        }

    /** The site or app whose logins a password request may use. */
    fun buildFillTarget(caller: CredentialCaller): FillTarget = when (caller) {
        is CredentialCaller.Browser -> FillTarget.Websites(listOf(caller.origin))
        is CredentialCaller.App -> FillTarget.App(caller.app, caller.label)
    }

    /** The passkeys and logins to offer, the unlock action, or nothing. */
    fun offer(caller: CredentialCaller, allOption: List<CredentialOption>): CredentialOffer {
        val allPasskeyRequest = allOption.withIndex().mapNotNull { (index, option) ->
            val request = (option as? CredentialOption.Passkey)?.request ?: return@mapNotNull null
            resolveRelyingParty(caller, request.relyingPartyID)?.let { Offered(index, request.copy(relyingPartyID = it)) }
        }
        val allPasswordIndex = allOption.indices.filter { allOption[it] == CredentialOption.Password }
        if (allPasskeyRequest.isEmpty() && allPasswordIndex.isEmpty()) return CredentialOffer.Nothing
        val status = backend.getStatus()
        return when {
            status.updateRequired != null -> CredentialOffer.Nothing
            status.state == VaultStatus.STATE_LOCKED -> CredentialOffer.Unlock
            !status.isUnlocked -> CredentialOffer.Nothing
            else -> CredentialOffer.Entries(
                allPasskey = allPasskeyRequest.flatMap { (index, request) ->
                    backend.listAllPasskeyForRelyingParty(request.relyingPartyID, request.allAllowedCredentialID).map { Offered(index, it) }
                },
                allLogin = allPasswordIndex.firstOrNull()?.let { index ->
                    LoginMatcher(backend, appWebsiteVerifier).listAllMatch(buildFillTarget(caller)).map { Offered(index, it) }
                }.orEmpty(),
            )
        }
    }

    private fun parseFingerprint(text: String): ByteArray = text.replace(":", "").chunked(2).map { it.toInt(16).toByte() }.toByteArray()

    private companion object {
        const val CLIENT_DATA_HASH_SIZE = 32
    }
}
