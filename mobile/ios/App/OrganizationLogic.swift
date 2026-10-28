import Foundation

/// The organization vaults of one organization on the Vaults screen, with a line for the vaults still waiting for a key.
struct OrganizationVaultGroup: Identifiable, Equatable {
    var id: String
    var name: String
    var allVault: [VaultSummary]
    var waitingMessage: String
}

/// Where a new vault goes: a shared vault, or an organization vault for one of the user's teams.
struct VaultLocation: Identifiable, Hashable {
    static let sharedVault = VaultLocation(organizationID: "", organizationName: "", teamID: "", teamName: "")

    var organizationID: String
    var organizationName: String
    var teamID: String
    var teamName: String

    var id: String { teamID.isEmpty ? "shared" : "\(organizationID)/\(teamID)" }
    var isSharedVault: Bool { teamID.isEmpty }
    var label: String { isSharedVault ? "Shared vault" : "\(organizationName) › \(teamName)" }
}

/// A team that has an organization vault, with its role for that vault.
struct TeamAccess: Identifiable, Equatable {
    var team: Team
    var role: String

    var id: String { team.id }
}

/// What an organization vault's sharing sheet shows: the teams that have the vault and what the user may change.
struct OrganizationVaultSharing: Equatable {
    var organizationID: String
    var organizationName: String
    var allTeamAccess: [TeamAccess]
    /// Admins give the vault to more teams and take it back; everyone else reads askAdminNote.
    var canChangeTeams: Bool
    var allTeamWithoutVault: [Team]

    var askAdminNote: String? { canChangeTeams ? nil : "Ask an admin of \(organizationName) to change who has this vault." }
}

enum OrganizationMemberAction: Equatable {
    case makeAdmin
    case makeMember
    case remove
}

struct GivableVault: Equatable {
    var vaultID: String
    var displayName: String
    var canOpen: Bool
}

/// What the organization screen does after loading the organization.
enum OrganizationLoad: Equatable {
    case loaded(Organization)
    /// The organizations no longer list it: it was deleted, or the user is no longer in it.
    case gone
    /// It could not be loaded, and the user may still be in it; AppModel.errorMessage says why.
    case failed
}

enum OrganizationLogic {
    static let maxNameLength = 100

    /// Organization and team names are 1 to 100 characters, as apitypes.CleanOrganizationName checks again.
    static func isValidName(_ name: String) -> Bool {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return !trimmed.isEmpty && trimmed.count <= maxNameLength
    }

