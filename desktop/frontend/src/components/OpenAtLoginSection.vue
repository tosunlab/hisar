<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { api, getErrorMessage, type LoginItemStatus } from "../api";
import { useSettingsRowVisibility } from "../settingsSections";
import { getTrayName } from "../util";

const isRowVisible = useSettingsRowVisibility();

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
    return { tone: "stopped", text: "Off." };
  }
  if (status.systemState === "requires-approval") {
    return { tone: "warning", text: "Waiting for your approval: turn Hisar on in System Settings → General → Login Items." };
  }
  if (status.systemState === "not-registered") {
    return { tone: "warning", text: "Hisar is no longer in your login items." };
  }
  return { tone: "running", text: `Hisar opens locked at login, without a window, and stays in the ${getTrayName()}.` };
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

function refreshStatus(): Promise<void> {
  return applyChange(() => api.getLoginItemStatus());
}

onMounted(() => {
  window.addEventListener("focus", refreshStatus);
  return refreshStatus();
});
onBeforeUnmount(() => window.removeEventListener("focus", refreshStatus));
</script>

<template>
  <section v-show="isRowVisible('open-at-login')" class="settings-block" data-testid="open-at-login-section">
    <p class="section-label">Startup</p>
    <div class="settings-group">
      <label class="setting">
        <div class="setting-text">
          <strong>Open Hisar at login</strong>
          <p class="muted">Keeps the SSH agent and the command-line tool available. ssh, git and commit signing cannot start Hisar themselves.</p>
          <p class="muted" data-testid="login-item-state">
            <span v-if="stateDescription.tone" class="state-dot" :class="stateDescription.tone"></span>{{ stateDescription.text }}
          </p>
        </div>
        <input
          class="switch"
          type="checkbox"
          name="openAtLogin"
          :checked="loginItemStatus?.isOpenAtLogin ?? false"
          :disabled="!loginItemStatus?.isAvailable || isChanging"
          @change="applyChange(() => api.setOpenAtLogin(($event.target as HTMLInputElement).checked))"
        />
      </label>
      <div v-if="loginItemStatus?.isOpenAtLogin && loginItemStatus.systemState !== 'enabled'" class="settings-group-detail">
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
    </div>
    <p v-if="errorMessage" class="error settings-note" role="alert" data-testid="login-item-error">{{ errorMessage }}</p>
  </section>
</template>
