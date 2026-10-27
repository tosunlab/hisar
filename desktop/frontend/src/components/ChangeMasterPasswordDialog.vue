<script setup lang="ts">
import { computed, ref } from "vue";
import { changeMasterPassword, clearError, store } from "../store";
import { describeSignedOutDevices, getMasterPasswordFeedback } from "../util";
import AccountDialog from "./AccountDialog.vue";
import StrengthMeter from "./StrengthMeter.vue";

const emit = defineEmits<{ close: [] }>();

const currentPassword = ref("");
const newPassword = ref("");
const repeatedPassword = ref("");
const signedOutDeviceCount = ref<number | null>(null);

const passwordFeedback = computed(() => getMasterPasswordFeedback(newPassword.value));
const isMismatched = computed(() => repeatedPassword.value !== "" && newPassword.value !== repeatedPassword.value);
const isSubmittable = computed(
  () =>
    currentPassword.value !== "" &&
    newPassword.value !== "" &&
    newPassword.value === repeatedPassword.value &&
    !passwordFeedback.value.isBlocking &&
    !store.isBusy,
);

async function submitChange(): Promise<void> {
  if (!isSubmittable.value) {
    return;
  }
  const result = await changeMasterPassword(currentPassword.value, newPassword.value);
  currentPassword.value = "";
  if (result) {
    newPassword.value = "";
    repeatedPassword.value = "";
    signedOutDeviceCount.value = result.signedOutDeviceCount;
  }
}

function close(): void {
  clearError();
  emit("close");
}
</script>

<template>
  <AccountDialog
    title="Change master password"
    subtitle="Your other devices will be signed out. Sign in on them with the new master password."
    icon="lock"
    :is-closable="!store.isBusy"
    data-testid="change-master-password-dialog"
    @close="close"
  >
    <template v-if="signedOutDeviceCount !== null">
      <ul class="consequences" data-testid="change-master-password-done">
        <li>Your master password was changed.</li>
        <li>{{ describeSignedOutDevices(signedOutDeviceCount) }} Sign in on them with the new master password.</li>
      </ul>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <div class="row actions">
        <button class="primary" data-testid="change-master-password-close" @click="close">Done</button>
      </div>
    </template>

    <form v-else novalidate @submit.prevent="submitChange">
      <ul class="consequences warning" data-testid="change-master-password-consequences">
        <li>Nobody can recover a forgotten master password, including Hisar.</li>
      </ul>
      <label>
        <span>Current master password</span>
        <input v-model="currentPassword" name="currentPassword" type="password" autocomplete="current-password" autofocus />
      </label>
      <label>
        <span>New master password</span>
        <input v-model="newPassword" name="newPassword" type="password" autocomplete="new-password" />
        <StrengthMeter :password="newPassword" />
        <p class="hint" :class="passwordFeedback.isBlocking && newPassword ? 'warning-text' : 'muted'" data-testid="new-password-feedback">
          {{ passwordFeedback.message }}
        </p>
      </label>
      <label>
        <span>Repeat new master password</span>
        <input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" />
        <p v-if="isMismatched" class="hint warning-text" data-testid="new-password-mismatch">The passwords don't match.</p>
      </label>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <div class="row actions">
        <button type="button" class="ghost" :disabled="store.isBusy" @click="close">Cancel</button>
        <button class="primary" type="submit" :disabled="!isSubmittable" data-testid="confirm-change-master-password">
          {{ store.isBusy ? "Changing…" : "Change master password" }}
        </button>
      </div>
    </form>
  </AccountDialog>
</template>
