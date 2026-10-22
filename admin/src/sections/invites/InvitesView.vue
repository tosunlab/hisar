<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getErrorMessage } from "../../api";
import { proveAdmin } from "../accounts/accounts";
import {
  createInvite,
  describeDelivery,
  describeSender,
  formatDate,
  listInvites,
  resendInvite,
  revokeInvite,
  roleLabels,
  type AdminInvite,
  type CreatedInvite,
  type InviteRole,
} from "./invites";

const invites = ref<AdminInvite[] | null>(null);
const loadError = ref("");
const rowError = ref("");
const busyInviteId = ref("");

const isFormOpen = ref(false);
const email = ref("");
const role = ref<InviteRole>("member");
const isCreating = ref(false);
const createError = ref("");
const secretKey = ref("");
const password = ref("");
const pendingResend = ref<AdminInvite | null>(null);
const isProving = ref(false);

function clearFactors(): void {
  secretKey.value = "";
  password.value = "";
}

function closeForm(): void {
  isFormOpen.value = false;
  pendingResend.value = null;
  clearFactors();
}

const created = ref<CreatedInvite | null>(null);
const copyMessage = ref("");

onMounted(async () => {
  try {
    invites.value = await listInvites();
  } catch (error) {
    loadError.value = getErrorMessage(error);
  }
});

function openForm(): void {
  closeForm();
  email.value = "";
  role.value = "member";
  createError.value = "";
  created.value = null;
  isFormOpen.value = true;
}

// showCreated puts a fresh invite at the top of the list, where the server sorts the newest, and shows its link once.
function showCreated(fresh: CreatedInvite, replacedId = ""): void {
  invites.value = [fresh.invite, ...(invites.value ?? []).filter((invite) => invite.id !== replacedId)];
  created.value = fresh;
  copyMessage.value = "";
}

async function submit(): Promise<void> {
  isCreating.value = true;
  createError.value = "";
  try {
    isProving.value = role.value === "admin";
    const proof = role.value === "admin" ? await proveAdmin(secretKey.value, password.value) : undefined;
    clearFactors();
    isProving.value = false;
    showCreated(await createInvite(email.value, role.value, proof));
    closeForm();
  } catch (error) {
    createError.value = getErrorMessage(error);
  } finally {
    isCreating.value = false;
    isProving.value = false;
  }
}

async function copyLink(): Promise<void> {
  try {
    await navigator.clipboard.writeText(created.value?.link ?? "");
    copyMessage.value = "Copied";
  } catch {
    copyMessage.value = "Select the link and copy it.";
  }
}

// changeInvite runs a revoke or a resend on one row; when the invite is gone meanwhile, the list is read again.
async function changeInvite(invite: AdminInvite, change: () => Promise<void>): Promise<void> {
  busyInviteId.value = invite.id;
  rowError.value = "";
  try {
    await change();
  } catch (error) {
    rowError.value = getErrorMessage(error);
    invites.value = await listInvites().catch(() => invites.value);
  } finally {
    busyInviteId.value = "";
  }
}

function revoke(invite: AdminInvite): Promise<void> {
  return changeInvite(invite, async () => {
    await revokeInvite(invite.id);
    invites.value = (invites.value ?? []).filter((listed) => listed.id !== invite.id);
    if (created.value?.invite.id === invite.id) {
      created.value = null;
    }
  });
}

async function resend(invite: AdminInvite): Promise<void> {
  if (invite.role === "admin") {
    closeForm();
    createError.value = "";
    pendingResend.value = invite;
    return;
  }
  await changeInvite(invite, async () => {
    closeForm();
    showCreated(await resendInvite(invite.id), invite.id);
  });
}

async function submitResend(): Promise<void> {
  const invite = pendingResend.value;
  if (!invite) return;
  isCreating.value = true;
  createError.value = "";
  try {
    isProving.value = true;
    const proof = await proveAdmin(secretKey.value, password.value);
    clearFactors();
    isProving.value = false;
    showCreated(await resendInvite(invite.id, proof), invite.id);
    closeForm();
  } catch (error) {
    createError.value = getErrorMessage(error);
  } finally {
    isProving.value = false;
    isCreating.value = false;
  }
}
</script>

