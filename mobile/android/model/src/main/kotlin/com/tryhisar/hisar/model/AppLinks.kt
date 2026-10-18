package com.tryhisar.hisar.model

import java.net.URI
import java.net.URISyntaxException
import java.net.URLDecoder

/** hisar://sign-in?server=…&email=…&secretKey=…, from the Emergency Kit's QR code. */
data class SignInLink(val server: String, val email: String, val secretKey: String) {
    companion object {
        fun parse(text: String): SignInLink? {
            val link = parseHisarURI(text, host = "sign-in") ?: return null
            fun readValue(name: String) = link.readQueryValue(name)?.trimmingWhitespaces().orEmpty()
            val signInLink = SignInLink(server = readValue("server"), email = readValue("email"), secretKey = readValue("secretKey"))
            return signInLink.takeUnless { it.server.isEmpty() && it.email.isEmpty() && it.secretKey.isEmpty() }
        }
    }
}

/** What Create account accepts: the bare code for the default server, otherwise the invite link on its server. */
data class InviteLink(val invite: String) {
    companion object {
        const val DEFAULT_SERVER = "https://api.tryhisar.com"
        private val codePattern = Regex("^[A-Za-z0-9_-]+$")

        /** hisar://invite/<code>?server=<origin>, from the invite page's Open in Hisar button. */
        fun parse(text: String): InviteLink? {
            val link = parseHisarURI(text, host = "invite") ?: return null
            val code = link.path.orEmpty().removePrefix("/")
            if (!codePattern.matches(code)) return null
            val serverText = link.readQueryValue("server").orEmpty()
            if (serverText.isEmpty()) return InviteLink(code)
            val server = resolveServer(serverText) ?: return null
            return InviteLink(if (server == DEFAULT_SERVER) code else "$server/invite/$code")
        }

        private fun resolveServer(text: String): String? {
            var server = text.trimmingWhitespacesAndNewlines().trimEnd('/')
            if (server.isNotEmpty() && "://" !in server) server = "https://$server"
            val url = try {
                URI(server)
            } catch (_: URISyntaxException) {
                return null
            }
            if (url.scheme?.lowercase() !in listOf("https", "http") || url.host.isNullOrEmpty()) return null
            return server
        }
    }
}

private fun parseHisarURI(text: String, host: String): URI? {
    val url = try {
        URI(text.trimmingWhitespacesAndNewlines())
    } catch (_: URISyntaxException) {
        return null
    }
    return url.takeIf { it.scheme?.lowercase() == "hisar" && it.host?.lowercase() == host }
}

// As Swift's URLComponents.queryItems: percent escapes are decoded, a plus stays a plus.
private fun URI.readQueryValue(name: String): String? = rawQuery.orEmpty().split('&')
    .map { it.split('=', limit = 2) }
    .firstOrNull { it.first() == name }
    ?.let { URLDecoder.decode(it.getOrElse(1) { "" }.replace("+", "%2B"), Charsets.UTF_8) }
