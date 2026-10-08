<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "./api";
import { addSSHApproval, handleLocked, loadStatus, removeSSHApproval, store } from "./store";
import SignInView from "./components/SignInView.vue";
import SecretKeyView from "./components/SecretKeyView.vue";
import UnlockView from "./components/UnlockView.vue";
import MainView from "./components/MainView.vue";
import AppIcon from "./components/AppIcon.vue";
import SSHApprovalModal from "./components/SSHApprovalModal.vue";

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
  <SecretKeyView v-if="store.pendingSecretKey" />
  <SignInView v-else-if="store.status.state === 'signed-out'" />
  <UnlockView v-else-if="store.status.state === 'locked'" />
  <MainView v-else />
  <SSHApprovalModal />
  <div v-if="store.noticeMessage" class="notice" role="status"><AppIcon name="check" /> {{ store.noticeMessage }}</div>
</template>
