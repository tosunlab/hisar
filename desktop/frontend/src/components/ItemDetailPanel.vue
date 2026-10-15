<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  convertToSecureNote,
  copyField,
  deleteItem,
  getAllShareLinkForItem,
  hasShareableField,
  openItemShareLinkDialog,
  openWebsite,
  startEditingItem,
  store,
  toggleSelectedItemFavorite,
} from "../store";
import { cardFieldName, creditCardCategory, formatCardSummary, formatKeyType, getDomain, getItemIcon, isMacPlatform, secureNoteCategory, sshKeyCategory } from "../util";
import AppIcon from "./AppIcon.vue";
import CommitSigningPanel from "./CommitSigningPanel.vue";
import ItemTile from "./ItemTile.vue";
import OneTimeCodeField from "./OneTimeCodeField.vue";
import PasskeyList from "./PasskeyList.vue";
import SecretReferenceButton from "./SecretReferenceButton.vue";
import SharedLinksList from "./SharedLinksList.vue";
import StrengthMeter from "./StrengthMeter.vue";

defineProps<{ canEdit: boolean; vaultName: string; isShared: boolean }>();

const shortcutModifier = isMacPlatform() ? "⌘" : "Ctrl+";
const isPasswordRevealed = ref(false);
const allRevealedFieldName = ref<string[]>([]);
const isConfirmingDelete = ref(false);
const isSSHKey = computed(() => store.selectedItem?.category === sshKeyCategory);
const isSecureNote = computed(() => store.selectedItem?.category === secureNoteCategory);
const isCreditCard = computed(() => store.selectedItem?.category === creditCardCategory);

