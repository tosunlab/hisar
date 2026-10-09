<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { closeVaultRecovery, getSelectedVault, setVaultRecovery, store } from "../store";
import AppIcon from "./AppIcon.vue";

const isConfirmingDisable = ref(false);

const vault = computed(() => getSelectedVault());
const isOwner = computed(() => vault.value?.role === "owner");
const creatorName = computed(() => store.recoveryStatus?.createdByName || "your admin");

const recoveryState = computed(() => {
  if (!store.recoveryStatus) {
    return { title: "Checking account recovery…", tone: "", label: "", text: "" };
  }
  if (vault.value?.recoverable && store.recoveryStatus.isCreatedByCurrentUser) {
    return {
      title: "Recovery is on",
      tone: "success",
      label: "On",
      text: "You set up recovery, so nobody can recover your own account. Keep your master password and Secret Key safe.",
    };
  }
  if (vault.value?.recoverable) {
    return {
      title: "Recoverable",
      tone: "success",
      label: "On",
      text: `If you forget your master password or lose your Secret Key, ${creatorName.value} can recover this vault for you. While doing so, their device can open it.`,
    };
  }
  if (vault.value?.recoveryOptOut) {
    return {
      title: "Recovery is off",
      tone: "warning",
      label: "Off",
      text: "Nobody can recover this vault. If you forget your master password or lose your Secret Key, its items are lost for good.",
    };
  }
  if (!store.recoveryStatus.isSetUp) {
    return {
      title: "Recovery is not set up",
      tone: "",
      label: "Off",
      text: "Your admin has not set up account recovery yet. If you lose access, this vault is lost.",
    };
  }
  return {
    title: "Not recoverable yet",
    tone: "",
    label: "Off",
    text: "This vault was created before recovery was set up. Turn recovery on so your admin can help you back in.",
  };
});
const canEnable = computed(() => isOwner.value && store.recoveryStatus?.isSetUp === true && vault.value?.recoverable === false);
const canDisable = computed(() => isOwner.value && vault.value?.recoverable === true);

async function confirmDisable(): Promise<void> {
  if (vault.value && (await setVaultRecovery(vault.value.id, false))) {
    isConfirmingDisable.value = false;
  }
}

async function submitEnable(): Promise<void> {
  if (vault.value) {
    await setVaultRecovery(vault.value.id, true);
  }
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeVaultRecovery();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeVaultRecovery">
    <div class="card panel" role="dialog" aria-modal="true" aria-labelledby="vault-recovery-title" data-testid="vault-recovery-panel">
      <header class="panel-header">
        <span class="panel-icon"><AppIcon name="lock" :size="18" /></span>
        <div class="panel-heading">
          <h2 id="vault-recovery-title">“{{ vault?.name }}” is private</h2>
          <p class="muted">Only you can open the items in this vault. Create a shared vault to share with your family.</p>
        </div>
        <button class="ghost icon-button" aria-label="Close" @click="closeVaultRecovery"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <p class="section-label">Account recovery</p>
      <div class="state-box" :class="recoveryState.tone" data-testid="vault-recovery-state">
        <AppIcon :name="recoveryState.tone === 'warning' ? 'warning' : 'shield'" class="state-icon" />
        <div class="state-text">
          <strong class="title-line">
            <span>{{ recoveryState.title }}</span>
            <span v-if="recoveryState.label" class="tag" :class="recoveryState.tone">{{ recoveryState.label }}</span>
          </strong>
          <p class="muted">{{ recoveryState.text }}</p>
        </div>
        <button v-if="canDisable && !isConfirmingDisable" class="danger" data-testid="disable-vault-recovery" @click="isConfirmingDisable = true">Turn off</button>
        <button v-if="canEnable" class="primary" data-testid="enable-vault-recovery" :disabled="store.isBusy" @click="submitEnable">Turn on</button>
      </div>

      <div v-if="isConfirmingDisable" class="confirm" data-testid="disable-vault-recovery-confirmation">
        <AppIcon name="warning" class="confirm-icon" />
        <div class="confirm-body">
          <p>
            <strong>If you turn off recovery and later forget your master password or lose your Secret Key, “{{ vault?.name }}” and everything in it
              is lost.</strong>
            Nobody, including your admin, can get it back.
          </p>
          <div class="row confirm-actions">
            <button class="ghost" :disabled="store.isBusy" @click="isConfirmingDisable = false">Keep recovery on</button>
            <button class="danger-solid" data-testid="confirm-disable-vault-recovery" :disabled="store.isBusy" @click="confirmDisable">Turn off recovery</button>
          </div>
        </div>
      </div>
      <p v-if="!isOwner" class="muted hint">Only the owner of this vault can change its recovery.</p>
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

.panel {
  width: min(520px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px rgb(0 0 0 / 50%);
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.panel-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.panel-heading {
  flex: 1;
  min-width: 0;
}

h2 {
  margin: 0;
  overflow: hidden;
  font-size: 17px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.panel-heading p {
  margin: 1px 0 0;
  font-size: 12px;
}

.state-box {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.state-box.warning {
  border-color: #5a4316;
}

.state-icon {
  flex: none;
  align-self: flex-start;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.state-box.success .state-icon {
  color: var(--color-success);
}

.state-box.warning .state-icon {
  color: var(--color-warning);
}

.state-text {
  flex: 1;
  min-width: 0;
}

.state-text p {
  margin: 3px 0 0;
  font-size: 12px;
}

.confirm {
  display: flex;
  gap: 10px;
  margin-top: 10px;
  padding: 10px 12px;
  border: 1px solid #4a1f22;
  border-radius: var(--radius);
  background: var(--color-danger-background);
  font-size: 12px;
}

.confirm-icon {
  flex: none;
  margin-top: 1px;
  color: #f2a7a9;
}

.confirm-body {
  flex: 1;
}

.confirm-body p {
  margin: 0;
  color: #f2c4c5;
}

.confirm-actions {
  justify-content: flex-end;
  margin-top: 10px;
}

.hint {
  margin: 10px 0 0;
  font-size: 12px;
}
</style>
