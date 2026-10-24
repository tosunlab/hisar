<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api, type ExportOptions } from "../api";
import { exportAllData, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import AppIcon from "./AppIcon.vue";

type ExportFormat = "encrypted" | "plain";

const isRowVisible = useSettingsRowVisibility();
const exportOptions = ref<ExportOptions>({ isBiometricAvailable: false, minPassphraseLength: 10 });
const format = ref<ExportFormat>("encrypted");
const passphrase = ref("");
const repeatedPassphrase = ref("");
const masterPassword = ref("");
const resultMessage = ref("");

const passphraseProblem = computed(() => {
  if (format.value !== "encrypted") {
    return "";
  }
  if ([...passphrase.value].length < exportOptions.value.minPassphraseLength) {
    return passphrase.value ? `Use at least ${exportOptions.value.minPassphraseLength} characters.` : "";
  }
  if (repeatedPassphrase.value && repeatedPassphrase.value !== passphrase.value) {
    return "The passphrases don't match.";
  }
  return "";
});

const canExport = computed(() => {
  if (store.isBusy) {
    return false;
  }
  if (!exportOptions.value.isBiometricAvailable && masterPassword.value === "") {
    return false;
  }
  if (format.value === "plain") {
    return true;
  }
  return passphraseProblem.value === "" && [...passphrase.value].length >= exportOptions.value.minPassphraseLength && repeatedPassphrase.value === passphrase.value;
});

async function submitExport(): Promise<void> {
  if (!canExport.value) {
    return;
  }
  resultMessage.value = "";
  const result = await exportAllData({
    format: format.value,
    passphrase: format.value === "encrypted" ? passphrase.value : "",
    password: exportOptions.value.isBiometricAvailable ? "" : masterPassword.value,
  });
  masterPassword.value = "";
  if (!result) {
    return;
  }
  passphrase.value = "";
  repeatedPassphrase.value = "";
  const itemText = result.itemCount === 1 ? "1 item" : `${result.itemCount} items`;
  const vaultText = result.vaultCount === 1 ? "1 vault" : `${result.vaultCount} vaults`;
  resultMessage.value = `${result.warning ? result.warning + " " : ""}Exported ${itemText} from ${vaultText} to ${result.path}.`;
}

onMounted(async () => {
  try {
    exportOptions.value = await api.getExportOptions();
  } catch {
    return;
  }
});
</script>

<template>
  <section v-show="isRowVisible('export')" class="settings-block" data-testid="export-section">
    <p class="section-label">Export your data</p>
    <p class="muted intro">Export every vault you can open, including shared vaults: logins, notes, one-time code secrets, SSH keys and other fields. Export works any time.</p>
    <p class="muted intro">Offline exports are cached copies. Changes since completed syncs may be absent. Unreadable records and vaults without a completed sync are reported as omissions in the result and file.</p>
    <form class="settings-group" @submit.prevent="submitExport">
      <label class="setting choice">
        <input v-model="format" type="radio" name="exportFormat" value="encrypted" data-testid="export-format-encrypted" />
        <span class="setting-text">
          <strong>Encrypted export (recommended)</strong>
          <p class="muted">Protected by an export passphrase you choose. Hisar can open it again with <code>hisar import</code>.</p>
        </span>
      </label>
      <div v-if="format === 'encrypted'" class="settings-group-detail passphrase-fields">
        <input
          v-model="passphrase"
          name="exportPassphrase"
          type="password"
          autocomplete="new-password"
          aria-label="Export passphrase"
          :placeholder="`Export passphrase (at least ${exportOptions.minPassphraseLength} characters)`"
          data-testid="export-passphrase"
        />
        <input
          v-model="repeatedPassphrase"
          name="repeatedExportPassphrase"
          type="password"
          autocomplete="new-password"
          aria-label="Repeat export passphrase"
          placeholder="Repeat export passphrase"
          data-testid="export-passphrase-repeat"
        />
        <p v-if="passphraseProblem" class="problem" data-testid="export-passphrase-problem">{{ passphraseProblem }}</p>
        <p class="muted">Keep the passphrase somewhere safe. Hisar can't recover it, and without it the export can't be opened.</p>
      </div>
      <label class="setting choice">
        <input v-model="format" type="radio" name="exportFormat" value="plain" data-testid="export-format-plain" />
        <span class="setting-text">
          <strong>Plain JSON</strong>
          <p class="muted">Readable by other tools and scripts.</p>
        </span>
      </label>
      <div v-if="format === 'plain'" class="settings-group-detail">
        <div class="warning-box" role="alert" data-testid="export-plain-warning">
          <AppIcon name="warning" :size="15" />
          <span>Plain JSON is not encrypted. Anyone who gets the file can read every password, one-time code secret and SSH private key in it. Delete it when you are done.</span>
        </div>
      </div>
      <div class="settings-group-detail export-actions">
        <input
          v-if="!exportOptions.isBiometricAvailable"
          v-model="masterPassword"
          name="exportMasterPassword"
          type="password"
          autocomplete="current-password"
          aria-label="Master password"
          placeholder="Master password to export"
          data-testid="export-master-password"
        />
        <button class="primary" type="submit" :disabled="!canExport" data-testid="export-submit">
          {{ exportOptions.isBiometricAvailable ? "Export with Touch ID…" : "Export…" }}
        </button>
      </div>
    </form>
    <p v-if="resultMessage" class="result" role="status" data-testid="export-result">
      <AppIcon name="check" :size="14" /><span>{{ resultMessage }}</span>
    </p>
  </section>
</template>

<style scoped>
.intro {
  margin: 0 2px 10px;
  font-size: 12px;
  line-height: 1.45;
}

.choice {
  align-items: flex-start;
  cursor: pointer;
}

.choice input[type="radio"] {
  flex: none;
  width: auto;
  margin: 3px 0 0;
}

.passphrase-fields {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.passphrase-fields p {
  margin: 0;
}

.problem {
  color: var(--color-danger-text);
  font-size: 12px;
}

.warning-box {
  display: flex;
  gap: 8px;
  padding: 9px 12px;
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  background: var(--color-warning-background);
  color: var(--color-warning-text);
  font-size: 12px;
  line-height: 1.45;
}

.warning-box svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-warning);
}

.export-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 10px;
  border-bottom: none;
}

.export-actions input {
  flex: 1;
}

.export-actions button {
  flex: none;
  white-space: nowrap;
}

.result {
  display: flex;
  gap: 8px;
  margin: 10px 2px 0;
  font-size: 12px;
  color: var(--color-text-secondary);
  user-select: text;
  -webkit-user-select: text;
}

.result svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-success);
}
</style>
