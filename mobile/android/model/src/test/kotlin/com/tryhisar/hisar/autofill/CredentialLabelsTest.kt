package com.tryhisar.hisar.autofill

import kotlin.test.Test
import kotlin.test.assertEquals

class CredentialLabelsTest {
    @Test
    fun loginSubtitleIncludesTheVaultWithOrWithoutUsername() {
        val labels = listOf(
            CredentialLabel("GitHub", "alice", "Personal", "a"),
            CredentialLabel("GitHub", "alice", "Work", "b"),
            CredentialLabel("GitHub", "", "Work", "c"),
        )
        assertEquals(listOf("alice · Personal", "alice · Work", "Work"), distinguishCredentialLabels(labels).map { it.subtitle })
    }

    @Test
    fun identicalAccountsInOneVaultGetShortestUniqueItemPrefixes() {
        val labels = listOf(
            CredentialLabel("GitHub", "alice", "Work", "a123"),
            CredentialLabel("GitHub", "alice", "Work", "a234"),
            CredentialLabel("GitHub", "bob", "Work", "a345"),
        )
        val named = distinguishCredentialLabels(labels)
        assertEquals(listOf("GitHub · Work · a1", "GitHub · Work · a2", "GitHub · Work"), named.map { it.displayName })
        assertEquals(listOf("alice · Work · a1", "alice · Work · a2", "bob · Work"), named.map { it.subtitle })
    }

    @Test
    fun passkeysInTheSameItemGetShortCredentialPrefixes() {
        val labels = listOf(
            CredentialLabel("GitHub", "alice", "Work", "item", "ab01"),
            CredentialLabel("GitHub", "alice", "Work", "item", "ac02"),
        )
        assertEquals(listOf("GitHub · Work · ab", "GitHub · Work · ac"), distinguishCredentialLabels(labels).map { it.displayName })
    }
}
