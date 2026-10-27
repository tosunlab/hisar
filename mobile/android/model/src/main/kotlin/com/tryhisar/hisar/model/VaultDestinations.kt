package com.tryhisar.hisar.model

data class VaultDestination(val id: String, val label: String, val context: String)
data class VaultDestinationGroup(val id: String, val name: String, val destinations: List<VaultDestination>)

object VaultDestinations {
    fun groups(vaults: List<VaultSummary>, organizations: List<OrganizationSummary>): List<VaultDestinationGroup> {
        fun groupID(vault: VaultSummary) = if (vault.isOrganization) "organization/${vault.organizationID}" else vault.type
        return vaults.groupBy(::groupID).map { (id, group) ->
            val vault = group.first()
            val organization = organizations.firstOrNull { it.id == vault.organizationID }
            var name = if (vault.isOrganization) organization?.name ?: "Organization ${vault.organizationID.take(8)}"
                else if (vault.type == VaultSummary.TYPE_PERSONAL) "Private vault" else "Shared vault"
            if (organization != null && organizations.any { it.id != organization.id && it.name == organization.name }) name += " (${organization.id.take(8)})"
            VaultDestinationGroup(id, name, group.map { destination ->
                val collisions = group.filter { it.name == destination.name }
                var length = 8
                while (collisions.any { it.id != destination.id && it.id.take(length) == destination.id.take(length) }) length++
                val suffix = if (collisions.size > 1) " (${destination.id.take(length)})" else ""
                val context = if (destination.isOrganization) "Shared through teams in $name."
                    else if (destination.type == VaultSummary.TYPE_PERSONAL) "Only you have access to this private vault."
                    else "Everyone with access to this shared vault can see the item."
                VaultDestination(destination.id, "$name / ${destination.name}$suffix", context)
            })
        }.sortedBy { it.name.lowercase() }
    }
}
