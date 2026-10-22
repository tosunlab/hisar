<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { APIError, auth, getErrorMessage } from "../../api";
import type { ReauthenticationProof } from "../../bridge/bridge";
import { navigate } from "../../navigation";
import {
  accountsPath,
  changeAccountRole,
  deleteAccount,
  describeDevice,
  formatActivity,
  formatDate,
  getAccount,
  ownAccountMessage,
  proveAdmin,
  previewAccountDeletion,
  reactivateAccount,
  revokeAccountDevice,
  roleLabels,
  signOutAccount,
  stateLabels,
  suspendAccount,
  type AccountRole,
  type AdminAccountDetail,
  type AccountDeletionPreview,
} from "./accounts";

type Dialog = "suspend" | "sign-out" | "role" | "delete";

// Step is what a running action waits for: the master password check, then the server.
type Step = "" | "proving" | "saving";

const props = defineProps<{ accountId: string }>();

const detail = ref<AdminAccountDetail | null>(null);
const loadError = ref("");
const openDialog = ref<Dialog | "">("");
const step = ref<Step>("");
const dialogError = ref("");
const actionError = ref("");
const newRole = ref<AccountRole>("member");
const typedEmail = ref("");
const secretKey = ref("");
const password = ref("");
const isSecretKeyShown = ref(false);
const deletionPreview = ref<AccountDeletionPreview | null>(null);
const isLoadingPlan = ref(false);

const account = computed(() => detail.value?.account);
const isOwnAccount = computed(() => account.value?.id === auth.session?.accountId);
const isBusy = computed(() => step.value !== "");
const canDelete = computed(() => !!deletionPreview.value?.planVersion && !isLoadingPlan.value && typedEmail.value.trim().toLowerCase() === account.value?.email && secretKey.value !== "" && password.value !== "");

async function load(): Promise<void> {
  try {
    detail.value = await getAccount(props.accountId);
  } catch (error) {
    loadError.value = getErrorMessage(error);
  }
}

onMounted(load);

async function open(dialog: Dialog): Promise<void> {
  openDialog.value = dialog;
  dialogError.value = "";
  actionError.value = "";
  typedEmail.value = "";
  secretKey.value = "";
  password.value = "";
  isSecretKeyShown.value = false;
  deletionPreview.value = null;
  if (dialog === "delete") {
    await loadDeletionPlan();
  }
  if (dialog === "role" && account.value) {
    newRole.value = account.value.role === "admin" ? "member" : "admin";
  }
}

async function loadDeletionPlan(): Promise<void> {
  deletionPreview.value = null;
  isLoadingPlan.value = true;
  try {
    const plan = await previewAccountDeletion(props.accountId);
    if (openDialog.value === "delete") {
      deletionPreview.value = plan;
    }
  } catch (error) {
    dialogError.value = getErrorMessage(error);
  } finally {
    isLoadingPlan.value = false;
  }
}

function close(): void {
  openDialog.value = "";
  secretKey.value = "";
  password.value = "";
}

// run runs a dialog's change: with needsProof, it first proves the admin's master password. The dialog stays open with
// the server's answer when the change is refused.
async function run(change: (proof: ReauthenticationProof | null) => Promise<void>, needsProof: boolean): Promise<boolean> {
  dialogError.value = "";
  try {
    step.value = needsProof ? "proving" : "saving";
    const proof = needsProof ? await proveAdmin(secretKey.value, password.value) : null;
    step.value = "saving";
    await change(proof);
    close();
    return true;
  } catch (error) {
    dialogError.value = getErrorMessage(error);
    return false;
  } finally {
    step.value = "";
  }
}

async function submitSuspend(): Promise<void> {
  if (await run(() => suspendAccount(props.accountId), false)) {
    await load();
  }
}

async function submitSignOut(): Promise<void> {
  if (await run(() => signOutAccount(props.accountId), false)) {
    await load();
  }
}

