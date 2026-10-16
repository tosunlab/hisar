<script setup lang="ts">
import { computed, ref } from "vue";
import type { Organization, OrganizationMember, OrganizationRole, SentOrganizationInvitation, Team, TeamVault, VaultRole } from "../api";
import {
  addTeamMember,
  cancelOrganizationInvitation,
  describeTeamVault,
  giveVaultToTeam,
  inviteToOrganization,
  listAllOrganizationVault,
  removeTeamMember,
  setOrganizationMemberRole,
  store,
  takeVaultFromTeam,
} from "../store";
import { formatCount, formatRole } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";
import OrganizationDialog, { type OrganizationDialogRequest } from "./OrganizationDialog.vue";

const props = defineProps<{ organization: Organization }>();

const allVaultRole: VaultRole[] = ["owner", "editor", "reader"];

const dialog = ref<OrganizationDialogRequest | null>(null);
const inviteEmail = ref("");
const inviteRole = ref<OrganizationRole>("member");
const cancellingInvitationID = ref("");
const addingTeamID = ref("");
const newTeamMemberID = ref("");

const allOrganizationVault = computed(() => listAllOrganizationVault(props.organization));

function listAllMemberToAdd(team: Team): OrganizationMember[] {
  return props.organization.members.filter((member) => !team.members.some((teamMember) => teamMember.accountId === member.accountId));
}

function canGiveVault(team: Team): boolean {
  return allOrganizationVault.value.some((organizationVault) => !team.vaults.some((teamVault) => teamVault.vaultId === organizationVault.vaultId));
}

async function submitInvite(): Promise<void> {
  if (await inviteToOrganization(props.organization, inviteEmail.value.trim(), inviteRole.value)) {
    inviteEmail.value = "";
    inviteRole.value = "member";
  }
}

async function confirmCancel(invitation: SentOrganizationInvitation): Promise<void> {
  if (await cancelOrganizationInvitation(props.organization, invitation)) {
    cancellingInvitationID.value = "";
  }
}

function startAddingMember(team: Team): void {
  addingTeamID.value = team.id;
  newTeamMemberID.value = listAllMemberToAdd(team)[0]?.accountId ?? "";
}

async function submitAddMember(team: Team): Promise<void> {
  const member = props.organization.members.find((candidate) => candidate.accountId === newTeamMemberID.value);
  if (member && (await addTeamMember(props.organization, team, member))) {
    addingTeamID.value = "";
  }
}

async function changeTeamVaultRole(team: Team, teamVault: TeamVault, event: Event): Promise<void> {
  const select = event.target as HTMLSelectElement;
  if (select.value !== teamVault.role && !(await giveVaultToTeam(props.organization, team, teamVault.vaultId, select.value as VaultRole))) {
    select.value = teamVault.role;
  }
}
</script>

