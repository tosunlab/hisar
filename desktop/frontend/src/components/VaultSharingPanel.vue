<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { VaultMember, VaultRole } from "../api";
import { changeVaultMemberRole, closeSharing, getFirstName, getSelectedVault, removeVaultMember, shareVault, store } from "../store";
import { formatRole } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";

const allVaultRole: VaultRole[] = ["owner", "editor", "reader"];

const expandedAccountID = ref("");
const removingAccountID = ref("");
const newMemberAccountID = ref("");
const newMemberRole = ref<VaultRole>("editor");

const vault = computed(() => getSelectedVault());
const isOwner = computed(() => vault.value?.role === "owner");
const allAvailableMember = computed(() =>
  store.allFamilyMember.filter((familyMember) => !store.allVaultMember.some((vaultMember) => vaultMember.accountId === familyMember.accountId)),
);
const memberCountText = computed(() => {
  const count = store.allVaultMember.length;
  return count === 1 ? "1 member" : `${count} members`;
});
const ownRole = computed(() => store.allVaultMember.find((vaultMember) => vaultMember.isCurrentUser)?.role ?? vault.value?.role ?? "");

function getDisplayName(member: VaultMember): string {
  return member.name || member.email;
}

function toggleSecurityCode(accountID: string): void {
  expandedAccountID.value = expandedAccountID.value === accountID ? "" : accountID;
}

async function submitShare(): Promise<void> {
  const familyMember = store.allFamilyMember.find((candidate) => candidate.accountId === newMemberAccountID.value);
  if (familyMember && (await shareVault(familyMember, newMemberRole.value))) {
    newMemberAccountID.value = "";
    newMemberRole.value = "editor";
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
          <p class="muted">{{ memberCountText }} · Everyone here can open the items in this vault</p>
        </div>
        <button class="ghost icon-button" aria-label="Close sharing" @click="closeSharing"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

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
              <p v-if="member.isCurrentUser" class="muted">This is your security code. Family members can compare it with the code they see for you.</p>
              <p v-else class="muted">Compare this code with {{ getFirstName(member) }} in person to be sure the server isn't intercepting.</p>
              <p v-if="member.isKeyChanged" class="warning-text">
                {{ getFirstName(member) }}'s key changed since you last shared with them. Hisar refuses to share until you sort this out with your admin.
              </p>
            </div>
          </div>
        </li>
      </ul>

      <form v-if="isOwner" class="add-member" data-testid="add-member" @submit.prevent="submitShare">
        <p class="section-label">Add member</p>
        <div v-if="allAvailableMember.length > 0" class="add-row">
          <select v-model="newMemberAccountID" class="member-select" aria-label="Family member" data-testid="new-member-select">
            <option value="" disabled>Choose a family member</option>
            <option v-for="familyMember in allAvailableMember" :key="familyMember.accountId" :value="familyMember.accountId">
              {{ familyMember.name || familyMember.email }} · {{ familyMember.email }}
            </option>
          </select>
          <select v-model="newMemberRole" class="role-select" aria-label="Role for the new member" data-testid="new-member-role">
            <option v-for="role in allVaultRole" :key="role" :value="role">{{ formatRole(role) }}</option>
          </select>
          <button class="primary" type="submit" :disabled="!newMemberAccountID || store.isBusy">Share</button>
        </div>
        <p v-else-if="store.allFamilyMember.length > 0" class="muted hint">Everyone in your family already has access. Invite someone new in Settings.</p>
        <p class="muted hint">Editors can add and change items, readers can only view them, and owners also manage who has access.</p>
      </form>
      <p v-else class="read-only-note" data-testid="sharing-read-only">
        <AppIcon name="lock" :size="14" />
        <span>You are {{ ownRole === "editor" ? "an editor" : `a ${ownRole || "member"}` }} in this vault. Only its owners can change who has access.</span>
      </p>
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
  background: rgb(0 0 0 / 55%);
}

.sharing {
  width: min(560px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px rgb(0 0 0 / 50%);
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
  border-color: #4a1f22;
  background: var(--color-danger-background);
}

.confirm p {
  flex: 1;
  color: #f2c4c5;
}

.confirm .detail-icon {
  color: #f2a7a9;
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

.member-select {
  flex: 1;
  min-width: 0;
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
</style>
