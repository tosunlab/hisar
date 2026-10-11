<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api, getErrorMessage, type LoginItemStatus } from "../api";
import AppIcon from "./AppIcon.vue";

const loginItemStatus = ref<LoginItemStatus | null>(null);
const isChanging = ref(false);
const errorMessage = ref("");

const stateDescription = computed(() => {
  const status = loginItemStatus.value;
  if (!status) {
    return { tone: "", text: "Checking your login items…" };
  }
  if (!status.isAvailable) {
    return { tone: "", text: "Only the installed Hisar app can open at login; this build cannot." };
  }
  if (!status.isOpenAtLogin) {
    return { tone: "stopped", text: "Off. Open Hisar yourself after you log in, or ssh, git and hisar fail until you do." };
  }
  if (status.systemState === "requires-approval") {
    return { tone: "warning", text: "Waiting for your approval: turn Hisar on in System Settings → General → Login Items." };
  }
  if (status.systemState === "not-registered") {
    return { tone: "warning", text: "Hisar is no longer in your login items." };
  }
  return { tone: "running", text: "Hisar opens locked at login, without a window, and stays in the menu bar." };
});

async function applyChange(change: () => Promise<LoginItemStatus>): Promise<void> {
  isChanging.value = true;
  errorMessage.value = "";
  try {
    loginItemStatus.value = await change();
    errorMessage.value = loginItemStatus.value.errorMessage;
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isChanging.value = false;
  }
}

async function openLoginItemSettings(): Promise<void> {
  errorMessage.value = "";
  try {
    await api.openLoginItemSettings();
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  }
}

onMounted(() => applyChange(() => api.getLoginItemStatus()));
</script>

<template>
  <section data-testid="open-at-login-section">
    <p class="section-label">App</p>
    <label class="setting open-at-login">
      <span class="setting-icon"><AppIcon name="laptop" /></span>
      <div class="setting-text">
        <strong>Open Hisar at login</strong>
        <p class="muted">Keeps the SSH agent and the command-line tool available. ssh, git and commit signing cannot start Hisar themselves.</p>
        <p class="muted" data-testid="login-item-state">
          <span v-if="stateDescription.tone" class="state-dot" :class="stateDescription.tone"></span>{{ stateDescription.text }}
        </p>
      </div>
      <input
        type="checkbox"
        name="openAtLogin"
        :checked="loginItemStatus?.isOpenAtLogin ?? false"
        :disabled="!loginItemStatus?.isAvailable || isChanging"
        @change="applyChange(() => api.setOpenAtLogin(($event.target as HTMLInputElement).checked))"
      />
    </label>
    <div v-if="loginItemStatus?.isOpenAtLogin && loginItemStatus.systemState !== 'enabled'" class="login-item-actions">
      <button
        v-if="loginItemStatus.systemState === 'requires-approval' && loginItemStatus.canOpenSystemSettings"
        data-testid="open-login-items"
        @click="openLoginItemSettings"
      >
        Open Login Items
      </button>
      <button v-if="loginItemStatus.systemState === 'not-registered'" data-testid="add-login-item" :disabled="isChanging" @click="applyChange(() => api.setOpenAtLogin(true))">
        Add again
      </button>
    </div>
    <p v-if="errorMessage" class="error" role="alert" data-testid="login-item-error">{{ errorMessage }}</p>
  </section>
</template>

<style scoped>
.open-at-login {
  cursor: pointer;
}

.open-at-login input {
  width: auto;
}

.login-item-actions {
  padding: 0 0 8px 44px;
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

.state-dot.warning {
  background: var(--color-warning);
}

.state-dot.stopped {
  background: var(--color-text-muted);
}
</style>