async function submitRole(): Promise<void> {
  if (await run((proof) => changeAccountRole(props.accountId, newRole.value, proof!), true)) {
    await load();
  }
}

async function submitDelete(): Promise<void> {
  const plan = deletionPreview.value;
  if (!canDelete.value || !plan || isBusy.value) {
    return;
  }
  dialogError.value = "";
  try {
    step.value = "proving";
    const proof = await proveAdmin(secretKey.value, password.value, "admin-account-delete", props.accountId, plan.planVersion);
    step.value = "saving";
    await deleteAccount(props.accountId, typedEmail.value.trim(), proof, plan.planVersion);
    close();
    navigate(accountsPath);
  } catch (error) {
    secretKey.value = "";
    password.value = "";
    typedEmail.value = "";
    if (error instanceof APIError && error.isDeletionPlanChanged) {
      await loadDeletionPlan();
      dialogError.value = deletionPreview.value
        ? "The deletion plan changed. Review the latest consequences, type the email again, and enter your credentials for a fresh confirmation."
        : dialogError.value;
    } else {
      dialogError.value = getErrorMessage(error);
    }
  } finally {
    step.value = "";
  }
}

async function runAction(action: () => Promise<void>): Promise<void> {
  step.value = "saving";
  actionError.value = "";
  try {
    await action();
    await load();
  } catch (error) {
    actionError.value = getErrorMessage(error);
  } finally {
    step.value = "";
  }
}

function buttonLabel(idle: string, saving: string): string {
  if (step.value === "proving") {
    return "Checking master password…";
  }
  return step.value === "saving" ? saving : idle;
}
</script>

