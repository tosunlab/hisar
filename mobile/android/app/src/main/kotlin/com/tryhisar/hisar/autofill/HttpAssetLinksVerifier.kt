package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.passkey.RelyingPartyVerifier
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.TimeUnit

/**
 * Asks websites for Digital Asset Links over HTTPS through the shared [AssetLinksReader]. Password matching and
 * native passkey checks use the same eight workers, bounded queue and one-hour cache. Overload and transient failures
 * do not become cached denials.
 */
object HttpAssetLinksVerifier : AppWebsiteVerifier, RelyingPartyVerifier {
    private const val THREAD_COUNT = 8
    private const val TIMEOUT_MILLISECONDS = 2_000
    private const val MAXIMUM_BYTES = 256 * 1024

    private val reader = AssetLinksReader(THREAD_COUNT, ::fetchStatements)

    override fun listAllVouchingWebsite(app: AndroidApp, allWebsite: List<LoginWebsite>): List<String> {
        val allCandidate = AssetLinks.listAllCandidate(app, allWebsite)
        val allStatementJSON = reader.readAll(allCandidate.map { it.statementURL }, TIMEOUT_MILLISECONDS * 2L)
        return allCandidate.zip(allStatementJSON).mapNotNull { (candidate, statementJSON) ->
            candidate.serviceIdentifier.takeIf { AssetLinks.isVouchingFor(statementJSON, app) }
        }
    }

    override fun isVouchingFor(relyingPartyID: String, app: AndroidApp): Boolean =
        AssetLinks.isVouchingFor(reader.read("https://$relyingPartyID/.well-known/assetlinks.json"), app)

    // The file must be served as is: no redirect, no error page, nothing larger than a statement list needs, and its body
    // within a few seconds. Null when the site could not be asked.
    private fun fetchStatements(url: String): String? = try {
        val connection = URL(url).openConnection() as HttpURLConnection
        try {
            connection.instanceFollowRedirects = false
            connection.connectTimeout = TIMEOUT_MILLISECONDS
            connection.readTimeout = TIMEOUT_MILLISECONDS
            connection.setRequestProperty("Accept", "application/json")
            val status = connection.responseCode
            if (status >= 500 || status == HttpURLConnection.HTTP_CLIENT_TIMEOUT || status == 429) {
                null
            } else if (status != HttpURLConnection.HTTP_OK) {
                ""
            } else {
                val deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(TIMEOUT_MILLISECONDS * 2L)
                val bytes = connection.inputStream.use { it.buffered().readBytesUpTo(MAXIMUM_BYTES + 1, deadline) }
                if (bytes.size > MAXIMUM_BYTES) "" else bytes.decodeToString()
            }
        } finally {
            connection.disconnect()
        }
    } catch (_: IOException) {
        null
    }
}

private fun java.io.InputStream.readBytesUpTo(limit: Int, deadline: Long): ByteArray {
    val output = java.io.ByteArrayOutputStream()
    val buffer = ByteArray(8 * 1024)
    while (output.size() < limit) {
        if (System.nanoTime() > deadline) throw IOException("the statement file is too slow")
        val count = read(buffer, 0, minOf(buffer.size, limit - output.size()))
        if (count < 0) break
        output.write(buffer, 0, count)
    }
    return output.toByteArray()
}
