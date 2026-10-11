<script setup lang="ts">
import { ref } from "vue";
import { signOut, store } from "../store";
import PlanSection from "./PlanSection.vue";

const isConfirmingSignOut = ref(false);
</script>

<template>
  <section class="settings-block" data-testid="account-section">
    <p class="section-label">Signed in as</p>
    <div class="settings-group">
      <div class="setting">
        <div class="setting-text">
          <strong>{{ store.status.name || store.status.email }}</strong>
          <p class="muted">{{ store.status.email }} · {{ store.status.server }}</p>
        </div>
        <button v-if="!isConfirmingSignOut" class="danger" @click="isConfirmingSignOut = true">Sign out</button>
        <template v-else>
          <button class="ghost" @click="isConfirmingSignOut = false">Cancel</button>
          <button class="danger-solid" data-testid="settings-confirm-sign-out" @click="signOut">Sign out</button>
        </template>
      </div>
    </div>
  </section>
  <PlanSection />
</template>
