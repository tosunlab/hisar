<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getErrorMessage, signIn } from "../api";
import { bridge } from "../bridge/bridge";

const email = ref("");
const secretKey = ref("");
const password = ref("");
const isSecretKeyShown = ref(false);
const isSigningIn = ref(false);
const errorMessage = ref("");

onMounted(() => bridge.start());

async function submit(): Promise<void> {
  isSigningIn.value = true;
  errorMessage.value = "";
  try {
    await signIn(email.value, secretKey.value, password.value);
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isSigningIn.value = false;
  }
}
</script>

<template>
  <main class="sign-in">
    <form class="sign-in-form" @submit.prevent="submit">
      <p class="product">Hisar · Admin dashboard</p>
      <h1>Sign in to the admin dashboard</h1>
      <p class="subtitle">Use the email, Secret Key and master password of an admin account on the Hisar server.</p>
      <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
      <label>
        <span>Email</span>
        <input v-model="email" name="email" type="email" autocomplete="username" placeholder="you@example.com" required />
      </label>
      <label>
        <span>Secret Key</span>
        <span class="with-toggle">
          <input
            v-model="secretKey"
            name="secretKey"
            :type="isSecretKeyShown ? 'text' : 'password'"
            class="monospace"
            placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX"
            autocomplete="off"
            spellcheck="false"
            required
          />
          <button type="button" class="ghost" :aria-pressed="isSecretKeyShown" @click="isSecretKeyShown = !isSecretKeyShown">
            {{ isSecretKeyShown ? "Hide" : "Show" }}
          </button>
        </span>
      </label>
      <label>
        <span>Master password</span>
        <input v-model="password" name="password" type="password" autocomplete="current-password" required />
      </label>
      <button class="primary submit" type="submit" :disabled="isSigningIn">{{ isSigningIn ? "Signing in…" : "Sign in" }}</button>
      <p class="hint">Lost your Secret Key? It's on your Emergency Kit.</p>
    </form>
  </main>
</template>

<style scoped>
.sign-in {
  display: grid;
  place-items: center;
  min-height: 100%;
  padding: 24px;
  background: radial-gradient(700px 500px at 0% 0%, var(--color-page-glow) 0%, transparent 70%), var(--color-sidebar);
}

.sign-in-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: min(400px, 100%);
  padding: 32px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-panel);
  box-shadow: 0 8px 24px var(--color-shadow);
}

.product {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 650;
  letter-spacing: -0.01em;
}

.subtitle {
  margin: -6px 0 4px;
  color: var(--color-text-muted);
}

.with-toggle {
  display: flex;
  gap: 8px;
}

.with-toggle input {
  flex: 1;
}

.submit {
  margin-top: 4px;
  padding: 10px 12px;
}

.hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
  text-align: center;
}
</style>
