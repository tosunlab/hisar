<script setup lang="ts">
import { dismissRemovedVaultNotice, store } from "../store";
import AppIcon from "./AppIcon.vue";
import SidebarNotice from "./SidebarNotice.vue";
</script>

<template>
  <div v-if="store.allRemovedVaultNotice.length > 0" class="banner-list" data-testid="removed-vault-banner">
    <SidebarNotice v-for="notice in store.allRemovedVaultNotice" :key="notice.id" icon="vault" :text="notice.message" :data-testid="`removed-vault-${notice.id}`">
      <button class="ghost icon-button" aria-label="Dismiss" :data-testid="`dismiss-removed-vault-${notice.id}`" @click="dismissRemovedVaultNotice(notice.id)">
        <AppIcon name="close" :size="13" />
      </button>
    </SidebarNotice>
  </div>
</template>

<style scoped>
.banner-list {
  margin: 0 0 10px;
}
</style>
