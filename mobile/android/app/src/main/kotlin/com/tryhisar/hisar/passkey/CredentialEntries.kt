package com.tryhisar.hisar.passkey

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.annotation.RequiresApi
import androidx.credentials.provider.Action
import androidx.credentials.provider.AuthenticationAction
import androidx.credentials.provider.BeginGetCredentialOption
import androidx.credentials.provider.BeginGetCredentialResponse
import androidx.credentials.provider.BeginGetPasswordOption
import androidx.credentials.provider.BeginGetPublicKeyCredentialOption
import androidx.credentials.provider.CreateEntry
import androidx.credentials.provider.PasswordCredentialEntry
import androidx.credentials.provider.PublicKeyCredentialEntry
import com.tryhisar.hisar.model.Base64URL
import java.util.UUID
import kotlinx.io.bytestring.ByteString

/** What an entry of the system's sheet asks [CredentialActivity] to do, carried in the entry's pending intent. */
@RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
sealed interface CredentialPick {
    data object Unlock : CredentialPick

    data object Search : CredentialPick

    data object SearchToOffer : CredentialPick

    data class SelectedPassword(val recordIdentifier: String) : CredentialPick

    data class Passkey(val credentialID: ByteString) : CredentialPick

    /** A login; its match is checked again against the caller before its password leaves. */
    data class Password(val recordIdentifier: String) : CredentialPick

    data object Create : CredentialPick

    companion object {
        private const val EXTRA_PICK = "com.tryhisar.hisar.passkey.PICK"
        private const val EXTRA_CREDENTIAL_ID = "com.tryhisar.hisar.passkey.CREDENTIAL_ID"
        private const val EXTRA_RECORD = "com.tryhisar.hisar.passkey.RECORD"

        fun read(intent: Intent): CredentialPick? = when (intent.getStringExtra(EXTRA_PICK)) {
            "unlock" -> Unlock
            "search" -> Search
            "search-to-offer" -> SearchToOffer
            "selected-password" -> intent.getStringExtra(EXTRA_RECORD)?.let(::SelectedPassword)
            "passkey" -> intent.getStringExtra(EXTRA_CREDENTIAL_ID)?.let(Base64URL::decode)?.let { Passkey(it) }
            "password" -> intent.getStringExtra(EXTRA_RECORD)?.let(::Password)
            "create" -> Create
            else -> null
        }

        private fun write(pick: CredentialPick, intent: Intent): Intent = when (pick) {
            Unlock -> intent.putExtra(EXTRA_PICK, "unlock")
            Search -> intent.putExtra(EXTRA_PICK, "search")
            SearchToOffer -> intent.putExtra(EXTRA_PICK, "search-to-offer")
            is SelectedPassword -> intent.putExtra(EXTRA_PICK, "selected-password").putExtra(EXTRA_RECORD, pick.recordIdentifier)
            is Passkey -> intent.putExtra(EXTRA_PICK, "passkey").putExtra(EXTRA_CREDENTIAL_ID, Base64URL.encode(pick.credentialID))
            is Password -> intent.putExtra(EXTRA_PICK, "password").putExtra(EXTRA_RECORD, pick.recordIdentifier)
            Create -> intent.putExtra(EXTRA_PICK, "create")
        }

        // Mutable, because the system adds the request to the intent. A random identifier per entry keeps every intent
        // apart, also from those an earlier process of Hisar handed to a sheet that is still open, so none is rebound.
        fun buildPendingIntent(context: Context, pick: CredentialPick): PendingIntent = PendingIntent.getActivity(
            context,
            0,
            write(pick, Intent(context, CredentialActivity::class.java).setIdentifier(UUID.randomUUID().toString())),
            PendingIntent.FLAG_MUTABLE,
        )
    }
}

/** Between the platform's request and response types and [CredentialAccess]. */
@RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
object CredentialEntries {
    /** The options Hisar answers, each with the platform option it came from; others are left to other providers. */
    fun readOptions(allOption: List<BeginGetCredentialOption>): List<Pair<BeginGetCredentialOption, CredentialOption>> = allOption.mapNotNull { option ->
        when (option) {
            is BeginGetPublicKeyCredentialOption -> WebAuthnJson.parseGetRequest(option.requestJson)?.let { option to CredentialOption.Passkey(it) }
            is BeginGetPasswordOption -> option to CredentialOption.Password
            else -> null
        }
    }

    /** Entries never carry verification evidence; final selection verifies the concrete challenge. */
    fun buildResponse(
        context: Context,
        offer: CredentialOffer,
        allOption: List<Pair<BeginGetCredentialOption, CredentialOption>>,
    ): BeginGetCredentialResponse {
        val hasPassword = allOption.any { it.second == CredentialOption.Password }
        val needsSearchGateway = hasPassword && when (offer) {
            CredentialOffer.Unlock -> allOption.none { it.second is CredentialOption.Passkey }
            is CredentialOffer.Entries -> offer.allLogin.isEmpty() && offer.allPasskey.isEmpty()
            CredentialOffer.Nothing -> false
        }
        val searchGateway = if (needsSearchGateway) {
            listOf(AuthenticationAction("Search Hisar", CredentialPick.buildPendingIntent(context, CredentialPick.SearchToOffer)))
        } else {
            emptyList()
        }
        val actions = if (offer != CredentialOffer.Nothing && hasPassword && !needsSearchGateway) {
            listOf(Action("Search Hisar", CredentialPick.buildPendingIntent(context, CredentialPick.Search), "Search all logins"))
        } else {
            emptyList()
        }
        return when (offer) {
            CredentialOffer.Nothing -> BeginGetCredentialResponse()
            CredentialOffer.Unlock -> BeginGetCredentialResponse(
                authenticationActions = if (needsSearchGateway) searchGateway else listOf(AuthenticationAction("Unlock Hisar", CredentialPick.buildPendingIntent(context, CredentialPick.Unlock))),
                actions = actions,
            )
            is CredentialOffer.Entries -> BeginGetCredentialResponse(
                credentialEntries = offer.allPasskey.mapNotNull { (index, passkey, displayName) ->
                    val credentialID = Base64URL.decode(passkey.credentialID) ?: return@mapNotNull null
                    PublicKeyCredentialEntry.Builder(
                        context,
                        passkey.userName.ifEmpty { passkey.userDisplayName.ifEmpty { passkey.title } },
                        CredentialPick.buildPendingIntent(context, CredentialPick.Passkey(credentialID)),
                        allOption[index].first as BeginGetPublicKeyCredentialOption,
                    ).setDisplayName(displayName).build()
                } + offer.allLogin.map { (index, match, displayName) ->
                    PasswordCredentialEntry.Builder(
                        context,
                        match.login.username.ifEmpty { match.login.title },
                        CredentialPick.buildPendingIntent(context, CredentialPick.Password(match.login.id)),
                        allOption[index].first as BeginGetPasswordOption,
                    ).setDisplayName(displayName).build()
                },
                actions = actions,
                authenticationActions = searchGateway,
            )
        }
    }

    fun buildCreateEntry(context: Context, accountName: String): CreateEntry =
        CreateEntry(accountName, CredentialPick.buildPendingIntent(context, CredentialPick.Create))
}
