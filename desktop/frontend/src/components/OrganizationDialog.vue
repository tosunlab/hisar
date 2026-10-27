<script setup lang="ts">
import { computed, ref } from "vue";
import type { Organization, OrganizationMember, Team, TeamVault, VaultRole } from "../api";
import {
  createTeam,
  deleteOrganization,
  deleteTeam,
  describeTeamVault,
  giveVaultToTeam,
  leaveOrganization,
  listAllOrganizationVault,
  removeOrganizationMember,
  renameOrganization,
  renameTeam,
  store,
} from "../store";
import { formatRole, vaultRoleDescription } from "../util";
import AccountDialog from "./AccountDialog.vue";
import type { IconName } from "./AppIcon.vue";

export type OrganizationDialogRequest =
  | { kind: "rename-organization" | "delete-organization" | "leave" | "new-team" }
  | { kind: "remove-member"; member: OrganizationMember }
  | { kind: "rename-team" | "delete-team" | "give-vault"; team: Team };

interface DialogText {
  title: string;
  subtitle: string;
  icon: IconName;
  isDanger: boolean;
  confirmLabel: string;
}

const props = defineProps<{ organization: Organization; request: OrganizationDialogRequest }>();
const emit = defineEmits<{ close: [] }>();

const allVaultRole: VaultRole[] = ["owner", "editor", "reader"];

const currentName = computed(() => {
  if (props.request.kind === "rename-organization") {
    return props.organization.name;
  }
  return props.request.kind === "rename-team" ? props.request.team.name : "";
});
const hasName = computed(() => props.request.kind === "rename-organization" || props.request.kind === "rename-team" || props.request.kind === "new-team");
const name = ref(currentName.value);

const allVaultToGive = computed(() => {
  if (props.request.kind !== "give-vault") {
    return [];
  }
  const team = props.request.team;
  return listAllOrganizationVault(props.organization).filter((organizationVault) => !team.vaults.some((teamVault) => teamVault.vaultId === organizationVault.vaultId));
});
const vaultID = ref(allVaultToGive.value[0]?.vaultId ?? "");
const role = ref<VaultRole>("editor");

const text = computed<DialogText>(() => {
  const organizationName = props.organization.name;
  const request = props.request;
  switch (request.kind) {
    case "rename-organization":
      return { title: `Rename “${organizationName}”`, subtitle: `Everyone in ${organizationName} sees the new name.`, icon: "edit", isDanger: false, confirmLabel: "Save" };
    case "delete-organization":
      return { title: `Delete ${organizationName}?`, subtitle: "Its teams and pending invitations are deleted with it.", icon: "trash", isDanger: true, confirmLabel: "Delete organization" };
    case "leave":
      return {
        title: `Leave ${organizationName}?`,
        subtitle: `You lose the vaults you have through ${organizationName}'s teams.`,
        icon: "signOut",
        isDanger: true,
        confirmLabel: `Leave ${organizationName}`,
      };
    case "remove-member": {
      const memberName = request.member.name || request.member.email;
      return {
        title: `Remove ${memberName} from ${organizationName}?`,
        subtitle: `${memberName} loses the vaults they have through ${organizationName}'s teams.`,
        icon: "trash",
        isDanger: true,
        confirmLabel: `Remove from ${organizationName}`,
      };
    }
    case "new-team":
      return { title: "New team", subtitle: `A team in ${organizationName}. Its members get the vaults it is given.`, icon: "users", isDanger: false, confirmLabel: "Create team" };
    case "rename-team":
      return { title: `Rename “${request.team.name}”`, subtitle: `Everyone in ${organizationName} sees the new name.`, icon: "edit", isDanger: false, confirmLabel: "Save" };
    case "delete-team":
      return {
        title: `Delete “${request.team.name}”?`,
        subtitle: `Its members lose the vaults they have only through ${request.team.name}.`,
        icon: "trash",
        isDanger: true,
        confirmLabel: "Delete team",
      };
    case "give-vault":
      return {
        title: `Give a vault to ${request.team.name}`,
        subtitle: `${request.team.name}'s members get it with the role you choose.`,
        icon: "vault",
        isDanger: false,
        confirmLabel: "Give",
      };
  }
  return { title: "", subtitle: "", icon: "building", isDanger: false, confirmLabel: "" };
});

