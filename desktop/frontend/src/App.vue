<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "./api";
import { handleLocked, loadStatus, store } from "./store";
import SignInView from "./components/SignInView.vue";
import SecretKeyView from "./components/SecretKeyView.vue";
import UnlockView from "./components/UnlockView.vue";
import MainView from "./components/MainView.vue";
import AppIcon from "./components/AppIcon.vue";

const activityReportInterval = 30_000;
let lastActivityReport = 0;
let removeLockedListener: (() => void) | undefined;

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
  removeLockedListener = Events.On("locked", () => handleLocked());
  window.addEventListener("keydown", reportActivity);
  window.addEventListener("pointerdown", reportActivity);
});

onBeforeUnmount(() => {
  removeLockedListener?.();
  window.removeEventListener("keydown", reportActivity);
  window.removeEventListener("pointerdown", reportActivity);
});
</script>

<template>
  <SecretKeyView v-if="store.pendingSecretKey" />
  <SignInView v-else-if="store.status.state === 'signed-out'" />
  <UnlockView v-else-if="store.status.state === 'locked'" />
  <MainView v-else />
  <div v-if="store.noticeMessage" class="notice" role="status"><AppIcon name="check" /> {{ store.noticeMessage }}</div>
</template>
