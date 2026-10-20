<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import type { AgentSession, VaultSummary } from "../api";
import { forgetAgentHost, loadAgentSessions, loadAgentVaults, revokeAgentSession, setVaultAvailableToAgents, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { describeAgentSession, formatCount, formatTimeAgo } from "../util";
import AppIcon from "./AppIcon.vue";

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

function describeSessionUse(session: AgentSession): string {
  const rememberedCount = session.allRememberedSecret.length;
  const remembered = rememberedCount === 0 ? "No remembered secrets" : formatCount(rememberedCount, "remembered secret");
  return `First used ${formatTimeAgo(session.firstRequestAt)} · ${remembered}`;
}

let allRemoveListener: (() => void)[] = [];

onMounted(() => {
  allRemoveListener = [Events.On("agent-sessions-changed", () => loadAgentSessions())];
  return Promise.all([loadAgentVaults(), loadAgentSessions()]);
});

onBeforeUnmount(() => allRemoveListener.forEach((removeListener) => removeListener()));
</script>

<template>
  <section v-show="isRowVisible('coding-agents')" class="settings-block" data-testid="coding-agents-section">
    <p class="section-label">Coding agents</p>
    <div class="settings-group">
      <div class="setting">
        <div class="setting-text">
          <strong>Use Hisar from coding agents</strong>
          <p class="muted">
            Add <span class="monospace">hisar mcp</span> to Claude Code, Cursor or another MCP client. Agents run commands with your secrets without seeing
            them; you approve each use here.
          </p>
        </div>
      </div>
      <div class="settings-group-detail">
        <pre class="code-block monospace" data-testid="coding-agents-snippet">claude mcp add hisar -- hisar mcp</pre>
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
            <li v-for="secret in session.allRememberedSecret" :key="`${secret.name}-${secret.isReveal}`" class="muted" :title="secret.name">
              <AppIcon :name="secret.isReveal ? 'eye' : 'key'" :size="11" /> {{ secret.name }}
            </li>
          </ul>
        </div>
        <button :data-testid="`revoke-agent-session-${session.pid}`" :disabled="store.isBusy" @click="revokeAgentSession(session)">Revoke</button>
      </div>
      <p v-if="store.allAgentSession.length === 0" class="muted empty-row" data-testid="agent-sessions-empty">No coding agent that used Hisar is running.</p>
    </div>
    <p class="muted settings-note">Revoking ends the agent’s approval and remembered secrets; its next request asks again.</p>
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
