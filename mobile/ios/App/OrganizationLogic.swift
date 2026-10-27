import Foundation

/// The organization vaults of one organization on the Vaults screen, with a line for the vaults still waiting for a key.
struct OrganizationVaultGroup: Identifiable, Equatable {
    var id: String
    var name: String
    var allVault: [VaultSummary]
    var waitingMessage: String
}

struct ImportVaultChoice: Identifiable, Equatable {
    var id: String
    var label: String
}

struct ImportVaultGroup: Identifiable, Equatable {
    var id: String
    var name: String
    var allChoice: [ImportVaultChoice]
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

enum OrganizationAccessChange {
    case removeTeam(String)
    case removeTeamMember(teamID: String, accountID: String)
    case removeMember(String)
    case takeVault(teamID: String, vaultID: String)
    case teamRole(teamID: String, vaultID: String, role: String)
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

    static func importVaultGroups(_ allVault: [VaultSummary], allOrganization: [OrganizationSummary]) -> [ImportVaultGroup] {
        let grouped = groupVaults(allVault, allOrganization: allOrganization, allWaiting: [])
        let personal = grouped.allOtherVault.filter { $0.type == "personal" }
        let shared = grouped.allOtherVault.filter { !$0.isOrganization && $0.type != "personal" }
        let missing = Dictionary(grouping: grouped.allOtherVault.filter(\.isOrganization), by: \.organizationID)
        var groups = [
            OrganizationVaultGroup(id: "_personal", name: "Personal", allVault: personal, waitingMessage: ""),
            OrganizationVaultGroup(id: "_shared", name: "Shared vaults", allVault: shared, waitingMessage: ""),
        ].filter { !$0.allVault.isEmpty }
        let known = grouped.allGroup.map {
            OrganizationVaultGroup(id: $0.id, name: $0.name.isEmpty ? $0.id : $0.name, allVault: $0.allVault, waitingMessage: "")
        }
        groups += (known + missing.map {
            OrganizationVaultGroup(id: $0.key, name: $0.key, allVault: $0.value, waitingMessage: "")
        }).sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        let sameName = Dictionary(grouping: groups, by: { $0.name.lowercased() })
        return groups.map { group in
            let peers = sameName[group.name.lowercased()] ?? []
            let name = peers.count > 1 ? "\(group.name) (\(shortImportID(group.id, among: peers.map(\.id))))" : group.name
            let sameVaultName = Dictionary(grouping: group.allVault, by: { $0.name.lowercased() })
            let choices = group.allVault.map { vault in
                let peers = sameVaultName[vault.name.lowercased()] ?? []
                let vaultName = peers.count > 1 ? "\(vault.name) (\(shortImportID(vault.id, among: peers.map(\.id))))" : vault.name
                let label = group.id == "_personal" && name == vaultName ? name : "\(name) › \(vaultName)"
                return ImportVaultChoice(id: vault.id, label: label)
            }
            return ImportVaultGroup(id: group.id, name: name, allChoice: choices)
        }
    }

