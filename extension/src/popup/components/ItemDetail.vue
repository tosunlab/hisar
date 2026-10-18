<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import type { ItemField, ItemResult, ItemSummary, OneTimeCode } from "../../shared/protocol";
import { closePopup, describeError, sendToBackground } from "../api";
import { writeToClipboard } from "../clipboard";
import AppIcon from "./AppIcon.vue";
import ItemIcon from "./ItemIcon.vue";
import OneTimeCodeRow from "./OneTimeCodeRow.vue";

// canFill: the item is a login saved for the current tab, so Fill can fill it (the app checks the site again).
const props = defineProps<{ summary: ItemSummary; tabId: number | undefined; canFill: boolean }>();
const emit = defineEmits<{ back: []; notice: [message: string]; favorite: [isFavorite: boolean]; filled: [] }>();

const item = ref<ItemResult>();
const errorMessage = ref("");
const isBusy = ref(false);
const allRevealed = reactive(new Map<string, string>());
const backButton = ref<HTMLButtonElement>();

async function load(): Promise<void> {
  try {
    item.value = await sendToBackground<ItemResult>({ type: "popup.getItem", vaultID: props.summary.vaultID, itemID: props.summary.itemID });
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

async function readValue(field: ItemField): Promise<string> {
  if (field.kind !== "secret") return field.value ?? "";
  const revealed = allRevealed.get(field.id);
  if (revealed !== undefined) return revealed;
  return sendToBackground<string>({ type: "popup.revealField", vaultID: props.summary.vaultID, itemID: props.summary.itemID, fieldID: field.id });
}

async function run(action: () => Promise<void>): Promise<void> {
  errorMessage.value = "";
  isBusy.value = true;
  try {
    await action();
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isBusy.value = false;
  }
}

function toggleReveal(field: ItemField): Promise<void> {
  return run(async () => {
    if (allRevealed.has(field.id)) {
      allRevealed.delete(field.id);
      return;
    }
    allRevealed.set(field.id, await readValue(field));
  });
}

function copyField(field: ItemField): Promise<void> {
  return run(async () => {
    await writeToClipboard(await readValue(field));
    emit("notice", `${field.label} copied`);
  });
}

// toWebsiteUrl opens only web pages: a saved "example.com" gets https://, any other scheme is not opened.
function toWebsiteUrl(url: string): string | undefined {
  if (/^https?:\/\//i.test(url)) return url;
  if (/^[a-z][a-z0-9+.-]*:/i.test(url)) return undefined;
  return `https://${url}`;
}

function openWebsite(url: string): void {
  const websiteUrl = toWebsiteUrl(url);
  if (!websiteUrl) return;
  void chrome.tabs.create({ url: websiteUrl });
  closePopup();
}

function toggleFavorite(): Promise<void> {
  return run(async () => {
    const isFavorite = !(item.value?.isFavorite ?? props.summary.isFavorite);
    await sendToBackground({ type: "popup.setFavorite", vaultID: props.summary.vaultID, itemID: props.summary.itemID, isFavorite });
    if (item.value) item.value.isFavorite = isFavorite;
    emit("favorite", isFavorite);
  });
}

function fill(): Promise<void> {
  return run(async () => {
    if (props.tabId === undefined) return;
    await sendToBackground({ type: "popup.fillLogin", tabId: props.tabId, vaultID: props.summary.vaultID, itemID: props.summary.itemID });
    emit("filled");
    closePopup();
  });
}

function requestCode(): Promise<OneTimeCode> {
  return sendToBackground<OneTimeCode>({ type: "popup.getItemCode", vaultID: props.summary.vaultID, itemID: props.summary.itemID });
}

onMounted(() => {
  backButton.value?.focus();
  void load();
});
</script>

<template>
  <div class="detail" data-testid="item-detail" @keydown.esc.prevent="emit('back')">
    <div class="top">
      <button ref="backButton" type="button" class="ghost icon-button" title="Back" aria-label="Back" data-testid="item-back" @click="emit('back')">
        <AppIcon name="back" />
      </button>
      <ItemIcon :category="summary.category" :url="summary.url" :size="36" />
      <div class="heading">
        <p class="title" data-testid="item-title">{{ summary.title }}</p>
        <p class="muted vault">{{ summary.vaultName }}</p>
      </div>
      <button
        type="button"
        class="ghost icon-button favorite"
        :class="{ active: item?.isFavorite ?? summary.isFavorite }"
        :title="(item?.isFavorite ?? summary.isFavorite) ? 'Remove from favorites' : 'Add to favorites'"
        :aria-label="(item?.isFavorite ?? summary.isFavorite) ? 'Remove from favorites' : 'Add to favorites'"
        :aria-pressed="item?.isFavorite ?? summary.isFavorite"
        :disabled="isBusy"
        data-testid="item-favorite"
        @click="toggleFavorite"
      >
        <AppIcon name="star" :is-filled="item?.isFavorite ?? summary.isFavorite" />
      </button>
    </div>
    <button v-if="canFill" type="button" class="primary fill" :disabled="isBusy" data-testid="item-fill" @click="fill">Fill on this page</button>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <p v-if="!item && !errorMessage" class="muted empty">Loading…</p>
    <ul v-else-if="item" class="fields">
      <li v-for="field in item.fields" :key="field.id" class="field" :data-testid="`field-${field.id}`">
        <p class="label muted">{{ field.label }}</p>
        <OneTimeCodeRow v-if="field.kind === 'one-time-code'" :login="summary" :request-code="requestCode" @notice="emit('notice', $event)" />
        <div v-else class="value-row">
          <p
            class="value"
            :class="{ monospace: field.kind === 'secret', notes: field.kind === 'notes' }"
            data-testid="field-value"
          >
            <template v-if="field.kind === 'secret'">{{ allRevealed.get(field.id) ?? "••••••••••" }}</template>
            <template v-else>{{ field.value }}</template>
          </p>
          <button
            v-if="field.kind === 'secret'"
            type="button"
            class="ghost icon-button"
            :title="allRevealed.has(field.id) ? 'Hide' : 'Reveal'"
            :aria-label="allRevealed.has(field.id) ? `Hide ${field.label}` : `Reveal ${field.label}`"
            :disabled="isBusy"
            data-testid="reveal-field"
            @click="toggleReveal(field)"
          >
            <AppIcon :name="allRevealed.has(field.id) ? 'eye-off' : 'eye'" />
          </button>
          <button
            v-if="field.kind === 'url' && field.value && toWebsiteUrl(field.value)"
            type="button"
            class="ghost icon-button"
            title="Open website"
            :aria-label="`Open ${field.value}`"
            data-testid="open-website"
            @click="openWebsite(field.value)"
          >
            <AppIcon name="external" />
          </button>
          <button
            type="button"
            class="ghost icon-button"
            :title="`Copy ${field.label.toLowerCase()}`"
            :aria-label="`Copy ${field.label}`"
            :disabled="isBusy"
            data-testid="copy-field"
            @click="copyField(field)"
          >
            <AppIcon name="copy" />
          </button>
        </div>
      </li>
      <li v-if="item.fields.length === 0" class="muted empty">This item has no fields to show here.</li>
    </ul>
  </div>
</template>

<style scoped>
.top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.heading {
  flex: 1;
  min-width: 0;
}

.heading p {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title {
  font-size: 14px;
  font-weight: 600;
}

.vault {
  font-size: 12px;
}

.favorite.active {
  color: var(--color-warning);
}

.fill {
  width: 100%;
  margin-bottom: 12px;
}

.empty {
  margin: 18px 0;
  text-align: center;
}

.fields {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-panel);
  list-style: none;
}

.field {
  padding: 8px 10px;
}

.field + .field {
  border-top: 1px solid var(--color-border);
}

.label {
  margin: 0 0 2px;
  font-size: 11px;
}

.value-row {
  display: flex;
  align-items: center;
  gap: 2px;
}

.value {
  flex: 1;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
}

.value.notes {
  max-height: 120px;
  overflow-y: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
