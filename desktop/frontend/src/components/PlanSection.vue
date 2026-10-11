<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { describePlanUsage, describeSubscription, monthlyPriceLabel, yearlyPriceLabel } from "../plan";
import { loadPlan, openCheckout, openManageSubscription, store } from "../store";

type BillingInterval = "monthly" | "yearly";

const allIntervalOption: { id: BillingInterval; label: string; price: string }[] = [
  { id: "yearly", label: "Yearly", price: yearlyPriceLabel },
  { id: "monthly", label: "Monthly", price: monthlyPriceLabel },
];

const chosenInterval = ref<BillingInterval>("yearly");
const isChoosingInterval = ref(false);
const isOpening = ref(false);
const planErrorMessage = ref("");
let removePlanListener: (() => void) | undefined;

const plan = computed(() => store.plan);
const description = computed(() => {
  const current = plan.value;
  if (!current) {
    return "";
  }
  if (current.isFoundingMember) {
    return "Pro, free for life. Thank you for helping shape Hisar.";
  }
  return describeSubscription(current) || (current.plan === "pro" ? "Everything in Free, with more room to share." : "Everything that runs on your devices is free.");
});
const chosenPrice = computed(() => allIntervalOption.find((option) => option.id === chosenInterval.value)?.price ?? "");

async function openWith(action: () => Promise<string>): Promise<void> {
  isOpening.value = true;
  planErrorMessage.value = await action();
  isOpening.value = false;
}

async function continueToCheckout(): Promise<void> {
  await openWith(() => openCheckout(chosenInterval.value));
  if (!planErrorMessage.value) {
    isChoosingInterval.value = false;
  }
}

onMounted(() => {
  loadPlan();
  removePlanListener = Events.On("plan-changed", () => loadPlan());
});
onBeforeUnmount(() => removePlanListener?.());
</script>

<template>
  <section class="settings-block" data-testid="plan-section">
    <p class="section-label">Plan</p>
    <p v-if="!plan && store.isPlanUnavailable" class="muted empty-note" data-testid="plan-unavailable">Plan details need a connection to the Hisar server.</p>
    <p v-else-if="!plan" class="muted empty-note">Loading your plan…</p>
    <div v-else class="settings-group">
      <div class="setting">
        <div class="setting-text">
          <strong class="title-line">
            <span class="title-text" data-testid="plan-title">{{ plan.title }}</span>
            <span v-if="plan.isFoundingMember" class="tag accent" data-testid="founding-member-badge">Founding member</span>
          </strong>
          <p class="muted" data-testid="plan-description">{{ description }}</p>
          <p class="muted" data-testid="plan-usage">{{ describePlanUsage(plan) }}</p>
        </div>
        <button v-if="plan.isManageAvailable" type="button" :disabled="isOpening" data-testid="manage-subscription" @click="openWith(openManageSubscription)">
          Manage subscription…
        </button>
        <button
          v-else-if="plan.isCheckoutAvailable && !isChoosingInterval"
          type="button"
          class="primary"
          data-testid="upgrade-to-pro"
          @click="isChoosingInterval = true"
        >
          Upgrade to Pro…
        </button>
      </div>
      <div v-if="plan.isCheckoutAvailable && isChoosingInterval" class="setting" data-testid="checkout-choice">
        <div class="setting-text">
          <strong id="billing-interval-label">Pro, {{ chosenPrice }}</strong>
          <p class="muted">Unlimited shared vaults with up to 10 people, 200 live share links and email support. Lemon Squeezy handles the payment.</p>
        </div>
        <div class="segmented" role="radiogroup" aria-labelledby="billing-interval-label">
          <button
            v-for="option in allIntervalOption"
            :key="option.id"
            type="button"
            role="radio"
            :aria-checked="chosenInterval === option.id"
            :data-testid="`billing-interval-${option.id}`"
            @click="chosenInterval = option.id"
          >
            {{ option.label }}
          </button>
        </div>
      </div>
      <div v-if="plan.isCheckoutAvailable && isChoosingInterval" class="setting checkout-actions">
        <button type="button" class="ghost" @click="isChoosingInterval = false">Cancel</button>
        <button type="button" class="primary" :disabled="isOpening" data-testid="continue-to-checkout" @click="continueToCheckout">Continue in browser…</button>
      </div>
    </div>
    <p v-if="planErrorMessage" class="error settings-note" role="alert" data-testid="plan-error">{{ planErrorMessage }}</p>
  </section>
</template>

<style scoped>
.segmented {
  display: inline-flex;
  flex: none;
  padding: 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
}

.segmented button {
  min-width: 64px;
  padding: 4px 12px;
  border: none;
  border-radius: var(--radius-small);
  background: transparent;
  color: var(--color-text-secondary);
}

.segmented button:hover:not([aria-checked="true"]) {
  background: var(--color-hover);
  color: var(--color-text);
}

.segmented button[aria-checked="true"] {
  background: var(--color-surface);
  box-shadow: 0 1px 2px var(--color-shadow), inset 0 0 0 1px var(--color-border-strong);
  color: var(--color-text);
  font-weight: 600;
}

.checkout-actions {
  justify-content: flex-end;
  gap: 8px;
}
</style>
