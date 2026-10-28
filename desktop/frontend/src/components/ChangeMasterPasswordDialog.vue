<script setup lang="ts">
import { computed, ref } from "vue";
import { changeMasterPassword, clearError, store } from "../store";
import { useMasterPasswordFeedback } from "../passwordStrength";
import { describeSignedOutDevices } from "../util";
import AccountDialog from "./AccountDialog.vue";
import StrengthMeter from "./StrengthMeter.vue";
import CredentialField from "./CredentialField.vue";

const emit = defineEmits<{ close: [] }>();

const currentPassword = ref("");
const newPassword = ref("");
const repeatedPassword = ref("");
const signedOutDeviceCount = ref<number | null>(null);
const validationAnnouncement = ref("");
const sameAsCurrentMessage = "The new master password is the same as the current one.";

const { value: passwordFeedback, isCurrent: isPasswordFeedbackCurrent } = useMasterPasswordFeedback(() => newPassword.value);
const isMismatched = computed(() => repeatedPassword.value !== "" && newPassword.value !== repeatedPassword.value);
const isSameAsCurrent = computed(() => newPassword.value !== "" && newPassword.value === currentPassword.value);
const isSubmittable = computed(
  () =>
    currentPassword.value !== "" &&
    newPassword.value !== "" &&
    newPassword.value === repeatedPassword.value &&
    !isSameAsCurrent.value &&
    isPasswordFeedbackCurrent.value &&
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
        <CredentialField secret-name="current master password"><input v-model="currentPassword" name="currentPassword" type="password" autocomplete="current-password" autofocus /></CredentialField>
      </label>
      <label>
        <span>New master password</span>
        <CredentialField secret-name="new master password">
          <input
            v-model="newPassword"
            name="newPassword"
            type="password"
            autocomplete="new-password"
            :aria-describedby="isSameAsCurrent ? 'change-password-feedback change-password-same' : 'change-password-feedback'"
            :aria-invalid="(passwordFeedback.isBlocking && !!newPassword) || isSameAsCurrent"
            @blur="validationAnnouncement = isSameAsCurrent ? sameAsCurrentMessage : passwordFeedback.isBlocking && newPassword ? passwordFeedback.message : ''"
          />
        </CredentialField>
      </label>
      <StrengthMeter :password="newPassword" />
      <p id="change-password-feedback" class="hint" :class="passwordFeedback.isBlocking && newPassword ? 'warning-text' : 'muted'" data-testid="new-password-feedback">
        {{ passwordFeedback.message }}
      </p>
      <p v-if="isSameAsCurrent" id="change-password-same" class="hint warning-text" data-testid="new-password-same">{{ sameAsCurrentMessage }}</p>
      <label>
        <span>Repeat new master password</span>
        <CredentialField secret-name="repeated new master password"
          ><input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" :aria-invalid="isMismatched" :aria-describedby="isMismatched ? 'change-password-mismatch' : undefined" @blur="validationAnnouncement = isMismatched ? `The passwords don't match.` : ''"
        /></CredentialField>
      </label>
      <p v-if="isMismatched" id="change-password-mismatch" class="hint warning-text" data-testid="new-password-mismatch">The passwords don't match.</p>
      <p class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ validationAnnouncement }}</p>
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
