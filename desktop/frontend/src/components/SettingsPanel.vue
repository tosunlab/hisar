<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { Device, FamilyMember, FamilyRole } from "../api";
import {
  configureSSHConfig,
  copyText,
  disableBiometricUnlock,
  enableBiometricUnlock,
  getFirstName,
  inviteMember,
  loadRecoveryData,
  loadSettingsData,
  loadSSHAgentStatus,
  openImport,
  removeSSHConfig,
  revokeDevice,
  saveEmergencyKit,
  signOut,
  startAccountRecovery,
  store,
} from "../store";
import { formatLastSeen, formatPlatform, formatRole, getPlatformIcon, isMacPlatform } from "../util";
import AccountRecoverySection from "./AccountRecoverySection.vue";
import BrowserExtensionSection from "./BrowserExtensionSection.vue";
import UpdatesSection from "./UpdatesSection.vue";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";

const isEnablingTouchID = ref(false);
const touchIDPassword = ref("");
const isConfirmingSignOut = ref(false);
const isInviting = ref(false);
const inviteEmail = ref("");
const inviteRole = ref<FamilyRole>("member");
const invitedEmail = ref("");
const inviteLink = ref("");
const revokingDeviceID = ref("");
const recoveringAccountID = ref("");
const recoveryLink = ref("");

const isAdmin = computed(() => store.allFamilyMember.some((familyMember) => familyMember.isCurrentUser && familyMember.role === "admin"));
const currentDeviceLabel = isMacPlatform() ? "This Mac" : "This device";

function canStartRecovery(familyMember: FamilyMember): boolean {
  return isAdmin.value && !familyMember.isCurrentUser && store.recoveryStatus?.isSetUp === true;
}

function startRecoveryConfirmation(familyMember: FamilyMember): void {
  recoveringAccountID.value = familyMember.accountId;
  recoveryLink.value = "";
}

async function submitStartRecovery(familyMember: FamilyMember): Promise<void> {
  const link = await startAccountRecovery(familyMember);
  if (link) {
    recoveryLink.value = link;
  }
}

async function submitInvite(): Promise<void> {
  const link = await inviteMember(inviteEmail.value.trim(), inviteRole.value);
  if (link) {
    invitedEmail.value = inviteEmail.value.trim();
    inviteLink.value = link;
    inviteEmail.value = "";
    inviteRole.value = "member";
    isInviting.value = false;
  }
}

function startInviting(): void {
  inviteLink.value = "";
  isInviting.value = true;
}

function describeActivity(device: Device): string {
  const lastSeen = formatLastSeen(device.lastSeenAt);
  if (device.isCurrent || lastSeen === "Active now") {
    return "Active now";
  }
  return lastSeen ? `Last seen ${lastSeen === "Yesterday" ? "yesterday" : lastSeen}` : "";
}

async function confirmRevoke(device: Device): Promise<void> {
  if (await revokeDevice(device)) {
    revokingDeviceID.value = "";
  }
}

const isSavingEmergencyKit = ref(false);
const emergencyKitPassword = ref("");

async function submitSaveEmergencyKit(): Promise<void> {
  if (await saveEmergencyKit(emergencyKitPassword.value)) {
    isSavingEmergencyKit.value = false;
  }
  emergencyKitPassword.value = "";
}

function closeSettings(): void {
  store.isSettingsOpen = false;
  store.errorMessage = "";
}

async function submitEnableTouchID(): Promise<void> {
  if (await enableBiometricUnlock(touchIDPassword.value)) {
    isEnablingTouchID.value = false;
  }
  touchIDPassword.value = "";
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeSettings();
  }
}

