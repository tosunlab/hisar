<script setup lang="ts">
import { onMounted } from "vue";
import type { VaultSummary } from "../api";
import { loadAgentVaults, setVaultAvailableToAgents, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
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

onMounted(loadAgentVaults);
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
      <p v-if="store.allVault.length === 0" class="muted empty-vaults">No vaults yet.</p>
    </div>
    <p class="muted settings-note" data-testid="coding-agents-explanation">
      Agents can find items in these vaults and use their secrets after you approve; other vaults stay invisible to them.
    </p>
    <p v-if="store.agentErrorMessage" class="error settings-note" role="alert" data-testid="coding-agents-error">{{ store.agentErrorMessage }}</p>
  </section>
</template>

<style scoped>
.empty-vaults {
  margin: 0;
  padding: 10px 14px;
  font-size: 12px;
}

.available-text {
  color: var(--color-success);
}
</style>
