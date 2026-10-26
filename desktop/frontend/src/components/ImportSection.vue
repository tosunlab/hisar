<script setup lang="ts">
import { openFromSettings } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";

const isRowVisible = useSettingsRowVisibility();
</script>

<template>
  <section v-show="isRowVisible('import')" class="settings-block" data-testid="import-section">
    <p class="section-label">Import</p>
    <div class="settings-group">
      <div class="setting">
        <div class="setting-text">
          <strong>Import from another password manager</strong>
          <p class="muted">1Password (.1pux or CSV), Bitwarden (JSON or CSV), LastPass (CSV), or a CSV export from Chrome, Firefox, Safari or Edge.</p>
        </div>
        <button data-testid="open-import" @click="openFromSettings('import')">Import…</button>
      </div>
    </div>
  </section>
</template>
