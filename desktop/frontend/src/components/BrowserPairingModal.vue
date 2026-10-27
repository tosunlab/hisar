<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { cancelBrowserPairing, store } from "../store";
import AppIcon from "./AppIcon.vue";

const now = ref(Date.now());
let clockTimer: ReturnType<typeof setInterval> | undefined;

const allCodeGroup = computed(() => {
  const code = store.browserPairing?.code ?? "";
  return [code.slice(0, 3), code.slice(3)];
});
const secondsLeft = computed(() => {
  const expiresAt = Date.parse(store.browserPairing?.expiresAt ?? "");
  return Number.isNaN(expiresAt) ? 0 : Math.max(0, Math.round((expiresAt - now.value) / 1000));
});
const timeLeft = computed(() => `${Math.floor(secondsLeft.value / 60)}:${String(secondsLeft.value % 60).padStart(2, "0")}`);

onMounted(() => {
  clockTimer = setInterval(() => {
    now.value = Date.now();
    if (store.browserPairing && secondsLeft.value === 0) {
      store.browserPairing = null;
    }
  }, 1000);
});
onBeforeUnmount(() => clearInterval(clockTimer));
</script>

<template>
  <div v-if="store.browserPairing" class="overlay">
    <div class="card pairing" role="alertdialog" aria-modal="true" aria-labelledby="pairing-title" data-testid="browser-pairing">
      <span class="pairing-icon"><AppIcon name="globe" :size="22" /></span>
      <h2 id="pairing-title">Pair {{ store.browserPairing.browser }} with Hisar?</h2>
      <p class="muted">
        Lets the Hisar extension browse your vaults, reveal and copy supported item fields, fill logins, cards, identities and addresses, and save new items while Hisar is
        unlocked.
      </p>
      <p class="muted">This browser stays paired after Hisar locks. Remove it in Settings → Browser.</p>
      <div class="code monospace" :aria-label="store.browserPairing.code" data-testid="pairing-code">
        <span v-for="group in allCodeGroup" :key="group">{{ group }}</span>
      </div>
      <p class="muted note">Enter this code in the Hisar extension. Pairing code expires in {{ timeLeft }}. If you did not start pairing in your browser, cancel.</p>
      <div class="row actions">
        <div class="spacer"></div>
        <button data-testid="cancel-pairing" @click="cancelBrowserPairing">Cancel</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--color-backdrop);
}

.pairing {
  width: min(400px, 100%);
  padding: 22px;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

.pairing-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

h2 {
  margin: 14px 0 6px;
  font-size: var(--font-size-15);
  font-weight: 600;
}

p {
  margin: 0;
  font-size: var(--font-size-13);
  line-height: 1.5;
}

p + p {
  margin-top: 8px;
}

.code {
  display: flex;
  justify-content: center;
  gap: 14px;
  margin: 18px 0;
  padding: 16px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--font-size-22);
  letter-spacing: 0.12em;
  user-select: text;
  -webkit-user-select: text;
}

.note {
  margin-bottom: 16px;
  font-size: var(--font-size-12);
}
</style>
