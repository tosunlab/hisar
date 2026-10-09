<script setup lang="ts">
import { computed, ref } from "vue";
import { confirmSecretKeySaved, copyText, saveNewAccountEmergencyKit, store } from "../store";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

const hasSaved = ref(false);
const allKeyPart = computed(() => store.pendingSecretKey.split("-"));
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="card page-card secret-key-card">
        <HisarLogo :size="32" />
        <h1>Save your Secret Key</h1>
        <p class="subtitle">
          You need it, together with your master password, to sign in on a new device. It never leaves your devices, so
          nobody, including your Hisar admin, can recover it for you.
        </p>
        <div class="key" data-testid="secret-key" :aria-label="store.pendingSecretKey">
          <span v-for="(part, index) in allKeyPart" :key="index" class="key-part monospace">{{ part }}</span>
        </div>
        <div class="row actions">
          <button @click="copyText(store.pendingSecretKey, 'Secret Key')"><AppIcon name="copy" /> Copy</button>
          <button data-testid="save-emergency-kit" @click="saveNewAccountEmergencyKit"><AppIcon name="note" /> Save Emergency Kit (PDF)</button>
        </div>
        <label class="row confirmation">
          <input v-model="hasSaved" type="checkbox" name="hasSaved" />
          <span class="confirmation-text">I have written down or stored my Secret Key somewhere safe</span>
        </label>
        <button class="primary full-width" :disabled="!hasSaved" @click="confirmSecretKeySaved">Continue to Hisar</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.secret-key-card {
  width: min(580px, 100%);
}

.key {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px;
  padding: 18px 12px;
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  user-select: text;
  -webkit-user-select: text;
}

.key-part {
  padding: 4px 6px;
  border-radius: var(--radius-small);
  background: var(--color-surface-raised);
  font-size: 14px;
}

.actions {
  justify-content: center;
  margin: 14px 0 20px;
}

.confirmation {
  margin-bottom: 18px;
  align-items: flex-start;
}

.confirmation-text {
  margin: 0;
  font-size: 13px;
  font-weight: 400;
  color: var(--color-text);
}
</style>
