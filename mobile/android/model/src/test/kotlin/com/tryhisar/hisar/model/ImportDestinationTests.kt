package com.tryhisar.hisar.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ImportDestinationTests {
    private fun vault(id: String, organization: String) = VaultSummary(id, "Production", "organization", "editor", organizationID = organization)

    @Test
    fun testImportDestinationsDistinguishOrganizations() {
        val groups = OrganizationLogic.importVaultGroups(
            listOf(
                vault("one", "acme"), vault("two", "beta"),
                VaultSummary("personal", "Personal", "personal", "owner"), VaultSummary("shared", "Production", "shared", "owner"),
            ),
            listOf(OrganizationSummary("acme", "Acme"), OrganizationSummary("beta", "Beta")),
        )
        assertEquals(listOf("Personal", "Shared vaults", "Acme", "Beta"), groups.map { it.name })
        assertEquals(listOf("Personal", "Shared vaults › Production", "Acme › Production", "Beta › Production"), groups.flatMap { it.allChoice }.map { it.label })
    }

    @Test
    fun testImportDestinationsKeepMissingOrganizationIdentity() {
        val groups = OrganizationLogic.importVaultGroups(listOf(vault("one", "missing-organization")), emptyList())
        assertEquals("missing-organization › Production", groups.first().allChoice.first().label)
    }

    @Test
    fun testImportDestinationsDisambiguateRepeatedNamesWithStableIDs() {
        val vaults = listOf(vault("12345678-a", "org-one"), vault("12345678-b", "org-one"), vault("third", "org-two"))
        val organizations = listOf(OrganizationSummary("org-one", "Acme"), OrganizationSummary("org-two", "Acme"))
        val groups = OrganizationLogic.importVaultGroups(vaults, organizations)
        val labels = groups.flatMap { it.allChoice }.map { it.label }
        assertEquals(3, labels.toSet().size)
        assertTrue("Acme (org-one) › Production (12345678-a)" in labels)
        assertTrue("Acme (org-one) › Production (12345678-b)" in labels)
        val reordered = OrganizationLogic.importVaultGroups(vaults.reversed(), organizations.reversed())
        assertEquals(labels.toSet(), reordered.flatMap { it.allChoice }.map { it.label }.toSet())
    }
}
