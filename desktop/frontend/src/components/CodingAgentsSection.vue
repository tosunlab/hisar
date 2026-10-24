<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { api, type AgentGrant, type AgentSession, type RememberedAgentSecret, type VaultSummary } from "../api";
import {
  forgetAgentHost,
  loadAgentSessions,
  loadAgentSignIn,
  loadAgentVaults,
  revokeAgentGrant,
  revokeAgentSession,
  setAgentSignInEnabled,
  setVaultAvailableToAgents,
  store,
} from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { describeAgentSession, formatCount, formatExpiresIn, formatTimeAgo } from "../util";
import AppIcon, { type IconName } from "./AppIcon.vue";
import CodingAgentConnections from "./CodingAgentConnections.vue";

const isRowVisible = useSettingsRowVisibility();

function isAvailableToAgents(vault: VaultSummary): boolean {
  return store.allAgentVaultID.includes(vault.id);
}

function describeItemCount(vault: VaultSummary): string {
  return vault.itemCount === 1 ? "1 item" : `${vault.itemCount} items`;
}

// The checkbox follows the saved setting, also when saving failed and the setting did not change.
async function toggleVault(vault: VaultSummary, checkbox: HTMLInputElement): Promise<void> {
  await setVaultAvailableToAgents(vault, checkbox.checked);
  checkbox.checked = isAvailableToAgents(vault);
}

async function toggleAgentSignIn(checkbox: HTMLInputElement): Promise<void> {
  await setAgentSignInEnabled(checkbox.checked);
  checkbox.checked = store.agentSignIn.isEnabled;
}

function openAccessibilitySettings(): Promise<void> {
  return api.openURL("x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility");
}

function describeGrant(grant: AgentGrant): string {
  switch (grant.kind) {
    case "sign-in":
      return `Sign in to ${grant.website} as ${grant.name}`;
    case "reveal":
      return `Read ${grant.name}`;
    case "code":
      return `One-time code of ${grant.name}`;
  }
  return `Use ${grant.name}`;
}

function getGrantIcon(grant: AgentGrant): IconName {
  switch (grant.kind) {
    case "sign-in":
      return "globe";
    case "reveal":
      return "eye";
  }
  return "key";
}

function describeGrantHolder(grant: AgentGrant): string {
  switch (grant.scope) {
    case "session":
      return `${grant.harnessName}, this session`;
    case "connection":
      return `${grant.harnessName}, while connected`;
  }
  return grant.harnessName;
}

function describeGrantExpiry(grant: AgentGrant): string {
  return grant.expiresAt ? formatExpiresIn(grant.expiresAt) : "Until you revoke it";
}

function describeRememberedSecret(secret: RememberedAgentSecret): string {
  return secret.website ? `Sign in to ${secret.website} as ${secret.name}` : secret.name;
}

function describeSessionUse(session: AgentSession): string {
  const rememberedCount = session.allRememberedSecret.length;
  const remembered = rememberedCount === 0 ? "No remembered secrets" : formatCount(rememberedCount, "remembered secret");
  return `First used ${formatTimeAgo(session.firstRequestAt)} · ${remembered}`;
}

let allRemoveListener: (() => void)[] = [];

// The Accessibility permission changes in System Settings, so the row reads it again when Hisar's window comes back.
function refreshAgentSignIn(): Promise<void> {
  return loadAgentSignIn();
}

onMounted(() => {
  window.addEventListener("focus", refreshAgentSignIn);
  allRemoveListener = [Events.On("agent-sessions-changed", () => loadAgentSessions()), () => window.removeEventListener("focus", refreshAgentSignIn)];
  return Promise.all([loadAgentVaults(), loadAgentSessions(), loadAgentSignIn()]);
});

onBeforeUnmount(() => allRemoveListener.forEach((removeListener) => removeListener()));
</script>