    private static func shortImportID(_ id: String, among allID: [String]) -> String {
        var length = min(8, id.count)
        while length < id.count && allID.contains(where: { $0 != id && $0.prefix(length) == id.prefix(length) }) {
            length += 1
        }
        return String(id.prefix(length))
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

    /// The organization's members an admin can add to the team; nobody for anyone else.
    static func listAllAddableMember(organization: Organization, team: Team) -> [Member] {
        guard organization.isAdmin else { return [] }
        let allTeamMemberID = Set(team.allMember.map(\.accountID))
        return organization.allMember.filter { !allTeamMemberID.contains($0.accountID) }
    }

    /// The organization's vaults an admin can give the team: the vaults of its other teams and those of the organization
    /// this device has, by name where this device can open them.
    static func listAllGivableVault(organization: Organization, team: Team, allVault: [VaultSummary]) -> [TeamVault] {
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
        return vaultByID.values.filter { !allTeamVaultID.contains($0.vaultID) }.sorted { first, second in
            if first.canOpen != second.canOpen { return first.canOpen }
            if first.canOpen { return first.name.localizedCaseInsensitiveCompare(second.name) == .orderedAscending }
            return first.vaultID < second.vaultID
        }
    }

    /// What an admin can do to a member from the organization screen; the user leaves instead of removing themselves.
    static func listAllMemberAction(organization: Organization, member: Member) -> [OrganizationMemberAction] {
        guard organization.isAdmin else { return [] }
        let roleAction: OrganizationMemberAction = member.role == OrganizationRole.admin ? .makeMember : .makeAdmin
        return member.isCurrentUser ? [roleAction] : [roleAction, .remove]
    }

    static func accessConsequences(organization: Organization, change: OrganizationAccessChange) -> [String] {
        var after = organization.allTeam
        switch change {
        case .removeTeam(let teamID):
            after.removeAll { $0.id == teamID }
        case .removeTeamMember(let teamID, let accountID):
            for index in after.indices where after[index].id == teamID {
                after[index].allMember.removeAll { $0.accountID == accountID }
            }
        case .removeMember(let accountID):
            for index in after.indices { after[index].allMember.removeAll { $0.accountID == accountID } }
        case .takeVault(let teamID, let vaultID):
            for index in after.indices where after[index].id == teamID {
                after[index].allVault.removeAll { $0.vaultID == vaultID }
            }
        case .teamRole(let teamID, let vaultID, let role):
            for index in after.indices where after[index].id == teamID {
                for vaultIndex in after[index].allVault.indices where after[index].allVault[vaultIndex].vaultID == vaultID {
                    after[index].allVault[vaultIndex].role = role
                }
            }
        }
        let rank = ["reader": 1, "editor": 2, "owner": 3]
        func strongest(_ teams: [Team], _ accountID: String, _ vaultID: String) -> String {
            teams.filter { $0.allMember.contains { $0.accountID == accountID } }
                .flatMap(\.allVault).filter { $0.vaultID == vaultID }
                .map(\.role).max { (rank[$0] ?? 0) < (rank[$1] ?? 0) } ?? ""
        }
        let vaultByID = Dictionary(organization.allTeam.flatMap(\.allVault).map { ($0.vaultID, $0) }, uniquingKeysWith: { first, second in first.canOpen ? first : second })
        let memberByID = Dictionary(organization.allTeam.flatMap(\.allMember).map { ($0.accountID, $0) }, uniquingKeysWith: { first, _ in first })
        var lines: [String] = []
        for vaultID in vaultByID.keys.sorted() {
            guard let vault = vaultByID[vaultID] else { continue }
            var losses: [String] = []
            var roleChanges: [String] = []
            for accountID in memberByID.keys.sorted() {
                guard let member = memberByID[accountID] else { continue }
                let beforeRole = strongest(organization.allTeam, accountID, vaultID)
                let afterRole = strongest(after, accountID, vaultID)
                guard beforeRole != afterRole else { continue }
                let name = member.name.isEmpty ? member.email : member.name
                if afterRole.isEmpty {
                    losses.append(member.isCurrentUser ? "You lose access too." : "\(name) loses access.")
                } else {
                    let subject = member.isCurrentUser ? "Your role" : "\(name)'s role"
                    roleChanges.append("\(subject) changes from \(beforeRole.capitalized) to \(afterRole.capitalized).")
                }
            }
            let name = vault.canOpen ? vault.name : "A vault you can't open (\(vaultID))"
            if !losses.isEmpty { lines.append("\(name): \(losses.joined(separator: " "))") }
            if !roleChanges.isEmpty { lines.append("\(name): \(roleChanges.joined(separator: " "))") }
        }
        if !lines.isEmpty { return lines }
        if case .removeTeamMember = change { return ["They keep their vault access through other teams."] }
        if case .removeMember = change { return ["They have no vault access through this organization's teams."] }
        return ["Everyone keeps their vault access through other teams."]
    }

    static func describeMemberCount(_ count: Int) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }
}