onMounted(() => {
  window.addEventListener("keydown", closeOnEscape);
  loadSSHAgentStatus();
  loadSettingsData();
  loadRecoveryData();
});
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeSettings">
    <div class="card settings" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <header class="row">
        <h2 id="settings-title">Settings</h2>
        <div class="spacer"></div>
        <button class="ghost icon-button" aria-label="Close settings" @click="closeSettings"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <section>
        <p class="section-label">Security</p>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="fingerprint" /></span>
          <div class="setting-text">
            <strong>Unlock with Touch ID</strong>
            <p class="muted">
              {{
                store.status.biometricAvailable
                  ? "Touch ID unlocks Hisar after it locks. Your master password is still needed when Hisar starts and at least every 14 days."
                  : "Touch ID is not available on this Mac."
              }}
            </p>
          </div>
          <button
            v-if="store.status.biometricEnabled"
            data-testid="disable-touch-id"
            :disabled="store.isBusy"
            @click="disableBiometricUnlock"
          >
            Turn off
          </button>
          <button
            v-else-if="!isEnablingTouchID"
            data-testid="enable-touch-id"
            :disabled="!store.status.biometricAvailable"
            @click="isEnablingTouchID = true"
          >
            Turn on
          </button>
        </div>
        <form v-if="isEnablingTouchID" class="enable-form" @submit.prevent="submitEnableTouchID">
          <input
            v-model="touchIDPassword"
            name="touchIDPassword"
            type="password"
            aria-label="Master password"
            placeholder="Enter your master password to turn on Touch ID"
            autocomplete="current-password"
            autofocus
            required
          />
          <button type="button" class="ghost" @click="isEnablingTouchID = false">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Turn on</button>
        </form>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="lock" /></span>
          <div class="setting-text">
            <strong>Auto-lock</strong>
            <p class="muted">Hisar locks after 10 minutes without activity. Copied values are cleared after 90 seconds.</p>
          </div>
        </div>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="note" /></span>
          <div class="setting-text">
            <strong>Emergency Kit</strong>
            <p class="muted">A printable PDF with your sign-in address, email and Secret Key, for signing in on a new device.</p>
          </div>
          <button v-if="!isSavingEmergencyKit" data-testid="save-emergency-kit" @click="isSavingEmergencyKit = true">Save PDF</button>
        </div>
        <form v-if="isSavingEmergencyKit" class="enable-form" @submit.prevent="submitSaveEmergencyKit">
          <input
            v-model="emergencyKitPassword"
            name="emergencyKitPassword"
            type="password"
            aria-label="Master password"
            placeholder="Enter your master password to save the Emergency Kit"
            autocomplete="current-password"
            autofocus
            required
          />
          <button type="button" class="ghost" @click="isSavingEmergencyKit = false">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Save</button>
        </form>
      </section>

      <section data-testid="import-section">
        <p class="section-label">Import</p>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="import" /></span>
          <div class="setting-text">
            <strong>Import from another password manager</strong>
            <p class="muted">1Password (.1pux or CSV), Bitwarden (JSON or CSV), or a CSV export from Chrome, Firefox, Safari or Edge.</p>
          </div>
          <button data-testid="open-import" @click="openImport">Import…</button>
        </div>
      </section>

      <section data-testid="family-section">
        <p class="section-label">Family</p>
        <p v-if="store.allFamilyMember.length === 0" class="muted empty-note">{{ store.isBusy ? "Loading family members…" : "No family members to show." }}</p>
        <template v-for="familyMember in store.allFamilyMember" :key="familyMember.accountId">
          <div class="setting" :data-testid="`family-member-${familyMember.accountId}`">
            <MemberAvatar :name="familyMember.name || familyMember.email" />
            <div class="setting-text">
              <strong class="title-line">
                <span class="title-text">{{ familyMember.name || familyMember.email }}</span>
                <span v-if="familyMember.isCurrentUser" class="tag accent">you</span>
                <span class="tag" :data-testid="`family-role-${familyMember.accountId}`">{{ formatRole(familyMember.role) }}</span>
              </strong>
              <p class="muted">{{ familyMember.email }}</p>
              <p v-if="familyMember.isKeyChanged" class="warning-text">Their key changed since you last shared with them. Compare security codes in person.</p>
            </div>
            <div class="security-code" :title="familyMember.isCurrentUser ? 'Your security code' : 'Compare this code in person'">
              <span class="code-label">Security code</span>
              <span class="monospace" data-testid="family-security-code">{{ familyMember.securityCode }}</span>
            </div>
            <button
              v-if="canStartRecovery(familyMember)"
              class="ghost recovery-button"
              :title="`Start account recovery for ${getFirstName(familyMember)}`"
              :data-testid="`start-recovery-${familyMember.accountId}`"
              @click="startRecoveryConfirmation(familyMember)"
            >
              Start recovery
            </button>
          </div>
          <div v-if="recoveringAccountID === familyMember.accountId && !recoveryLink" class="inline-confirmation" data-testid="start-recovery-confirmation">
            <p>
              Create a recovery link for {{ getFirstName(familyMember) }}? They use it to choose a new master password and get a new Secret Key. Their
              account keeps working until you complete the recovery.
            </p>
            <div class="row">
              <button class="ghost" :disabled="store.isBusy" @click="recoveringAccountID = ''">Cancel</button>
              <button class="primary" data-testid="confirm-start-recovery" :disabled="store.isBusy" @click="submitStartRecovery(familyMember)">Create link</button>
            </div>
          </div>
          <div v-if="recoveringAccountID === familyMember.accountId && recoveryLink" class="invite-result" data-testid="recovery-link-result">
            <div class="invite-link">
              <span class="monospace" data-testid="recovery-link">{{ recoveryLink }}</span>
              <button data-testid="copy-recovery-link" @click="copyText(recoveryLink, 'Recovery link')"><AppIcon name="copy" :size="14" /> Copy</button>
            </div>
            <p class="muted">
              Send this link to {{ getFirstName(familyMember) }}. It is valid for 7 days and works once. Their recovery appears under Account recovery
              once they use it.
            </p>
          </div>
        </template>
        <template v-if="isAdmin">
          <div class="setting">
            <span class="setting-icon"><AppIcon name="mail" /></span>
            <div class="setting-text">
              <strong>Invite someone</strong>
              <p class="muted">Create a link they use to set up their account on this server.</p>
            </div>
            <button v-if="!isInviting" data-testid="start-invite" @click="startInviting">Invite</button>
          </div>
          <form v-if="isInviting" class="inline-form" data-testid="invite-form" @submit.prevent="submitInvite">
            <input v-model="inviteEmail" name="inviteEmail" type="email" aria-label="Email" placeholder="Email address" autocomplete="off" autofocus required />
            <select v-model="inviteRole" class="invite-role" aria-label="Family role" data-testid="invite-role">
              <option value="member">Member</option>
              <option value="admin">Admin</option>
            </select>
            <button type="button" class="ghost" @click="isInviting = false">Cancel</button>
            <button class="primary" type="submit" :disabled="store.isBusy || !inviteEmail.trim()">Create link</button>
          </form>
          <div v-if="inviteLink" class="invite-result" data-testid="invite-result">
            <div class="invite-link">
              <span class="monospace" data-testid="invite-link">{{ inviteLink }}</span>
              <button data-testid="copy-invite-link" @click="copyText(inviteLink, 'Invite link')"><AppIcon name="copy" :size="14" /> Copy</button>
            </div>
            <p class="muted">Send this link to {{ invitedEmail }}. It is valid for 7 days and works once.</p>
          </div>
        </template>
      </section>

      <AccountRecoverySection />

      <section data-testid="devices-section">
        <p class="section-label">Devices</p>
        <p v-if="store.allDevice.length === 0" class="muted empty-note">{{ store.isBusy ? "Loading devices…" : "No devices to show." }}</p>
        <div v-for="device in store.allDevice" :key="device.id" class="setting" :data-testid="`device-${device.id}`">
          <span class="setting-icon"><AppIcon :name="getPlatformIcon(device.platform)" /></span>
          <div class="setting-text">
            <strong class="title-line">
              <span class="title-text">{{ device.name }}</span>
              <span v-if="device.isCurrent" class="tag accent">{{ currentDeviceLabel }}</span>
            </strong>
            <p class="muted">{{ formatPlatform(device.platform) }} · {{ describeActivity(device) }}</p>
            <p v-if="revokingDeviceID === device.id" class="revoke-note">
              {{ device.name }} will be signed out and needs the master password and Secret Key to sign in again.
            </p>
          </div>
          <template v-if="!device.isCurrent">
            <button v-if="revokingDeviceID !== device.id" class="danger" :data-testid="`revoke-device-${device.id}`" @click="revokingDeviceID = device.id">Revoke</button>
            <template v-else>
              <button class="ghost" @click="revokingDeviceID = ''">Cancel</button>
              <button class="danger-solid" data-testid="confirm-revoke-device" :disabled="store.isBusy" @click="confirmRevoke(device)">Sign out device</button>
            </template>
          </template>
        </div>
      </section>

      <BrowserExtensionSection />

      <section data-testid="ssh-agent-section">
        <p class="section-label">SSH agent</p>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="terminal" /></span>
          <div class="setting-text">
            <strong>Hisar SSH agent</strong>
            <p v-if="!store.sshAgentStatus" class="muted">Checking the SSH agent…</p>
            <p v-else-if="store.sshAgentStatus.isRunning" class="muted" data-testid="ssh-agent-state">
              <span class="state-dot running"></span>Running at <span class="monospace">{{ store.sshAgentStatus.socketPath }}</span>
            </p>
            <p v-else class="muted" data-testid="ssh-agent-state">
              <span class="state-dot stopped"></span>Not running<template v-if="store.sshAgentStatus.errorMessage">: {{ store.sshAgentStatus.errorMessage }}</template>
            </p>
          </div>
          <template v-if="store.sshAgentStatus">
            <button v-if="store.sshAgentStatus.isSSHConfigured" data-testid="remove-ssh-config" :disabled="store.isBusy" @click="removeSSHConfig">
              Remove from SSH config
            </button>
            <button v-else class="primary" data-testid="configure-ssh-config" :disabled="store.isBusy" @click="configureSSHConfig">Use Hisar for SSH</button>
          </template>
        </div>
        <p v-if="store.sshAgentErrorMessage" class="error" role="alert">{{ store.sshAgentErrorMessage }}</p>
        <div v-if="store.sshAgentStatus" class="ssh-explanation">
          <p class="muted">
            <template v-if="store.sshAgentStatus.isSSHConfigured">
              <span class="monospace">ssh</span> and <span class="monospace">git</span> use the SSH keys in your vaults while Hisar is unlocked.
            </template>
            <template v-else>
              “Use Hisar for SSH” adds this section to the top of <span class="monospace">{{ store.sshAgentStatus.configPath }}</span>, after saving a backup, so
              <span class="monospace">ssh</span> and <span class="monospace">git</span> use the SSH keys in your vaults:
            </template>
          </p>
          <pre v-if="!store.sshAgentStatus.isSSHConfigured" class="config-snippet monospace" data-testid="ssh-config-snippet">{{ store.sshAgentStatus.configSection }}</pre>
          <p class="muted">
            {{
              store.sshAgentStatus.isTouchIDApproval
                ? "Each new program that uses a key asks for Touch ID. Approvals last for an hour, or until Hisar locks."
                : "Each new program that uses a key asks for your approval in Hisar. Approvals last for an hour, or until Hisar locks."
            }}
          </p>
        </div>
      </section>

      <UpdatesSection />

      <section>
        <p class="section-label">Account</p>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="user" /></span>
          <div class="setting-text">
            <strong>{{ store.status.name || store.status.email }}</strong>
            <p class="muted">{{ store.status.email }} · {{ store.status.server }}</p>
          </div>
          <button v-if="!isConfirmingSignOut" class="danger" @click="isConfirmingSignOut = true">Sign out</button>
          <template v-else>
            <button class="ghost" @click="isConfirmingSignOut = false">Cancel</button>
            <button class="danger-solid" data-testid="settings-confirm-sign-out" @click="signOut">Sign out</button>
          </template>
        </div>
      </section>

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

