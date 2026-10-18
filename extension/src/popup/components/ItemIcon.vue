<script lang="ts">
import type { IconName } from "./AppIcon.vue";

const allCategoryIcon: Record<string, IconName> = {
  login: "globe",
  "credit-card": "card",
  identity: "identity",
  address: "address",
  "secure-note": "note",
  "ssh-key": "terminal",
  "api-credential": "key",
  server: "server",
  database: "database",
  "software-license": "license",
};

export function getCategoryIcon(category: string): IconName {
  return allCategoryIcon[category] ?? "lock";
}

// The popup asks Hisar for each website's icon once while it is open; "" is a website without one.
const allIconByHost = new Map<string, Promise<string>>();

function getHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return "";
  }
}
</script>

<script setup lang="ts">
import { ref, watchEffect } from "vue";
import { sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ category: string; url?: string; size?: number }>();

const imageUrl = ref("");

watchEffect(async () => {
  imageUrl.value = "";
  const host = props.url ? getHost(props.url) : "";
  if (!host || !props.url) return;
  let icon = allIconByHost.get(host);
  if (!icon) {
    icon = sendToBackground<string>({ type: "popup.getIcon", url: props.url }).catch(() => "");
    allIconByHost.set(host, icon);
  }
  const url = props.url;
  const result = await icon;
  if (props.url === url) imageUrl.value = result;
});
</script>

<template>
  <div class="item-icon" :style="{ width: `${size ?? 30}px`, height: `${size ?? 30}px` }" aria-hidden="true">
    <img v-if="imageUrl" :src="imageUrl" alt="" data-testid="item-icon-image" />
    <AppIcon v-else :name="getCategoryIcon(category)" :size="Math.round((size ?? 30) * 0.55)" />
  </div>
</template>

<style scoped>
.item-icon {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.item-icon img {
  width: 70%;
  height: 70%;
  object-fit: contain;
}
</style>
