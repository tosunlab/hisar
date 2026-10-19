package com.tryhisar.hisar.platform

import androidx.credentials.providerevents.exception.ImportCredentialsCancellationException
import androidx.credentials.providerevents.exception.ImportCredentialsInvalidJsonException
import androidx.credentials.providerevents.exception.ImportCredentialsNoExportOptionException
import androidx.credentials.providerevents.exception.ImportCredentialsProviderConfigurationException
import androidx.credentials.providerevents.exception.ImportCredentialsSystemErrorException
import androidx.credentials.providerevents.exception.ImportCredentialsUnknownCallerException
import androidx.credentials.providerevents.exception.ImportCredentialsUnknownErrorException
import com.tryhisar.hisar.app.CredentialExchangeResult
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** How the import from another app reads each way Android's credential transfer fails. */
class CredentialTransferTests {
    @Test
    fun testClosingTheTransferSaysNothing() {
        assertEquals(CredentialExchangeResult.Cancelled, CredentialTransfer.describe(ImportCredentialsCancellationException("closed")))
    }

    @Test
    fun testEveryTransferFailureHasItsOwnSentence() {
        val allFailure = listOf(
            ImportCredentialsNoExportOptionException("no entry"),
            ImportCredentialsProviderConfigurationException("not supported"),
            ImportCredentialsInvalidJsonException("not json"),
            ImportCredentialsSystemErrorException("binder"),
            ImportCredentialsUnknownCallerException("caller"),
            ImportCredentialsUnknownErrorException("unknown"),
        )
        val allMessage = allFailure.map { failure ->
            val result = CredentialTransfer.describe(failure)
            assertTrue(result is CredentialExchangeResult.Failed, "${failure.type} is a failure")
            assertTrue(failure.errorMessage.orEmpty() !in result.message, "${failure.type} speaks to the user, not in the system's words")
            result.message
        }
        assertEquals(allMessage.size, allMessage.toSet().size, "each failure says what went wrong: $allMessage")
        assertEquals(CredentialTransfer.UNKNOWN_ERROR_MESSAGE, allMessage.last())
    }
}
