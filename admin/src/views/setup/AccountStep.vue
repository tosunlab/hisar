<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { getErrorMessage, signUp } from "../../api";
import { bridge, type SignUpResult } from "../../bridge/bridge";

const props = defineProps<{ inviteToken: string }>();
const emit = defineEmits<{ signedUp: [result: SignUpResult] }>();

const name = ref("");
const email = ref("");
const password = ref("");
const repeatedPassword = ref("");
const isCreating = ref(false);
const errorMessage = ref("");
const isPasswordMismatch = computed(() => repeatedPassword.value !== "" && password.value !== repeatedPassword.value);

onMounted(() => bridge.start());
watch(
  () => props.inviteToken,
  () => {
    errorMessage.value = "";
  },
);

async function submit(): Promise<void> {
  if (password.value !== repeatedPassword.value) {
    return;
  }
  isCreating.value = true;
  errorMessage.value = "";
  try {
    const result = await signUp(props.inviteToken, email.value, name.value, password.value);
    password.value = "";
    repeatedPassword.value = "";
    emit("signedUp", result);
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isCreating.value = false;
  }
}
</script>

<template>
  <form class="step" @submit.prevent="submit">
    <h1>Create the first admin</h1>
    <p class="subtitle">This account manages the Hisar server from this dashboard. It is also a normal Hisar account for the apps.</p>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <label>
      <span>Name</span>
      <input v-model="name" name="name" autocomplete="name" required />
    </label>
    <label>
      <span>Email</span>
      <input v-model="email" name="email" type="email" autocomplete="username" placeholder="you@example.com" required />
    </label>
    <label>
      <span>Master password</span>
      <input v-model="password" name="password" type="password" autocomplete="new-password" minlength="10" placeholder="At least 10 characters" required />
    </label>
    <label>
      <span>Repeat master password</span>
      <input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" required />
    </label>
    <p v-if="isPasswordMismatch" class="error">The passwords do not match.</p>
    <button class="primary submit" type="submit" :disabled="isCreating || isPasswordMismatch">{{ isCreating ? "Creating account…" : "Create account" }}</button>
    <p class="hint">Your master password never leaves this browser. A forgotten master password can't be recovered by anyone, including Hisar.</p>
  </form>
</template>
