package com.tryhisar.hisar.model

import java.net.URI
import java.net.URISyntaxException

object VaultLogic {
    /** The http or https URL a typed website means, adding https:// when it has no scheme, or null when it is none. */
    fun convertToOpenableURL(text: String): URI? {
        val trimmedText = text.trimmingWhitespacesAndNewlines()
        if (trimmedText.isEmpty()) return null
        val fullText = if ("://" in trimmedText) trimmedText else "https://$trimmedText"
        val url = try {
            URI(fullText)
        } catch (_: URISyntaxException) {
            return null
        }
        if (url.scheme?.lowercase() !in listOf("http", "https") || url.host == null) return null
        return url
    }

    fun extractHost(text: String): String? = convertToOpenableURL(text)?.host?.lowercase()
}
