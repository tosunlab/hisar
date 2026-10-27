<script setup lang="ts">
import { ref } from "vue";
import { confirmSecretKeySaved, saveNewAccountEmergencyKit, store } from "../store";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";
import SecretKeyBlock from "./SecretKeyBlock.vue";

const hasSaved = ref(false);
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="card page-card secret-key-card">
        <HisarLogo :size="32" />
        <h1>Save your Secret Key</h1>
        <p class="subtitle">
          You need it, together with your master password, to sign in on a new device. Save your Emergency Kit: it holds this Secret Key, and
          nobody else has a copy.
        </p>
        <p class="master-password-warning" data-testid="master-password-warning">
          <AppIcon name="warning" />
          <span>A forgotten master password can't be recovered by anyone, including Hisar.</span>
        </p>
        <SecretKeyBlock :secret-key="store.pendingSecretKey" />
        <div class="row actions">
          <button data-testid="save-emergency-kit" @click="saveNewAccountEmergencyKit"><AppIcon name="note" /> Save Emergency Kit…</button>
        </div>
        <label class="row confirmation">
          <input v-model="hasSaved" type="checkbox" name="hasSaved" />
          <span class="confirmation-text">I have written down or stored my Secret Key somewhere safe</span>
        </label>
        <button class="primary full-width" :disabled="!hasSaved" @click="confirmSecretKeySaved">
          Continue to Hisar
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.secret-key-card {
  width: min(580px, 100%);
}


.actions {
  justify-content: center;
  margin: 14px 0 20px;
}

.master-password-warning {
  display: flex;
  gap: 8px;
  margin: 0 0 16px;
  padding: 9px 12px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: var(--font-size-13);
}

.master-password-warning :deep(.icon) {
  flex: none;
  margin-top: 1px;
  color: var(--color-warning);
}

.confirmation {
  margin-bottom: 18px;
  align-items: flex-start;
}

.confirmation-text {
  margin: 0;
  font-size: var(--font-size-13);
  font-weight: 400;
  color: var(--color-text);
}
</style>
