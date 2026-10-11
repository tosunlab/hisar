<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { ShareLink } from "../api";
import { closeShareLinkDialog, copyText, createShareLink, store } from "../store";
import { allShareLinkExpiry, describeShareLinkContent, formatDateTime, formatViewLimit } from "../util";
import AppIcon from "./AppIcon.vue";

const minimumPassphraseLength = 8;
const maximumViewCount = 100;

const dialog = computed(() => store.shareLinkDialog);
const isTextMode = computed(() => dialog.value?.mode === "text");
const allChosenFieldKey = ref<string[]>((dialog.value?.allField ?? []).filter((field) => field.isDefault).map((field) => field.key));
const textTitle = ref("");
const text = ref("");
const expiresInSeconds = ref(604800);
const viewCount = ref(1);
const isUnlimitedViews = ref(false);
const isPassphraseRequired = ref(false);
const passphrase = ref("");
const validationMessage = ref("");
const createdShareLink = ref<ShareLink | null>(null);

const heading = computed(() => (isTextMode.value ? "Share text" : `Share “${dialog.value?.title ?? ""}”`));

function validate(): string {
  if (isTextMode.value && !text.value.trim()) {
    return "Enter the text to share.";
  }
  if (!isTextMode.value && allChosenFieldKey.value.length === 0) {
    return "Choose at least one field to share.";
  }
  if (!isUnlimitedViews.value && (!Number.isInteger(viewCount.value) || viewCount.value < 1 || viewCount.value > maximumViewCount)) {
    return `Choose 1 to ${maximumViewCount} views, or unlimited.`;
  }
  if (isPassphraseRequired.value && [...passphrase.value].length < minimumPassphraseLength) {
    return `Use a passphrase of at least ${minimumPassphraseLength} characters.`;
  }
  return "";
}

async function submitShareLink(): Promise<void> {
  const current = dialog.value;
  validationMessage.value = validate();
  if (!current || validationMessage.value) {
    return;
  }
  const created = await createShareLink({
    vaultID: current.vaultID,
    itemID: current.itemID,
    fieldKeys: isTextMode.value ? [] : current.allField.map((field) => field.key).filter((key) => allChosenFieldKey.value.includes(key)),
    title: isTextMode.value ? textTitle.value.trim() : "",
    text: isTextMode.value ? text.value : "",
    expiresInSeconds: expiresInSeconds.value,
    maxViews: isUnlimitedViews.value ? 0 : viewCount.value,
    passphrase: isPassphraseRequired.value ? passphrase.value : "",
  });
  if (created) {
    passphrase.value = "";
    text.value = "";
    createdShareLink.value = created;
  }
}

async function copyCreatedLink(): Promise<void> {
  if (createdShareLink.value) {
    await copyText(createdShareLink.value.url, "Link");
  }
}

function capitalizeFirstLetter(label: string): string {
  return label.charAt(0).toUpperCase() + label.slice(1);
}

