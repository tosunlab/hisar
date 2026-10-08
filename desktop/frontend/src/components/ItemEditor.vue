<script setup lang="ts">
import { reactive, ref } from "vue";
import { createEmptyItem, type ItemDetail } from "../api";
import { cancelEditing, saveItem, store } from "../store";
import PasswordGenerator from "./PasswordGenerator.vue";

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
    <h2>{{ store.editorMode === "create" ? "New item" : "Edit item" }}</h2>
    <label><span>Title</span><input v-model="draft.title" name="title" required autofocus /></label>
    <label><span>Username</span><input v-model="draft.username" name="username" autocomplete="off" /></label>
    <label>
      <span>Password</span>
      <div class="row">
        <input
          v-model="draft.password"
          name="password"
          class="monospace"
          :type="isPasswordVisible ? 'text' : 'password'"
          autocomplete="new-password"
        />
        <button type="button" @click="isPasswordVisible = !isPasswordVisible">{{ isPasswordVisible ? "Hide" : "Show" }}</button>
        <button type="button" @click="isGeneratorOpen = !isGeneratorOpen">Generate</button>
      </div>
    </label>
    <PasswordGenerator v-if="isGeneratorOpen" @use="usePassword" @close="isGeneratorOpen = false" />
    <label>
      <span>Websites (one per line)</span>
      <textarea v-model="urlText" name="urls" rows="2" placeholder="https://example.com"></textarea>
    </label>

    <div class="fields">
      <span class="fields-title">Other fields</span>
      <div v-for="(field, index) in draft.fields" :key="index" class="row custom-field">
        <input v-model="field.name" :name="`fieldName${index}`" placeholder="Name" aria-label="Field name" />
        <input
          v-model="field.value"
          :name="`fieldValue${index}`"
          :type="field.concealed ? 'password' : 'text'"
          placeholder="Value"
          aria-label="Field value"
        />
        <label class="row concealed"><input v-model="field.concealed" type="checkbox" /> Secret</label>
        <button type="button" class="danger" aria-label="Remove field" @click="removeField(index)">×</button>
      </div>
      <button type="button" class="link" @click="addField">+ Add field</button>
    </div>

    <label><span>Notes</span><textarea v-model="draft.notes" name="notes" rows="4"></textarea></label>

    <div class="row">
      <div class="spacer"></div>
      <button type="button" @click="cancelEditing">Cancel</button>
      <button class="primary" type="submit" :disabled="store.isBusy">Save</button>
    </div>
  </form>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
  font-size: 22px;
}

.item-editor {
  max-width: 560px;
}

.row input {
  flex: 1;
}

.fields {
  margin-bottom: 12px;
}

.fields-title {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-muted);
}

.custom-field {
  margin-bottom: 6px;
}

.concealed {
  margin: 0;
  white-space: nowrap;
}

.concealed input {
  flex: none;
  width: auto;
}
</style>
