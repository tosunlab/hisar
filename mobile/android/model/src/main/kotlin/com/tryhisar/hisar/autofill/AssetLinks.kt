package com.tryhisar.hisar.autofill

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * Digital Asset Links from a website to an app, read the way Proton Pass reads them: the website's own
 * `/.well-known/assetlinks.json`, never Google's API, with a `get_login_creds` statement naming the app's package and
 * signing certificate.
 */
object AssetLinks {
    const val LOGIN_RELATION = "delegate_permission/common.get_login_creds"

    /** A site Hisar may ask, the service identifier its logins match on, and the address of its statement file. */
    data class Candidate(val serviceIdentifier: String, val statementURL: String)

    /**
     * The websites worth asking about an app: those whose registrable domain's name is a part of the app's package, such as
     * github.com for com.github.android. Asking a site tells it that the vault holds a login for it, so Hisar does not ask
     * every site of the vault; a site's answer, not this guess, decides. Registrable domains come before other hosts: they
     * are few and are asked first when a fill's time runs short.
     */
    fun listAllCandidate(app: AndroidApp, allWebsite: List<LoginWebsite>): List<Candidate> {
        val allPackagePart = app.packageName.lowercase().split('.').toSet()
        val allMatching = allWebsite.filter { website -> website.registrableDomain.substringBefore('.').let { it.isNotEmpty() && it in allPackagePart } }
        return (allMatching.map { it.registrableDomain } + allMatching.map { it.host })
            .filter { it.isNotEmpty() }
            .distinct()
            .map { Candidate("https://$it", "https://$it/.well-known/assetlinks.json") }
    }

    /** Whether a statement file lets the app use the site's logins. A file that is not a list of statements vouches for nothing. */
    fun isVouchingFor(statementJSON: String, app: AndroidApp): Boolean {
        val allStatement = try {
            Json.parseToJsonElement(statementJSON) as? JsonArray
        } catch (_: IllegalArgumentException) {
            null
        } ?: return false
        val certificate = normalizeFingerprint(app.certificateSHA256)
        return allStatement.filterIsInstance<JsonObject>().any { statement ->
            val target = statement["target"] as? JsonObject ?: return@any false
            readStrings(statement["relation"]).contains(LOGIN_RELATION) &&
                readString(target["namespace"]) == "android_app" &&
                readString(target["package_name"]) == app.packageName &&
                readStrings(target["sha256_cert_fingerprints"]).any { normalizeFingerprint(it) == certificate }
        }
    }

    private fun normalizeFingerprint(text: String): String = text.replace(":", "").uppercase()

    private fun readString(element: JsonElement?): String? = (element as? JsonPrimitive)?.takeIf { it.isString }?.content

    private fun readStrings(element: JsonElement?): List<String> = (element as? JsonArray)?.mapNotNull(::readString).orEmpty()
}
