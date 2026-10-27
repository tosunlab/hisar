import type { Organization, Team, VaultRole } from "./api";
import { formatRole } from "./util";

export type OrganizationAccessChange =
  | { kind: "remove-team"; teamID: string }
  | { kind: "remove-team-member"; teamID: string; accountID: string }
  | { kind: "remove-member"; accountID: string }
  | { kind: "take-vault"; teamID: string; vaultID: string }
  | { kind: "team-role"; teamID: string; vaultID: string; role: VaultRole };

const roleRank: Record<string, number> = { reader: 1, editor: 2, owner: 3 };

export function describeOrganizationAccessChange(organization: Organization, change: OrganizationAccessChange): string[] {
  const after: Team[] = organization.teams.filter((team) => change.kind !== "remove-team" || team.id !== change.teamID).map((team) => ({
    ...team,
    members: team.members.filter((member) => !((change.kind === "remove-member" || (change.kind === "remove-team-member" && team.id === change.teamID)) && member.accountId === change.accountID)),
    vaults: team.vaults.filter((vault) => !(change.kind === "take-vault" && team.id === change.teamID && vault.vaultId === change.vaultID)).map((vault) =>
      change.kind === "team-role" && team.id === change.teamID && vault.vaultId === change.vaultID ? { ...vault, role: change.role } : vault),
  }));
  function strongest(teams: Team[], accountID: string, vaultID: string): string {
    let role = "";
    for (const team of teams) {
      if (!team.members.some((member) => member.accountId === accountID)) continue;
      const candidate = team.vaults.find((vault) => vault.vaultId === vaultID)?.role ?? "";
      if ((roleRank[candidate] ?? 0) > (roleRank[role] ?? 0)) role = candidate;
    }
    return role;
  }
  const vaultByID = new Map(organization.teams.flatMap((team) => team.vaults.map((vault) => [vault.vaultId, vault] as const)));
  const memberByID = new Map(organization.teams.flatMap((team) => team.members.map((member) => [member.accountId, member] as const)));
  const lines: string[] = [];
  for (const [vaultID, vault] of [...vaultByID].sort(([a], [b]) => a.localeCompare(b))) {
    const effects: string[] = [];
    for (const [accountID, member] of memberByID) {
      const beforeRole = strongest(organization.teams, accountID, vaultID);
      const afterRole = strongest(after, accountID, vaultID);
      if (beforeRole === afterRole) continue;
      const name = member.name || member.email;
      effects.push(!afterRole ? (member.isCurrentUser ? "You lose access too." : `${name} loses access.`)
        : `${member.isCurrentUser ? "Your role" : `${name}'s role`} changes from ${formatRole(beforeRole)} to ${formatRole(afterRole)}.`);
    }
    if (effects.length) lines.push(`${vault.canOpen ? vault.name : `A vault you can't open (${vaultID})`}: ${effects.join(" ")}`);
  }
  return lines.length ? lines : ["Everyone keeps their vault access through other teams."];
}

export function vaultRoleChangeTitle(name: string, vaultName: string, role: string): string {
  return role === "owner" ? `Make ${name} an owner of ${vaultName}?` : `Change ${name}'s role from Owner to ${formatRole(role)}?`;
}

export function vaultRoleChangeMessage(role: string): string {
  return role === "owner"
    ? "Owners can share, rename and delete this vault, change roles and remove other owners, including you."
    : "They lose the ability to share, rename and delete this vault, change roles and remove owners unless another team gives them Owner.";
}
