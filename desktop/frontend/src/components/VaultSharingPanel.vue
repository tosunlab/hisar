<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { SentShareInvitation, Team, TeamVault, VaultMember, VaultRole } from "../api";
import {
  cancelShareInvitation,
  changeVaultMemberRole,
  closeSharing,
  getAllSentShareInvitation,
  getFirstName,
  getSelectedVault,
  giveVaultToTeam,
  removeVaultMember,
  shareVault,
  store,
  takeVaultFromTeam,
} from "../store";
import { formatRole } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";
import ShareRecipientConfirmation from "./ShareRecipientConfirmation.vue";

const allVaultRole: VaultRole[] = ["owner", "editor", "reader"];

const expandedAccountID = ref("");
const removingAccountID = ref("");
const newMemberEmail = ref("");
const newMemberRole = ref<VaultRole>("editor");
const cancellingInvitationID = ref("");

const vault = computed(() => getSelectedVault());
const isOwner = computed(() => vault.value?.role === "owner");
const allSentInvitation = computed(() => (vault.value ? getAllSentShareInvitation(vault.value.id) : []));
const memberCountText = computed(() => {
  const count = store.allVaultMember.length;
  return count === 1 ? "1 member" : `${count} members`;
});
const ownRole = computed(() => store.allVaultMember.find((vaultMember) => vaultMember.isCurrentUser)?.role ?? vault.value?.role ?? "");

// An organization vault reaches people through its organization's teams, so its panel shows those teams instead of members.
const organizationID = computed(() => vault.value?.organizationId ?? "");
const organization = computed(() => (organizationID.value ? store.organizationByID[organizationID.value] : undefined));
const organizationName = computed(
  () => organization.value?.name ?? store.organizations.organizations.find((candidate) => candidate.id === organizationID.value)?.name ?? "",
);
const allTeamWithVault = computed(() =>
  (organization.value?.teams ?? []).flatMap((team) => {
    const teamVault = team.vaults.find((candidate) => candidate.vaultId === vault.value?.id);
    const memberText = team.members.map((member) => (member.isCurrentUser ? `${member.name || member.email} (you)` : member.name || member.email)).join(", ");
    return teamVault ? [{ team, teamVault, memberText }] : [];
  }),
);
const allTeamWithoutVault = computed(() => (organization.value?.teams ?? []).filter((team) => !team.vaults.some((candidate) => candidate.vaultId === vault.value?.id)));
const isGivingToTeam = ref(false);
const givingTeamID = ref("");
const givingRole = ref<VaultRole>("editor");

function startGivingToTeam(): void {
  givingTeamID.value = allTeamWithoutVault.value[0]?.id ?? "";
  givingRole.value = "editor";
  isGivingToTeam.value = true;
}

async function submitGiveToTeam(): Promise<void> {
  const team = allTeamWithoutVault.value.find((candidate) => candidate.id === givingTeamID.value);
  if (organization.value && vault.value && team && (await giveVaultToTeam(organization.value, team, vault.value.id, givingRole.value))) {
    isGivingToTeam.value = false;
  }
}

async function changeTeamRole(team: Team, teamVault: TeamVault, event: Event): Promise<void> {
  const select = event.target as HTMLSelectElement;
  if (organization.value && select.value !== teamVault.role && !(await giveVaultToTeam(organization.value, team, teamVault.vaultId, select.value as VaultRole))) {
    select.value = teamVault.role;
  }
}

function getDisplayName(member: VaultMember): string {
  return member.name || member.email;
}

function toggleSecurityCode(accountID: string): void {
  expandedAccountID.value = expandedAccountID.value === accountID ? "" : accountID;
}

async function submitShare(): Promise<void> {
  if (await shareVault(newMemberEmail.value.trim(), newMemberRole.value)) {
    newMemberEmail.value = "";
    newMemberRole.value = "editor";
  }
}

async function confirmCancel(invitation: SentShareInvitation): Promise<void> {
  if (await cancelShareInvitation(invitation)) {
    cancellingInvitationID.value = "";
  }
}

async function changeRole(member: VaultMember, event: Event): Promise<void> {
  const role = (event.target as HTMLSelectElement).value as VaultRole;
  if (role !== member.role) {
    await changeVaultMemberRole(member, role);
  }
}

