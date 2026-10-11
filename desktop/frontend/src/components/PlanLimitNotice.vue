<script setup lang="ts">
import { computed, ref } from "vue";
import { getShownPlanLimit, openCheckout } from "../store";

const planLimit = computed(() => getShownPlanLimit());
const checkoutErrorMessage = ref("");

async function upgrade(): Promise<void> {
  checkoutErrorMessage.value = await openCheckout("yearly");
}
</script>

<template>
  <p v-if="planLimit && planLimit.plan === 'free'" class="plan-limit-notice" data-testid="plan-limit-notice">
    <button type="button" class="link" data-testid="plan-limit-upgrade" @click="upgrade">Upgrade to Pro on tryhisar.com</button>
    <span v-if="checkoutErrorMessage" class="muted"> {{ checkoutErrorMessage }}</span>
  </p>
</template>

<style scoped>
.plan-limit-notice {
  margin: -4px 0 12px;
  font-size: 12px;
}
</style>
