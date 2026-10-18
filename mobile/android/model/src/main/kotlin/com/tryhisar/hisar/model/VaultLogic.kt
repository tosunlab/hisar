package com.tryhisar.hisar.model

import java.net.URI
import java.net.URISyntaxException

/** An item with the vault it is in, as the lists that span vaults show it. */
data class VaultItemReference(val vaultID: String, val vaultName: String, val item: ItemDetail) {
    val id: String get() = LoginMatch.buildRecordIdentifier(vaultID, item.id)
}

object VaultLogic {
    fun sortAllItem(allItem: List<VaultItemReference>): List<VaultItemReference> =
        allItem.sortedWith(compareBy<VaultItemReference, String>(String.CASE_INSENSITIVE_ORDER) { it.item.title }.thenBy(String.CASE_INSENSITIVE_ORDER) { it.item.username })

    /** The Secret Key as the backend reads it: upper case, without the spaces and line breaks a paste brings. */
    fun formatSecretKey(text: String): String = text.uppercase().filterNot { it.isWhitespace() }

    /** The server's host, and its port when it has one, as the sign-in form names it. */
    fun describeServer(server: String): String {
        val url = convertToOpenableURL(server) ?: return server
        return if (url.port == -1) url.host else "${url.host}:${url.port}"
    }

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
