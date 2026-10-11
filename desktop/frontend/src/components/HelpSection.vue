<script setup lang="ts">
import { computed, onMounted } from "vue";
import { copyText, loadUpdateStatus, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { buildDiagnostics, formatAppVersion, supportEmailAddress } from "../support";
import AppIcon from "./AppIcon.vue";

const isRowVisible = useSettingsRowVisibility();
const currentDevice = computed(() => store.allDevice.find((device) => device.isCurrent) ?? null);
const diagnostics = computed(() => buildDiagnostics({ updateStatus: store.updateStatus, server: store.status.server, allDevice: store.allDevice }));

onMounted(() => {
  if (!store.updateStatus) {
    loadUpdateStatus();
  }
});
</script>

<template>
  <section data-testid="help-section">
    <div v-show="isRowVisible('support')" class="settings-block">
      <p class="section-label">Contact support</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong class="selectable" data-testid="support-email">{{ supportEmailAddress }}</strong>
            <p class="muted">Email us about a problem or an idea. Paste the diagnostics below into your email so we can help faster.</p>
          </div>
          <button data-testid="copy-support-email" @click="copyText(supportEmailAddress, 'Support address')"><AppIcon name="copy" :size="14" />Copy</button>
        </div>
      </div>
    </div>

    <div v-show="isRowVisible('diagnostics')" class="settings-block">
      <p class="section-label">Diagnostics</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong>Hisar version</strong>
          </div>
          <span class="selectable value" data-testid="help-app-version">{{ formatAppVersion(store.updateStatus) }}</span>
        </div>
        <div class="setting">
          <div class="setting-text">
            <strong>System</strong>
          </div>
          <span class="selectable value" data-testid="help-operating-system">{{ store.updateStatus?.operatingSystem || "unknown" }}</span>
        </div>
        <div class="setting">
          <div class="setting-text">
            <strong>Server</strong>
            <p class="muted">The server does not report its version.</p>
          </div>
          <span class="selectable value" data-testid="help-server">{{ store.status.server || "not signed in" }}</span>
        </div>
        <div class="setting">
          <div class="setting-text">
            <strong>Device ID</strong>
          </div>
          <span class="selectable value monospace" data-testid="help-device-id">{{ currentDevice?.id ?? "unknown" }}</span>
        </div>
        <div class="setting">
          <div class="setting-text">
            <strong>Copy diagnostics</strong>
            <p class="muted">Copies the lines above as text. It never includes passwords, keys, item names or other people's details.</p>
          </div>
          <button data-testid="copy-diagnostics" @click="copyText(diagnostics, 'Diagnostics')"><AppIcon name="copy" :size="14" />Copy diagnostics</button>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.selectable {
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
}

.value {
  color: var(--color-text-muted);
  font-size: 13px;
  text-align: right;
}
</style>
