<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { PopupView } from "../../shared/messages";
import type { UpdateRequirement } from "../../shared/protocol";
import AppIcon from "./AppIcon.vue";

// isPasswordUnlockAvailable: the locked view asks for the master password, and still offers to unlock in the app.
const props = defineProps<{
  view: PopupView;
  detail?: string;
  isWaiting?: boolean;
  updateRequired?: UpdateRequirement;
  isPasswordUnlockAvailable?: boolean;
  isUnlocking?: boolean;
  unlockError?: string;
}>();
const emit = defineEmits<{ openHisar: []; unlock: []; unlockWithPassword: [password: string]; refresh: []; openUpdatePage: [url: string] }>();

const password = ref("");
const passwordInput = ref<HTMLInputElement>();

function submitPassword(): void {
  if (!password.value || props.isUnlocking) return;
  emit("unlockWithPassword", password.value);
  password.value = "";
}

onMounted(() => passwordInput.value?.focus());

const isExtensionUpdate = computed(() => props.view === "update-required" && props.updateRequired?.target === "extension");

const content = computed(() => {
  switch (props.view) {
    case "update-required": {
      const requirement = props.updateRequired;
      if (requirement?.target === "extension") {
        return {
          title: "Update the Hisar extension",
          subtitle: `This server needs version ${requirement.minimumVersion} or later of the extension; this is ${requirement.currentVersion}.`,
        };
      }
      if (requirement) {
        return {
          title: "Hisar needs an update",
          subtitle: `This server needs Hisar ${requirement.minimumVersion} or later. This Mac has ${requirement.currentVersion}. Open Hisar to update it.`,
        };
      }
      return { title: "Hisar needs an update", subtitle: "This server needs a newer Hisar. Open Hisar to update it." };
    }
    case "desktop-unavailable":
      return {
        title: "Hisar is not running",
        subtitle: "The extension needs the Hisar app on this computer. Open it, then try again.",
      };
    case "signed-out":
      return { title: "Sign in to Hisar", subtitle: "Sign in to your account in the Hisar app on this computer, then try again." };
    default:
      return {
        title: "Hisar is locked",
        subtitle: props.isPasswordUnlockAvailable ? "Enter your master password to unlock Hisar." : "Unlock Hisar to fill and save logins.",
      };
  }
});
</script>

<template>
  <section class="panel" :data-testid="`status-${view}`">
    <div class="icon"><AppIcon :name="view === 'locked' ? 'lock' : view === 'update-required' ? 'refresh' : 'laptop'" :size="20" /></div>
    <h1>{{ content.title }}</h1>
    <p class="subtitle">{{ content.subtitle }}</p>
    <p v-if="detail && detail !== content.title" class="detail muted">{{ detail }}</p>
    <form v-if="view === 'locked' && isPasswordUnlockAvailable" class="unlock-form" data-testid="unlock-form" @submit.prevent="submitPassword">
      <input
        ref="passwordInput"
        v-model="password"
        type="password"
        class="password"
        placeholder="Master password"
        aria-label="Master password"
        autocomplete="current-password"
        :disabled="isUnlocking"
        data-testid="master-password"
      />
      <p v-if="unlockError" class="error" role="alert" data-testid="unlock-error">{{ unlockError }}</p>
      <button type="submit" class="primary full-width" :disabled="!password || isUnlocking" data-testid="unlock-with-password">
        <AppIcon name="lock" /> {{ isUnlocking ? "Unlocking…" : "Unlock" }}
      </button>
      <button type="button" class="ghost full-width" :disabled="isWaiting || isUnlocking" data-testid="unlock" @click="emit('unlock')">
        {{ isWaiting ? "Waiting for Hisar…" : "Unlock in the Hisar app" }}
      </button>
    </form>
    <button
      v-else-if="view === 'locked'"
      type="button"
      class="primary full-width"
      :disabled="isWaiting"
      data-testid="unlock"
      @click="emit('unlock')"
    >
      <AppIcon name="lock" /> {{ isWaiting ? "Waiting for Hisar…" : "Unlock in Hisar" }}
    </button>
    <button
      v-else-if="isExtensionUpdate"
      type="button"
      class="primary full-width"
      data-testid="update-extension"
      @click="emit('openUpdatePage', updateRequired?.updateURL ?? '')"
    >
      <AppIcon name="external" /> Update extension
    </button>
    <button
      v-else-if="view === 'desktop-unavailable' || view === 'update-required'"
      type="button"
      class="primary full-width"
      :disabled="isWaiting"
      data-testid="open-hisar"
      @click="emit('openHisar')"
    >
      <AppIcon name="external" /> {{ isWaiting ? "Opening Hisar…" : "Open Hisar" }}
    </button>
    <button v-else type="button" class="full-width" :disabled="isWaiting" data-testid="refresh" @click="emit('refresh')">
      <AppIcon name="refresh" /> Try again
    </button>
  </section>
</template>

<style scoped>
.icon {
  display: inline-flex;
  margin-bottom: 12px;
  padding: 9px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-secondary);
}

.unlock-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.unlock-form .error {
  margin: 0;
  text-align: left;
}

.password {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.password:focus {
  outline: none;
  border-color: var(--color-focus);
  box-shadow: 0 0 0 3px rgb(138 149 165 / 18%);
}

.detail {
  margin: -8px 0 16px;
  font-size: 12px;
  overflow-wrap: anywhere;
}
</style>
