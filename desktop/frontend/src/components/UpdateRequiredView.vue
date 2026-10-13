<script setup lang="ts">
import { computed, ref } from "vue";
import { checkMinimumVersion, store, updateNow } from "../store";
import AppIcon from "./AppIcon.vue";

const isChecking = ref(false);
const requirement = computed(() => store.status.updateRequired);
const currentVersion = computed(() => requirement.value?.currentVersion || "a development build");

async function checkAgain(): Promise<void> {
  isChecking.value = true;
  await checkMinimumVersion();
  isChecking.value = false;
}
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="card page-card update-required-card" data-testid="update-required" role="alertdialog" aria-labelledby="update-required-title">
        <span class="update-icon"><AppIcon name="refresh" :size="28" /></span>
        <h1 id="update-required-title">Update required</h1>
        <p class="subtitle" data-testid="update-required-versions">
          This server needs Hisar {{ requirement?.minimumVersion }} or later. This Mac has {{ currentVersion }}.
        </p>
        <p class="subtitle">
          Until Hisar is updated, the SSH agent, the hisar command-line tool and the browser extension answer with "Hisar needs an update". Your
          items stay encrypted on this Mac and on the server.
        </p>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
        <button class="primary full-width" data-testid="update-now" :disabled="store.isBusy" @click="updateNow">Update now</button>
        <button class="ghost full-width check-again" data-testid="check-again" :disabled="store.isBusy || isChecking" @click="checkAgain">
          {{ isChecking ? "Checking…" : "Check again" }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.update-required-card {
  width: min(460px, 100%);
}

.update-icon {
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

.check-again {
  margin-top: 8px;
}
</style>
