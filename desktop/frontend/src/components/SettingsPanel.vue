<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import type { Device } from "../api";
import {
  configureSSHConfig,
  disableBiometricUnlock,
  enableBiometricUnlock,
  loadSettingsData,
  loadSSHAgentStatus,
  openImport,
  removeSSHConfig,
  setCompactSSHApproval,
  setPhoneApproval,
  revokeDevice,
  saveEmergencyKit,
  setAutoLock,
  signOut,
  store,
} from "../store";
import { formatLastSeen, formatPlatform, getPlatformIcon, isMacPlatform } from "../util";
import BrowserExtensionSection from "./BrowserExtensionSection.vue";
import CodingAgentsSection from "./CodingAgentsSection.vue";
import CommandLineSection from "./CommandLineSection.vue";
import OpenAtLoginSection from "./OpenAtLoginSection.vue";
import UpdatesSection from "./UpdatesSection.vue";
import AppIcon from "./AppIcon.vue";
import PeopleSection from "./PeopleSection.vue";

const allAutoLockOption = [
  { seconds: 30, label: "30 seconds" },
  { seconds: 60, label: "1 minute" },
  { seconds: 300, label: "5 minutes" },
  { seconds: 600, label: "10 minutes" },
  { seconds: 3600, label: "1 hour" },
];
const isEnablingTouchID = ref(false);
const touchIDPassword = ref("");
const isConfirmingSignOut = ref(false);
const revokingDeviceID = ref("");
const currentDeviceLabel = isMacPlatform() ? "This Mac" : "This device";

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
            <p class="muted">Hisar locks after this long without activity. Copied values are cleared after 90 seconds.</p>
          </div>
          <select
            class="auto-lock"
            aria-label="Auto-lock after"
            data-testid="auto-lock"
            :value="store.status.autoLockSeconds"
            :disabled="store.isBusy"
            @change="setAutoLock(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="option in allAutoLockOption" :key="option.seconds" :value="option.seconds">{{ option.label }}</option>
          </select>
        </div>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="note" /></span>
          <div class="setting-text">
            <strong>Emergency Kit</strong>
            <p class="muted">A printable PDF with your sign-in address, email and Secret Key, for signing in on a new device. It is the only way back in if you lose your devices.</p>
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

      <PeopleSection />

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
                ? "Each use of a key asks for Touch ID, unless you let the app skip asking for an hour. Commit signatures always ask."
                : "Each use of a key asks for your approval in Hisar, unless you let the app skip asking for an hour. Commit signatures always ask."
            }}
          </p>
        </div>
        <label v-if="store.sshAgentStatus" class="setting compact-approval">
          <span class="setting-icon"><AppIcon name="fingerprint" /></span>
          <div class="setting-text">
            <strong>Compact SSH approvals</strong>
            <p class="muted">Ask in a small card at the top right instead of a dialog; click the card for the details. Locked and forwarded requests still use the dialog.</p>
          </div>
          <input
            type="checkbox"
            name="compactSSHApproval"
            :checked="store.sshAgentStatus.isCompactApproval"
            :disabled="store.isBusy"
            @change="setCompactSSHApproval(($event.target as HTMLInputElement).checked)"
          />
        </label>
        <label v-if="store.sshAgentStatus" class="setting compact-approval">
          <span class="setting-icon"><AppIcon name="phone" /></span>
          <div class="setting-text">
            <strong>Allow approvals from my iPhone</strong>
            <p class="muted">While Hisar is unlocked here, SSH and command-line requests also appear in Hisar on your iPhone when it is open and unlocked. Approving there never unlocks this Mac.</p>
          </div>
          <input
            type="checkbox"
            name="phoneApproval"
            data-testid="phone-approval-switch"
            :checked="store.sshAgentStatus.isPhoneApproval"
            :disabled="store.isBusy"
            @change="setPhoneApproval(($event.target as HTMLInputElement).checked)"
          />
        </label>
      </section>

      <CommandLineSection />

      <CodingAgentsSection />

      <OpenAtLoginSection />

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
.compact-approval {
  margin-top: 8px;
  cursor: pointer;
}

.compact-approval input {
  width: auto;
}

.auto-lock {
  flex: none;
  width: auto;
}

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

.enable-form {
  display: flex;
  gap: 8px;
  padding: 0 0 12px 44px;
}

.setting-text .revoke-note {
  margin-top: 6px;
  color: #f2a7a9;
}
</style>