// allLostVaultLine names what goes: the vaults the teams in question have, unopened ones counted together.
const allLostVaultLine = computed(() => {
  const request = props.request;
  let allTeam: Team[] = [];
  if (request.kind === "leave") {
    allTeam = props.organization.teams.filter((team) => team.members.some((member) => member.isCurrentUser));
  } else if (request.kind === "remove-member") {
    allTeam = props.organization.teams.filter((team) => team.members.some((member) => member.accountId === request.member.accountId));
  } else if (request.kind === "delete-team") {
    allTeam = [request.team];
  }
  const vaultByID = new Map<string, TeamVault>(allTeam.flatMap((team) => team.vaults.map((teamVault) => [teamVault.vaultId, teamVault] as const)));
  const allTeamVault = [...vaultByID.values()];
  const allLine = allTeamVault.filter((teamVault) => teamVault.canOpen).map(describeTeamVault);
  const unopenedCount = allTeamVault.length - allLine.length;
  if (unopenedCount > 0) {
    allLine.push(unopenedCount === 1 ? "A vault you can't open" : `${unopenedCount} vaults you can't open`);
  }
  return allLine;
});

const canSubmit = computed(() => {
  if (store.isBusy) {
    return false;
  }
  if (hasName.value) {
    return name.value.trim() !== "" && name.value.trim() !== currentName.value;
  }
  return props.request.kind !== "give-vault" || vaultID.value !== "";
});

async function runRequest(): Promise<boolean> {
  const request = props.request;
  switch (request.kind) {
    case "rename-organization":
      return renameOrganization(props.organization, name.value.trim());
    case "delete-organization":
      return deleteOrganization(props.organization);
    case "leave":
      return leaveOrganization(props.organization);
    case "remove-member":
      return removeOrganizationMember(props.organization, request.member);
    case "new-team":
      return createTeam(props.organization, name.value.trim());
    case "rename-team":
      return renameTeam(props.organization, request.team, name.value.trim());
    case "delete-team":
      return deleteTeam(props.organization, request.team);
    case "give-vault":
      return giveVaultToTeam(props.organization, request.team, vaultID.value, role.value);
  }
  return false;
}

async function submit(): Promise<void> {
  if (canSubmit.value && (await runRequest())) {
    emit("close");
  }
}
</script>

<template>
  <AccountDialog
    :title="text.title"
    :subtitle="text.subtitle"
    :icon="text.icon"
    :is-danger="text.isDanger"
    :is-closable="!store.isBusy"
    :data-testid="`organization-dialog-${request.kind}`"
    @close="emit('close')"
  >
    <form novalidate @submit.prevent="submit">
      <label v-if="hasName">
        <span>Name</span>
        <input v-model="name" name="organizationDialogName" maxlength="100" autocomplete="off" autofocus data-testid="organization-dialog-name" />
        <span class="muted hint">Organization and team names are visible to the server.</span>
      </label>
      <template v-if="request.kind === 'give-vault'">
        <label>
          <span>Vault</span>
          <select v-model="vaultID" name="organizationDialogVault" data-testid="organization-dialog-vault">
            <option v-for="organizationVault in allVaultToGive" :key="organizationVault.vaultId" :value="organizationVault.vaultId">
              {{ organizationVault.canOpen ? organizationVault.name : `A vault you can't open (${organizationVault.allTeamName.join(", ")})` }}
            </option>
          </select>
        </label>
        <label>
          <span>Role</span>
          <select v-model="role" name="organizationDialogRole" aria-describedby="give-vault-role-legend" data-testid="organization-dialog-role">
            <option v-for="vaultRole in allVaultRole" :key="vaultRole" :value="vaultRole">{{ formatRole(vaultRole) }}</option>
          </select>
        </label>
        <p id="give-vault-role-legend" class="muted hint">{{ vaultRoleDescription }}</p>
      </template>
      <ul v-if="allLostVaultLine.length > 0" class="consequences warning" data-testid="organization-dialog-vaults">
        <li v-for="line in allLostVaultLine" :key="line">{{ line }}</li>
      </ul>
      <div class="row actions">
        <button type="button" class="ghost" :disabled="store.isBusy" @click="emit('close')">Cancel</button>
        <button :class="text.isDanger ? 'danger-solid' : 'primary'" type="submit" data-testid="organization-dialog-confirm" :disabled="!canSubmit">
          {{ text.confirmLabel }}
        </button>
      </div>
    </form>
  </AccountDialog>
</template>

<style scoped>
select {
  width: 100%;
}

label .hint {
  display: block;
  margin: 6px 0 0;
  font-size: var(--font-size-11);
  font-weight: 400;
  color: var(--color-text-muted);
}
</style>