// The card's values as rows; the number shows only its last four digits until revealed.
const allCardRow = computed(() => {
  const card = store.selectedItem?.card;
  if (!card) {
    return [];
  }
  return [
    { name: cardFieldName.cardholder, value: card.cardholder, concealed: false, masked: "", warning: "" },
    { name: cardFieldName.type, value: card.type, concealed: false, masked: "", warning: "" },
    { name: cardFieldName.number, value: card.number, concealed: true, masked: card.last4 ? `•••• •••• •••• ${card.last4}` : "", warning: card.numberWarning },
    { name: cardFieldName.expiry, value: card.expiry, concealed: false, masked: "", warning: card.expiryWarning },
    { name: cardFieldName.securityCode, value: card.securityCode, concealed: true, masked: "", warning: "" },
    { name: cardFieldName.pin, value: card.pin, concealed: true, masked: "", warning: "" },
    { name: cardFieldName.billingAddress, value: card.billingAddress, concealed: false, masked: "", warning: "" },
  ].filter((row) => row.value !== "");
});
const isShareable = computed(() => store.selectedItem !== null && hasShareableField(store.selectedItem));
const allItemShareLink = computed(() => (store.selectedItem ? getAllShareLinkForItem(store.selectedVaultID, store.selectedItem.id) : []));

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
      <ItemTile :title="store.selectedItem.title" :size="52" :icon="getItemIcon(store.selectedItem.category)" />
      <div class="heading">
        <h2>{{ store.selectedItem.title }}</h2>
        <div class="meta">
          <span class="badge"><AppIcon :name="isShared ? 'users' : 'user'" :size="12" /> {{ vaultName }}</span>
          <span v-if="isShared" class="badge">shared</span>
          <span v-if="isSSHKey" class="muted">SSH key</span>
          <span v-else-if="isSecureNote" class="muted">Secure note</span>
          <span v-else-if="isCreditCard" class="muted" data-testid="card-summary">{{ formatCardSummary(store.selectedItem.card) }}</span>
          <span v-else-if="store.selectedItem.urls[0]" class="muted">{{ getDomain(store.selectedItem.urls[0]) }}</span>
        </div>
      </div>
      <div class="header-actions">
        <button
          class="ghost icon-button favorite-button"
          :class="{ 'is-favorite': store.selectedItem.isFavorite }"
          :aria-label="store.selectedItem.isFavorite ? 'Remove from Favorites' : 'Add to Favorites'"
          :aria-pressed="store.selectedItem.isFavorite"
          :title="`${store.selectedItem.isFavorite ? 'Remove from Favorites' : 'Add to Favorites'} (${shortcutModifier}D)`"
          data-testid="favorite-toggle"
          @click="toggleSelectedItemFavorite"
        >
          <AppIcon :name="store.selectedItem.isFavorite ? 'starFilled' : 'star'" />
        </button>
        <button v-if="isShareable" title="Share with anyone through an end-to-end encrypted link" data-testid="share-link" @click="openItemShareLinkDialog">
          <AppIcon name="link" /> Share link…
        </button>
        <template v-if="canEdit">
          <button @click="startEditingItem"><AppIcon name="edit" /> Edit</button>
          <button v-if="!isConfirmingDelete" class="ghost icon-button" aria-label="Delete item" title="Delete" @click="isConfirmingDelete = true">
            <AppIcon name="trash" />
          </button>
        </template>
      </div>
    </header>

    <div v-if="isConfirmingDelete" class="card delete-confirm" role="alertdialog">
      <span>Delete <strong>{{ store.selectedItem.title }}</strong> from {{ vaultName }}?</span>
      <div class="row">
        <button class="ghost" @click="isConfirmingDelete = false">Cancel</button>
        <button class="danger-solid" data-testid="confirm-delete" @click="deleteItem(store.selectedItem.id)">Delete item</button>
      </div>
    </div>

    <template v-if="isSSHKey">
      <section class="card field-card" data-testid="ssh-key-fields">
        <div class="field">
          <span class="field-label">Key type</span>
          <span class="field-value">{{ formatKeyType(store.selectedItem.keyType) }}</span>
          <span class="field-actions"></span>
        </div>
        <div class="field">
          <span class="field-label">Fingerprint</span>
          <span class="field-value monospace" data-testid="fingerprint">{{ store.selectedItem.fingerprint }}</span>
          <span class="field-actions">
            <button class="ghost icon-button" aria-label="Copy fingerprint" title="Copy" @click="copyField('fingerprint', 'Fingerprint')"><AppIcon name="copy" /></button>
            <SecretReferenceButton field-name="fingerprint" label="fingerprint" />
          </span>
        </div>
        <div class="field">
          <span class="field-label">Private key</span>
          <span class="field-value muted">Never leaves Hisar; the SSH agent uses it</span>
          <span class="field-actions"></span>
        </div>
        <div class="public-key-field">
          <div class="row">
            <span class="field-label">Public key</span>
            <div class="spacer"></div>
            <button data-testid="copy-public-key" @click="copyField('public key', 'Public key')"><AppIcon name="copy" /> Copy public key</button>
            <SecretReferenceButton field-name="public key" label="public key" />
          </div>
          <p class="public-key monospace" data-testid="public-key">{{ store.selectedItem.publicKey }}</p>
        </div>
      </section>
      <p class="agent-hint muted"><AppIcon name="terminal" :size="14" /> Hisar's SSH agent offers this key while Hisar is unlocked.</p>
      <CommitSigningPanel v-if="store.selectedItem.publicKey" :public-key="store.selectedItem.publicKey" />
    </template>

    <section v-else-if="isCreditCard" class="card field-card" data-testid="credit-card-fields">
      <div v-for="row in allCardRow" :key="row.name" class="field">
        <span class="field-label">{{ row.name }}</span>
        <span class="field-value">
          <span :class="{ monospace: row.concealed, 'multiline-value': row.name === cardFieldName.billingAddress }" :data-testid="`card-${row.name}`">
            {{ row.concealed && !allRevealedFieldName.includes(row.name) ? row.masked || maskSecret(row.value) : row.value }}
          </span>
          <small v-if="row.warning" class="field-warning" :data-testid="`card-warning-${row.name}`">{{ row.warning }}</small>
        </span>
        <span class="field-actions">
          <button
            v-if="row.concealed"
            class="ghost icon-button"
            :aria-label="`${allRevealedFieldName.includes(row.name) ? 'Hide' : 'Reveal'} ${row.name}`"
            @click="toggleFieldReveal(row.name)"
          >
            <AppIcon :name="allRevealedFieldName.includes(row.name) ? 'eyeOff' : 'eye'" />
          </button>
          <button class="ghost icon-button" :aria-label="`Copy ${row.name}`" title="Copy" @click="copyField(row.name, row.name)"><AppIcon name="copy" /></button>
          <SecretReferenceButton :field-name="row.name" :label="row.name" />
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
          <SecretReferenceButton :field-name="field.name" :label="field.name" />
        </span>
      </div>
      <p v-if="allCardRow.length === 0 && store.selectedItem.fields.length === 0" class="muted no-fields">This card has no details yet.</p>
    </section>

    <template v-else-if="isSecureNote">
      <section class="card notes-card secure-note" data-testid="secure-note-notes">
        <div class="row">
          <p class="section-label">Notes</p>
          <div class="spacer"></div>
          <template v-if="store.selectedItem.notes">
            <button data-testid="copy-notes" @click="copyField('notes', 'Notes')"><AppIcon name="copy" /> Copy</button>
            <SecretReferenceButton field-name="notes" label="notes" />
          </template>
        </div>
        <p v-if="store.selectedItem.notes" class="notes">{{ store.selectedItem.notes }}</p>
        <p v-else class="muted no-fields">This note is empty.</p>
      </section>
      <section v-if="store.selectedItem.fields.length > 0" class="card field-card note-fields">
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
            <SecretReferenceButton :field-name="field.name" :label="field.name" />
          </span>
        </div>
      </section>
    </template>

    <section v-else class="card field-card">
      <div v-if="store.selectedItem.username" class="field">
        <span class="field-label">Username</span>
        <span class="field-value">{{ store.selectedItem.username }}</span>
        <span class="field-actions">
          <button class="ghost icon-button" aria-label="Copy username" title="Copy" @click="copyField('username', 'Username')"><AppIcon name="copy" /></button>
          <SecretReferenceButton field-name="username" label="username" />
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
          <SecretReferenceButton field-name="password" label="password" />
        </span>
      </div>

      <OneTimeCodeField v-if="store.selectedItem.hasOneTimePassword" :vault-i-d="store.selectedVaultID" :item-i-d="store.selectedItem.id" />

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
          <SecretReferenceButton :field-name="field.name" :label="field.name" />
        </span>
      </div>

      <p
        v-if="!store.selectedItem.username && !store.selectedItem.password && store.selectedItem.urls.length === 0 && store.selectedItem.fields.length === 0 && (store.selectedItem.passkeys ?? []).length === 0"
        class="muted no-fields"
      >
        This item has no fields yet.
      </p>
    </section>

    <PasskeyList v-if="(store.selectedItem.passkeys ?? []).length > 0" :passkeys="store.selectedItem.passkeys ?? []" :can-delete="canEdit" />

    <section v-if="store.selectedItem.notes && !isSecureNote" class="card notes-card">
      <p class="section-label">Notes</p>
      <p class="notes">{{ store.selectedItem.notes }}</p>
    </section>

    <p v-if="canEdit && store.selectedItem.canConvertToSecureNote" class="convert-note muted" data-testid="convert-note">
      This login has no username, password or website.
      <button class="ghost" data-testid="convert-to-secure-note" @click="convertToSecureNote"><AppIcon name="note" /> Convert to secure note</button>
    </p>

    <section v-if="allItemShareLink.length > 0" class="card share-links-card" data-testid="item-share-links">
      <p class="section-label">Shared links</p>
      <SharedLinksList :all-share-link="allItemShareLink" :is-title-shown="false" />
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

.favorite-button.is-favorite {
  color: var(--color-warning);
}

.delete-confirm {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border-color: var(--color-danger-border);
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

.public-key-field {
  padding: 12px 10px 14px 16px;
}

.public-key {
  margin: 8px 0 0;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

.agent-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 2px 0;
  font-size: 12px;
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

.secure-note {
  margin-top: 0;
}

.secure-note .row {
  margin-bottom: 8px;
}

.secure-note .section-label {
  margin: 0;
}

.note-fields {
  margin-top: 14px;
}

.field-warning {
  display: block;
  margin-top: 2px;
  color: var(--color-warning);
  font-size: 12px;
}

.multiline-value {
  white-space: pre-wrap;
}

.convert-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 2px 0;
  font-size: 12px;
}

.share-links-card {
  margin-top: 14px;
  padding-top: 14px;
}

.share-links-card .section-label {
  padding: 0 16px;
}

.footer {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 18px;
  font-size: 11px;
}
</style>
