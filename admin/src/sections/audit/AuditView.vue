<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { getErrorMessage } from "../../api";
import { describeDetail, exportAuditEvents, getAuditTypes, listAuditEvents, type AuditEvent, type AuditFilters, type AuditTypeGroup } from "./audit";

const filters = reactive<AuditFilters>({ account: "", kind: "", from: "", to: "" });
const groups = ref<AuditTypeGroup[]>([]);
const events = ref<AuditEvent[]>([]);
const nextCursor = ref("");
const isLoading = ref(false);
const isExporting = ref(false);
const errorMessage = ref("");
let latestLoad = 0;

const labelsByType = computed(() => new Map(groups.value.flatMap((group) => group.types.map(({ type, label }) => [type, label] as const))));
const timeFormat = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "medium" });

async function load(isNextPage: boolean): Promise<void> {
  const loadNumber = ++latestLoad;
  const cursor = isNextPage ? nextCursor.value : "";
  if (!isNextPage) {
    events.value = [];
    nextCursor.value = "";
  }
  isLoading.value = true;
  errorMessage.value = "";
  try {
    const page = await listAuditEvents(filters, cursor);
    if (loadNumber !== latestLoad) {
      return;
    }
    events.value = [...events.value, ...page.events];
    nextCursor.value = page.nextCursor;
  } catch (error) {
    if (loadNumber === latestLoad) {
      errorMessage.value = getErrorMessage(error);
    }
  } finally {
    if (loadNumber === latestLoad) {
      isLoading.value = false;
    }
  }
}

async function exportCSV(): Promise<void> {
  isExporting.value = true;
  errorMessage.value = "";
  try {
    await exportAuditEvents(filters);
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isExporting.value = false;
  }
}

onMounted(() => {
  getAuditTypes().then((answer) => (groups.value = answer), (error) => (errorMessage.value = getErrorMessage(error)));
  void load(false);
});
</script>

<template>
  <section class="audit">
    <h1>Audit log</h1>
    <form class="filters" aria-label="Filters" @submit.prevent>
      <label class="field account">
        <span>Account</span>
        <input v-model="filters.account" name="account" type="search" placeholder="Email or account ID" spellcheck="false" @change="load(false)" @input="filters.account === '' && load(false)" />
      </label>
      <label class="field">
        <span>Type</span>
        <select v-model="filters.kind" name="type" aria-label="Type" @change="load(false)">
          <option value="">All events</option>
          <optgroup v-for="group in groups" :key="group.id" :label="group.label">
            <option :value="`group:${group.id}`">All {{ group.label.toLowerCase() }}</option>
            <option v-for="type in group.types" :key="type.type" :value="`type:${type.type}`">{{ type.label }}</option>
          </optgroup>
        </select>
      </label>
      <label class="field">
        <span>From</span>
        <input v-model="filters.from" name="from" type="datetime-local" @change="load(false)" />
      </label>
      <label class="field">
        <span>To</span>
        <input v-model="filters.to" name="to" type="datetime-local" @change="load(false)" />
      </label>
      <div class="actions">
        <button type="button" :disabled="isExporting" @click="exportCSV">{{ isExporting ? "Exporting…" : "Export CSV" }}</button>
      </div>
    </form>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <table v-if="events.length > 0">
      <thead>
        <tr>
          <th scope="col">Time</th>
          <th scope="col">Event</th>
          <th scope="col">Account</th>
          <th scope="col">Details</th>
          <th scope="col">IP address</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="event in events" :key="event.id" :data-type="event.type">
          <td class="time">
            <time :datetime="event.createdAt">{{ timeFormat.format(new Date(event.createdAt)) }}</time>
          </td>
          <td>
            <span class="label">{{ labelsByType.get(event.type) ?? event.type }}</span>
            <span class="type monospace">{{ event.type }}</span>
          </td>
          <td class="account">
            <template v-if="event.accountEmail">{{ event.accountEmail }}</template>
            <template v-else-if="event.accountId">
              <span class="monospace">{{ event.accountId }}</span>
              <span class="muted">Deleted account</span>
            </template>
            <span v-else class="muted">No account</span>
          </td>
          <td class="detail">{{ describeDetail(event.detail) }}</td>
          <td class="monospace">{{ event.ip }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="!isLoading && !errorMessage" class="muted">No events match these filters.</p>
    <div class="footer">
      <span class="muted" role="status">{{ isLoading ? "Loading…" : `${events.length} ${events.length === 1 ? "event" : "events"}` }}</span>
      <button v-if="nextCursor" type="button" :disabled="isLoading" @click="load(true)">Load more</button>
    </div>
  </section>
</template>

<style scoped>
h1 {
  margin: 0 0 20px;
  font-size: 22px;
  font-weight: 650;
}

.audit {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.filters {
  display: grid;
  grid-template-columns: minmax(200px, 2fr) minmax(180px, 2fr) minmax(170px, 1fr) minmax(170px, 1fr);
  align-items: end;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
}

.actions {
  display: flex;
  grid-column: 1 / -1;
  justify-content: flex-end;
  gap: 10px;
}

select {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: inherit;
  font: inherit;
}

select:focus {
  outline: none;
  border-color: var(--color-focus);
  box-shadow: 0 0 0 3px var(--color-focus-ring);
}

table {
  width: 100%;
  border-collapse: collapse;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
  font-size: 13px;
}

th {
  padding: 9px 12px;
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 500;
  text-align: left;
}

td {
  padding: 9px 12px;
  border-bottom: 1px solid var(--color-border);
  vertical-align: top;
}

tbody tr:last-child td {
  border-bottom: none;
}

.time {
  white-space: nowrap;
}

.label,
.account > * {
  display: block;
}

.type {
  color: var(--color-text-muted);
  font-size: 11px;
}

.detail {
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.muted {
  margin: 0;
  color: var(--color-text-muted);
}

.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
</style>
