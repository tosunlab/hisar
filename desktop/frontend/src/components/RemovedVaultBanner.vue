<script setup lang="ts">
import { dismissRemovedVaultNotice, store } from "../store";
import AppIcon from "./AppIcon.vue";
</script>

<template>
  <div v-if="store.allRemovedVaultNotice.length > 0" class="banner-list" data-testid="removed-vault-banner">
    <div v-for="notice in store.allRemovedVaultNotice" :key="notice.id" class="banner" role="status" :data-testid="`removed-vault-${notice.id}`">
      <AppIcon name="vault" :size="15" />
      <span class="banner-text">{{ notice.message }}</span>
      <button class="ghost icon-button" aria-label="Dismiss" :data-testid="`dismiss-removed-vault-${notice.id}`" @click="dismissRemovedVaultNotice(notice.id)">
        <AppIcon name="close" :size="13" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.banner-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 0 14px;
}

.banner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
  line-height: 1.4;
  color: var(--color-text-secondary);
}

.banner > svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.banner-text {
  flex: 1;
  min-width: 0;
}

.banner button {
  flex: none;
  margin: -4px -4px 0 0;
  padding: 3px;
}
</style>
