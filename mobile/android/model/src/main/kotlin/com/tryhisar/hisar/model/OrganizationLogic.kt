package com.tryhisar.hisar.model

/** The organization vaults of one organization on the Vaults screen, with a line for the vaults still waiting for a key. */
data class OrganizationVaultGroup(val id: String, val name: String, val allVault: List<VaultSummary>, val waitingMessage: String)

/** The Vaults screen's vaults: those listed under Vaults, and one group per organization. */
data class VaultGrouping(val allOtherVault: List<VaultSummary>, val allGroup: List<OrganizationVaultGroup>)

/** Where a new vault goes: a shared vault, or an organization vault for one of the user's teams. */
data class VaultLocation(val organizationID: String, val organizationName: String, val teamID: String, val teamName: String) {
    val id: String get() = if (teamID.isEmpty()) "shared" else "$organizationID/$teamID"
    val isSharedVault: Boolean get() = teamID.isEmpty()
    val label: String get() = if (isSharedVault) "Shared vault" else "$organizationName › $teamName"

    companion object {
        val sharedVault = VaultLocation(organizationID = "", organizationName = "", teamID = "", teamName = "")
    }
}

object OrganizationLogic {
    const val MAX_NAME_LENGTH = 100

    /** Organization and team names are 1 to 100 characters, as apitypes.CleanOrganizationName checks again. */
    fun isValidName(name: String): Boolean {
        val trimmed = name.trimmingWhitespacesAndNewlines()
        return trimmed.isNotEmpty() && trimmed.codePointCount(0, trimmed.length) <= MAX_NAME_LENGTH
    }

    /**
     * Splits the vaults into the ones listed as before and one group per organization, by name. An organization vault
     * whose organization this device has not listed (offline since signing in) stays with the others.
     */
    fun groupVaults(allVault: List<VaultSummary>, allOrganization: List<OrganizationSummary>, allWaiting: List<WaitingVaultKey>): VaultGrouping {
        val organizationNameByID = allOrganization.reversed().associate { it.id to it.name }
        val (allGroupedVault, allOtherVault) = allVault.partition { it.isOrganization && it.organizationID in organizationNameByID }
        val allVaultByOrganizationID = allGroupedVault.groupBy { it.organizationID }
        val allVaultID = allVault.map { it.id }.toSet()
        val allStillWaiting = allWaiting.filter { it.vaultID !in allVaultID }
        val waitingCountByOrganizationID = allStillWaiting.groupingBy { it.organizationID }.eachCount()
        val waitingNameByOrganizationID = allStillWaiting.associate { it.organizationID to it.organizationName }
        val allGroup = (allVaultByOrganizationID.keys + waitingCountByOrganizationID.keys).map { organizationID ->
            val name = organizationNameByID[organizationID] ?: waitingNameByOrganizationID[organizationID].orEmpty()
            OrganizationVaultGroup(
                id = organizationID,
                name = name,
                allVault = allVaultByOrganizationID[organizationID].orEmpty(),
                waitingMessage = describeWaiting(waitingCountByOrganizationID[organizationID] ?: 0, name),
            )
        }
        return VaultGrouping(allOtherVault, allGroup.sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.name }))
    }

    fun describeWaiting(count: Int, organizationName: String): String = when (count) {
        0 -> ""
        1 -> "Waiting for an owner of a vault in $organizationName to give you its key"
        else -> "Waiting for the owners of $count vaults in $organizationName to give you their keys"
    }

    /** The Vaults screen's warnings when this app could not give a team member a vault key or re-encrypt a vault it owns. */
    fun describeKeyWarnings(waitingVaultKeys: WaitingVaultKeys): List<String> = listOfNotNull(
        waitingVaultKeys.deliveryError.takeIf { it.isNotEmpty() }?.let { "Hisar couldn't give every vault key: $it" },
        waitingVaultKeys.rotationError.takeIf { it.isNotEmpty() }?.let { "Hisar couldn't re-encrypt every vault that needs a new key: $it" },
    )

    /** The new vault's Where choices: a shared vault first, then each team the user is in, as "Acme › Platform". */
    fun listAllVaultLocation(allOrganization: List<Organization>): List<VaultLocation> {
        val allTeamLocation = allOrganization.flatMap { organization ->
            organization.allTeam.filter { it.hasCurrentUser }.map { team -> VaultLocation(organization.id, organization.name, team.id, team.name) }
        }
        return listOf(VaultLocation.sharedVault) + allTeamLocation.sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.label })
    }

    /**
     * The Where a new vault is created in. Null, so Create waits, until the choices are loaded, after they failed to load,
     * and when the chosen team is no longer listed: a failed lookup never falls back to a shared vault.
     */
    fun findVaultLocation(id: String, allLocation: List<VaultLocation>?): VaultLocation? = allLocation?.firstOrNull { it.id == id }
}