async function confirmRemove(member: VaultMember): Promise<void> {
  if (await removeVaultMember(member)) {
    removingAccountID.value = "";
  }
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeSharing();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeSharing">
    <div class="card sharing" role="dialog" aria-modal="true" aria-labelledby="sharing-title" data-testid="vault-sharing-panel">
      <header class="sharing-header">
        <span class="sharing-icon"><AppIcon name="users" :size="18" /></span>
        <div class="sharing-heading">
          <h2 id="sharing-title">Share “{{ vault?.name }}”</h2>
          <p v-if="organizationID" class="muted" data-testid="sharing-organization">Shared through teams in {{ organizationName }}</p>
          <p v-else class="muted">{{ memberCountText }} · Everyone here can open the items in this vault</p>
        </div>
        <button class="ghost icon-button" aria-label="Close sharing" @click="closeSharing"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <template v-if="organizationID">
        <p class="section-label">Teams</p>
        <p v-if="!organization" class="muted loading">{{ store.isBusy ? "Loading teams…" : "" }}</p>
        <ul class="member-list" aria-label="Teams that have this vault" data-testid="vault-teams">
          <li v-for="{ team, teamVault, memberText } in allTeamWithVault" :key="team.id" class="member" :data-testid="`vault-team-${team.id}`">
            <div class="member-row">
              <span class="team-icon"><AppIcon name="users" :size="15" /></span>
              <div class="member-text">
                <span class="member-name"><span class="name-text">{{ team.name }}</span></span>
                <span class="member-email" :title="memberText">{{ memberText || "No members yet" }}</span>
              </div>
              <template v-if="organization?.isAdmin">
                <select
                  class="role-select"
                  :value="teamVault.role"
                  :aria-label="`Role for ${team.name}`"
                  title="Readers view items, editors also change them, owners also share, rename and delete the vault."
                  :disabled="store.isBusy"
                  :data-testid="`vault-team-role-${team.id}`"
                  @change="changeTeamRole(team, teamVault, $event)"
                >
                  <option v-for="role in allVaultRole" :key="role" :value="role">{{ formatRole(role) }}</option>
                </select>
                <button
                  class="ghost take-back-button"
                  :disabled="store.isBusy"
                  :data-testid="`take-back-from-${team.id}`"
                  @click="organization && takeVaultFromTeam(organization, team, teamVault)"
                >
                  Take back
                </button>
              </template>
              <span v-else class="role-label">{{ formatRole(teamVault.role) }}</span>
            </div>
          </li>
        </ul>
        <template v-if="organization?.isAdmin">
          <form v-if="isGivingToTeam" class="add-member" data-testid="give-to-team-form" @submit.prevent="submitGiveToTeam">
            <p class="section-label">Give to a team</p>
            <div class="add-row">
              <select v-model="givingTeamID" class="team-select" aria-label="Team" data-testid="give-to-team-team">
                <option v-for="team in allTeamWithoutVault" :key="team.id" :value="team.id">{{ team.name }}</option>
              </select>
              <select v-model="givingRole" class="role-select" aria-label="Role for the team" title="Readers view items, editors also change them, owners also share, rename and delete the vault." data-testid="give-to-team-role">
                <option v-for="role in allVaultRole" :key="role" :value="role">{{ formatRole(role) }}</option>
              </select>
              <button type="button" class="ghost" @click="isGivingToTeam = false">Cancel</button>
              <button class="primary" type="submit" data-testid="give-to-team-confirm" :disabled="!givingTeamID || store.isBusy">Give</button>
            </div>
          </form>
          <button v-else-if="allTeamWithoutVault.length > 0" data-testid="give-to-team" @click="startGivingToTeam">
            <AppIcon name="plus" :size="14" />Give to a team…
          </button>
        </template>
        <p v-else-if="organization" class="read-only-note" data-testid="sharing-organization-read-only">
          <AppIcon name="lock" :size="14" />
          <span>Ask an admin of {{ organizationName }} to change who has this vault.</span>
        </p>
      </template>

      <template v-else>
        <p class="section-label">Members</p>
        <p v-if="store.allVaultMember.length === 0" class="muted loading">{{ store.isBusy ? "Loading members…" : "No members to show." }}</p>
        <ul class="member-list" aria-label="Vault members">
          <li v-for="member in store.allVaultMember" :key="member.accountId" class="member" :data-testid="`vault-member-${member.accountId}`">
            <div class="member-row">
              <MemberAvatar :name="getDisplayName(member)" />
              <div class="member-text">
                <span class="member-name">
                  <span class="name-text">{{ getDisplayName(member) }}</span>
                  <span v-if="member.isCurrentUser" class="tag accent">you</span>
                  <span v-if="member.isKeyChanged" class="warning-text" title="Key changed"><AppIcon name="warning" :size="13" /></span>
                </span>
                <span class="member-email">{{ member.email }}</span>
              </div>
              <button
                class="ghost code-toggle"
                :class="{ expanded: expandedAccountID === member.accountId }"
                :aria-expanded="expandedAccountID === member.accountId"
                :aria-label="`Security code for ${getDisplayName(member)}`"
                title="Security code"
                :data-testid="`security-code-toggle-${member.accountId}`"
                @click="toggleSecurityCode(member.accountId)"
              >
                <AppIcon name="shield" :size="14" /><AppIcon name="chevronDown" :size="12" class="chevron" />
              </button>
              <template v-if="isOwner && !member.isCurrentUser">
                <select
                  class="role-select"
                  :value="member.role"
                  :aria-label="`Role for ${getDisplayName(member)}`"
                  title="Readers view items, editors also change them, owners also share, rename and delete the vault."
                  :disabled="store.isBusy"
                  :data-testid="`role-select-${member.accountId}`"
                  @change="changeRole(member, $event)"
                >
                  <option v-for="role in allVaultRole" :key="role" :value="role">{{ formatRole(role) }}</option>
                </select>
                <button
                  class="ghost icon-button remove-button"
                  :aria-label="`Remove ${getDisplayName(member)}`"
                  title="Remove from vault"
                  :disabled="store.isBusy"
                  :data-testid="`remove-member-${member.accountId}`"
                  @click="removingAccountID = member.accountId"
                >
                  <AppIcon name="trash" />
                </button>
              </template>
              <span v-else class="role-label" :class="{ aligned: isOwner }">{{ formatRole(member.role) }}</span>
            </div>

            <div v-if="removingAccountID === member.accountId" class="member-detail confirm" data-testid="remove-confirmation">
              <AppIcon name="refresh" class="detail-icon" />
              <p>
                Removing {{ getFirstName(member) }} re-encrypts this vault with a new key.
                <span class="muted">Copies they already downloaded stay readable to them.</span>
              </p>
              <button class="ghost" :disabled="store.isBusy" @click="removingAccountID = ''">Cancel</button>
              <button class="danger-solid" :disabled="store.isBusy" data-testid="confirm-remove-member" @click="confirmRemove(member)">Remove</button>
            </div>

            <div v-if="expandedAccountID === member.accountId" class="member-detail code-detail" data-testid="security-code-detail">
              <AppIcon name="shield" class="detail-icon" />
              <div>
                <span class="security-code monospace" data-testid="security-code">{{ member.securityCode }}</span>
                <p v-if="member.isCurrentUser" class="muted">This is your security code. People you share with can compare it with the code they see for you.</p>
                <p v-else class="muted">Compare this code with {{ getFirstName(member) }} in person or over a channel you trust, to be sure the server isn't intercepting.</p>
                <p v-if="member.isKeyChanged" class="warning-text">
                  {{ getFirstName(member) }}'s key changed since you last shared with them. Hisar refuses to share until you compare security codes again.
                </p>
              </div>
            </div>
          </li>
        </ul>

        <template v-if="isOwner">
          <template v-if="allSentInvitation.length > 0">
            <p class="section-label">Invited</p>
            <ul class="member-list" aria-label="Pending invitations" data-testid="vault-invitations">
              <li v-for="invitation in allSentInvitation" :key="invitation.id" class="member" :data-testid="`vault-invitation-${invitation.id}`">
                <div class="member-row">
                  <span class="invitation-icon"><AppIcon name="mail" :size="15" /></span>
                  <div class="member-text">
                    <span class="member-name"><span class="name-text">{{ invitation.email }}</span></span>
                    <span class="member-email">{{ invitation.requiresConfirmation ? "Accepted · Review recipient identity" : invitation.state === "accepted" ? "Accepted · Hisar is finishing sharing" : "Waiting for them to accept" }}</span>
                  </div>
                  <template v-if="cancellingInvitationID !== invitation.id">
                    <span class="role-label">{{ formatRole(invitation.role) }}</span>
                    <button
                      class="ghost icon-button remove-button"
                      :aria-label="`Cancel the invitation to ${invitation.email}`"
                      title="Cancel invitation"
                      :data-testid="`cancel-vault-invitation-${invitation.id}`"
                      @click="cancellingInvitationID = invitation.id"
                    >
                      <AppIcon name="close" />
                    </button>
                  </template>
                  <template v-else>
                    <button class="ghost" @click="cancellingInvitationID = ''">Keep</button>
                    <button class="danger-solid" data-testid="confirm-cancel-vault-invitation" :disabled="store.isBusy" @click="confirmCancel(invitation)">
                      Cancel invitation
                    </button>
                  </template>
                </div>
                <ShareRecipientConfirmation :invitation="invitation" />
              </li>
            </ul>
          </template>
          <form class="add-member" data-testid="add-member" @submit.prevent="submitShare">
            <p class="section-label">Invite by email</p>
            <div class="add-row">
              <input
                v-model="newMemberEmail"
                class="member-email-input"
                name="shareEmail"
                type="email"
                aria-label="Email"
                placeholder="name@example.com"
                autocomplete="off"
                data-testid="new-member-email"
              />
              <select v-model="newMemberRole" class="role-select" aria-label="Role for the new member" title="Readers view items, editors also change them, owners also share, rename and delete the vault." data-testid="new-member-role">
                <option v-for="role in allVaultRole" :key="role" :value="role">{{ formatRole(role) }}</option>
              </select>
              <button class="primary" type="submit" data-testid="send-share-invitation" :disabled="!newMemberEmail.trim() || store.isBusy">Share</button>
            </div>
            <p class="muted hint">
              Shares this vault. They get an email and accept in Hisar; without an account, the email lets them create one first. Your Hisar then
              shares the vault key with them.
            </p>
          </form>
        </template>
        <p v-else class="read-only-note" data-testid="sharing-read-only">
          <AppIcon name="lock" :size="14" />
          <span>You are {{ ownRole === "editor" ? "an editor" : `a ${ownRole || "member"}` }} in this vault. Only its owners can change who has access.</span>
        </p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--color-backdrop);
}

