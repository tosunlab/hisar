package com.tryhisar.hisar.platform

import android.app.Activity
import android.util.Log
import androidx.credentials.providerevents.ProviderEventsManager
import androidx.credentials.providerevents.exception.ImportCredentialsCancellationException
import androidx.credentials.providerevents.exception.ImportCredentialsException
import androidx.credentials.providerevents.exception.ImportCredentialsInvalidJsonException
import androidx.credentials.providerevents.exception.ImportCredentialsNoExportOptionException
import androidx.credentials.providerevents.exception.ImportCredentialsProviderConfigurationException
import androidx.credentials.providerevents.exception.ImportCredentialsSystemErrorException
import androidx.credentials.providerevents.exception.ImportCredentialsUnknownCallerException
import androidx.credentials.providerevents.transfer.CredentialTypes
import androidx.credentials.providerevents.transfer.ImportCredentialsRequest
import com.tryhisar.hisar.app.CredentialExchangeReceiver
import com.tryhisar.hisar.app.CredentialExchangeResult
import kotlinx.coroutines.CancellationException

/**
 * Hisar as the importer of Credential Manager's credential transfer: Google Play services lists the apps that registered
 * an export of these credential types, the user picks one, and that app hands over the Credential Exchange (CXF) JSON,
 * which core/importer reads.
 */
class CredentialTransfer(private val activity: Activity) : CredentialExchangeReceiver {
    override suspend fun receive(): CredentialExchangeResult = try {
        val response = ProviderEventsManager.create(activity).importCredentials(activity, buildRequest())
        CredentialExchangeResult.Received(response.response.responseJson.encodeToByteArray())
    } catch (error: CancellationException) {
        throw error
    } catch (error: ImportCredentialsException) {
        Log.i("Hisar", "Credential transfer: ${error.type}")
        describe(error)
    } catch (error: Exception) {
        Log.e("Hisar", "Credential transfer failed", error)
        CredentialExchangeResult.Failed(UNKNOWN_ERROR_MESSAGE)
    }

    companion object {
        /**
         * Asks for the CXF credential types core/importer reads. Files have no place in Hisar, and an item reference only
         * links two items of the export. Hisar knows no CXF extension, so it asks for none.
         */
        private fun buildRequest() = ImportCredentialsRequest(
            credentialTypes = setOf(
                CredentialTypes.CREDENTIAL_TYPE_BASIC_AUTH,
                CredentialTypes.CREDENTIAL_TYPE_PUBLIC_KEY,
                CredentialTypes.CREDENTIAL_TYPE_TOTP,
                CredentialTypes.CREDENTIAL_TYPE_GENERATED_PASSWORD,
                CredentialTypes.CREDENTIAL_TYPE_NOTE,
                CredentialTypes.CREDENTIAL_TYPE_CUSTOM_FIELDS,
                CredentialTypes.CREDENTIAL_TYPE_CREDIT_CARD,
                CredentialTypes.CREDENTIAL_TYPE_API_KEY,
                CredentialTypes.CREDENTIAL_TYPE_ADDRESS,
                CredentialTypes.CREDENTIAL_TYPE_PERSON_NAME,
                CredentialTypes.CREDENTIAL_TYPE_IDENTITY_DOCUMENT,
                CredentialTypes.CREDENTIAL_TYPE_PASSPORT,
                CredentialTypes.CREDENTIAL_TYPE_DRIVERS_LICENSE,
                CredentialTypes.CREDENTIAL_TYPE_WIFI,
                CredentialTypes.CREDENTIAL_TYPE_SSH_KEY,
            ),
            knownExtensions = emptySet(),
        )

        /** The cache folder through which the library receives the other app's JSON (its import_export_filepaths). */
        const val CREDENTIAL_TRANSFER_FOLDER_NAME = "import_export_temp"

        const val UNKNOWN_ERROR_MESSAGE = "Hisar couldn't receive the items from the other app. Try again."

        /** The sentence for each way the transfer fails; closing the system's list or the other app says nothing. */
        fun describe(error: ImportCredentialsException): CredentialExchangeResult = when (error) {
            is ImportCredentialsCancellationException -> CredentialExchangeResult.Cancelled
            is ImportCredentialsNoExportOptionException -> CredentialExchangeResult.Failed(
                "No app on this device offered its passwords and passkeys to Hisar, or the app you chose declined. " +
                    "Update the other app, check that it can export to another app, and try again.",
            )
            is ImportCredentialsProviderConfigurationException -> CredentialExchangeResult.Failed(
                "This device can't move passwords between apps yet. Update Google Play services and try again.",
            )
            is ImportCredentialsInvalidJsonException -> CredentialExchangeResult.Failed(
                "The other app sent items Hisar couldn't read. Update the other app and export again.",
            )
            is ImportCredentialsSystemErrorException -> CredentialExchangeResult.Failed(
                "Android couldn't pass on the items from the other app. Try again.",
            )
            is ImportCredentialsUnknownCallerException -> CredentialExchangeResult.Failed(
                "Android couldn't confirm that this import comes from Hisar. Install Hisar from Google Play and try again.",
            )
            else -> CredentialExchangeResult.Failed(UNKNOWN_ERROR_MESSAGE)
        }
    }
}