<template>
  <section class="account">
    <a :href="accountsPath" class="back" @click.prevent="navigate(accountsPath)">← Accounts</a>
    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p>
    <p v-else-if="!account" class="muted">Loading…</p>
    <template v-else>
      <header>
        <h1>{{ account.name }}</h1>
        <p class="email">{{ account.email }}</p>
      </header>
      <dl class="facts">
        <div>
          <dt>Role</dt>
          <dd data-testid="role">{{ roleLabels[account.role] }}</dd>
        </div>
        <div>
          <dt>State</dt>
          <dd data-testid="state">
            <span class="state" :class="account.state">{{ stateLabels[account.state] }}</span>
          </dd>
        </div>
        <div>
          <dt>Signed up</dt>
          <dd>{{ formatDate(account.createdAt) }}</dd>
        </div>
        <div>
          <dt>Last activity</dt>
          <dd>{{ formatActivity(account.lastSeenAt) }}</dd>
        </div>
        <div>
          <dt>Vaults</dt>
          <dd>{{ account.vaultCount }}</dd>
        </div>
      </dl>

      <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
      <div class="actions">
        <template v-if="!isOwnAccount">
          <button v-if="account.state === 'active'" type="button" :disabled="isBusy" @click="open('suspend')">Suspend…</button>
          <button v-else type="button" :disabled="isBusy" @click="runAction(() => reactivateAccount(accountId))">Reactivate</button>
          <button type="button" :disabled="isBusy" @click="open('role')">Change role…</button>
        </template>
        <button type="button" :disabled="isBusy" @click="open('sign-out')">Sign out everywhere…</button>
        <button v-if="!isOwnAccount" type="button" class="danger" :disabled="isBusy" @click="open('delete')">Delete account…</button>
      </div>
      <p v-if="isOwnAccount" class="hint">{{ ownAccountMessage }}</p>

      <form v-if="openDialog === 'suspend'" class="dialog" role="dialog" aria-label="Suspend account" @submit.prevent="submitSuspend">
        <h2>Suspend {{ account.name }}?</h2>
        <p class="description">
          {{ account.email }} cannot sign in, and their devices stay signed in but cannot reach the server until you reactivate the
          account. Their share links stop opening until then. Their vaults, items and organizations stay as they are. To remove the
          devices, use Sign out everywhere.
        </p>
        <p v-if="dialogError" class="error" role="alert">{{ dialogError }}</p>
        <div class="dialog-actions">
          <button type="button" class="ghost" :disabled="isBusy" @click="close">Cancel</button>
          <button type="submit" class="primary" :disabled="isBusy">{{ buttonLabel("Suspend", "Suspending…") }}</button>
        </div>
      </form>

      <form v-if="openDialog === 'sign-out'" class="dialog" role="dialog" aria-label="Sign out everywhere" @submit.prevent="submitSignOut">
        <h2>Sign out {{ account.name }} everywhere?</h2>
        <p class="description">Every device of {{ account.email }} is signed out. They can sign in again with their master password and Secret Key.</p>
        <p v-if="dialogError" class="error" role="alert">{{ dialogError }}</p>
        <div class="dialog-actions">
          <button type="button" class="ghost" :disabled="isBusy" @click="close">Cancel</button>
          <button type="submit" class="primary" :disabled="isBusy">{{ buttonLabel("Sign out everywhere", "Signing out…") }}</button>
        </div>
      </form>

      <form v-if="openDialog === 'role' || openDialog === 'delete'" class="dialog" role="dialog" :aria-label="openDialog === 'role' ? 'Change role' : 'Delete account'" @submit.prevent="openDialog === 'role' ? submitRole() : submitDelete()">
        <template v-if="openDialog === 'role'">
          <h2>Change the role of {{ account.name }}</h2>
          <fieldset class="choices" :disabled="isBusy">
            <legend>Role on the Hisar server</legend>
            <label class="choice">
              <input v-model="newRole" type="radio" name="role" value="admin" />
              <span>Admin: manages the Hisar server from this dashboard</span>
            </label>
            <label class="choice">
              <input v-model="newRole" type="radio" name="role" value="member" />
              <span>Member: uses the Hisar apps only</span>
            </label>
          </fieldset>
        </template>
        <template v-else>
          <h2>Delete {{ account.name }}?</h2>
          <p class="description">
            The account is deleted with its vaults that have no other owner and their items, its access to other vaults, its organization
            memberships, devices, share links and invitations. Nobody can undo this. Hisar emails {{ account.email }} that an admin deleted it.
          </p>
          <p v-if="isLoadingPlan" class="muted">Loading the current deletion plan…</p>
          <p v-if="deletionPreview && !deletionPreview.planVersion" class="error" role="alert">Update this server before deleting an account. It cannot confirm the reviewed deletion plan.</p>
          <ul v-if="deletionPreview" data-testid="deletion-consequences">
            <li v-for="vault in deletionPreview.ownedVaults" :key="vault.vaultId">
              Vault {{ vault.vaultId }} ({{ vault.type }}) and all its items are deleted.
              {{ vault.otherMemberCount }} other people lose access.
            </li>
            <li v-for="vault in deletionPreview.memberVaults" :key="vault.vaultId">
              Access to vault {{ vault.vaultId }} ({{ vault.type }}, {{ vault.role }}) is removed. The vault stays with its other owners.
            </li>
            <li v-for="organization in deletionPreview.organizations" :key="organization.organizationId">
              <template v-if="organization.deleted">Organization {{ organization.name }} is deleted with its teams, vaults, and items.</template>
              <template v-else-if="organization.newAdminId || organization.newAdminName">{{ organization.newAdminName || organization.newAdminId }} becomes an admin of {{ organization.name }}.</template>
              <template v-else>Membership in {{ organization.name }} is removed.</template>
            </li>
            <li>{{ deletionPreview.shareLinkCount }} share links stop working.</li>
            <li>{{ deletionPreview.sentInvitationCount }} sent invitations and {{ deletionPreview.receivedInvitationCount }} received invitations are removed.</li>
            <li>{{ deletionPreview.deviceCount }} devices are signed out.</li>
          </ul>
          <p class="hint">Vault names are encrypted. The server can identify them only by ID.</p>
          <label>
            <span>Type {{ account.email }} to confirm</span>
            <input v-model="typedEmail" name="confirmEmail" type="email" autocomplete="off" spellcheck="false" :disabled="isBusy" required />
          </label>
        </template>
        <p class="description">Enter your own Secret Key and master password to confirm it's you.</p>
        <label>
          <span>Your Secret Key</span>
          <span class="with-toggle">
            <input
              v-model="secretKey"
              name="secretKey"
              :type="isSecretKeyShown ? 'text' : 'password'"
              class="monospace"
              placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX"
              autocomplete="off"
              spellcheck="false"
              :disabled="isBusy"
              required
            />
            <button type="button" class="ghost" :aria-pressed="isSecretKeyShown" @click="isSecretKeyShown = !isSecretKeyShown">
              {{ isSecretKeyShown ? "Hide" : "Show" }}
            </button>
          </span>
        </label>
        <label>
          <span>Your master password</span>
          <input v-model="password" name="password" type="password" autocomplete="current-password" :disabled="isBusy" required />
        </label>
        <p v-if="dialogError" class="error" role="alert">{{ dialogError }}</p>
        <div class="dialog-actions">
          <button type="button" class="ghost" :disabled="isBusy" @click="close">Cancel</button>
          <button v-if="openDialog === 'role'" type="submit" class="primary" :disabled="isBusy || newRole === account.role">
            {{ buttonLabel("Change role", "Saving…") }}
          </button>
          <button v-else type="submit" class="primary danger" :disabled="isBusy || !canDelete">{{ buttonLabel("Delete account", "Deleting…") }}</button>
        </div>
      </form>

      <h2>Devices</h2>
      <ul v-if="detail && detail.devices.length > 0" class="devices">
        <li v-for="device in detail.devices" :key="device.id" :data-testid="`device-${device.id}`">
          <span class="device-text">
            <span class="device-name">{{ device.name }}</span>
            <span class="device-meta">
              {{ device.platform }} · Signed in {{ formatDate(device.createdAt) }} · {{ describeDevice(device, account.state) }}
            </span>
          </span>
          <button v-if="!device.revokedAt" type="button" :disabled="isBusy" @click="runAction(() => revokeAccountDevice(accountId, device.id))">Sign out</button>
        </li>
      </ul>
      <p v-else class="muted">No devices.</p>
    </template>
  </section>
