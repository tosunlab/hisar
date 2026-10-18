<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { getErrorMessage } from "../../api";
import { currentPath, navigate } from "../../navigation";
import AccountView from "./AccountView.vue";
import { formatActivity, formatDate, formatDeviceCount, listAccounts, readAccountId, roleLabels, stateLabels, type AdminAccount } from "./accounts";

const searchDelayMs = 200;

const accountId = computed(() => readAccountId(currentPath.value));
const query = ref("");
const accounts = ref<AdminAccount[]>([]);
const nextCursor = ref("");
const isLoading = ref(false);
const errorMessage = ref("");
let searchTimer: ReturnType<typeof setTimeout> | undefined;
let loadNumber = 0;

// load replaces the list, or appends the next page with cursor; an answer to an older search is dropped.
async function load(cursor = ""): Promise<void> {
  const currentLoad = ++loadNumber;
  isLoading.value = true;
  errorMessage.value = "";
  try {
    const page = await listAccounts(query.value.trim(), cursor);
    if (currentLoad !== loadNumber) {
      return;
    }
    accounts.value = cursor ? [...accounts.value, ...page.accounts] : page.accounts;
    nextCursor.value = page.nextCursor ?? "";
  } catch (error) {
    if (currentLoad === loadNumber) {
      errorMessage.value = getErrorMessage(error);
    }
  } finally {
    if (currentLoad === loadNumber) {
      isLoading.value = false;
    }
  }
}

watch(query, () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => load(), searchDelayMs);
});

watch(
  accountId,
  (shownId) => {
    if (shownId === "") {
      load();
    }
  },
  { immediate: true },
);

onUnmounted(() => clearTimeout(searchTimer));
</script>

<template>
  <AccountView v-if="accountId" :key="accountId" :account-id="accountId" />
  <section v-else class="accounts">
    <h1>Accounts</h1>
    <label class="search">
      <span>Search</span>
      <input v-model="query" name="query" type="search" placeholder="Email or name" autocomplete="off" spellcheck="false" />
    </label>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <table v-if="accounts.length > 0">
      <thead>
        <tr>
          <th scope="col">Account</th>
          <th scope="col">Role</th>
          <th scope="col">State</th>
          <th scope="col">Signed up</th>
          <th scope="col">Last activity</th>
          <th scope="col" class="number">Devices</th>
          <th scope="col" class="number">Vaults</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="account in accounts" :key="account.id" :data-testid="`account-${account.email}`">
          <td>
            <a :href="`/admin/accounts/${account.id}`" class="account-link" @click.prevent="navigate(`/admin/accounts/${account.id}`)">
              <span class="name">{{ account.name }}</span>
              <span class="email">{{ account.email }}</span>
            </a>
          </td>
          <td>{{ roleLabels[account.role] }}</td>
          <td>
            <span class="state" :class="account.state">{{ stateLabels[account.state] }}</span>
          </td>
          <td>{{ formatDate(account.createdAt) }}</td>
          <td>{{ formatActivity(account.lastSeenAt) }}</td>
          <td class="number">{{ formatDeviceCount(account) }}</td>
          <td class="number">{{ account.vaultCount }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="isLoading" class="muted">Loading…</p>
    <p v-else-if="!errorMessage" class="muted">{{ query.trim() ? "No account matches the search." : "No accounts yet." }}</p>
    <button v-if="nextCursor" type="button" class="more" :disabled="isLoading" @click="load(nextCursor)">
      {{ isLoading ? "Loading…" : "Show more" }}
    </button>
  </section>
</template>

<style scoped>
h1 {
  margin: 0 0 20px;
  font-size: 22px;
  font-weight: 650;
}

.accounts {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 960px;
}

.search {
  max-width: 360px;
}

table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
}

th,
td {
  padding: 10px 14px;
  border-bottom: 1px solid var(--color-border);
  text-align: left;
  vertical-align: middle;
}

tbody tr:last-child td {
  border-bottom: none;
}

th {
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 500;
}

tbody tr:hover {
  background: var(--color-hover);
}

.number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.account-link {
  display: flex;
  flex-direction: column;
  color: inherit;
  text-decoration: none;
}

.name {
  font-weight: 600;
}

.email {
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.state {
  padding: 1px 8px;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.state.suspended {
  border-color: var(--color-warning-border);
  background: var(--color-warning-background);
  color: var(--color-warning-text);
}

.muted {
  margin: 0;
  color: var(--color-text-muted);
}

.more {
  align-self: flex-start;
}
</style>
