<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { startThemeSync } from "./theme";
import { startWebsiteIconSync } from "./websiteicons";
import {
  handleLocked,
  handleUnlockedElsewhere,
  isMainWindowAction,
  loadAllShareLinkQuietly,
  loadStatus,
  openFromSettings,
  openInviteLink,
  openSettings,
  reportActivity,
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
import AppNotice from "./components/AppNotice.vue";
import MissingAgentKeyNotice from "./components/MissingAgentKeyNotice.vue";
import BrowserPairingModal from "./components/BrowserPairingModal.vue";
import PendingRecoveryView from "./components/PendingRecoveryView.vue";

let allRemoveListener: (() => void)[] = [];

onMounted(() => {
  loadStatus().then(async () => { await takePendingInviteQuietly(); await restoreBrowserPairing(); });
  allRemoveListener = [
    Events.On("update-required", (event) => event.data && showUpdateRequired(event.data)),
    Events.On("invite-link", (event) => openInviteLink(event.data ?? null)),
    startThemeSync(),
    startWebsiteIconSync(),
    Events.On("locked", () => handleLocked()),
    Events.On("unlocked", () => handleUnlockedElsewhere()),
    Events.On("remote-change", () => syncInBackground()),
    Events.On("share-links-changed", () => loadAllShareLinkQuietly()),
    Events.On("browser-pairing", (event) => showBrowserPairing(event.data ?? null)),
    Events.On("show-item", (event) => showItem(event.data)),
    Events.On("show-settings", (event) => openSettings(event.data ?? "")),
    Events.On("open-from-settings", (event) => isMainWindowAction(event.data) && openFromSettings(event.data)),
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
  <UpdateRequiredView v-if="store.status.updateRequired" />
  <PendingRecoveryView v-else-if="store.isRecoveryOpen && store.pendingRecovery.length > 0" />
  <UnlockView v-else-if="store.status.state === 'locked'" />
  <SecretKeyView v-else-if="store.onboardingStep === 'kit'" />
  <TouchIDOfferView v-else-if="store.onboardingStep === 'biometric'" />
  <SignInView v-else-if="store.status.state === 'signed-out'" />
  <MainView v-else />
  <BrowserPairingModal v-if="store.status.state === 'unlocked'" />
  <MissingAgentKeyNotice v-if="store.status.state === 'unlocked'" />
  <AppNotice />
</template>

