<script setup lang="ts">
import { ref } from "vue";
import type { SSHKeyType } from "../api";
import { cancelEditing, generateSSHKey, importSSHKey, store } from "../store";
import AppIcon from "./AppIcon.vue";
import ItemTile from "./ItemTile.vue";

type Mode = "generate" | "import";

const mode = ref<Mode>("generate");
const title = ref("");
const keyType = ref<SSHKeyType>("ed25519");
const privateKeyText = ref("");
const passphrase = ref("");

const allKeyTypeOption: { value: SSHKeyType; label: string; description: string }[] = [
  { value: "ed25519", label: "Ed25519", description: "Recommended. Small, fast and supported by current servers and GitHub." },
  { value: "rsa", label: "RSA 4096", description: "For older servers that do not accept Ed25519 keys." },
];

async function submitKey(): Promise<void> {
  if (mode.value === "generate") {
    await generateSSHKey(title.value, keyType.value);
    return;
  }
  if (await importSSHKey(title.value, privateKeyText.value, passphrase.value)) {
    privateKeyText.value = "";
  }
  passphrase.value = "";
}
</script>

<template>
  <form class="ssh-key-form" @submit.prevent="submitKey">
    <header>
      <ItemTile :title="title || 'SSH key'" :size="52" icon="key" />
      <input v-model="title" class="title-input" name="title" placeholder="SSH key title" aria-label="Title" required autofocus />
    </header>

    <div class="segmented" role="tablist" aria-label="How to add the key">
      <button type="button" role="tab" :aria-selected="mode === 'generate'" :class="{ active: mode === 'generate' }" @click="mode = 'generate'">
        Generate
      </button>
      <button type="button" role="tab" :aria-selected="mode === 'import'" :class="{ active: mode === 'import' }" @click="mode = 'import'">Import</button>
    </div>

    <section v-if="mode === 'generate'" class="card editor-card" role="radiogroup" aria-label="Key type">
      <label v-for="option in allKeyTypeOption" :key="option.value" class="key-type" :class="{ selected: keyType === option.value }">
        <input v-model="keyType" type="radio" name="keyType" :value="option.value" />
        <span class="key-type-text">
          <strong>{{ option.label }}</strong>
          <span class="muted">{{ option.description }}</span>
        </span>
      </label>
    </section>

    <section v-else class="card editor-card">
      <label>
        <span>Private key</span>
        <textarea
          v-model="privateKeyText"
          name="privateKey"
          class="monospace"
          rows="7"
          spellcheck="false"
          autocomplete="off"
          placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
          required
        ></textarea>
      </label>
      <label class="last">
        <span>Passphrase (optional)</span>
        <input v-model="passphrase" name="passphrase" type="password" autocomplete="off" placeholder="Only if the key is protected by one" />
      </label>
      <p class="hint muted">Hisar stores the key without its passphrase; your vault encryption protects it.</p>
    </section>

    <div class="row footer">
      <span class="muted"><AppIcon name="shield" :size="12" /> The private key never leaves Hisar's encrypted vault</span>
      <div class="spacer"></div>
      <button type="button" class="ghost" @click="cancelEditing">Cancel</button>
      <button class="primary" type="submit" :disabled="store.isBusy || !title.trim() || (mode === 'import' && !privateKeyText.trim())">
        {{ mode === "generate" ? "Generate key" : "Import key" }}
      </button>
    </div>
  </form>
</template>

<style scoped>
.ssh-key-form {
  max-width: 640px;
}

header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 18px;
}

.title-input {
  padding: 6px 8px;
  border-color: transparent;
  background: transparent;
  font-size: 20px;
  font-weight: 650;
}

.title-input:hover {
  border-color: var(--color-border);
}

.segmented {
  display: grid;
  grid-template-columns: 1fr 1fr;
  max-width: 280px;
  margin-bottom: 12px;
  padding: 3px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.segmented button {
  border: none;
  background: transparent;
  color: var(--color-text-muted);
}

.segmented button.active {
  background: var(--color-selected);
  color: var(--color-text);
  box-shadow: 0 1px 2px var(--color-shadow);
}

.editor-card {
  margin-bottom: 12px;
  padding: 16px;
}

.key-type {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin: 0 0 8px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  cursor: pointer;
}

.key-type:last-child {
  margin-bottom: 0;
}

.key-type.selected {
  border-color: var(--color-border-strong);
  background: var(--color-surface-raised);
}

.key-type input {
  width: 15px;
  height: 15px;
  margin: 2px 0 0;
  accent-color: var(--color-primary);
}

.key-type-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.key-type-text .muted {
  font-size: 12px;
}

label.last {
  margin-bottom: 0;
}

.hint {
  margin: 10px 0 0;
  font-size: 12px;
}

.footer {
  margin-top: 4px;
}

.footer .muted {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}
</style>