    /// Splits the vaults into the ones listed as before and one group per organization, by name. An organization vault
    /// whose organization this device has not listed (offline since signing in) stays with the others.
    static func groupVaults(_ allVault: [VaultSummary], allOrganization: [OrganizationSummary], allWaiting: [WaitingVaultKey]) -> (allOtherVault: [VaultSummary], allGroup: [OrganizationVaultGroup]) {
        let organizationNameByID = Dictionary(allOrganization.map { ($0.id, $0.name) }, uniquingKeysWith: { first, _ in first })
        var allOtherVault: [VaultSummary] = []
        var allVaultByOrganizationID: [String: [VaultSummary]] = [:]
        for vault in allVault {
            if vault.isOrganization, organizationNameByID[vault.organizationID] != nil {
                allVaultByOrganizationID[vault.organizationID, default: []].append(vault)
            } else {
                allOtherVault.append(vault)
            }
        }
        let allVaultID = Set(allVault.map(\.id))
        var waitingCountByOrganizationID: [String: Int] = [:]
        var waitingNameByOrganizationID: [String: String] = [:]
        for waiting in allWaiting where !allVaultID.contains(waiting.vaultID) {
            waitingCountByOrganizationID[waiting.organizationID, default: 0] += 1
            waitingNameByOrganizationID[waiting.organizationID] = waiting.organizationName
        }
        let allOrganizationID = Set(allVaultByOrganizationID.keys).union(waitingCountByOrganizationID.keys)
        let allGroup = allOrganizationID.map { organizationID in
            let name = organizationNameByID[organizationID] ?? waitingNameByOrganizationID[organizationID] ?? ""
            return OrganizationVaultGroup(
                id: organizationID,
                name: name,
                allVault: allVaultByOrganizationID[organizationID] ?? [],
                waitingMessage: describeWaiting(count: waitingCountByOrganizationID[organizationID] ?? 0, organizationName: name)
            )
        }
        return (allOtherVault, allGroup.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending })
    }

    static func describeWaiting(count: Int, organizationName: String) -> String {
        switch count {
        case 0: return ""
        case 1: return "Waiting for an owner of a vault in \(organizationName) to give you its key"
        default: return "Waiting for the owners of \(count) vaults in \(organizationName) to give you their keys"
        }
    }

    /// The Vaults screen's warnings when this app could not give a team member a vault key or re-encrypt a vault it owns.
    static func describeKeyWarnings(_ waitingVaultKeys: WaitingVaultKeys) -> [String] {
        var allWarning: [String] = []
        if !waitingVaultKeys.deliveryError.isEmpty {
            allWarning.append("Hisar couldn't give every vault key: \(waitingVaultKeys.deliveryError)")
        }
        if !waitingVaultKeys.rotationError.isEmpty {
            allWarning.append("Hisar couldn't re-encrypt every vault that needs a new key: \(waitingVaultKeys.rotationError)")
        }
        return allWarning
    }

    /// The new vault's Where choices: a shared vault first, then each team the user is in, as "Acme › Platform".
    static func listAllVaultLocation(_ allOrganization: [Organization]) -> [VaultLocation] {
        let allTeamLocation = allOrganization.flatMap { organization in
            organization.allTeam.filter(\.hasCurrentUser).map { team in
                VaultLocation(organizationID: organization.id, organizationName: organization.name, teamID: team.id, teamName: team.name)
            }
        }
        return [.sharedVault] + allTeamLocation.sorted { $0.label.localizedCaseInsensitiveCompare($1.label) == .orderedAscending }
    }

    /// The Where a new vault is created in. Nil, so Create waits, until the choices are loaded, after they failed to load,
    /// and when the chosen team is no longer listed: a failed lookup never falls back to a shared vault.
    static func findVaultLocation(id: String, in allLocation: [VaultLocation]?) -> VaultLocation? {
        allLocation?.first { $0.id == id }
    }

    static func describeVaultSharing(organization: Organization, vaultID: String) -> OrganizationVaultSharing {
        var allTeamAccess: [TeamAccess] = []
        var allTeamWithoutVault: [Team] = []
        for team in organization.allTeam {
            if let teamVault = team.allVault.first(where: { $0.vaultID == vaultID }) {
                allTeamAccess.append(TeamAccess(team: team, role: teamVault.role))
            } else {
                allTeamWithoutVault.append(team)
            }
        }
        return OrganizationVaultSharing(
            organizationID: organization.id,
            organizationName: organization.name,
            allTeamAccess: allTeamAccess,
            canChangeTeams: organization.isAdmin,
            allTeamWithoutVault: organization.isAdmin ? allTeamWithoutVault : []
        )
    }

    static func listAllTeamMember(organization: Organization, team: Team) -> [Member] {
        let allTeamMemberID = Set(team.allMember.map(\.accountID))
        return organization.allMember.filter { allTeamMemberID.contains($0.accountID) }
    }

    static func describeMemberChoice(_ member: Member) -> String {
        let name = member.name.isEmpty || member.name == member.email ? member.email : "\(member.name) (\(member.email))"
        return member.isCurrentUser ? "\(name) (you)" : name
    }

    /// The organization's members an admin can add to the team; nobody for anyone else.
    static func listAllAddableMember(organization: Organization, team: Team) -> [Member] {
        guard organization.isAdmin else { return [] }
        let allTeamMemberID = Set(team.allMember.map(\.accountID))
        return organization.allMember.filter { !allTeamMemberID.contains($0.accountID) }
    }

    /// The organization's vaults an admin can give the team: the vaults of its other teams and those of the organization
    /// this device has, by name where this device can open them.
    static func listAllGivableVault(organization: Organization, team: Team, allVault: [VaultSummary]) -> [GivableVault] {
        guard organization.isAdmin else { return [] }
        var vaultByID: [String: TeamVault] = [:]
        for vault in allVault where vault.isOrganization && vault.organizationID == organization.id {
            vaultByID[vault.id] = TeamVault(vaultID: vault.id, name: vault.name, role: "", canOpen: true)
        }
        for otherTeam in organization.allTeam {
            for vault in otherTeam.allVault where vaultByID[vault.vaultID]?.canOpen != true {
                vaultByID[vault.vaultID] = vault
            }
        }
        let allTeamVaultID = Set(team.allVault.map(\.vaultID))
        let allGivable = vaultByID.values.filter { !allTeamVaultID.contains($0.vaultID) }.sorted { first, second in
            if first.canOpen != second.canOpen { return first.canOpen }
            if first.canOpen { return first.name.localizedCaseInsensitiveCompare(second.name) == .orderedAscending }
            return first.vaultID < second.vaultID
        }
        let allChoice = allGivable.map { vault in
            if vault.canOpen { return GivableVault(vaultID: vault.vaultID, displayName: vault.name, canOpen: true) }
            let teams = organization.allTeam.filter { $0.allVault.contains { $0.vaultID == vault.vaultID } }.map(\.name).joined(separator: ", ")
            return GivableVault(vaultID: vault.vaultID, displayName: "\(TeamVault.unopenableName) (\(teams))", canOpen: false)
        }
        let labelCounts = Dictionary(grouping: allChoice.filter { !$0.canOpen }, by: \.displayName).mapValues(\.count)
        return allChoice.map { choice in
            var choice = choice
            if !choice.canOpen && labelCounts[choice.displayName, default: 0] > 1 {
                choice.displayName += " · \(choice.vaultID.prefix(6))"
            }
            return choice
        }
    }

    /// What an admin can do to a member from the organization screen; the user leaves instead of removing themselves.
    static func listAllMemberAction(organization: Organization, member: Member) -> [OrganizationMemberAction] {
        guard organization.isAdmin else { return [] }
        let roleAction: OrganizationMemberAction = member.role == OrganizationRole.admin ? .makeMember : .makeAdmin
        return member.isCurrentUser ? [roleAction] : [roleAction, .remove]
    }

    static func describeMemberCount(_ count: Int) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }
}
