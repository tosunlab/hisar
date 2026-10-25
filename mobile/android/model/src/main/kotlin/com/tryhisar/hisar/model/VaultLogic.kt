package com.tryhisar.hisar.model

import java.net.URI

/** An item with the vault it is in, as the lists that span vaults show it. */
data class VaultItemReference(val vaultID: String, val vaultName: String, val item: ItemDetail) {
    val id: String get() = LoginMatch.buildRecordIdentifier(vaultID, item.id)
}

/** A tag with how many items carry it. */
data class TagSummary(val name: String, val itemCount: Int)

/** The item types the Vaults screen lists under By type, in its order; [id] names the row in tests, as on the iPhone. */
enum class ItemCategory(val id: String, val title: String) {
    LOGINS("logins", "Logins"),
    SECURE_NOTES("secureNotes", "Secure notes"),
    CREDIT_CARDS("creditCards", "Credit cards"),
    IDENTITIES("identities", "Identities"),
    API_CREDENTIALS("apiCredentials", "API credentials"),
    ADDRESSES("addresses", "Addresses"),
    SOFTWARE_LICENSES("softwareLicenses", "Software licenses"),
    SERVERS("servers", "Servers"),
    DATABASES("databases", "Databases"),
    SSH_KEYS("sshKeys", "SSH keys"),
    ;

    val emptyTitle: String get() = "No ${title.lowercase()}"

    fun includes(item: ItemDetail): Boolean = when (this) {
        LOGINS -> item.isLogin
        SECURE_NOTES -> item.isSecureNote
        CREDIT_CARDS -> item.isCreditCard
        IDENTITIES -> item.isIdentity
        API_CREDENTIALS -> item.isAPICredential
        ADDRESSES -> item.isAddress
        SOFTWARE_LICENSES -> item.isSoftwareLicense
        SERVERS -> item.isServer
        DATABASES -> item.isDatabase
        SSH_KEYS -> item.isSSHKey
    }
}

object VaultLogic {
    /** The items in which every word of [searchText] appears, ignoring case; all of them for an empty search. */
    fun filterAllItem(allItem: List<VaultItemReference>, searchText: String): List<VaultItemReference> {
        val allWord = searchText.lowercase().split(Regex("\\s+")).filter { it.isNotEmpty() }
        if (allWord.isEmpty()) return allItem
        return allItem.filter { reference ->
            val searchable = listSearchableText(reference).joinToString(" ").lowercase()
            allWord.all { it in searchable }
        }
    }

    // An identity's list summary keeps its name, email and address, an API credential's its service, key ID and host, an
    // address's first street line, city and country, a license's product, version, licensed-to name and email (never the
    // key), a server's host and port, a database's engine, address and name (never the connection string). The list shows
    // some of them under the title; search reads them all, so an identity imported with an address is found by its city.
    private fun listSearchableText(reference: VaultItemReference): List<String> {
        val item = reference.item
        return listOf(item.title, item.username, reference.vaultName) + item.allURL + item.allTag +
            item.identity?.let { listOf(it.fullName, it.email, it.street, it.city, it.country) }.orEmpty() +
            item.apiCredential?.let { listOf(it.service, it.keyID, it.host) }.orEmpty() +
            item.address?.let { listOf(it.street, it.city, it.country) }.orEmpty() +
            item.softwareLicense?.let { listOf(it.product, it.version, it.licensedTo, it.email) }.orEmpty() +
            item.server?.let { listOf(it.address, it.host, it.port) }.orEmpty() +
            item.database?.let { listOf(it.engineName, it.location, it.host, it.port, it.name) }.orEmpty()
    }

    fun filterAllItemByTag(allItem: List<VaultItemReference>, tag: String): List<VaultItemReference> = allItem.filter { tag in it.item.allTag }

    /** Every tag of the items with how many carry it, sorted by name. */
    fun listAllTag(allItem: List<VaultItemReference>): List<TagSummary> =
        allItem.flatMap { it.item.allTag }.groupingBy { it }.eachCount()
            .map { (name, count) -> TagSummary(name, count) }
            .sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.name })

    fun sortAllItem(allItem: List<VaultItemReference>): List<VaultItemReference> =
        allItem.sortedWith(compareBy<VaultItemReference, String>(String.CASE_INSENSITIVE_ORDER) { it.item.title }.thenBy(String.CASE_INSENSITIVE_ORDER) { it.item.username })

    /** The line under an item's title in lists; lists that span vaults name the vault too. */
    fun describeRow(item: ItemDetail, vaultName: String? = null): String {
        val detail = when {
            item.isSSHKey -> item.keyType.ifEmpty { "SSH key" }
            item.isSecureNote -> "Secure note"
            item.isCreditCard -> (item.card ?: ItemCard()).summary
            item.isIdentity -> (item.identity ?: ItemIdentity()).summary
            item.isAPICredential -> (item.apiCredential ?: ItemAPICredential()).summary
            item.isAddress -> (item.address ?: ItemAddress()).summary
            item.isSoftwareLicense -> (item.softwareLicense ?: ItemSoftwareLicense()).summary
            item.isServer -> (item.server ?: ItemServer()).summary(item.username)
            item.isDatabase -> (item.database ?: ItemDatabase()).summary
            else -> item.username
        }
        if (vaultName == null) return detail
        return if (detail.isEmpty()) vaultName else "$detail · $vaultName"
    }

    fun describeVaultRole(role: String): String = when (role) {
        Member.VAULT_ROLE_OWNER -> "Owner"
        Member.VAULT_ROLE_EDITOR -> "Editor"
        Member.VAULT_ROLE_READER -> "Reader"
        else -> role.replaceFirstChar { it.uppercase() }
    }

    fun describeVaultRoleAbility(role: String): String = when (role) {
        Member.VAULT_ROLE_OWNER -> "Owners open and change items, share the vault, change roles and remove people."
        Member.VAULT_ROLE_EDITOR -> "Editors open and change items."
        Member.VAULT_ROLE_READER -> "Readers open items but cannot change them."
        else -> ""
    }

    fun describeItemCount(count: Int): String = if (count == 1) "1 item" else "$count items"

    /** The Secret Key as the backend reads it: upper case, without the spaces and line breaks a paste brings. */
    fun formatSecretKey(text: String): String = text.uppercase().filterNot { it.isWhitespace() }

    /** The server's host, and its port when it has one, as the sign-in form names it. */
    fun describeServer(server: String): String {
        val url = convertToOpenableURL(server) ?: return server
        return if (url.port == -1) url.host else "${url.host}:${url.port}"
    }

    /** The HTTP(S) URI to open, adding https:// and converting IDN hosts without changing the stored website. */
    fun convertToOpenableURL(text: String): URI? {
        val trimmedText = text.trimmingWhitespacesAndNewlines()
        if (trimmedText.isEmpty()) return null
        val fullText = if ("://" in trimmedText) trimmedText else "https://$trimmedText"
        return parseHTTPURI(fullText)
    }

    fun extractHost(text: String): String? = convertToOpenableURL(text)?.host?.lowercase()
}
