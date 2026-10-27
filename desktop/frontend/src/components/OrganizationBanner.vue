<script setup lang="ts">
import { store } from "../store";
import SidebarNotice from "./SidebarNotice.vue";
</script>

<!-- Only what Hisar could not do with vault keys shows above the vaults; invitations and waiting keys are in the Inbox. -->
<template>
  <div v-if="store.waitingVaultKeys.deliveryError || store.waitingVaultKeys.rotationError" class="banner-list" data-testid="organization-banner">
    <SidebarNotice
      v-if="store.waitingVaultKeys.deliveryError"
      icon="warning"
      is-warning
      :text="`Hisar couldn't give every vault key: ${store.waitingVaultKeys.deliveryError}`"
      data-testid="organization-banner-delivery-error"
    />
    <SidebarNotice
      v-if="store.waitingVaultKeys.rotationError"
      icon="warning"
      is-warning
      :text="`Hisar couldn't re-encrypt every vault that needs a new key: ${store.waitingVaultKeys.rotationError}`"
      data-testid="organization-banner-rotation-error"
    />
  </div>
</template>

<style scoped>
.banner-list {
  margin: 0 0 10px;
}
</style>
