<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "./api";
import { isSettingsSectionID, saveSelectedSectionID } from "./settingsSections";
import { loadSettingsWindow, lockOnShortcut, reportActivity, store } from "./store";
import { startThemeSync } from "./theme";
import { startWebsiteIconSync } from "./websiteicons";
import AppNotice from "./components/AppNotice.vue";
import OrganizationAlert from "./components/OrganizationAlert.vue";
import SettingsPanel from "./components/SettingsPanel.vue";

// The Settings window shows Settings only. Go keeps the section it was asked to open at until this page takes it,
// so a request made while the page reloads, or while Hisar is locked, still opens it; the event only wakes the page. Go
// hides the window when Hisar locks or signs out, and its page reloads whenever it closes. Items, removed-vault notices
// and pending recovery belong to the main window: actions that need them hand off to it. Closing Settings here hides the
// window.
const openCount = ref(0);
let allRemoveListener: (() => void)[] = [];

async function openRequestedSection(): Promise<void> {
  const sectionID = await api.takeSettingsWindowRequest();
  if (sectionID === null) {
    return;
  }
  await loadSettingsWindow();
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

function handleKeydown(event: KeyboardEvent): void {
  reportActivity();
  lockOnShortcut(event);
}

onMounted(() => {
  allRemoveListener = [
    startThemeSync(),
    startWebsiteIconSync(),
    Events.On("open-settings-window", () => openRequestedSection()),
    Events.On("remote-change", () => store.isSettingsOpen && loadSettingsWindow()),
  ];
  window.addEventListener("keydown", handleKeydown);
  window.addEventListener("pointerdown", reportActivity);
  openRequestedSection();
});

onBeforeUnmount(() => {
  allRemoveListener.forEach((removeListener) => removeListener());
  window.removeEventListener("keydown", handleKeydown);
  window.removeEventListener("pointerdown", reportActivity);
});
</script>

<template>
  <SettingsPanel v-if="store.isSettingsOpen && store.status.state === 'unlocked'" :key="openCount" />
  <OrganizationAlert />
  <AppNotice />
</template>