<template>
  <section class="invites">
    <header class="title">
      <h1>Invites</h1>
      <button type="button" class="primary" :disabled="isCreating" @click="openForm">Invite to Hisar…</button>
    </header>
    <p class="description">Sign-up invites nobody has used yet. Each invite works once and expires after 7 days.</p>

    <form v-if="isFormOpen" class="panel" role="dialog" aria-label="Invite to Hisar" @submit.prevent="submit">
      <label class="field">
        <span>Email</span>
        <input v-model="email" name="email" type="email" placeholder="name@example.com" autocomplete="off" spellcheck="false" :disabled="isCreating" required />
      </label>
      <fieldset class="choices" :disabled="isCreating">
        <legend>Role</legend>
        <label v-for="(label, value) in roleLabels" :key="value" class="choice">
          <input v-model="role" type="radio" name="role" :value="value" />
          <span>{{ label }}</span>
        </label>
        <span class="hint">An admin can also sign in to this dashboard.</span>
      </fieldset>
      <template v-if="role === 'admin'">
        <label class="field">
          <span>Your Secret Key</span>
          <input v-model="secretKey" name="secretKey" type="password" autocomplete="off" spellcheck="false" :disabled="isCreating" required />
        </label>
        <label class="field">
          <span>Your master password</span>
          <input v-model="password" name="password" type="password" autocomplete="current-password" :disabled="isCreating" required />
        </label>
      </template>
      <p v-if="createError" class="error" role="alert">{{ createError }}</p>
      <div class="actions">
        <button type="button" class="ghost" :disabled="isCreating" @click="closeForm">Cancel</button>
        <button type="submit" class="primary" :disabled="isCreating">{{ isProving ? "Checking master password…" : isCreating ? "Creating…" : "Create invite" }}</button>
      </div>
    </form>

    <form v-if="pendingResend" class="panel" role="dialog" aria-label="Resend admin invite" @submit.prevent="submitResend">
      <p>Send a new admin invite to {{ pendingResend.email }}. The old link will stop working.</p>
      <label class="field">
        <span>Your Secret Key</span>
        <input v-model="secretKey" name="secretKey" type="password" autocomplete="off" spellcheck="false" :disabled="isCreating" required />
      </label>
      <label class="field">
        <span>Your master password</span>
        <input v-model="password" name="password" type="password" autocomplete="current-password" :disabled="isCreating" required />
      </label>
      <p v-if="createError" class="error" role="alert">{{ createError }}</p>
      <div class="actions">
        <button type="button" class="ghost" :disabled="isCreating" @click="closeForm">Cancel</button>
        <button type="submit" class="primary" :disabled="isCreating">{{ isProving ? "Checking master password…" : isCreating ? "Sending…" : "Resend invite" }}</button>
      </div>
    </form>

    <section v-if="created" class="panel" aria-label="New invite" data-testid="created-invite">
      <p class="delivery" role="status">{{ describeDelivery(created) }}</p>
      <div class="link">
        <input :value="created.link" name="link" class="monospace" readonly aria-label="Invite link" @focus="($event.target as HTMLInputElement).select()" />
        <button type="button" @click="copyLink">Copy link</button>
      </div>
      <p class="hint">
        <span v-if="copyMessage" role="status">{{ copyMessage }} · </span>The link is shown only now. Resend the invite for a new one.
      </p>
      <div class="actions">
        <button type="button" class="ghost" @click="created = null">Done</button>
      </div>
    </section>

    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p>
    <p v-else-if="!invites" class="muted">Loading…</p>
    <template v-else>
      <p v-if="rowError" class="error" role="alert">{{ rowError }}</p>
      <p v-if="invites.length === 0" class="empty muted" data-testid="no-invites">No pending invites.</p>
      <table v-else>
        <thead>
          <tr>
            <th scope="col">Email</th>
            <th scope="col">Role</th>
            <th scope="col">Sent by</th>
            <th scope="col">Sent</th>
            <th scope="col">Expires</th>
            <th scope="col"><span class="visually-hidden">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="invite in invites" :key="invite.id" :data-invite="invite.email">
            <td class="email">{{ invite.email }}</td>
            <td>{{ roleLabels[invite.role] }}</td>
            <td :class="{ muted: !invite.createdBy }">{{ describeSender(invite) }}</td>
            <td>{{ formatDate(invite.createdAt) }}</td>
            <td>{{ formatDate(invite.expiresAt) }}</td>
            <td class="row-actions">
              <button type="button" class="ghost" :disabled="busyInviteId !== '' || isCreating" @click="resend(invite)">Resend</button>
              <button type="button" class="ghost danger" :disabled="busyInviteId !== '' || isCreating" @click="revoke(invite)">Revoke</button>
            </td>
          </tr>
        </tbody>
      </table>
    </template>
  </section>
</template>

<style scoped>
h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 650;
}

.invites {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 960px;
}

.title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.description,
.muted {
  margin: 0;
  color: var(--color-text-muted);
}

.panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  max-width: 640px;
  padding: 20px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
}

.choices {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  border: none;
}

legend {
  margin-bottom: 6px;
  padding: 0;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 500;
}

.choice {
  display: flex;
  align-items: center;
  gap: 8px;
}

.choice input {
  width: auto;
}

.hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
}

.delivery {
  margin: 0;
}

.link {
  display: flex;
  gap: 8px;
}

.link button {
  flex-shrink: 0;
  white-space: nowrap;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.empty {
  padding: 24px 0;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th {
  padding: 8px 10px;
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 500;
  text-align: left;
}

td {
  padding: 8px 10px;
  border-bottom: 1px solid var(--color-border);
  vertical-align: middle;
}

td.email {
  overflow-wrap: anywhere;
}

.row-actions {
  white-space: nowrap;
  text-align: right;
}

button.danger {
  color: var(--color-danger-text);
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}
</style>
