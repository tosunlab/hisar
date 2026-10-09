<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { MatchResult } from "../../shared/messages";
import type { LoginSummary } from "../../shared/protocol";
import { closePopup, describeError, sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ tabId: number | undefined }>();
const emit = defineEmits<{ notice: [message: string] }>();

const allLogin = ref<LoginSummary[]>([]);
const pageHost = ref("");
const query = ref("");
const isLoading = ref(true);
const errorMessage = ref("");
const busyItemID = ref("");

const allVisibleLogin = computed(() => {
  const normalizedQuery = query.value.trim().toLowerCase();
  if (!normalizedQuery) return allLogin.value;
  return allLogin.value.filter((login) =>
    [login.title, login.username, login.vaultName].some((text) => text.toLowerCase().includes(normalizedQuery)),
  );
});

function getHostFromUrl(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return "";
  }
}

async function loadLogins(): Promise<void> {
  if (props.tabId === undefined) {
    isLoading.value = false;
    return;
  }
  try {
    const result = await sendToBackground<MatchResult>({ type: "popup.matchLogins", tabId: props.tabId });
    allLogin.value = result.allLogin;
    pageHost.value = getHostFromUrl(result.url);
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isLoading.value = false;
  }
}

async function runForLogin(login: LoginSummary, action: () => Promise<void>): Promise<void> {
  busyItemID.value = login.itemID;
  errorMessage.value = "";
  try {
    await action();
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    busyItemID.value = "";
  }
}

function fillLogin(login: LoginSummary): Promise<void> {
  return runForLogin(login, async () => {
    if (props.tabId === undefined) return;
    await sendToBackground({ type: "popup.fillLogin", tabId: props.tabId, vaultID: login.vaultID, itemID: login.itemID });
    closePopup();
  });
}

function copyUsername(login: LoginSummary): Promise<void> {
  return runForLogin(login, async () => {
    await sendToBackground({ type: "popup.copyText", text: login.username });
    emit("notice", "Username copied");
  });
}

function copyPassword(login: LoginSummary): Promise<void> {
  return runForLogin(login, async () => {
    if (props.tabId === undefined) return;
    await sendToBackground({ type: "popup.copyPassword", tabId: props.tabId, vaultID: login.vaultID, itemID: login.itemID });
    emit("notice", "Password copied. Clears in 90 s");
  });
}

onMounted(loadLogins);
</script>

<template>
  <div class="logins" data-testid="login-list">
    <div v-if="allLogin.length > 0" class="search">
      <AppIcon name="search" class="search-icon" />
      <input v-model="query" type="search" placeholder="Search logins for this site" aria-label="Search" data-testid="search" />
    </div>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <p v-if="isLoading" class="muted empty">Looking for logins…</p>
    <p v-else-if="allLogin.length === 0" class="muted empty" data-testid="no-logins">
      No saved logins for {{ pageHost || "this page" }}.
    </p>
    <p v-else-if="allVisibleLogin.length === 0" class="muted empty">No logins match "{{ query }}".</p>
    <ul v-else class="list">
      <li v-for="login in allVisibleLogin" :key="login.vaultID + login.itemID" class="login" data-testid="login-row">
        <div class="avatar" aria-hidden="true">{{ (login.title || "?").slice(0, 1).toUpperCase() }}</div>
        <div class="text">
          <p class="title-row">
            <span class="title">{{ login.title }}</span>
            <span class="tag">{{ login.vaultName }}</span>
          </p>
          <p class="username muted">{{ login.username || "No username" }}</p>
        </div>
        <button
          type="button"
          class="ghost icon-button"
          title="Copy username"
          aria-label="Copy username"
          :disabled="busyItemID !== '' || !login.username"
          data-testid="copy-username"
          @click="copyUsername(login)"
        >
          <AppIcon name="user" />
        </button>
        <button
          type="button"
          class="ghost icon-button"
          title="Copy password"
          aria-label="Copy password"
          :disabled="busyItemID !== ''"
          data-testid="copy-password"
          @click="copyPassword(login)"
        >
          <AppIcon name="key" />
        </button>
        <button
          type="button"
          class="primary small"
          :disabled="busyItemID !== ''"
          data-testid="fill"
          @click="fillLogin(login)"
        >
          Fill
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.search {
  position: relative;
  margin-bottom: 10px;
}

.search-icon {
  position: absolute;
  top: 50%;
  left: 10px;
  transform: translateY(-50%);
  color: var(--color-text-muted);
}

.search input {
  padding-left: 32px;
}

.empty {
  margin: 18px 0;
  text-align: center;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.login {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 8px;
  border-radius: var(--radius);
}

.login:hover {
  background: var(--color-hover);
}

.avatar {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  margin-right: 4px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.text {
  flex: 1;
  min-width: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
}

.title-row .tag {
  flex: none;
}

.title,
.username {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title {
  font-weight: 500;
}

.username {
  font-size: 12px;
}
</style>
