<script setup lang="ts">
import { ref } from "vue";
import type { Device } from "../api";
import { revokeDevice, store } from "../store";
import { formatLastSeen, formatPlatform, getPlatformIcon, isMacPlatform } from "../util";
import AppIcon from "./AppIcon.vue";

const revokingDeviceID = ref("");
const currentDeviceLabel = isMacPlatform() ? "This Mac" : "This device";

function describeActivity(device: Device): string {
  const lastSeen = formatLastSeen(device.lastSeenAt);
  if (device.isCurrent || lastSeen === "Active now") {
    return "Active now";
  }
  return lastSeen ? `Last seen ${lastSeen === "Yesterday" ? "yesterday" : lastSeen}` : "";
}

async function confirmRevoke(device: Device): Promise<void> {
  if (await revokeDevice(device)) {
    revokingDeviceID.value = "";
  }
}
</script>

<template>
  <section class="settings-block" data-testid="devices-section">
    <p class="section-label">Signed-in devices</p>
    <p v-if="store.allDevice.length === 0" class="muted empty-note">{{ store.isBusy ? "Loading devices…" : "No devices to show." }}</p>
    <div v-else class="settings-group">
      <div v-for="device in store.allDevice" :key="device.id" class="setting" :data-testid="`device-${device.id}`">
        <span class="settings-row-icon"><AppIcon :name="getPlatformIcon(device.platform)" :size="14" /></span>
        <div class="setting-text">
          <strong class="title-line">
            <span class="title-text">{{ device.name }}</span>
            <span v-if="device.isCurrent" class="tag accent">{{ currentDeviceLabel }}</span>
          </strong>
          <p class="muted">{{ formatPlatform(device.platform) }} · {{ describeActivity(device) }}</p>
          <p v-if="revokingDeviceID === device.id" class="revoke-note">
            {{ device.name }} will be signed out and needs the master password and Secret Key to sign in again.
          </p>
        </div>
        <template v-if="!device.isCurrent">
          <button v-if="revokingDeviceID !== device.id" class="danger" :data-testid="`revoke-device-${device.id}`" @click="revokingDeviceID = device.id">Sign out…</button>
          <template v-else>
            <button class="ghost" @click="revokingDeviceID = ''">Cancel</button>
            <button class="danger-solid" data-testid="confirm-revoke-device" :disabled="store.isBusy" @click="confirmRevoke(device)">Sign out device</button>
          </template>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.setting-text .revoke-note {
  margin-top: 6px;
  color: var(--color-danger-text);
}
</style>
