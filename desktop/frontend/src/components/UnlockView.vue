<script setup lang="ts">
import { ref } from "vue";
import { signOut, store, unlock } from "../store";

const password = ref("");
const isConfirmingSignOut = ref(false);

async function submitUnlock(): Promise<void> {
  await unlock(password.value);
  password.value = "";
}
</script>

<template>
  <div class="centered-page">
    <div class="title-bar" style="width: 100%"></div>
    <div class="card">
      <h1>Hisar is locked</h1>
      <p class="subtitle muted">{{ store.status.email }} · {{ store.status.server }}</p>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <form @submit.prevent="submitUnlock">
        <label>
          <span>Master password</span>
          <input v-model="password" name="password" type="password" autocomplete="current-password" autofocus required />
        </label>
        <button class="primary" type="submit" :disabled="store.isBusy">{{ store.isBusy ? "Unlocking…" : "Unlock" }}</button>
      </form>
      <div class="sign-out">
        <button v-if="!isConfirmingSignOut" class="link" @click="isConfirmingSignOut = true">Sign out of this device</button>
        <template v-else>
          <p class="muted">You will need your Secret Key to sign in again.</p>
          <div class="row confirm-row">
            <button @click="isConfirmingSignOut = false">Cancel</button>
            <button class="danger" data-testid="confirm-sign-out" @click="signOut">Sign out</button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
form button[type="submit"] {
  width: 100%;
}

.sign-out {
  margin-top: 20px;
  text-align: center;
}

.confirm-row {
  justify-content: center;
}
</style>
