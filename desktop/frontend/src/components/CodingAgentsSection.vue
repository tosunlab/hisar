<script setup lang="ts">
import { onMounted } from "vue";
import type { VaultSummary } from "../api";
import { loadAgentVaults, setVaultAvailableToAgents, store } from "../store";
import AppIcon from "./AppIcon.vue";

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

onMounted(loadAgentVaults);
</script>

<template>
  <section data-testid="coding-agents-section">
    <p class="section-label">Coding agents</p>
    <div class="setting">
      <span class="setting-icon"><AppIcon name="terminal" /></span>
      <div class="setting-text">
        <strong>Use Hisar from coding agents</strong>
        <p class="muted">
          Add <span class="monospace">hisar mcp</span> to Claude Code, Cursor or another MCP client. Agents run commands with your secrets without seeing
          them; you approve each use here.
        </p>
      </div>
    </div>
    <pre class="agent-snippet monospace" data-testid="coding-agents-snippet">claude mcp add hisar -- hisar mcp</pre>
    <p class="agent-explanation muted" data-testid="coding-agents-explanation">
      Agents can find items in these vaults and use their secrets after you approve; other vaults stay invisible to them.
    </p>
    <label v-for="vault in store.allVault" :key="vault.id" class="setting agent-vault" :data-testid="`agent-vault-${vault.id}`">
      <span class="setting-icon"><AppIcon name="vault" /></span>
      <div class="setting-text">
        <strong>{{ vault.name }}</strong>
        <p class="muted">
          {{ describeItemCount(vault) }} ·
          <span :class="{ 'available-text': isAvailableToAgents(vault) }">{{ isAvailableToAgents(vault) ? "Available to agents" : "Invisible to agents" }}</span>
        </p>
      </div>
      <input
        type="checkbox"
        :name="`agentVault-${vault.id}`"
        :aria-label="`Available to agents: ${vault.name}`"
        :checked="isAvailableToAgents(vault)"
        :disabled="store.isBusy"
        @change="toggleVault(vault, $event.target as HTMLInputElement)"
      />
    </label>
    <p v-if="store.allVault.length === 0" class="muted agent-explanation">No vaults yet.</p>
    <p v-if="store.agentErrorMessage" class="error" role="alert" data-testid="coding-agents-error">{{ store.agentErrorMessage }}</p>
  </section>
</template>

<style scoped>
.agent-snippet {
  margin: 0 0 8px 44px;
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
  color: var(--color-text-secondary);
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

.agent-explanation {
  margin: 0 0 6px 44px;
  font-size: 12px;
}

.agent-vault {
  cursor: pointer;
}

.agent-vault input {
  width: auto;
}

.available-text {
  color: var(--color-success);
}
</style>