.settings {
  width: min(560px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px rgb(0 0 0 / 50%);
}

h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
}

header {
  margin-bottom: 16px;
}

section {
  margin-bottom: 18px;
}

.ssh-explanation {
  padding: 0 0 4px 44px;
}

.ssh-explanation p {
  margin: 0 0 8px;
  font-size: 12px;
}

.config-snippet {
  margin: 0 0 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
  color: var(--color-text-secondary);
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

.state-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  margin-right: 6px;
  border-radius: 50%;
  vertical-align: 1px;
}

.state-dot.running {
  background: var(--color-success);
}

.state-dot.stopped {
  background: var(--color-danger);
}

.enable-form,
.inline-form {
  display: flex;
  gap: 8px;
  padding: 0 0 12px 44px;
}

.invite-role {
  flex: none;
  width: 110px;
}

.inline-form button {
  flex: none;
  white-space: nowrap;
}

.security-code {
  display: flex;
  flex: none;
  flex-direction: column;
  align-items: flex-end;
  font-size: 12px;
  color: var(--color-text-secondary);
  user-select: text;
  -webkit-user-select: text;
}

.code-label {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.invite-result {
  padding: 0 0 12px 44px;
}

.invite-link {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 6px 6px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.invite-link .monospace {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
  -webkit-user-select: text;
}

.invite-result p {
  margin: 6px 0 0;
  font-size: 12px;
}

.recovery-button {
  flex: none;
  padding: 5px 8px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.inline-confirmation {
  margin: 0 0 12px 44px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
}

.inline-confirmation p {
  margin: 0 0 8px;
}

.inline-confirmation .row {
  justify-content: flex-end;
}

.setting-text .revoke-note {
  margin-top: 6px;
  color: #f2a7a9;
}
</style>
