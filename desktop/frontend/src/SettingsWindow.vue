<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "./api";
import { isSettingsSectionID, saveSelectedSectionID } from "./settingsSections";
import { loadStatus, reportActivity, store, syncInBackground } from "./store";
import { startThemeSync } from "./theme";
import AppIcon from "./components/AppIcon.vue";
import OrganizationAlert from "./components/OrganizationAlert.vue";
import SettingsPanel from "./components/SettingsPanel.vue";

// The macOS Settings window. Go shows it after asking it to open a section, and hides it when Hisar locks or signs out,
// from any window; its page reloads whenever it closes, so each opening loads the account afresh. Closing Settings here
// hides the window.
const openCount = ref(0);
let allRemoveListener: (() => void)[] = [];

async function open(sectionID: string): Promise<void> {
  await loadStatus();
  if (store.status.state !== "unlocked") {
    await api.hideSettingsWindow();
    return;
  }
  if (isSettingsSectionID(sectionID)) {
    saveSelectedSectionID(sectionID);
  }
  openCount.value++;
  store.isSettingsOpen = true;
}

watch(
  () => store.isSettingsOpen,
  (isOpen) => {
    if (!isOpen) {
      api.hideSettingsWindow();
    }
  },
);

onMounted(() => {
  allRemoveListener = [
    startThemeSync(),
    Events.On("open-settings-window", (event) => open(event.data ?? "")),
    Events.On("remote-change", () => syncInBackground()),
  ];
  window.addEventListener("keydown", reportActivity);
  window.addEventListener("pointerdown", reportActivity);
});

onBeforeUnmount(() => {
  allRemoveListener.forEach((removeListener) => removeListener());
  window.removeEventListener("keydown", reportActivity);
  window.removeEventListener("pointerdown", reportActivity);
});
</script>

<template>
  <SettingsPanel v-if="store.isSettingsOpen && store.status.state === 'unlocked'" :key="openCount" />
  <OrganizationAlert />
  <div v-if="store.noticeMessage" class="notice" role="status"><AppIcon name="check" /> {{ store.noticeMessage }}</div>
</template>
