package com.tryhisar.hisar.model

import java.net.IDN
import java.net.URI
import java.net.URISyntaxException
import java.net.URLDecoder
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.util.Base64
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** The plain-text Emergency Kit payload, or a legacy printed sign-in URI. */
data class SignInLink(val server: String, val email: String, val secretKey: String) {
    companion object {
        fun parse(text: String): SignInLink? {
            if (text.startsWith("HISARKIT")) return parsePayload(text)
            val link = parseHisarURI(text, host = "sign-in") ?: return null
            fun readValue(name: String) = link.readQueryValue(name).orEmpty()
            val signInLink = SignInLink(server = readValue("server"), email = readValue("email"), secretKey = readValue("secretKey"))
            return signInLink.takeUnless { it.server.isEmpty() && it.email.isEmpty() && it.secretKey.isEmpty() }
        }

        private fun parsePayload(text: String): SignInLink? {
            if (!text.startsWith("HISARKIT1 ")) return null
            val encoded = text.removePrefix("HISARKIT1 ")
            if (!Regex("^[A-Za-z0-9_-]+$").matches(encoded)) return null
            return runCatching {
                val bytes = Base64.getUrlDecoder().decode(encoded)
                if (Base64.getUrlEncoder().withoutPadding().encodeToString(bytes) != encoded) return null
                val json = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
                    .decode(ByteBuffer.wrap(bytes)).toString()
                val fields = Json.parseToJsonElement(json) as? JsonObject ?: return null
                if (fields.keys != setOf("server", "email", "secretKey")) return null
                fun read(name: String): String? = (fields[name] as? JsonPrimitive)?.takeIf { it.isString }?.content
                val result = SignInLink(read("server") ?: return null, read("email") ?: return null, read("secretKey") ?: return null)
                if (!isServerAddress(result.server) || result.email.isBlank()) return null
                val normalizedKey = result.secretKey.trim().uppercase().replace(Regex("[- ]"), "")
                    .replace('O', '0').replace('I', '1').replace('L', '1')
                if (!Regex("^H1[0123456789ABCDEFGHJKMNPQRSTVWXYZ]{31}[048CGMRW]$").matches(normalizedKey)) return null
                result
            }.getOrNull()
        }

        private fun isServerAddress(text: String): Boolean {
            val server = parseHTTPURI(text) ?: return false
            return '@' !in server.rawAuthority && server.fragment == null
        }
    }
}

/**
 * What Create account accepts: [invite] is the bare code for the default server, otherwise the invite link on its
 * server. [server] is that server's resolved origin. When an organization or vault share invitation sent the link,
 * [invitation] is [INVITATION_ORGANIZATION] or [INVITATION_SHARE] and [email] is the address it is for: an app signed
 * in to that account on that server shows the invitation instead of creating one.
 */
data class InviteLink(
    val invite: String,
    val server: String = DEFAULT_SERVER,
    val invitation: String? = null,
    val email: String = "",
) {
    companion object {
        const val DEFAULT_SERVER = "https://eu.tryhisar.com"
        const val INVITATION_ORGANIZATION = "organization"
        const val INVITATION_SHARE = "share"
        private val codePattern = Regex("^[A-Za-z0-9_-]+$")

        /** hisar://invite/<code>?server=<origin>[&invitation=organization|share&email=<address>], from Open in Hisar. */
        fun parse(text: String): InviteLink? {
            val link = parseHisarURI(text, host = "invite") ?: return null
            val code = link.path.orEmpty().removePrefix("/")
            if (!codePattern.matches(code)) return null
            val serverText = link.readQueryValue("server").orEmpty()
            val server = if (serverText.isEmpty()) DEFAULT_SERVER else resolveServer(serverText) ?: return null
            val invite = if (server == DEFAULT_SERVER) code else "$server/invite/$code"
            // An unknown invitation is a plain invite, as in an older app.
            val invitation = link.readQueryValue("invitation")?.takeIf { it == INVITATION_ORGANIZATION || it == INVITATION_SHARE }
            val email = if (invitation == null) "" else link.readQueryValue("email").orEmpty().trim().lowercase()
            return InviteLink(invite = invite, server = server, invitation = invitation, email = email)
        }

        private fun resolveServer(text: String): String? {
            var server = text.trimmingWhitespacesAndNewlines().trimEnd('/')
            if (server.isNotEmpty() && "://" !in server) server = "https://$server"
            if (parseHTTPURI(server) == null) return null
            return server
        }
    }
}

/**
 * Parses an HTTP(S) URI. java.net.URI reads an internationalized host only as a registry name, so that host is checked
 * in its ASCII form but kept as written: java.net.IDN is IDNA2003, which turns faß.de into the other domain fass.de.
 */
internal fun parseHTTPURI(text: String): URI? {
    return try {
        val url = URI(text)
        if (!url.scheme.equals("https", ignoreCase = true) && !url.scheme.equals("http", ignoreCase = true)) return null
        if (!url.host.isNullOrEmpty()) return url
        val authority = url.rawAuthority ?: return null
        val hostStart = authority.lastIndexOf('@') + 1
        val hostEnd = authority.indexOf(':', hostStart).takeIf { it >= 0 } ?: authority.length
        val host = IDN.toASCII(authority.substring(hostStart, hostEnd), IDN.USE_STD3_ASCII_RULES)
        val authorityStart = url.scheme.length + 3
        url.takeIf { !URI(text.replaceRange(authorityStart + hostStart, authorityStart + hostEnd, host)).host.isNullOrEmpty() }
    } catch (_: URISyntaxException) {
        null
    } catch (_: IllegalArgumentException) {
        null
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
