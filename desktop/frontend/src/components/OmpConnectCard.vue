<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api, getErrorMessage, type OmpState } from "../api";
import { store } from "../store";
import AppIcon from "./AppIcon.vue";

const ompState = ref<OmpState | null>(null);
const isChanging = ref(false);
const errorMessage = ref("");

async function applyChange(change: () => Promise<OmpState>): Promise<void> {
  isChanging.value = true;
  errorMessage.value = "";
  try {
    ompState.value = await change();
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isChanging.value = false;
  }
}

// The vault switches sit below the card in the same section.
function showVaultSwitches(): void {
  document.querySelector<HTMLInputElement>('input[name^="agentVault-"]')?.focus();
}

onMounted(() => applyChange(() => api.getOmpState()));
</script>

<template>
  <div class="settings-group omp-card" data-testid="omp-card">
    <div class="setting">
      <span class="settings-row-icon"><AppIcon name="terminal" :size="14" /></span>
      <div class="setting-text">
        <strong>omp</strong>
        <p v-if="ompState === null" class="muted">Looking for omp…</p>
        <p v-else-if="ompState === 'not-installed'" class="muted" data-testid="omp-state">
          <span class="state-dot"></span>omp is not installed. Install omp, or Tern, which comes with it.
        </p>
        <p v-else-if="ompState === 'available'" class="muted" data-testid="omp-state">
          Connect to let omp sessions run commands with your secrets through <span class="monospace">hisar run</span> and show Hisar's state in their
          status line.
        </p>
        <template v-else>
          <p class="muted" data-testid="omp-state"><span class="state-dot running"></span>Connected</p>
          <p class="muted" data-testid="omp-restart-hint">New omp sessions load Hisar. Restart the ones already running.</p>
        </template>
      </div>
      <button v-if="ompState === 'available'" class="primary" data-testid="connect-omp" :disabled="isChanging" @click="applyChange(api.connectOmp)">
        Connect
      </button>
      <button v-else-if="ompState === 'connected'" data-testid="disconnect-omp" :disabled="isChanging" @click="applyChange(api.disconnectOmp)">
        Disconnect
      </button>
    </div>
    <div v-if="(ompState === 'available' || ompState === 'connected') && store.allAgentVaultID.length === 0" class="settings-group-detail">
      <p class="warning-text" data-testid="omp-no-vaults">
        omp sees no secrets until a vault is available to agents.
        <button class="link" data-testid="show-agent-vaults" @click="showVaultSwitches">Choose vaults</button>
      </p>
    </div>
  </div>
  <p v-if="errorMessage" class="error settings-note" role="alert" data-testid="omp-error">{{ errorMessage }}</p>
</template>

<style scoped>
.omp-card {
  margin-bottom: 10px;
}
</style>
