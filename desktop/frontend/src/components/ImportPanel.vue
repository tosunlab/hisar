<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { changeImportVault, chooseImportFile, closeImport, continueImport, deleteImportFile, getAllEditableVault, startImport, store } from "../store";
import { getDomain } from "../util";
import AppIcon from "./AppIcon.vue";

const allEditableVault = computed(() => getAllEditableVault());

function getInitialVaultID(): string {
  const selectedVault = allEditableVault.value.find((vault) => vault.id === store.selectedVaultID);
  return (selectedVault ?? allEditableVault.value[0])?.id ?? "";
}

const vaultID = ref(getInitialVaultID());
const format = ref("");
const isSkippingDuplicate = ref(true);

const allCount = computed(() => {
  const preview = store.importPreview;
  if (!preview) {
    return [];
  }
  return [
    { label: preview.loginCount === 1 ? "Login" : "Logins", count: preview.loginCount, icon: "globe" as const, testID: "login" },
    { label: preview.secureNoteCount === 1 ? "Secure note" : "Secure notes", count: preview.secureNoteCount, icon: "note" as const, testID: "secure-note" },
    { label: preview.sshKeyCount === 1 ? "SSH key" : "SSH keys", count: preview.sshKeyCount, icon: "key" as const, testID: "ssh-key" },
    { label: preview.creditCardCount === 1 ? "Credit card" : "Credit cards", count: preview.creditCardCount, icon: "card" as const, testID: "credit-card" },
    { label: preview.identityCount === 1 ? "Identity" : "Identities", count: preview.identityCount, icon: "identity" as const, testID: "identity" },
  ];
});
const importCount = computed(() => {
  const preview = store.importPreview;
  if (!preview) {
    return 0;
  }
  return preview.itemCount - (isSkippingDuplicate.value ? preview.allDuplicate.length : 0);
});
const progressPercent = computed(() => {
  const progress = store.importProgress;
  if (!progress || progress.totalCount === 0) {
    return 0;
  }
  return Math.round(((progress.savedCount + progress.allFailed.length) / progress.totalCount) * 100);
});

function formatCount(count: number): string {
  return `${count} ${count === 1 ? "item" : "items"}`;
}

async function submitChooseFile(): Promise<void> {
  if (await chooseImportFile(vaultID.value, format.value)) {
    isSkippingDuplicate.value = true;
  }
}

