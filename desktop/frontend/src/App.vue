<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "./api";
import { startThemeSync } from "./theme";
import {
  handleLocked,
  handleUnlockedElsewhere,
  loadAllShareLinkQuietly,
  loadStatus,
  openInviteLink,
  openSettings,
  restoreBrowserPairing,
  showBrowserPairing,
  showUpdateRequired,
  showItem,
  store,
  syncInBackground,
  takePendingInviteQuietly,
} from "./store";
import UpdateRequiredView from "./components/UpdateRequiredView.vue";
import SignInView from "./components/SignInView.vue";
import SecretKeyView from "./components/SecretKeyView.vue";
import TouchIDOfferView from "./components/TouchIDOfferView.vue";
import UnlockView from "./components/UnlockView.vue";
import MainView from "./components/MainView.vue";
import AppIcon from "./components/AppIcon.vue";
import BrowserPairingModal from "./components/BrowserPairingModal.vue";

const activityReportInterval = 30_000;
let lastActivityReport = 0;
let allRemoveListener: (() => void)[] = [];

function reportActivity(): void {
  const now = Date.now();
  if (store.status.state !== "unlocked" || now - lastActivityReport < activityReportInterval) {
    return;
  }
  lastActivityReport = now;
  api.reportActivity().catch(() => undefined);
}

onMounted(() => {
  loadStatus().then(takePendingInviteQuietly);
  allRemoveListener = [
    Events.On("update-required", (event) => event.data && showUpdateRequired(event.data)),
    Events.On("invite-link", (event) => openInviteLink(event.data ?? "")),
    startThemeSync(),
    Events.On("locked", () => handleLocked()),
    Events.On("unlocked", () => handleUnlockedElsewhere()),
    Events.On("remote-change", () => syncInBackground()),
    Events.On("share-links-changed", () => loadAllShareLinkQuietly()),
    Events.On("browser-pairing", (event) => showBrowserPairing(event.data ?? null)),
    Events.On("show-item", (event) => showItem(event.data)),
    Events.On("show-settings", (event) => openSettings(event.data ?? "")),
  ];
  restoreBrowserPairing();
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
  <UpdateRequiredView v-if="store.status.updateRequired" />
  <SecretKeyView v-else-if="store.pendingSecretKey" />
  <TouchIDOfferView v-else-if="store.isTouchIDOfferShown" />
  <SignInView v-else-if="store.status.state === 'signed-out'" />
  <UnlockView v-else-if="store.status.state === 'locked'" />
  <MainView v-else />
  <BrowserPairingModal />
  <div v-if="store.noticeMessage" class="notice" role="status"><AppIcon name="check" /> {{ store.noticeMessage }}</div>
</template>