<template>
  <div data-testid="organization-detail">
    <button class="ghost back-button" data-testid="organization-back" @click="store.selectedOrganizationID = ''">
      <AppIcon name="chevronRight" :size="14" class="back-chevron" />Organizations
    </button>

    <header class="organization-header">
      <span class="organization-icon"><AppIcon name="building" :size="18" /></span>
      <div class="organization-heading">
        <h3 data-testid="organization-name">{{ organization.name }}</h3>
        <p class="muted">{{ formatRole(organization.role) }} · {{ formatCount(organization.members.length, "member") }}</p>
      </div>
      <button v-if="organization.isAdmin" class="ghost" data-testid="rename-organization" @click="dialog = { kind: 'rename-organization' }">
        Rename organization…
      </button>
    </header>

    <div class="settings-block" data-testid="organization-members">
      <p class="section-label">Members</p>
      <div class="settings-group">
        <div v-for="member in organization.members" :key="member.accountId" class="setting" :data-testid="`organization-member-${member.accountId}`">
          <MemberAvatar :name="member.name || member.email" />
          <div class="setting-text">
            <strong class="title-line">
              <span class="title-text">{{ member.name || member.email }}</span>
              <span v-if="member.isCurrentUser" class="tag accent">you</span>
              <span class="tag" :class="{ success: member.role === 'admin' }">{{ formatRole(member.role) }}</span>
            </strong>
            <p class="muted">{{ member.email }}</p>
            <p v-if="member.isKeyChanged" class="warning-text" data-testid="organization-member-key-changed">
              Their key changed since you last shared with them. Compare security codes before you share more.
            </p>
          </div>
          <span
            class="security-code monospace"
            data-testid="organization-member-security-code"
            :title="member.isCurrentUser ? 'Your security code' : 'Compare this code with them over a channel you trust'"
            :aria-label="`Security code ${member.securityCode}`"
            >{{ member.securityCode }}</span
          >
          <template v-if="organization.isAdmin">
            <button
              class="ghost role-button"
              :disabled="store.isBusy"
              :data-testid="`toggle-organization-role-${member.accountId}`"
              @click="setOrganizationMemberRole(organization, member, member.role === 'admin' ? 'member' : 'admin')"
            >
              {{ member.role === "admin" ? "Make member" : "Make admin" }}
            </button>
            <button
              v-if="!member.isCurrentUser"
              class="ghost icon-button remove-button"
              :aria-label="`Remove from ${organization.name}…`"
              :title="`Remove from ${organization.name}…`"
              :data-testid="`remove-organization-member-${member.accountId}`"
              @click="dialog = { kind: 'remove-member', member }"
            >
              <AppIcon name="trash" />
            </button>
            <span v-else class="icon-spacer"></span>
          </template>
        </div>
      </div>
      <p class="muted settings-note">Compare a security code with that person over a channel you trust.</p>
    </div>

    <div v-if="organization.isAdmin" class="settings-block" data-testid="organization-invite">
      <p class="section-label">Invite by email</p>
      <div class="settings-group">
        <form class="invite-form" @submit.prevent="submitInvite">
          <input
            v-model="inviteEmail"
            class="invite-email"
            name="organizationInviteEmail"
            type="email"
            aria-label="Email"
            placeholder="name@example.com"
            autocomplete="off"
            data-testid="organization-invite-email"
          />
          <select v-model="inviteRole" class="role-select" aria-label="Role" data-testid="organization-invite-role">
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>
          <button class="primary" type="submit" data-testid="send-organization-invitation" :disabled="store.isBusy || !inviteEmail.trim()">Send invitation</button>
        </form>
        <p class="muted invite-hint">
          They get an email and join {{ organization.name }} when they accept in Hisar; without an account, the email lets them create one first.
          {{
            inviteRole === "admin"
              ? "Admins manage members, teams and which teams have which vaults; being an admin gives no vault."
              : "Members get the vaults of the teams they are in."
          }}
        </p>
      </div>
    </div>

    <div v-if="organization.isAdmin && organization.invitations.length > 0" class="settings-block" data-testid="organization-sent-invitations">
      <p class="section-label">Invited</p>
      <div class="settings-group">
        <div v-for="invitation in organization.invitations" :key="invitation.id" class="setting" :data-testid="`organization-sent-invitation-${invitation.id}`">
          <span class="settings-row-icon"><AppIcon name="mail" :size="14" /></span>
          <div class="setting-text">
            <strong class="title-line">
              <span class="title-text">{{ invitation.email }}</span>
              <span class="tag">{{ formatRole(invitation.role) }}</span>
            </strong>
            <p class="muted">Waiting for them to accept</p>
          </div>
          <button
            v-if="cancellingInvitationID !== invitation.id"
            class="ghost"
            :data-testid="`cancel-organization-invitation-${invitation.id}`"
            @click="cancellingInvitationID = invitation.id"
          >
            Cancel
          </button>
          <template v-else>
            <button class="ghost" @click="cancellingInvitationID = ''">Keep</button>
            <button class="danger-solid" data-testid="confirm-cancel-organization-invitation" :disabled="store.isBusy" @click="confirmCancel(invitation)">
              Cancel invitation
            </button>
          </template>
        </div>
      </div>
    </div>

    <div class="settings-block" data-testid="organization-teams">
      <div class="block-heading">
        <p class="section-label">Teams</p>
        <button v-if="organization.isAdmin" class="ghost small-button" data-testid="new-team" @click="dialog = { kind: 'new-team' }">
          <AppIcon name="plus" :size="13" />New team…
        </button>
      </div>
      <p v-if="organization.teams.length === 0" class="muted empty-note" data-testid="organization-no-teams">
        No teams yet. A team's members get the vaults it is given.
      </p>
      <div v-for="team in organization.teams" :key="team.id" class="settings-group team" :data-testid="`team-${team.id}`">
        <div class="setting team-header">
          <span class="settings-row-icon"><AppIcon name="users" :size="14" /></span>
          <div class="setting-text">
            <strong class="title-line"><span class="title-text" data-testid="team-name">{{ team.name }}</span></strong>
            <p class="muted">{{ formatCount(team.members.length, "member") }} · {{ formatCount(team.vaults.length, "vault") }}</p>
          </div>
          <template v-if="organization.isAdmin">
            <button class="ghost" :data-testid="`rename-team-${team.id}`" @click="dialog = { kind: 'rename-team', team }">Rename…</button>
            <button class="ghost danger" :data-testid="`delete-team-${team.id}`" @click="dialog = { kind: 'delete-team', team }">Delete team…</button>
          </template>
        </div>

        <p class="team-label">Members</p>
        <p v-if="team.members.length === 0" class="muted team-empty">No members yet</p>
        <div v-for="member in team.members" :key="member.accountId" class="setting team-row" :data-testid="`team-member-${team.id}-${member.accountId}`">
          <MemberAvatar :name="member.name || member.email" :size="24" />
          <div class="setting-text">
            <span class="title-line">
              <span class="title-text">{{ member.name || member.email }}</span>
              <span v-if="member.isCurrentUser" class="tag accent">you</span>
            </span>
          </div>
          <span class="muted team-row-detail">{{ member.email }}</span>
          <button
            v-if="organization.isAdmin"
            class="ghost"
            :disabled="store.isBusy"
            :data-testid="`remove-team-member-${team.id}-${member.accountId}`"
            @click="removeTeamMember(organization, team, member)"
          >
            Remove
          </button>
        </div>
        <form v-if="addingTeamID === team.id" class="settings-group-form team-add" data-testid="team-add-member-form" @submit.prevent="submitAddMember(team)">
          <select v-model="newTeamMemberID" aria-label="Member to add" data-testid="team-add-member-select">
            <option v-for="member in listAllMemberToAdd(team)" :key="member.accountId" :value="member.accountId">
              {{ member.name ? `${member.name} (${member.email})` : member.email }}
            </option>
          </select>
          <button type="button" class="ghost" @click="addingTeamID = ''">Cancel</button>
          <button class="primary" type="submit" data-testid="team-add-member-confirm" :disabled="store.isBusy || !newTeamMemberID">Add</button>
        </form>

        <p class="team-label">Vaults</p>
        <p v-if="team.vaults.length === 0" class="muted team-empty">No vaults yet</p>
        <div v-for="teamVault in team.vaults" :key="teamVault.vaultId" class="setting team-row" :data-testid="`team-vault-${team.id}-${teamVault.vaultId}`">
          <span class="vault-icon"><AppIcon name="vault" :size="14" /></span>
          <div class="setting-text">
            <span class="title-text" :class="{ 'unopened-vault': !teamVault.canOpen }" data-testid="team-vault-name">{{ describeTeamVault(teamVault) }}</span>
          </div>
          <template v-if="organization.isAdmin">
            <select
              class="role-select"
              :value="teamVault.role"
              :aria-label="`Role of ${team.name} in ${describeTeamVault(teamVault)}`"
              :disabled="store.isBusy"
              :data-testid="`team-vault-role-${team.id}-${teamVault.vaultId}`"
              @change="changeTeamVaultRole(team, teamVault, $event)"
            >
              <option v-for="role in allVaultRole" :key="role" :value="role">{{ formatRole(role) }}</option>
            </select>
            <button
              class="ghost"
              :disabled="store.isBusy"
              :data-testid="`take-back-${team.id}-${teamVault.vaultId}`"
              @click="takeVaultFromTeam(organization, team, teamVault)"
            >
              Take back
            </button>
          </template>
          <span v-else class="tag">{{ formatRole(teamVault.role) }}</span>
        </div>

        <div v-if="organization.isAdmin" class="team-actions">
          <button
            v-if="addingTeamID !== team.id && listAllMemberToAdd(team).length > 0"
            class="ghost small-button"
            :data-testid="`add-team-member-${team.id}`"
            @click="startAddingMember(team)"
          >
            <AppIcon name="plus" :size="13" />Add member
          </button>
          <button v-if="canGiveVault(team)" class="ghost small-button" :data-testid="`give-vault-${team.id}`" @click="dialog = { kind: 'give-vault', team }">
            <AppIcon name="plus" :size="13" />Give a vault…
          </button>
        </div>
      </div>
    </div>

    <div class="settings-block" data-testid="organization-membership">
      <p class="section-label">Your membership</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong>Leave {{ organization.name }}</strong>
            <p class="muted">You lose the vaults you have through {{ organization.name }}'s teams.</p>
          </div>
          <button class="danger" data-testid="leave-organization" @click="dialog = { kind: 'leave' }">Leave {{ organization.name }}…</button>
        </div>
        <div v-if="organization.isAdmin" class="setting">
          <div class="setting-text">
            <strong>Delete organization</strong>
            <p class="muted">Its teams and pending invitations are deleted with it.</p>
          </div>
          <button class="danger" data-testid="delete-organization" @click="dialog = { kind: 'delete-organization' }">Delete organization…</button>
        </div>
      </div>
    </div>

    <OrganizationDialog v-if="dialog" :organization="organization" :request="dialog" @close="dialog = null" />
  </div>