async function changeVault(event: Event): Promise<void> {
  await changeImportVault((event.target as HTMLSelectElement).value);
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeImport();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeImport">
    <div class="card panel" role="dialog" aria-modal="true" aria-labelledby="import-title" data-testid="import-panel">
      <header class="panel-header">
        <span class="panel-icon"><AppIcon name="import" :size="18" /></span>
        <div class="panel-heading">
          <h2 id="import-title">Import passwords</h2>
          <p class="muted">From 1Password, Bitwarden, or a CSV file from your browser</p>
        </div>
        <button class="ghost icon-button" aria-label="Close" :disabled="store.importStep === 'saving'" @click="closeImport"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <form v-if="store.importStep === 'choose'" data-testid="import-choose" @submit.prevent="submitChooseFile">
        <label>
          <span>Import into</span>
          <select v-model="vaultID" name="importVault" data-testid="import-vault">
            <option v-for="vault in allEditableVault" :key="vault.id" :value="vault.id">{{ vault.name }}</option>
          </select>
        </label>
        <label>
          <span>Format</span>
          <select v-model="format" name="importFormat" data-testid="import-format">
            <option value="">Detect from the file</option>
            <option v-for="importFormat in store.allImportFormat" :key="importFormat.id" :value="importFormat.id">{{ importFormat.name }}</option>
          </select>
        </label>
        <details class="how-to">
          <summary>How to export from your password manager</summary>
          <dl>
            <div><dt>1Password</dt><dd>File → Export → choose the account, then the 1PUX format.</dd></div>
            <div><dt>Bitwarden</dt><dd>Tools → Export vault → File format “.json”, not the encrypted one.</dd></div>
            <div><dt>Chrome, Edge</dt><dd>Settings → Passwords → Export passwords.</dd></div>
            <div><dt>Firefox</dt><dd>Passwords (about:logins) → ⋯ → Export passwords.</dd></div>
            <div><dt>Safari</dt><dd>File → Export → Passwords.</dd></div>
          </dl>
        </details>
        <p class="muted hint">Hisar reads the file on this Mac and shows what it found before saving anything.</p>
        <div class="row actions">
          <button type="button" class="ghost" @click="closeImport">Cancel</button>
          <button class="primary" type="submit" data-testid="choose-import-file" :disabled="store.isBusy || !vaultID">
            <AppIcon name="file" :size="14" /> Choose file…
          </button>
        </div>
      </form>

      <div v-else-if="store.importStep === 'preview' && store.importPreview" data-testid="import-preview">
        <div class="file-row">
          <AppIcon name="file" class="file-icon" />
          <div class="file-text">
            <strong data-testid="import-file-name">{{ store.importPreview.fileName }}</strong>
            <span class="muted">{{ store.importPreview.formatName }}</span>
          </div>
        </div>
        <label>
          <span>Import into</span>
          <select :value="store.importPreview.vaultId" name="importVault" data-testid="import-preview-vault" :disabled="store.isBusy" @change="changeVault">
            <option v-for="vault in allEditableVault" :key="vault.id" :value="vault.id">{{ vault.name }}</option>
          </select>
        </label>
        <div class="count-grid">
          <div v-for="countEntry in allCount" :key="countEntry.testID" class="count" :data-testid="`import-count-${countEntry.testID}`">
            <AppIcon :name="countEntry.icon" class="count-icon" />
            <strong>{{ countEntry.count }}</strong>
            <span class="muted">{{ countEntry.label }}</span>
          </div>
        </div>

        <section v-if="store.importPreview.allDuplicate.length > 0" data-testid="import-duplicates">
          <p class="section-label">Already in {{ store.importPreview.vaultName }} · {{ store.importPreview.allDuplicate.length }}</p>
          <ul class="entry-list">
            <li v-for="(duplicate, index) in store.importPreview.allDuplicate" :key="index">
              <strong>{{ duplicate.title }}</strong>
              <span class="muted">{{ [duplicate.username, duplicate.url ? getDomain(duplicate.url) : ""].filter(Boolean).join(" · ") }}</span>
            </li>
          </ul>
          <label class="checkbox-row">
            <input v-model="isSkippingDuplicate" type="checkbox" name="skipDuplicates" data-testid="skip-duplicates" />
            <span>Skip items with the same title, username and website</span>
          </label>
        </section>

        <section v-if="store.importPreview.allSkipped.length > 0" data-testid="import-skipped">
          <p class="section-label">Not imported · {{ store.importPreview.allSkipped.length }}</p>
          <ul class="entry-list">
            <li v-for="(skipped, index) in store.importPreview.allSkipped" :key="index">
              <strong>{{ skipped.title }}</strong>
              <span class="muted">{{ skipped.reason }}</span>
            </li>
          </ul>
        </section>

        <div class="row actions sticky-actions">
          <button class="ghost" :disabled="store.isBusy" @click="closeImport">Cancel</button>
          <button class="primary" data-testid="start-import" :disabled="store.isBusy || importCount === 0" @click="startImport(isSkippingDuplicate)">
            Import {{ formatCount(importCount) }}
          </button>
        </div>
      </div>

      <div v-else-if="store.importStep === 'saving'" data-testid="import-saving">
        <p class="progress-text">
          <template v-if="store.importProgress">Saving {{ store.importProgress.savedCount }} of {{ store.importProgress.totalCount }} items to {{ store.importProgress.vaultName }}…</template>
          <template v-else>Preparing the import…</template>
        </p>
        <div class="progress" role="progressbar" :aria-valuenow="progressPercent" aria-valuemin="0" aria-valuemax="100">
          <div class="progress-bar" :style="{ width: `${progressPercent}%` }"></div>
        </div>
        <p class="muted hint">Each item is encrypted on this Mac before it is saved. Keep Hisar open until the import finishes.</p>
        <div v-if="store.errorMessage && !store.isBusy" class="row actions">
          <button class="primary" data-testid="continue-import" @click="continueImport">Try again</button>
        </div>
      </div>

      <div v-else-if="store.importStep === 'done' && store.importProgress" data-testid="import-done">
        <div class="state-box success">
          <AppIcon name="check" class="state-icon" />
          <div class="state-text">
            <strong data-testid="import-result">Imported {{ formatCount(store.importProgress.savedCount) }} into {{ store.importProgress.vaultName }}</strong>
          </div>
        </div>
        <section v-if="store.importProgress.allFailed.length > 0" data-testid="import-failed">
          <p class="section-label">Could not be saved · {{ store.importProgress.allFailed.length }}</p>
          <ul class="entry-list">
            <li v-for="(failed, index) in store.importProgress.allFailed" :key="index">
              <strong>{{ failed.title }}</strong>
              <span class="muted">{{ failed.reason }}</span>
            </li>
          </ul>
        </section>
        <div v-if="!store.isImportFileDeleted" class="confirm" data-testid="delete-import-file-offer">
          <AppIcon name="warning" class="confirm-icon" />
          <div class="confirm-body">
            <p>
              <strong>{{ store.importProgress.fileName }} holds all these passwords in plain text.</strong>
              Now that they are in Hisar, delete it, and remove any other copies of the export.
            </p>
            <p class="monospace file-path">{{ store.importProgress.filePath }}</p>
            <div class="row confirm-actions">
              <button class="ghost" :disabled="store.isBusy" @click="closeImport">Keep file</button>
              <button class="danger-solid" data-testid="delete-import-file" :disabled="store.isBusy" @click="deleteImportFile">Delete file</button>
            </div>
          </div>
        </div>
        <template v-else>
          <p class="muted hint" data-testid="import-file-deleted">Deleted {{ store.importProgress.fileName }}.</p>
          <div class="row actions">
            <button class="primary" @click="closeImport">Done</button>
          </div>
        </template>
      </div>
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
  width: min(540px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

.panel-header {
  display: flex;
  align-items: center;
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
  font-size: 17px;
  font-weight: 650;
}

.panel-heading p {
  margin: 1px 0 0;
  font-size: 12px;
}

select {
  width: 100%;
}

.how-to {
  margin-bottom: 12px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.how-to summary {
  cursor: pointer;
}

.how-to dl {
  margin: 8px 0 0;
}

.how-to dl div {
  display: flex;
  gap: 10px;
  padding: 3px 0;
}

.how-to dt {
  width: 92px;
  flex: none;
  font-weight: 600;
  color: var(--color-text);
}

.how-to dd {
  margin: 0;
}

.hint {
  margin: 0 0 4px;
  font-size: 12px;
}

.actions {
  justify-content: flex-end;
  margin-top: 16px;
}

.sticky-actions {
  position: sticky;
  bottom: -20px;
  margin: 16px -22px -20px;
  padding: 12px 22px 20px;
  border-top: 1px solid var(--color-border);
  background: var(--color-panel);
}

.file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.file-icon {
  color: var(--color-text-secondary);
}

.file-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.file-text strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-text span {
  font-size: 12px;
}

.count-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-bottom: 16px;
}

.count {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.count strong {
  font-size: 20px;
  font-weight: 650;
}

.count span {
  font-size: 12px;
}

.count-icon {
  color: var(--color-text-muted);
}

section {
  margin-bottom: 14px;
}

.entry-list {
  max-height: 150px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  list-style: none;
}

.entry-list li {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 7px 12px;
  border-bottom: 1px solid var(--color-border);
  font-size: 12px;
}

.entry-list li:last-child {
  border-bottom: none;
}

.entry-list strong {
  flex: none;
  max-width: 45%;
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.entry-list span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.checkbox-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 10px 0 0;
  font-size: 12px;
}

.checkbox-row span {
  margin: 0;
}

.progress-text {
  margin: 0 0 10px;
}

.progress {
  height: 6px;
  margin-bottom: 12px;
  overflow: hidden;
  border-radius: 3px;
  background: var(--color-surface-raised);
}

.progress-bar {
  height: 100%;
  background: var(--color-primary);
}

.state-box {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.state-icon {
  flex: none;
  color: var(--color-success);
}

.state-text {
  flex: 1;
  min-width: 0;
}

.confirm {
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  background: var(--color-warning-background);
  font-size: 12px;
}

.confirm-icon {
  flex: none;
  margin-top: 1px;
  color: var(--color-warning);
}

.confirm-body {
  flex: 1;
  min-width: 0;
}

.confirm-body p {
  margin: 0 0 6px;
  color: var(--color-warning-text);
}

.file-path {
  overflow: hidden;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
  -webkit-user-select: text;
}

.confirm-actions {
  justify-content: flex-end;
  margin-top: 10px;
}
</style>
