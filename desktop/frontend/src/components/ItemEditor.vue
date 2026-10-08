<script setup lang="ts">
import { reactive, ref } from "vue";
import { createEmptyItem, type ItemDetail } from "../api";
import { sshKeyCategory } from "../util";
import { cancelEditing, saveItem, store } from "../store";
import AppIcon from "./AppIcon.vue";
import ItemTile from "./ItemTile.vue";
import PasswordGenerator from "./PasswordGenerator.vue";
import StrengthMeter from "./StrengthMeter.vue";

function createDraft(): ItemDetail {
  if (store.editorMode === "edit" && store.selectedItem) {
    const selected = store.selectedItem;
    return { ...selected, urls: [...selected.urls], fields: selected.fields.map((field) => ({ ...field })) };
  }
  return createEmptyItem();
}

const draft = reactive<ItemDetail>(createDraft());
const urlText = ref(draft.urls.join("\n"));
const isPasswordVisible = ref(false);
const isGeneratorOpen = ref(store.editorMode === "create");
const isSSHKey = draft.category === sshKeyCategory;

function usePassword(password: string): void {
  draft.password = password;
  isGeneratorOpen.value = false;
  isPasswordVisible.value = true;
}

function addField(): void {
  draft.fields.push({ name: "", value: "", concealed: false });
}

function removeField(index: number): void {
  draft.fields.splice(index, 1);
}

async function submitItem(): Promise<void> {
  draft.urls = urlText.value
    .split("\n")
    .map((link) => link.trim())
    .filter((link) => link !== "");
  await saveItem({ ...draft });
}
</script>

<template>
  <form class="item-editor" @submit.prevent="submitItem">
    <header>
      <ItemTile :title="draft.title || '?'" :size="52" :icon="isSSHKey ? 'key' : undefined" />
      <input v-model="draft.title" class="title-input" name="title" placeholder="Item title" aria-label="Title" required autofocus />
    </header>

    <p v-if="isSSHKey" class="card ssh-key-note muted" data-testid="ssh-key-note">
      <AppIcon name="key" :size="14" /> The key itself cannot be changed. To replace it, add a new SSH key.
    </p>

    <section v-if="!isSSHKey" class="card editor-card">
      <label>
        <span>Username</span>
        <input v-model="draft.username" name="username" autocomplete="off" placeholder="Username or email" />
      </label>
      <label>
        <span>Password</span>
        <span class="row">
          <input
            v-model="draft.password"
            name="password"
            class="monospace"
            :type="isPasswordVisible ? 'text' : 'password'"
            autocomplete="new-password"
          />
          <button
            type="button"
            class="icon-button"
            :aria-label="isPasswordVisible ? 'Hide password' : 'Show password'"
            @click="isPasswordVisible = !isPasswordVisible"
          >
            <AppIcon :name="isPasswordVisible ? 'eyeOff' : 'eye'" />
          </button>
          <button type="button" :class="{ active: isGeneratorOpen }" @click="isGeneratorOpen = !isGeneratorOpen">
            <AppIcon name="refresh" /> Generate
          </button>
        </span>
        <StrengthMeter :password="draft.password" />
      </label>
      <PasswordGenerator v-if="isGeneratorOpen" @use="usePassword" @close="isGeneratorOpen = false" />
      <label class="last">
        <span>Websites</span>
        <textarea v-model="urlText" name="urls" rows="2" placeholder="https://example.com (one per line)"></textarea>
      </label>
    </section>

    <section v-if="!isSSHKey" class="card editor-card">
      <p class="section-label">Other fields</p>
      <div v-for="(field, index) in draft.fields" :key="index" class="custom-field">
        <input v-model="field.name" :name="`fieldName${index}`" placeholder="Name" aria-label="Field name" />
        <input
          v-model="field.value"
          :name="`fieldValue${index}`"
          :type="field.concealed ? 'password' : 'text'"
          placeholder="Value"
          aria-label="Field value"
        />
        <label class="concealed" :title="'Hide this value until revealed'">
          <input v-model="field.concealed" type="checkbox" /> <span>Secret</span>
        </label>
        <button type="button" class="ghost icon-button" aria-label="Remove field" @click="removeField(index)"><AppIcon name="close" /></button>
      </div>
      <button type="button" class="ghost add-field" @click="addField"><AppIcon name="plus" /> Add field</button>
    </section>

    <section class="card editor-card">
      <label class="last">
        <span>Notes</span>
        <textarea v-model="draft.notes" name="notes" rows="4" placeholder="Anything else worth keeping safe"></textarea>
      </label>
    </section>

    <div class="row footer">
      <span class="muted"><AppIcon name="shield" :size="12" /> Encrypted on this device before saving</span>
      <div class="spacer"></div>
      <button type="button" class="ghost" @click="cancelEditing">Cancel</button>
      <button class="primary" type="submit" :disabled="store.isBusy">{{ store.editorMode === "create" ? "Add item" : "Save" }}</button>
    </div>
  </form>
</template>

<style scoped>
.item-editor {
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

.editor-card {
  margin-bottom: 12px;
  padding: 16px;
}

label.last {
  margin-bottom: 0;
}

.ssh-key-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 12px;
  padding: 12px 16px;
  font-size: 12px;
}

.row input {
  flex: 1;
}

button.active {
  background: var(--color-selected);
}

.custom-field {
  display: grid;
  grid-template-columns: 1fr 1.4fr auto auto;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.concealed {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  white-space: nowrap;
}

.concealed span {
  margin: 0;
  font-size: 12px;
  font-weight: 400;
  color: var(--color-text-secondary);
}

.add-field {
  padding-left: 0;
  color: var(--color-text-muted);
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
