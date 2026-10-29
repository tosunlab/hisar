<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api, type ExportOptions } from "../api";
import { exportAllData, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { exportWithBiometric, useBiometricInstead } from "../util";
import AppIcon from "./AppIcon.vue";
import CredentialField from "./CredentialField.vue";

type ExportFormat = "encrypted" | "plain";

const isRowVisible = useSettingsRowVisibility();
const exportOptions = ref<ExportOptions>({ isBiometricAvailable: false, minPassphraseLength: 10 });
const format = ref<ExportFormat>("encrypted");
const passphrase = ref("");
const repeatedPassphrase = ref("");
const masterPassword = ref("");
const resultMessage = ref("");
const authentication = ref<"biometric" | "password">("biometric");
const isPasswordNeeded = computed(() => authentication.value === "password" || !exportOptions.value.isBiometricAvailable);
const validationAnnouncement = ref("");
const isPassphraseTooShort = computed(() => format.value === "encrypted" && passphrase.value !== "" && [...passphrase.value].length < exportOptions.value.minPassphraseLength);
const isPassphraseMismatched = computed(() => format.value === "encrypted" && !isPassphraseTooShort.value && repeatedPassphrase.value !== "" && repeatedPassphrase.value !== passphrase.value);

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
  if (isPasswordNeeded.value && masterPassword.value === "") {
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
    password: isPasswordNeeded.value ? masterPassword.value : "",
  });
  masterPassword.value = "";
  if (!result) {
    try {
      exportOptions.value = await api.getExportOptions();
    } catch {
      authentication.value = "password";
    }
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
    <p class="muted intro">Export every vault you can open, including shared vaults: logins, notes, one-time code secrets, SSH keys and other fields.</p>
    <p class="muted intro">Offline exports are cached copies. Changes since completed syncs may be absent. Unreadable records and vaults without a completed sync are reported as omissions in the result and file.</p>
    <form class="settings-group" @submit.prevent="submitExport">
      <label class="setting choice">
        <input v-model="format" type="radio" name="exportFormat" value="encrypted" data-testid="export-format-encrypted" />
        <span class="setting-text">
          <strong>Encrypted export (recommended)</strong>
          <p class="muted">Protected by an export passphrase you choose. Hisar can import it again on any of your devices.</p>
        </span>
      </label>
      <div v-if="format === 'encrypted'" class="settings-group-detail passphrase-fields">
        <CredentialField secret-name="export passphrase">
        <input
          v-model="passphrase"
          name="exportPassphrase"
          type="password"
          autocomplete="new-password"
          aria-label="Export passphrase"
          :placeholder="`Export passphrase (at least ${exportOptions.minPassphraseLength} characters)`"
          data-testid="export-passphrase"
          :aria-invalid="isPassphraseTooShort"
          :aria-describedby="isPassphraseTooShort ? 'export-passphrase-problem' : undefined"
          @blur="validationAnnouncement = isPassphraseTooShort ? passphraseProblem : ''"
        />
        </CredentialField>
        <CredentialField secret-name="repeated export passphrase">
        <input
          v-model="repeatedPassphrase"
          name="repeatedExportPassphrase"
          type="password"
          autocomplete="new-password"
          aria-label="Repeat export passphrase"
          placeholder="Repeat export passphrase"
          data-testid="export-passphrase-repeat"
          :aria-invalid="isPassphraseMismatched"
          :aria-describedby="isPassphraseMismatched ? 'export-passphrase-problem' : undefined"
          @blur="validationAnnouncement = isPassphraseMismatched ? passphraseProblem : ''"
        />
        </CredentialField>
        <p v-if="passphraseProblem" id="export-passphrase-problem" class="problem" data-testid="export-passphrase-problem">{{ passphraseProblem }}</p>
        <p class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ validationAnnouncement }}</p>
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
        <CredentialField v-if="isPasswordNeeded">
        <input
          v-model="masterPassword"
          name="exportMasterPassword"
          type="password"
          autocomplete="current-password"
          aria-label="Master password"
          placeholder="Master password to export"
          data-testid="export-master-password"
        />
        </CredentialField>
        <button class="primary" type="submit" :disabled="!canExport" data-testid="export-submit">
          {{ isPasswordNeeded ? "Export…" : exportWithBiometric() }}
        </button>
        <button v-if="exportOptions.isBiometricAvailable" class="link" type="button" :disabled="store.isBusy" data-testid="export-switch-authentication" @click="authentication = isPasswordNeeded ? 'biometric' : 'password'">
          {{ isPasswordNeeded ? useBiometricInstead() : "or use your master password" }}
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
  font-size: var(--font-size-12);
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
  font-size: var(--font-size-12);
}

.warning-box {
  display: flex;
  gap: 8px;
  padding: 9px 12px;
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  background: var(--color-warning-background);
  color: var(--color-warning-text);
  font-size: var(--font-size-12);
  line-height: 1.45;
}

.warning-box svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-warning);
}

.export-actions {
  display: flex;
  flex-wrap: wrap;
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


.export-actions .link {
  flex-basis: 100%;
  text-align: right;
}
.result {
  display: flex;
  gap: 8px;
  margin: 10px 2px 0;
  font-size: var(--font-size-12);
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