<template>
  <section v-show="isRowVisible('coding-agents')" class="settings-block" data-testid="coding-agents-section">
    <p class="section-label">Coding agents</p>
    <CodingAgentConnections />
    <div class="settings-group">
      <div class="setting">
        <div class="setting-text">
          <strong>Vaults for coding agents</strong>
          <p class="muted">Agents pass references to commands after you approve access here. Commands receive real secrets; masking only reduces accidental disclosure.</p>
        </div>
      </div>
      <label v-for="vault in store.allVault" :key="vault.id" class="setting" :data-testid="`agent-vault-${vault.id}`">
        <span class="settings-row-icon"><AppIcon name="vault" :size="14" /></span>
        <div class="setting-text">
          <strong>{{ vault.name }}</strong>
          <p class="muted">
            {{ describeItemCount(vault) }} ·
            <span :class="{ 'available-text': isAvailableToAgents(vault) }">{{ isAvailableToAgents(vault) ? "Available to agents" : "Invisible to agents" }}</span>
          </p>
        </div>
        <input
          class="switch"
          type="checkbox"
          :name="`agentVault-${vault.id}`"
          :aria-label="`Available to agents: ${vault.name}`"
          :checked="isAvailableToAgents(vault)"
          :disabled="store.isBusy"
          @change="toggleVault(vault, $event.target as HTMLInputElement)"
        />
      </label>
      <p v-if="store.allVault.length === 0" class="muted empty-row">No vaults yet.</p>
    </div>
    <p class="muted settings-note" data-testid="coding-agents-explanation">
      Agents can find items in these vaults and use their secrets after you approve; other vaults stay invisible to them.
    </p>
    <p v-if="store.agentErrorMessage" class="error settings-note" role="alert" data-testid="coding-agents-error">{{ store.agentErrorMessage }}</p>
  </section>

  <section v-show="isRowVisible('agent-sign-in')" class="settings-block" data-testid="agent-sign-in-section">
    <p class="section-label">Website sign-in</p>
    <div class="settings-group">
      <label class="setting">
        <span class="settings-row-icon"><AppIcon name="globe" :size="14" /></span>
        <div class="setting-text">
          <strong>Let agents sign in to websites in your browser</strong>
          <p class="muted">
            An agent names its browser window. Hisar enters the login and submits it there. The sign_in tool returns no password or one-time code.
          </p>
        </div>
        <input
          class="switch"
          type="checkbox"
          name="agentSignIn"
          aria-label="Let agents sign in to websites in your browser"
          data-testid="agent-sign-in-switch"
          :checked="store.agentSignIn.isEnabled"
          :disabled="store.isBusy || !store.agentSignIn.isAvailable"
          @change="toggleAgentSignIn($event.target as HTMLInputElement)"
        />
      </label>
      <div v-if="store.agentSignIn.isEnabled && !store.agentSignIn.hasAccessibility" class="setting" data-testid="agent-sign-in-accessibility">
        <span class="settings-row-icon warning-icon"><AppIcon name="warning" :size="14" /></span>
        <div class="setting-text">
          <strong>Accessibility permission needed</strong>
          <p class="muted">Hisar needs the Accessibility permission to fill browser windows.</p>
        </div>
        <button data-testid="open-accessibility-settings" @click="openAccessibilitySettings">Open System Settings</button>
      </div>
      <div v-else-if="store.agentSignIn.isEnabled" class="setting" data-testid="agent-sign-in-ready">
        <span class="settings-row-icon"><AppIcon name="check" :size="14" /></span>
        <div class="setting-text">
          <strong>Accessibility allowed</strong>
          <p class="muted">Hisar can fill browser windows.</p>
        </div>
      </div>
    </div>
    <p class="muted settings-note" data-testid="agent-sign-in-note">
      {{
        store.agentSignIn.isAvailable
          ? "Sign-ins ask you unless you allowed that login on that website for longer."
          : "Only Hisar for Mac signs agents in to websites."
      }}
    </p>
  </section>

  <section v-show="isRowVisible('agent-grants')" class="settings-block" data-testid="agent-grants-section">
    <p class="section-label">Allowed without asking</p>
    <div class="settings-group">
      <div v-for="(grant, index) in store.allAgentGrant" :key="grant.id" class="setting" data-testid="agent-grant">
        <span class="settings-row-icon"><AppIcon :name="getGrantIcon(grant)" :size="14" /></span>
        <div class="setting-text">
          <strong class="title-text" :title="describeGrant(grant)" data-testid="agent-grant-name">{{ describeGrant(grant) }}</strong>
          <p class="muted" data-testid="agent-grant-expiry">{{ describeGrantHolder(grant) }} · {{ describeGrantExpiry(grant) }}</p>
        </div>
        <button :data-testid="`revoke-agent-grant-${index}`" :disabled="store.isBusy" @click="revokeAgentGrant(grant)">Revoke</button>
      </div>
      <p v-if="store.allAgentGrant.length === 0" class="muted empty-row" data-testid="agent-grants-empty">Every agent asks before it uses a secret.</p>
    </div>
    <p class="muted settings-note">
      Longer choices cover omp, Claude Code, Codex or Hermes across sessions only when Hisar can identify the program and save approvals.
      Otherwise, For 1 hour ends with the session or connection. Each approval ends when it expires, you revoke it here, or you sign out.
      Locking Hisar keeps them.
    </p>
  </section>

  <section v-show="isRowVisible('agent-sessions')" class="settings-block" data-testid="agent-sessions-section">
    <p class="section-label">Running agents</p>
    <div class="settings-group">
      <div v-for="session in store.allAgentSession" :key="`${session.pid}-${session.startTime}`" class="setting" data-testid="agent-session">
        <img v-if="session.hostIconURL" class="app-icon" :src="session.hostIconURL" alt="" />
        <span v-else class="settings-row-icon"><AppIcon name="terminal" :size="14" /></span>
        <div class="setting-text">
          <strong class="title-line">
            <span class="title-text" :title="session.projectPath" data-testid="agent-session-title">{{
              [describeAgentSession(session), session.projectName].filter(Boolean).join(", ")
            }}</span>
            <span
              v-if="session.isSigned"
              class="signed-mark"
              :title="`${session.harnessName} is signed by its developer`"
              :aria-label="`${session.harnessName} is signed by its developer`"
              data-testid="agent-session-signed"
            >
              <AppIcon name="check" :size="10" />
            </span>
          </strong>
          <p class="muted" data-testid="agent-session-use">{{ describeSessionUse(session) }}</p>
          <ul v-if="session.allRememberedSecret.length > 0" class="remembered-list" data-testid="agent-session-secrets">
            <li
              v-for="secret in session.allRememberedSecret"
              :key="`${secret.name}-${secret.isReveal}-${secret.website}`"
              class="muted"
              :title="describeRememberedSecret(secret)"
            >
              <AppIcon :name="secret.website ? 'globe' : secret.isReveal ? 'eye' : 'key'" :size="11" /> {{ describeRememberedSecret(secret) }}
            </li>
          </ul>
        </div>
        <button :data-testid="`revoke-agent-session-${session.pid}`" :disabled="store.isBusy" @click="revokeAgentSession(session)">Revoke</button>
      </div>
      <p v-if="store.allAgentSession.length === 0" class="muted empty-row" data-testid="agent-sessions-empty">No coding agent that used Hisar is running.</p>
    </div>
    <p class="muted settings-note">Revoking ends the session’s approval and what you allowed only for that session. What you allowed across sessions stays until it expires, you revoke it above, or you sign out.</p>
  </section>

  <section v-show="isRowVisible('agent-hosts')" class="settings-block" data-testid="agent-hosts-section">
    <p class="section-label">Agent hosts</p>
    <div class="settings-group">
      <div v-for="host in store.allAgentHost" :key="`${host.path}-${host.teamID}`" class="setting" data-testid="agent-host">
        <img v-if="host.iconURL" class="app-icon" :src="host.iconURL" alt="" />
        <span v-else class="settings-row-icon"><AppIcon name="laptop" :size="14" /></span>
        <div class="setting-text">
          <strong>{{ host.name }}</strong>
          <p class="muted" :title="host.path">{{ host.path }}</p>
        </div>
        <button :data-testid="`forget-agent-host-${host.name}`" :disabled="store.isBusy" @click="forgetAgentHost(host)">Forget</button>
      </div>
      <p v-if="store.allAgentHost.length === 0" class="muted empty-row" data-testid="agent-hosts-empty">No app has run a coding agent.</p>
    </div>
    <p class="muted settings-note" data-testid="agent-hosts-explanation">Apps that ran a coding agent ask for every command-line request without a terminal.</p>
  </section>
</template>

<style scoped>
.empty-row {
  margin: 0;
  padding: 10px 14px;
  font-size: 12px;
}

.available-text {
  color: var(--color-success);
}

.warning-icon {
  color: var(--color-warning);
}

.app-icon {
  width: 26px;
  height: 26px;
  flex: none;
  object-fit: contain;
}

.signed-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  flex: none;
  border-radius: 50%;
  background: var(--color-success);
  color: var(--color-surface);
}

.remembered-list {
  margin: 4px 0 0;
  padding: 0;
  list-style: none;
  font-size: 12px;
}

.remembered-list li {
  display: flex;
  align-items: center;
  gap: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