</template>

<style scoped>
h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 650;
}

h2 {
  margin: 8px 0 0;
  font-size: 15px;
  font-weight: 650;
}

.account {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 720px;
}

.back {
  align-self: flex-start;
  color: var(--color-text-secondary);
  text-decoration: none;
}

.email {
  margin: 4px 0 0;
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.facts,
.devices {
  display: grid;
  gap: 1px;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-border);
  list-style: none;
}

.facts div {
  display: grid;
  grid-template-columns: 160px 1fr;
  gap: 16px;
  padding: 12px 16px;
  background: var(--color-surface);
}

dt {
  color: var(--color-text-muted);
}

dd {
  margin: 0;
}

.state {
  padding: 1px 8px;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.state.suspended {
  border-color: var(--color-warning-border);
  background: var(--color-warning-background);
  color: var(--color-warning-text);
}

.actions,
.dialog-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.dialog-actions {
  justify-content: flex-end;
}

button.danger {
  color: var(--color-danger-text);
}

button.primary.danger {
  border-color: var(--color-danger-solid);
  background: var(--color-danger-solid);
  color: var(--color-on-danger);
}

.dialog {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
}

.dialog h2 {
  margin: 0;
}

.description,
.muted,
.hint {
  margin: 0;
  color: var(--color-text-muted);
}

.hint {
  font-size: 12px;
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

.with-toggle {
  display: flex;
  gap: 8px;
}

.with-toggle input {
  flex: 1;
}

.devices li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  background: var(--color-surface);
}

.device-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.device-name {
  font-weight: 600;
}

.device-meta {
  color: var(--color-text-secondary);
  font-size: 12px;
}
</style>