</template>

<style scoped>
.back-button {
  margin: 0 0 10px -8px;
  padding: 4px 8px;
}

.back-chevron {
  transform: rotate(180deg);
}

.organization-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.organization-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.organization-heading {
  flex: 1;
  min-width: 0;
}

.organization-heading h3 {
  margin: 0;
  overflow: hidden;
  font-size: 15px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.organization-heading p {
  margin: 2px 0 0;
  font-size: 12px;
}

.security-code {
  flex: none;
  font-size: 12px;
  color: var(--color-text-secondary);
  user-select: text;
  -webkit-user-select: text;
}

.role-button {
  flex: none;
  width: 104px;
  padding: 5px 8px;
  white-space: nowrap;
  font-size: 12px;
}

.remove-button:hover:not(:disabled) {
  color: var(--color-danger);
}

.icon-spacer {
  width: 30px;
  flex: none;
}

.invite-form {
  display: flex;
  gap: 8px;
  padding: 10px 14px 0;
}

.invite-email {
  flex: 1;
  min-width: 0;
}

.role-select {
  width: 96px;
  flex: none;
  padding-top: 5px;
  padding-bottom: 5px;
  font-size: 12px;
}

.invite-form .role-select {
  padding-top: 7px;
  padding-bottom: 7px;
}

.invite-form button {
  flex: none;
  white-space: nowrap;
}

.invite-hint {
  margin: 0;
  padding: 8px 14px 10px;
  font-size: 12px;
  line-height: 1.45;
}

.block-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 0 0 6px 2px;
}

.block-heading .section-label {
  margin: 0;
}

.small-button {
  gap: 5px;
  padding: 4px 8px;
  font-size: 12px;
}

.team + .team {
  margin-top: 10px;
}

.team-header {
  background: var(--color-surface-raised);
}

.team-label {
  margin: 0;
  padding: 8px 14px 2px;
  font-size: 11px;
  font-weight: 600;
  color: var(--color-text-muted);
}

.team-empty {
  margin: 0;
  padding: 2px 14px 8px;
  font-size: 12px;
}

.settings-group > .team-row {
  gap: 10px;
  padding-top: 6px;
  padding-bottom: 6px;
}

.team-row-detail {
  flex: none;
  max-width: 40%;
  overflow: hidden;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.team-row .setting-text {
  display: flex;
  align-items: center;
}

.team-row button {
  flex: none;
  padding: 4px 8px;
  font-size: 12px;
}

.vault-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  flex: none;
  color: var(--color-text-muted);
}

.unopened-vault {
  font-style: italic;
  color: var(--color-text-muted);
}

.team-add {
  padding-top: 6px;
}

.team-add select {
  flex: 1;
  min-width: 0;
}

.team-actions {
  display: flex;
  gap: 4px;
  padding: 6px 10px 8px;
}
</style>
