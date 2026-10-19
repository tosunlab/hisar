package com.tryhisar.hisar.passkey

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.annotation.RequiresApi
import androidx.credentials.provider.AuthenticationAction
import androidx.credentials.provider.BeginGetCredentialOption
import androidx.credentials.provider.BeginGetCredentialResponse
import androidx.credentials.provider.BeginGetPasswordOption
import androidx.credentials.provider.BeginGetPublicKeyCredentialOption
import androidx.credentials.provider.CreateEntry
import androidx.credentials.provider.PasswordCredentialEntry
import androidx.credentials.provider.PublicKeyCredentialEntry
import com.tryhisar.hisar.model.Base64URL
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.io.bytestring.ByteString

/** What an entry of the system's sheet asks [CredentialActivity] to do, carried in the entry's pending intent. */
@RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
sealed interface CredentialPick {
    data object Unlock : CredentialPick

    /** A passkey; [isUserVerified] when the entry came from the unlock action, whose unlock verified the user for this request. */
    data class Passkey(val credentialID: ByteString, val isUserVerified: Boolean) : CredentialPick

    /** A login; its match is checked again against the caller before its password leaves. */
    data class Password(val recordIdentifier: String) : CredentialPick

    data object Create : CredentialPick

    companion object {
        private const val EXTRA_PICK = "com.tryhisar.hisar.passkey.PICK"
        private const val EXTRA_CREDENTIAL_ID = "com.tryhisar.hisar.passkey.CREDENTIAL_ID"
        private const val EXTRA_RECORD = "com.tryhisar.hisar.passkey.RECORD"
        private const val EXTRA_USER_VERIFIED = "com.tryhisar.hisar.passkey.USER_VERIFIED"

        fun read(intent: Intent): CredentialPick? = when (intent.getStringExtra(EXTRA_PICK)) {
            "unlock" -> Unlock
            "passkey" -> intent.getStringExtra(EXTRA_CREDENTIAL_ID)?.let(Base64URL::decode)?.let { Passkey(it, intent.getBooleanExtra(EXTRA_USER_VERIFIED, false)) }
            "password" -> intent.getStringExtra(EXTRA_RECORD)?.let(::Password)
            "create" -> Create
            else -> null
        }

        private fun write(pick: CredentialPick, intent: Intent): Intent = when (pick) {
            Unlock -> intent.putExtra(EXTRA_PICK, "unlock")
            is Passkey -> intent.putExtra(EXTRA_PICK, "passkey").putExtra(EXTRA_CREDENTIAL_ID, Base64URL.encode(pick.credentialID))
                .putExtra(EXTRA_USER_VERIFIED, pick.isUserVerified)
            is Password -> intent.putExtra(EXTRA_PICK, "password").putExtra(EXTRA_RECORD, pick.recordIdentifier)
            Create -> intent.putExtra(EXTRA_PICK, "create")
        }

        private val requestCodes = AtomicInteger()

        // Mutable, because the system adds the request to the intent; a request code per entry keeps the intents apart.
        fun buildPendingIntent(context: Context, pick: CredentialPick): PendingIntent = PendingIntent.getActivity(
            context,
            requestCodes.incrementAndGet(),
            write(pick, Intent(context, CredentialActivity::class.java)),
            PendingIntent.FLAG_MUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
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

    /** The sheet's entries; [isUserVerified] when the user just unlocked Hisar in this request's unlock action. */
    fun buildResponse(
        context: Context,
        offer: CredentialOffer,
        allOption: List<Pair<BeginGetCredentialOption, CredentialOption>>,
        isUserVerified: Boolean,
    ): BeginGetCredentialResponse =
        when (offer) {
            CredentialOffer.Nothing -> BeginGetCredentialResponse()
            CredentialOffer.Unlock -> BeginGetCredentialResponse(
                authenticationActions = listOf(AuthenticationAction("Unlock Hisar", CredentialPick.buildPendingIntent(context, CredentialPick.Unlock))),
            )
            is CredentialOffer.Entries -> BeginGetCredentialResponse(
                credentialEntries = offer.allPasskey.mapNotNull { (index, passkey) ->
                    val credentialID = Base64URL.decode(passkey.credentialID) ?: return@mapNotNull null
                    PublicKeyCredentialEntry.Builder(
                        context,
                        passkey.userName.ifEmpty { passkey.userDisplayName.ifEmpty { passkey.title } },
                        CredentialPick.buildPendingIntent(context, CredentialPick.Passkey(credentialID, isUserVerified)),
                        allOption[index].first as BeginGetPublicKeyCredentialOption,
                    ).setDisplayName(passkey.title).build()
                } + offer.allLogin.map { (index, match) ->
                    PasswordCredentialEntry.Builder(
                        context,
                        match.login.username.ifEmpty { match.login.title },
                        CredentialPick.buildPendingIntent(context, CredentialPick.Password(match.login.id)),
                        allOption[index].first as BeginGetPasswordOption,
                    ).setDisplayName(match.login.title).build()
                },
            )
        }

    fun buildCreateEntry(context: Context, accountName: String): CreateEntry =
        CreateEntry(accountName, CredentialPick.buildPendingIntent(context, CredentialPick.Create))
}
