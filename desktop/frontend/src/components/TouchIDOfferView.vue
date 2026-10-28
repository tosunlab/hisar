<script setup lang="ts">
import { acceptTouchIDOffer, declineTouchIDOffer, store } from "../store";
import { touchIDConsent } from "../util";
import AppIcon from "./AppIcon.vue";
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="card page-card touch-id-offer-card" data-testid="touch-id-offer">
        <span class="touch-id-icon"><AppIcon name="fingerprint" :size="28" /></span>
        <h1>Unlock with Touch ID?</h1>
        <p class="subtitle">{{ touchIDConsent }}</p>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
        <button class="primary full-width" data-testid="accept-touch-id-offer" :disabled="store.isBusy" @click="acceptTouchIDOffer">
          {{ store.isBusy ? "Waiting for Touch ID…" : "Turn on Touch ID" }}
        </button>
        <button class="ghost full-width not-now" data-testid="decline-touch-id-offer" :disabled="store.isBusy" @click="declineTouchIDOffer">
          Not now
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.touch-id-offer-card {
  width: min(460px, 100%);
}

.touch-id-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-accent-text);
}

.not-now {
  margin-top: 8px;
}
</style>
