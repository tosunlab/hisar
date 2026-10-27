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

/** A team that has an organization vault, with its role for that vault. */
data class TeamAccess(val team: Team, val role: String) {
    val id: String get() = team.id
}

/** What an organization vault's sharing sheet shows: the teams that have the vault and what the user may change. */
data class OrganizationVaultSharing(
    val organizationID: String,
    val organizationName: String,
    val allTeamAccess: List<TeamAccess>,
    /** Admins give the vault to more teams and take it back; everyone else reads [askAdminNote]. */
    val canChangeTeams: Boolean,
    val allTeamWithoutVault: List<Team>,
) {
    val askAdminNote: String? get() = if (canChangeTeams) null else "Ask an admin of $organizationName to change who has this vault."
}

enum class OrganizationMemberAction { MAKE_ADMIN, MAKE_MEMBER, REMOVE }

data class MemberChoice(val title: String, val detail: String?)

/** What the organization screen does after loading the organization. */
sealed interface OrganizationLoad {
    data class Loaded(val organization: Organization) : OrganizationLoad

    /** The organizations no longer list it: it was deleted, or the user is no longer in it. */
    data object Gone : OrganizationLoad

    /** It could not be loaded, and the user may still be in it; the error message says why. */
    data object Failed : OrganizationLoad
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

    fun describeVaultSharing(organization: Organization, vaultID: String): OrganizationVaultSharing {
        val (allTeamWithVault, allTeamWithoutVault) = organization.allTeam.partition { team -> team.allVault.any { it.vaultID == vaultID } }
        return OrganizationVaultSharing(
            organizationID = organization.id,
            organizationName = organization.name,
            allTeamAccess = allTeamWithVault.map { team -> TeamAccess(team, team.allVault.first { it.vaultID == vaultID }.role) },
            canChangeTeams = organization.isAdmin,
            allTeamWithoutVault = if (organization.isAdmin) allTeamWithoutVault else emptyList(),
        )
    }

    fun describeMemberChoice(member: Member): MemberChoice {
        val name = member.name.ifEmpty { member.email }
        return MemberChoice(
            title = if (member.isCurrentUser) "$name (you)" else name,
            detail = member.email.takeIf { member.name.isNotEmpty() && member.name != member.email },
        )
    }

    /** The organization's members an admin can add to the team; nobody for anyone else. */
    fun listAllAddableMember(organization: Organization, team: Team): List<Member> {
        if (!organization.isAdmin) return emptyList()
        val allTeamMemberID = team.allMember.map { it.accountID }.toSet()
        return organization.allMember.filter { it.accountID !in allTeamMemberID }
    }

    /**
     * The organization's vaults an admin can give the team: the vaults of its other teams and those of the organization
     * this device has, by name where this device can open them.
     */
    fun listAllGivableVault(organization: Organization, team: Team, allVault: List<VaultSummary>): List<TeamVault> {
        if (!organization.isAdmin) return emptyList()
        val vaultByID = linkedMapOf<String, TeamVault>()
        for (vault in allVault) {
            if (vault.isOrganization && vault.organizationID == organization.id) {
                vaultByID[vault.id] = TeamVault(vaultID = vault.id, name = vault.name, role = "", canOpen = true)
            }
        }
        for (otherTeam in organization.allTeam) {
            for (vault in otherTeam.allVault) {
                if (vaultByID[vault.vaultID]?.canOpen != true) vaultByID[vault.vaultID] = vault
            }
        }
        val allTeamVaultID = team.allVault.map { it.vaultID }.toSet()
        return vaultByID.values.filter { it.vaultID !in allTeamVaultID }.sortedWith(
            compareByDescending<TeamVault> { it.canOpen }
                .thenComparator { first, second ->
                    if (first.canOpen) String.CASE_INSENSITIVE_ORDER.compare(first.name, second.name) else first.vaultID.compareTo(second.vaultID)
                },
        )
    }

    /** What an admin can do to a member from the organization screen; the user leaves instead of removing themselves. */
    fun listAllMemberAction(organization: Organization, member: Member): List<OrganizationMemberAction> {
        if (!organization.isAdmin) return emptyList()
        val roleAction = if (member.role == OrganizationRole.ADMIN) OrganizationMemberAction.MAKE_MEMBER else OrganizationMemberAction.MAKE_ADMIN
        return if (member.isCurrentUser) listOf(roleAction) else listOf(roleAction, OrganizationMemberAction.REMOVE)
    }

    fun describeMemberCount(count: Int): String = if (count == 1) "1 member" else "$count members"
}
