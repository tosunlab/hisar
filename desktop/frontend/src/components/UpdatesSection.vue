<script setup lang="ts">
import { computed, onMounted } from "vue";
import { checkForUpdates, loadUpdateStatus, setAutomaticUpdateCheck, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";

const isRowVisible = useSettingsRowVisibility();

const updateStatus = computed(() => store.updateStatus);
const canCheck = computed(() => updateStatus.value?.isAvailable === true && updateStatus.value.isSignedIn && !store.isBusy);

const versionLabel = computed(() => {
  const status = updateStatus.value;
  if (!status) {
    return "Hisar";
  }
  if (status.version === "development") {
    return "Hisar development build";
  }
  return `Hisar ${status.version}`;
});

const explanation = computed(() => {
  const status = updateStatus.value;
  if (!status) {
    return "Checking the installed version…";
  }
  if (!status.isAvailable) {
    return "This build cannot update itself. Install Hisar from a release to get updates.";
  }
  if (!status.isSignedIn) {
    return "Sign in to check for updates; they come from the Hisar server.";
  }
  return `Updates come from ${store.status.server || "the Hisar server"} and are verified before they install.`;
});

onMounted(loadUpdateStatus);
</script>

<template>
  <section v-show="isRowVisible('version') || isRowVisible('automatic-updates')" class="settings-block" data-testid="updates-section">
    <p class="section-label">Updates</p>
    <div class="settings-group">
      <div v-show="isRowVisible('version')" class="setting">
        <div class="setting-text">
          <strong data-testid="installed-version">{{ versionLabel }}</strong>
          <p class="muted" data-testid="update-explanation">{{ explanation }}</p>
        </div>
        <button data-testid="check-for-updates" :disabled="!canCheck" @click="checkForUpdates">Check for updates</button>
      </div>
      <label v-show="isRowVisible('automatic-updates')" class="setting">
        <div class="setting-text">
          <strong>Automatically check for updates</strong>
          <p class="muted">Once a day while you are signed in.</p>
        </div>
        <input
          class="switch"
          type="checkbox"
          name="automaticUpdateCheck"
          :checked="updateStatus?.isAutomaticCheckEnabled ?? false"
          :disabled="!updateStatus?.isAvailable || store.isBusy"
          @change="setAutomaticUpdateCheck(($event.target as HTMLInputElement).checked)"
        />
      </label>
    </div>
  </section>
</template>
