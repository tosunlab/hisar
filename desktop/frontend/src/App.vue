<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "./api";
import { addSSHApproval, handleLocked, loadBrowserStatus, loadStatus, removeSSHApproval, showBrowserPairing, store } from "./store";
import SignInView from "./components/SignInView.vue";
import SecretKeyView from "./components/SecretKeyView.vue";
import UnlockView from "./components/UnlockView.vue";
import MainView from "./components/MainView.vue";
import AppIcon from "./components/AppIcon.vue";
import SSHApprovalModal from "./components/SSHApprovalModal.vue";
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
  loadStatus();
  allRemoveListener = [
    Events.On("locked", () => handleLocked()),
    Events.On("ssh-approval", (event) => addSSHApproval(event.data)),
    Events.On("ssh-approval-closed", (event) => removeSSHApproval(event.data)),
    Events.On("browser-pairing", (event) => showBrowserPairing(event.data ?? null)),
  ];
  loadBrowserStatus();
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
  <SecretKeyView v-if="store.pendingSecretKey" />
  <SignInView v-else-if="store.status.state === 'signed-out'" />
  <UnlockView v-else-if="store.status.state === 'locked'" />
  <MainView v-else />
  <SSHApprovalModal />
  <BrowserPairingModal />
  <div v-if="store.noticeMessage" class="notice" role="status"><AppIcon name="check" /> {{ store.noticeMessage }}</div>
</template>
