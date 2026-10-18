package com.tryhisar.hisar.autofill

import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * Asks websites for their Digital Asset Links directly over HTTPS. Answers are kept for an hour per address, so a fill
 * request does not wait on the network each time, and every candidate is asked at once, since the system gives a fill
 * request about five seconds.
 */
object HttpAssetLinksVerifier : AppWebsiteVerifier {
    private const val CACHE_MILLISECONDS = 60 * 60 * 1000L
    private const val TIMEOUT_MILLISECONDS = 2_000
    private const val MAXIMUM_BYTES = 256 * 1024

    private class Answer(val statementJSON: String, val fetchedAt: Long)

    private val answerByURL = ConcurrentHashMap<String, Answer>()
    private val executor = Executors.newCachedThreadPool()

    override fun listAllVouchingWebsite(app: AndroidApp, allWebsite: List<LoginWebsite>): List<String> {
        val allCandidate = AssetLinks.listAllCandidate(app, allWebsite)
        val allAnswer = allCandidate.map { candidate -> executor.submit<String> { readStatements(candidate.statementURL) } }
        return allCandidate.zip(allAnswer).mapNotNull { (candidate, answer) ->
            val statementJSON = runCatching { answer.get(TIMEOUT_MILLISECONDS * 2L, TimeUnit.MILLISECONDS) }.getOrDefault("")
            candidate.serviceIdentifier.takeIf { AssetLinks.isVouchingFor(statementJSON, app) }
        }
    }

    // The file must be served as is: no redirect, no error page, nothing larger than a statement list needs.
    private fun readStatements(url: String): String {
        val now = System.currentTimeMillis()
        answerByURL[url]?.takeIf { now - it.fetchedAt < CACHE_MILLISECONDS }?.let { return it.statementJSON }
        val statementJSON = try {
            val connection = URL(url).openConnection() as HttpURLConnection
            try {
                connection.instanceFollowRedirects = false
                connection.connectTimeout = TIMEOUT_MILLISECONDS
                connection.readTimeout = TIMEOUT_MILLISECONDS
                connection.setRequestProperty("Accept", "application/json")
                if (connection.responseCode != HttpURLConnection.HTTP_OK) {
                    ""
                } else {
                    val bytes = connection.inputStream.use { it.readNBytes(MAXIMUM_BYTES + 1) }
                    if (bytes.size > MAXIMUM_BYTES) "" else bytes.decodeToString()
                }
            } finally {
                connection.disconnect()
            }
        } catch (_: IOException) {
            return ""
        }
        answerByURL[url] = Answer(statementJSON, now)
        return statementJSON
    }
}
