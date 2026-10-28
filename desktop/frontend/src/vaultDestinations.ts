import type { VaultSummary } from "./api";

export interface VaultDestination {
  id: string;
  label: string;
  context: string;
}

export interface VaultDestinationGroup {
  id: string;
  name: string;
  destinations: VaultDestination[];
}

export function groupVaultDestinations(vaults: VaultSummary[], organizations: { id: string; name: string }[]): VaultDestinationGroup[] {
  const groups = new Map<string, VaultDestinationGroup>();
  for (const vault of vaults) {
    const organizationID = vault.organizationId;
    const id = organizationID ? `organization/${organizationID}` : vault.type;
    const organization = organizations.find((entry) => entry.id === organizationID);
    let name = organizationID ? organization?.name ?? `Organization ${organizationID.slice(0, 8)}` : vault.type === "personal" ? "Private vault" : "Shared vault";
    if (organization && organizations.some((entry) => entry.id !== organizationID && entry.name === organization.name)) name += ` (${organizationID!.slice(0, 8)})`;
    const group = groups.get(id) ?? { id, name, destinations: [] };
    const collisions = vaults.filter((entry) => entry.name === vault.name && (entry.organizationId ? `organization/${entry.organizationId}` : entry.type) === id);
    let length = 8;
    while (collisions.some((entry) => entry.id !== vault.id && entry.id.slice(0, length) === vault.id.slice(0, length))) length++;
    const suffix = collisions.length > 1 ? ` (${vault.id.slice(0, length)})` : "";
    group.destinations.push({ id: vault.id, label: `${name} / ${vault.name}${suffix}`, context: organizationID ? `Shared through teams in ${name}.` : vault.type === "personal" ? "Only you have access to this private vault." : "Everyone with access to this shared vault can see the item." });
    groups.set(id, group);
  }
  return [...groups.values()].sort((first, second) => first.name.localeCompare(second.name));
}
