<script setup lang="ts">
import { store } from "../store";
import AccountDialog from "./AccountDialog.vue";
</script>

<template>
  <AccountDialog
    v-if="store.organizationAlert"
    :title="store.organizationAlert.title"
    subtitle="Hisar couldn't make this change."
    icon="warning"
    data-testid="organization-alert"
    @close="store.organizationAlert = null"
  >
    <p class="alert-message" role="alert" data-testid="organization-alert-message">
      {{ store.organizationAlert.message.charAt(0).toUpperCase() + store.organizationAlert.message.slice(1) }}
    </p>
    <div class="row actions">
      <button class="primary" autofocus data-testid="organization-alert-ok" @click="store.organizationAlert = null">OK</button>
    </div>
  </AccountDialog>
</template>

<style scoped>
.alert-message {
  margin: 0 0 12px;
  line-height: 1.45;
  user-select: text;
  -webkit-user-select: text;
}
</style>
