<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { changeImportVault, chooseImportFile, closeImport, continueImport, deleteImportFile, getAllEditableVault, mapImportGroup, openImportFile, startImport, store } from "../store";
import AppIcon from "./AppIcon.vue";
import { getThisDeviceName } from "../util";

const allEditableVault = computed(() => getAllEditableVault());

function getInitialVaultID(): string {
  const selectedVault = allEditableVault.value.find((vault) => vault.id === store.selectedVaultID);
  return (allEditableVault.value.find((vault) => vault.type === "personal") ?? selectedVault)?.id ?? "";
}

const vaultID = ref(getInitialVaultID());
const format = ref("");
const isSkippingDuplicate = ref(true);
const passphrase = ref("");

const allCount = computed(() => {
  const preview = store.importPreview;
  if (!preview) {
    return [];
  }
  return [
    { label: preview.loginCount === 1 ? "login" : "logins", count: preview.loginCount, testID: "login" },
    { label: preview.secureNoteCount === 1 ? "secure note" : "secure notes", count: preview.secureNoteCount, testID: "secure-note" },
    { label: preview.sshKeyCount === 1 ? "SSH key" : "SSH keys", count: preview.sshKeyCount, testID: "ssh-key" },
    { label: preview.creditCardCount === 1 ? "credit card" : "credit cards", count: preview.creditCardCount, testID: "credit-card" },
    { label: preview.identityCount === 1 ? "identity" : "identities", count: preview.identityCount, testID: "identity" },
    { label: preview.apiCredentialCount === 1 ? "API credential" : "API credentials", count: preview.apiCredentialCount, testID: "api-credential" },
    { label: preview.addressCount === 1 ? "address" : "addresses", count: preview.addressCount, testID: "address" },
    { label: preview.softwareLicenseCount === 1 ? "software license" : "software licenses", count: preview.softwareLicenseCount, testID: "software-license" },
    { label: preview.serverCount === 1 ? "server" : "servers", count: preview.serverCount, testID: "server" },
    { label: preview.databaseCount === 1 ? "database" : "databases", count: preview.databaseCount, testID: "database" },
  ].filter((entry) => entry.count > 0);
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
// exportedLine names the account and date a Hisar export came from; it omits the part that is missing.
const exportedLine = computed(() => {
  const preview = store.importPreview;
  if (!preview || (!preview.exportedFrom && !preview.exportedAt)) {
    return "";
  }
  const prefix = preview.exportedFrom ? `Exported from ${preview.exportedFrom}` : "Exported";
  const date = preview.exportedAt ? new Date(preview.exportedAt).toLocaleDateString(undefined, { dateStyle: "medium" }) : "";
  return date ? `${prefix} on ${date}` : prefix;
});

function formatCount(count: number): string {
  return `${count} ${count === 1 ? "item" : "items"}`;
}

async function submitChooseFile(): Promise<void> {
  if (await chooseImportFile(vaultID.value, format.value)) {
    isSkippingDuplicate.value = true;
  }
}

// The passphrase field is emptied after every try, so it never stays in the page.
async function submitPassphrase(): Promise<void> {
  const entered = passphrase.value;
  passphrase.value = "";
  await openImportFile(entered);
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
          <h2 id="import-title">Import items</h2>
          <p class="muted">From a Hisar export, 1Password, Bitwarden, LastPass, or a CSV file from your browser</p>
        </div>
        <button class="ghost icon-button" aria-label="Close" :disabled="store.isBusy" @click="closeImport"><AppIcon name="close" /></button>
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
            <div><dt>Hisar</dt><dd>Settings → Export your data on any device. Import the .hisar or .json file here; an encrypted export asks for its passphrase.</dd></div>
            <div><dt>1Password</dt><dd>File → Export → choose the account, then the 1PUX format. 1Password desktop .1pux exports do not include passkeys. Keep 1Password until you have moved your passkeys separately.</dd></div>
            <div><dt>Bitwarden</dt><dd>Tools → Export vault → File format “.json”, not the encrypted one.</dd></div>
            <div><dt>LastPass</dt><dd>Account options → Advanced → Export, then save the file as CSV.</dd></div>
            <div><dt>Chrome, Edge</dt><dd>Settings → Passwords → Export passwords.</dd></div>
            <div><dt>Firefox</dt><dd>Passwords (about:logins) → ⋯ → Export passwords.</dd></div>
            <div><dt>Safari</dt><dd>File → Export → Passwords.</dd></div>
          </dl>
        </details>
        <p class="muted hint">Hisar reads the file on {{ getThisDeviceName() }} and shows what it found before saving anything.</p>
        <div class="row actions">
          <button type="button" class="ghost" @click="closeImport">Cancel</button>
          <button class="primary" type="submit" data-testid="choose-import-file" :disabled="store.isBusy || !vaultID">
            <AppIcon name="file" :size="14" /> Choose file…
          </button>
        </div>
      </form>

      <form v-else-if="store.importStep === 'passphrase' && store.importPreview" data-testid="import-passphrase" @submit.prevent="submitPassphrase">
        <div class="file-row">
          <AppIcon name="file" class="file-icon" />
          <div class="file-text">
            <strong data-testid="import-file-name">{{ store.importPreview.fileName }}</strong>
            <span class="muted">{{ store.importPreview.formatName }}</span>
          </div>
        </div>
        <h3>Enter the export passphrase</h3>
        <p class="muted">This export is encrypted. Enter the passphrase it was exported with.</p>
        <label>
          <span>Export passphrase</span>
          <input v-model="passphrase" type="password" name="exportPassphrase" autocomplete="off" placeholder="Export passphrase" data-testid="import-export-passphrase" autofocus />
        </label>
        <div class="row actions">
          <button type="button" class="ghost" :disabled="store.isBusy" @click="closeImport">Cancel</button>
          <button class="primary" type="submit" data-testid="open-import-file" :disabled="store.isBusy || !passphrase">Open</button>
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
        <p v-if="store.importPreview.notice" class="warning" data-testid="import-notice">{{ store.importPreview.notice }}</p>
        <p v-if="exportedLine" class="muted hint" data-testid="import-exported-from">{{ exportedLine }}</p>
        <label v-if="!store.importPreview.groups?.length">
          <span>Import into</span>
          <select :value="store.importPreview.vaultId" name="importVault" data-testid="import-preview-vault" :disabled="store.isBusy" @change="changeVault">
            <option v-for="vault in allEditableVault" :key="vault.id" :value="vault.id">{{ vault.name }}</option>
          </select>
        </label>
        <section v-for="group in store.importPreview.groups ?? []" :key="group.id" data-testid="import-source-group">
          <label>
            <span>{{ group.name }} · {{ group.itemCount }} items · {{ group.isPrivate ? "Private source" : "Shared or unspecified source" }}</span>
            <select :value="group.vaultId" :disabled="store.isBusy" @change="mapImportGroup(group.id, ($event.target as HTMLSelectElement).value)">
              <option v-for="vault in allEditableVault" :key="vault.id" :value="vault.id">{{ vault.name }} · {{ vault.type }} · {{ vault.id }}</option>
            </select>
          </label>
          <p class="muted hint">Source ID {{ group.id }}</p>
          <p v-if="allEditableVault.find(vault => vault.id === group.vaultId)?.type === 'personal'" class="muted hint">Only you can access Personal.</p>
          <template v-else>
            <p class="warning">Shared destination. Imported items are readable by its current members. Access can change later.</p>
            <ul>
              <li v-for="member in group.members ?? []" :key="member.accountId">{{ member.name || member.accountId }} &lt;{{ member.email }}&gt; · {{ member.role }}{{ member.isCurrentUser ? " (you)" : "" }}</li>
            </ul>
          </template>
        </section>
        <h3 data-testid="import-headline">{{ formatCount(importCount) }} to import into {{ store.importPreview.vaultName }}</h3>
        <p v-if="allCount.length > 0" class="category-counts muted" data-testid="import-category-counts">
          Before skipping duplicates: {{ formatCount(store.importPreview.itemCount) }}.
          <template v-for="(countEntry, index) in allCount" :key="countEntry.testID">
            <template v-if="index > 0"> · </template>
            <span :data-testid="`import-count-${countEntry.testID}`">{{ countEntry.count }} {{ countEntry.label }}</span>
          </template>
        </p>
        <p v-if="store.importPreview.format === '1pux'" class="muted hint" data-testid="import-passkey-warning">
          1Password desktop .1pux exports do not include passkeys. Keep 1Password until you have moved your passkeys separately.
        </p>

        <section v-if="store.importPreview.allDuplicate.length > 0" data-testid="import-duplicates">
          <p class="section-label">Already in {{ store.importPreview.vaultName }} · {{ store.importPreview.allDuplicate.length }}</p>
          <ul class="entry-list">
            <li v-for="(duplicate, index) in store.importPreview.allDuplicate" :key="index">
              <strong>{{ duplicate.title }}</strong>
              <span class="muted">{{ duplicate.detail }}</span>
            </li>
          </ul>
          <label class="checkbox-row">
            <input v-model="isSkippingDuplicate" type="checkbox" name="skipDuplicates" data-testid="skip-duplicates" />
            <span>Skip items that are already in the vault</span>
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
        <p class="muted hint">Each item is encrypted on {{ getThisDeviceName() }} before it is saved. Keep Hisar open until the import finishes.</p>
        <div v-if="store.errorMessage && !store.isBusy" class="row actions">
          <button class="primary" data-testid="continue-import" @click="continueImport">Try again</button>
          <button class="ghost" @click="closeImport">Stop and keep the source file</button>
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
        <div v-if="!store.isImportFileDeleted && !store.importProgress.isSourceKept && store.importProgress.leftBehindCount === 0" class="confirm" data-testid="delete-import-file-offer">
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
          <p v-if="store.isImportFileDeleted" class="muted hint" data-testid="import-file-deleted">Deleted {{ store.importProgress.fileName }}.</p>
          <p v-else-if="store.importProgress.isSourceKept" class="muted hint" data-testid="import-source-kept">Hisar keeps this file: it is encrypted, or a copy that may not hold everything.</p>
          <p v-else class="muted hint">Keep the source file. Review the entries that were not imported before deleting it yourself.</p>
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
  font-size: var(--font-size-15);
  font-weight: 600;
}

.panel-heading p {
  margin: 1px 0 0;
  font-size: var(--font-size-12);
}

select {
  width: 100%;
}

.how-to {
  margin-bottom: 12px;
  font-size: var(--font-size-12);
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
  font-size: var(--font-size-12);
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
  font-size: var(--font-size-12);
}

.category-counts {
  margin: 4px 0 16px;
}

.category-counts span {
  white-space: nowrap;
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
  flex-direction: column;
  gap: 3px;
  padding: 7px 12px;
  border-bottom: 1px solid var(--color-border);
  font-size: var(--font-size-12);
}

.entry-list li:last-child {
  border-bottom: none;
}

.entry-list strong {
  min-width: 0;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.entry-list span {
  min-width: 0;
  white-space: normal;
  line-height: normal;
  overflow-wrap: anywhere;
}

.checkbox-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 10px 0 0;
  font-size: var(--font-size-12);
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
  font-size: var(--font-size-12);
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
  font-size: var(--font-size-11);
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
