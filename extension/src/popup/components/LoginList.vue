<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { MatchResult } from "../../shared/messages";
import type { LoginSummary } from "../../shared/protocol";
import { closePopup, describeError, sendToBackground } from "../api";
import { writeToClipboard } from "../clipboard";
import AppIcon from "./AppIcon.vue";
import ItemIcon from "./ItemIcon.vue";
import OneTimeCodeRow from "./OneTimeCodeRow.vue";

// In the vault view, query comes from its search box, which takes the place of this list's own; the list then also
// reports what it found and opens a login's details.
const props = defineProps<{ tabId: number | undefined; query?: string }>();
const emit = defineEmits<{ notice: [message: string]; loaded: [allLogin: LoginSummary[]]; open: [login: LoginSummary, url: string] }>();

const allLogin = ref<LoginSummary[]>([]);
const pageHost = ref("");
const query = ref("");
const isLoading = ref(true);
const errorMessage = ref("");
const busyItemID = ref("");

const isEmbedded = computed(() => props.query !== undefined);
const pageUrl = ref("");

const allVisibleLogin = computed(() => {
  const normalizedQuery = (props.query ?? query.value).trim().toLowerCase();
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
    // The service worker puts the logins filled most recently first, as the inline menu and the fill shortcut show them.
    const result = await sendToBackground<MatchResult>({ type: "popup.matchLogins", tabId: props.tabId });
    allLogin.value = result.allLogin;
    pageUrl.value = result.url;
    pageHost.value = getHostFromUrl(result.url);
    emit("loaded", result.allLogin);
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
    await writeToClipboard(login.username);
    emit("notice", "Username copied");
  });
}

function copyPassword(login: LoginSummary): Promise<void> {
  return runForLogin(login, async () => {
    if (props.tabId === undefined) return;
    const password = await sendToBackground<string>({ type: "popup.getPassword", tabId: props.tabId, vaultID: login.vaultID, itemID: login.itemID });
    await writeToClipboard(password);
    emit("notice", "Password copied");
  });
}

onMounted(loadLogins);
</script>

<template>
  <div class="logins" data-testid="login-list">
    <div v-if="allLogin.length > 0 && !isEmbedded" class="search">
      <AppIcon name="search" class="search-icon" />
      <input v-model="query" type="search" placeholder="Search logins for this site" aria-label="Search" data-testid="search" />
    </div>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <p v-if="isLoading" class="muted empty">Looking for logins…</p>
    <p v-else-if="allLogin.length === 0" class="muted empty" :class="{ compact: isEmbedded }" data-testid="no-logins">
      No saved logins for {{ pageHost || "this page" }}.
    </p>
    <p v-else-if="allVisibleLogin.length === 0" class="muted empty" :class="{ compact: isEmbedded }">No logins match "{{ props.query ?? query }}".</p>
    <ul v-else class="list">
      <li v-for="login in allVisibleLogin" :key="login.vaultID + login.itemID" class="entry" data-testid="login-row">
        <div class="login">
          <ItemIcon category="login" :url="pageUrl" class="avatar" />
          <div
            class="text"
            :class="{ openable: isEmbedded }"
            :role="isEmbedded ? 'button' : undefined"
            :tabindex="isEmbedded ? 0 : undefined"
            data-testid="open-login"
            data-row
            @click="isEmbedded && emit('open', login, pageUrl)"
            @keydown.enter="isEmbedded && emit('open', login, pageUrl)"
          >
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
            data-copy
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
        </div>
        <OneTimeCodeRow v-if="login.hasOneTimePassword && tabId !== undefined" :tab-id="tabId" :login="login" @notice="emit('notice', $event)" />
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

.entry {
  padding: 7px 8px;
  border-radius: var(--radius);
}

.entry:hover {
  background: var(--color-hover);
}

.login {
  display: flex;
  align-items: center;
  gap: 6px;
}

.avatar {
  margin-right: 4px;
}

.empty.compact {
  margin: 6px 0 10px;
  font-size: 12px;
}

.openable {
  cursor: pointer;
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
