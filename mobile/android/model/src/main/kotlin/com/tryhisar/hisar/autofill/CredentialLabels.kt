package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.LoginMatch

/** The public names that distinguish choices without reading a password or private key. */
data class CredentialLabel(
    val title: String,
    val username: String,
    val vaultName: String,
    val itemID: String,
    val credentialID: String = "",
    val suffix: String = "",
) {
    val displayName: String get() = listOf(title, vaultName, suffix).filter { it.isNotEmpty() }.joinToString(" · ")
    val subtitle: String get() = listOf(username, vaultName, suffix).filter { it.isNotEmpty() }.joinToString(" · ")
}

fun LoginMatch.credentialLabel(): CredentialLabel = CredentialLabel(title, username, vaultName, itemID)

/** Identical public labels get only the shortest ID prefix needed to distinguish the actual credentials. */
fun distinguishCredentialLabels(labels: List<CredentialLabel>): List<CredentialLabel> {
    val groups = labels.groupBy { Triple(it.title, it.username, it.vaultName) }
    return labels.map { label ->
        val group = groups.getValue(Triple(label.title, label.username, label.vaultName))
        val itemIDs = group.map { it.itemID }.distinct()
        val credentialIDs = group.filter { it.itemID == label.itemID }.map { it.credentialID }.distinct()
        val suffix = buildList {
            if (itemIDs.size > 1) add(shortestUniquePrefix(label.itemID, itemIDs))
            if (credentialIDs.size > 1) add(shortestUniquePrefix(label.credentialID, credentialIDs))
        }.joinToString(" · ")
        label.copy(suffix = suffix)
    }
}

private fun shortestUniquePrefix(id: String, allID: List<String>): String {
    val length = (1..id.length).firstOrNull { length -> allID.none { it != id && it.startsWith(id.take(length)) } } ?: id.length
    return id.take(length)
}
