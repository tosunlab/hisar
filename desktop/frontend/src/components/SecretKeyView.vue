<script setup lang="ts">
import { ref } from "vue";
import { confirmSecretKeySaved, copyText, store } from "../store";

const hasSaved = ref(false);
</script>

<template>
  <div class="centered-page">
    <div class="title-bar" style="width: 100%"></div>
    <div class="card">
      <h1>Your Secret Key</h1>
      <p class="subtitle muted">
        You need it together with your master password to sign in on a new device. It never leaves your devices, so nobody
        can recover it for you. Write it down and keep it somewhere safe.
      </p>
      <p class="secret-key monospace" data-testid="secret-key">{{ store.pendingSecretKey }}</p>
      <div class="row">
        <button @click="copyText(store.pendingSecretKey, 'Secret Key')">Copy</button>
      </div>
      <label class="row confirmation">
        <input v-model="hasSaved" type="checkbox" name="hasSaved" />
        I have written down or stored my Secret Key
      </label>
      <button class="primary" :disabled="!hasSaved" @click="confirmSecretKeySaved">Continue</button>
    </div>
  </div>
</template>

<style scoped>
.secret-key {
  padding: 16px;
  border: 1px dashed var(--color-border);
  border-radius: var(--radius);
  font-size: 16px;
  text-align: center;
  word-break: break-all;
  user-select: text;
  -webkit-user-select: text;
}

.confirmation {
  margin: 16px 0;
}

.confirmation input {
  width: auto;
}
</style>