function selectLinkText(event: Event): void {
  (event.target as HTMLInputElement).select();
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeShareLinkDialog();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div v-if="dialog" class="overlay" @click.self="closeShareLinkDialog">
    <div class="card panel" role="dialog" aria-modal="true" aria-labelledby="share-link-title" data-testid="share-link-dialog">
      <header class="panel-header">
        <span class="panel-icon"><AppIcon name="link" :size="18" /></span>
        <div class="panel-heading">
          <h2 id="share-link-title">{{ heading }}</h2>
          <p class="muted">Anyone with the link can open it, even without a Hisar account. It's end-to-end encrypted: the server never sees what you share.</p>
        </div>
        <button class="ghost icon-button" aria-label="Close" @click="closeShareLinkDialog"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <div v-if="createdShareLink" data-testid="share-link-created">
        <label>
          <span>Link</span>
          <div class="row">
            <input class="monospace link-input" :value="createdShareLink.url" readonly aria-label="Share link" data-testid="share-link-url" @focus="selectLinkText" />
            <button class="primary" data-testid="copy-share-link" @click="copyCreatedLink"><AppIcon name="copy" /> Copy</button>
          </div>
        </label>
        <ul class="summary">
          <li><AppIcon name="field" :size="14" /> {{ describeShareLinkContent(createdShareLink) }}</li>
          <li><AppIcon name="shield" :size="14" /> Expires {{ formatDateTime(createdShareLink.expiresAt) }} · {{ formatViewLimit(createdShareLink.maxViews) }}</li>
          <li v-if="createdShareLink.hasPassphrase"><AppIcon name="lock" :size="14" /> Needs the passphrase: send it another way than the link</li>
        </ul>
        <p class="muted hint">Each view counts when the recipient clicks Reveal. You can copy or revoke the link later under Shared links.</p>
        <div class="row actions">
          <button class="primary" @click="closeShareLinkDialog">Done</button>
        </div>
      </div>

      <form v-else novalidate @submit.prevent="submitShareLink">
        <template v-if="isTextMode">
          <label>
            <span>Title</span>
            <input v-model="textTitle" name="shareTextTitle" placeholder="Shared text" maxlength="200" />
          </label>
          <label>
            <span>Text</span>
            <textarea v-model="text" name="shareText" rows="5" class="monospace" data-testid="share-text"></textarea>
          </label>
        </template>
        <fieldset v-else class="field-list">
          <legend>Fields to share</legend>
          <label v-for="field in dialog.allField" :key="field.key" class="check-row">
            <input v-model="allChosenFieldKey" type="checkbox" :value="field.key" :name="`shareField-${field.key}`" :data-testid="`share-field-${field.key}`" />
            <span class="check-label">{{ capitalizeFirstLetter(field.label) }}</span>
            <AppIcon v-if="field.isConcealed" name="lock" :size="12" class="muted" />
          </label>
          <p class="muted hint">SSH private keys, passkeys and one-time password secrets are never shared.</p>
        </fieldset>

        <div class="option-grid">
          <label>
            <span>Expires after</span>
            <select v-model.number="expiresInSeconds" name="shareExpiry" data-testid="share-expiry">
              <option v-for="expiry in allShareLinkExpiry" :key="expiry.seconds" :value="expiry.seconds">{{ expiry.label }}</option>
            </select>
          </label>
          <label>
            <span>Views</span>
            <input
              v-model.number="viewCount"
              type="number"
              name="shareViews"
              min="1"
              :max="maximumViewCount"
              :disabled="isUnlimitedViews"
              data-testid="share-views"
            />
          </label>
        </div>
        <label class="check-row">
          <input v-model="isUnlimitedViews" type="checkbox" name="shareUnlimitedViews" data-testid="share-unlimited-views" />
          <span class="check-label">Unlimited views until it expires</span>
        </label>

        <label class="check-row">
          <input v-model="isPassphraseRequired" type="checkbox" name="sharePassphraseRequired" data-testid="share-passphrase-required" />
          <span class="check-label">Require a passphrase</span>
        </label>
        <label v-if="isPassphraseRequired">
          <span>Passphrase</span>
          <input v-model="passphrase" type="password" name="sharePassphrase" autocomplete="new-password" data-testid="share-passphrase" />
          <small class="muted">At least {{ minimumPassphraseLength }} characters. Send it another way than the link, so a leaked link alone isn't enough.</small>
        </label>

        <p v-if="validationMessage" class="error" role="alert" data-testid="share-link-validation">{{ validationMessage }}</p>

        <div class="row actions">
          <button type="button" class="ghost" @click="closeShareLinkDialog">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy" data-testid="create-share-link"><AppIcon name="link" /> Create link</button>
        </div>
      </form>
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
  background: var(--color-backdrop);
}

.panel {
  width: min(500px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

.panel-header {
  display: flex;
  align-items: flex-start;
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
  margin: 3px 0 0;
  font-size: 12px;
}

.field-list {
  margin: 0 0 14px;
  padding: 10px 12px 4px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

legend {
  padding: 0 4px;
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text-secondary);
}

.check-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  cursor: pointer;
}

.check-row .check-label {
  margin: 0;
  font-size: 13px;
  color: var(--color-text);
}

.option-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

select,
input[type="number"] {
  width: 100%;
}

small {
  display: block;
  margin-top: 6px;
  font-size: 11px;
}

.hint {
  margin: 0 0 8px;
  font-size: 12px;
}

.link-input {
  flex: 1;
  min-width: 0;
  font-size: 12px;
}

.summary {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 0 14px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  list-style: none;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.summary li {
  display: flex;
  align-items: center;
  gap: 8px;
}

.actions {
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
