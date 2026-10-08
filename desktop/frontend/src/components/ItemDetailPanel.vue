<script setup lang="ts">
import { ref, watch } from "vue";
import { copyField, deleteItem, openWebsite, startEditingItem, store } from "../store";
import { getDomain } from "../util";
import AppIcon from "./AppIcon.vue";
import ItemTile from "./ItemTile.vue";
import StrengthMeter from "./StrengthMeter.vue";

defineProps<{ canEdit: boolean; vaultName: string; isShared: boolean }>();

const isPasswordRevealed = ref(false);
const allRevealedFieldName = ref<string[]>([]);
const isConfirmingDelete = ref(false);

watch(
  () => store.selectedItem?.id,
  () => {
    isPasswordRevealed.value = false;
    allRevealedFieldName.value = [];
    isConfirmingDelete.value = false;
  },
);

function toggleFieldReveal(fieldName: string): void {
  allRevealedFieldName.value = allRevealedFieldName.value.includes(fieldName)
    ? allRevealedFieldName.value.filter((name) => name !== fieldName)
    : [...allRevealedFieldName.value, fieldName];
}

function maskSecret(secret: string): string {
  return "•".repeat(Math.min(Math.max(secret.length, 10), 20));
}
</script>

<template>
  <article v-if="store.selectedItem" class="item-detail">
    <header>
      <ItemTile :title="store.selectedItem.title" :size="52" />
      <div class="heading">
        <h2>{{ store.selectedItem.title }}</h2>
        <div class="meta">
          <span class="badge"><AppIcon :name="isShared ? 'users' : 'user'" :size="12" /> {{ vaultName }}</span>
          <span v-if="isShared" class="badge">shared</span>
          <span v-if="store.selectedItem.urls[0]" class="muted">{{ getDomain(store.selectedItem.urls[0]) }}</span>
        </div>
      </div>
      <div v-if="canEdit" class="header-actions">
        <button @click="startEditingItem"><AppIcon name="edit" /> Edit</button>
        <button v-if="!isConfirmingDelete" class="ghost icon-button" aria-label="Delete item" title="Delete" @click="isConfirmingDelete = true">
          <AppIcon name="trash" />
        </button>
      </div>
    </header>

    <div v-if="isConfirmingDelete" class="card delete-confirm" role="alertdialog">
      <span>Delete <strong>{{ store.selectedItem.title }}</strong> from {{ vaultName }}?</span>
      <div class="row">
        <button class="ghost" @click="isConfirmingDelete = false">Cancel</button>
        <button class="danger-solid" data-testid="confirm-delete" @click="deleteItem(store.selectedItem.id)">Delete item</button>
      </div>
    </div>

    <section class="card field-card">
      <div v-if="store.selectedItem.username" class="field">
        <span class="field-label">Username</span>
        <span class="field-value">{{ store.selectedItem.username }}</span>
        <span class="field-actions">
          <button class="ghost icon-button" aria-label="Copy username" title="Copy" @click="copyField('username', 'Username')"><AppIcon name="copy" /></button>
        </span>
      </div>

      <div v-if="store.selectedItem.password" class="field">
        <span class="field-label">Password</span>
        <span class="field-value">
          <span class="monospace secret" data-testid="password-value">
            {{ isPasswordRevealed ? store.selectedItem.password : maskSecret(store.selectedItem.password) }}
          </span>
          <StrengthMeter :password="store.selectedItem.password" />
        </span>
        <span class="field-actions">
          <button
            class="ghost icon-button"
            :aria-label="isPasswordRevealed ? 'Hide password' : 'Reveal password'"
            :title="isPasswordRevealed ? 'Hide' : 'Reveal'"
            @click="isPasswordRevealed = !isPasswordRevealed"
          >
            <AppIcon :name="isPasswordRevealed ? 'eyeOff' : 'eye'" />
          </button>
          <button class="ghost icon-button" aria-label="Copy password" title="Copy" @click="copyField('password', 'Password')"><AppIcon name="copy" /></button>
        </span>
      </div>

      <div v-for="link in store.selectedItem.urls" :key="link" class="field">
        <span class="field-label">Website</span>
        <span class="field-value">{{ link }}</span>
        <span class="field-actions">
          <button class="ghost icon-button" aria-label="Open website" title="Open in browser" @click="openWebsite(link)"><AppIcon name="external" /></button>
        </span>
      </div>

      <div v-for="field in store.selectedItem.fields" :key="field.name" class="field">
        <span class="field-label">{{ field.name }}</span>
        <span class="field-value" :class="{ monospace: field.concealed }">
          {{ field.concealed && !allRevealedFieldName.includes(field.name) ? maskSecret(field.value) : field.value }}
        </span>
        <span class="field-actions">
          <button
            v-if="field.concealed"
            class="ghost icon-button"
            :aria-label="`${allRevealedFieldName.includes(field.name) ? 'Hide' : 'Reveal'} ${field.name}`"
            @click="toggleFieldReveal(field.name)"
          >
            <AppIcon :name="allRevealedFieldName.includes(field.name) ? 'eyeOff' : 'eye'" />
          </button>
          <button class="ghost icon-button" :aria-label="`Copy ${field.name}`" title="Copy" @click="copyField(field.name, field.name)"><AppIcon name="copy" /></button>
        </span>
      </div>

      <p v-if="!store.selectedItem.username && !store.selectedItem.password && store.selectedItem.urls.length === 0 && store.selectedItem.fields.length === 0" class="muted no-fields">
        This item has no fields yet.
      </p>
    </section>

    <section v-if="store.selectedItem.notes" class="card notes-card">
      <p class="section-label">Notes</p>
      <p class="notes">{{ store.selectedItem.notes }}</p>
    </section>

    <p class="footer muted"><AppIcon name="shield" :size="12" /> End-to-end encrypted with the {{ vaultName }} vault key</p>
  </article>
</template>

<style scoped>
.item-detail {
  max-width: 640px;
}

header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 22px;
}

.heading {
  flex: 1;
  min-width: 0;
}

h2 {
  margin: 0 0 6px;
  overflow: hidden;
  font-size: 20px;
  font-weight: 650;
  letter-spacing: -0.01em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 7px;
  border: 1px solid var(--color-border-strong);
  border-radius: 10px;
  color: var(--color-text-secondary);
}

.header-actions {
  display: flex;
  gap: 4px;
}

.delete-confirm {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border-color: #4a1f22;
  background: var(--color-danger-background);
}

.field-card {
  padding: 4px 0;
}

.field {
  display: grid;
  grid-template-columns: 110px 1fr auto;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  padding: 8px 10px 8px 16px;
  border-bottom: 1px solid var(--color-border);
}

.field:last-child {
  border-bottom: none;
}

.field-label {
  font-size: 12px;
  color: var(--color-text-muted);
}

.field-value {
  min-width: 0;
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.secret {
  letter-spacing: 0.06em;
}

.field-actions {
  display: flex;
  gap: 2px;
  opacity: 0.55;
  transition: opacity 120ms;
}

.field:hover .field-actions,
.field-actions:focus-within {
  opacity: 1;
}

.no-fields {
  margin: 0;
  padding: 14px 16px;
}

.notes-card {
  margin-top: 14px;
  padding: 14px 16px;
}

.notes {
  margin: 0;
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

.footer {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 18px;
  font-size: 11px;
}
</style>