.sharing {
  width: min(560px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

.sharing-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.sharing-icon {
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

.sharing-heading {
  flex: 1;
  min-width: 0;
}

h2 {
  margin: 0;
  overflow: hidden;
  font-size: 17px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sharing-heading p {
  margin: 1px 0 0;
  font-size: 12px;
}

.loading {
  margin: 0 0 12px;
  font-size: 12px;
}

.member-list {
  margin: 0 0 18px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  list-style: none;
}

.member-list:empty {
  display: none;
}

.member + .member {
  border-top: 1px solid var(--color-border);
}

.member-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
}

.member-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.member-name {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-weight: 600;
}

.name-text,
.member-email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-email {
  font-size: 12px;
  color: var(--color-text-muted);
}

.code-toggle {
  gap: 2px;
  padding: 5px 6px;
  color: var(--color-text-muted);
}

.code-toggle .chevron {
  transition: transform 120ms;
}

.code-toggle.expanded {
  background: var(--color-hover);
  color: var(--color-text);
}

.code-toggle.expanded .chevron {
  transform: rotate(180deg);
}

.role-select {
  width: 96px;
  padding-top: 5px;
  padding-bottom: 5px;
  font-size: 12px;
}

.role-label {
  width: 96px;
  font-size: 12px;
  color: var(--color-text-secondary);
  text-align: right;
}

.role-label.aligned {
  width: 134px;
  padding-left: 11px;
  text-align: left;
}

.remove-button:hover:not(:disabled) {
  color: var(--color-danger);
}

.member-detail {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 0 12px 10px 54px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-panel);
  font-size: 12px;
}

.member-detail p {
  margin: 0;
}

.member-detail .detail-icon {
  margin-top: 1px;
  color: var(--color-text-muted);
}

.confirm {
  align-items: center;
  border-color: var(--color-danger-border);
  background: var(--color-danger-background);
}

.confirm p {
  flex: 1;
  color: var(--color-danger-text);
}

.confirm .detail-icon {
  color: var(--color-danger-text);
}

.code-detail > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.security-code {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.12em;
  color: var(--color-text);
  user-select: text;
  -webkit-user-select: text;
}

.add-member {
  margin: 0;
}

.add-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

.member-email-input {
  flex: 1;
  min-width: 0;
}

.invitation-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex: none;
  border: 1px dashed var(--color-border-strong);
  border-radius: 50%;
  color: var(--color-text-muted);
}

.hint {
  margin: 0;
  font-size: 12px;
}

.hint + .hint {
  margin-top: 6px;
}

.read-only-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  font-size: 12px;
  color: var(--color-text-secondary);
}

.team-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.take-back-button {
  flex: none;
  padding: 5px 8px;
  font-size: 12px;
}

.team-select {
  flex: 1;
  min-width: 0;
}
</style>
