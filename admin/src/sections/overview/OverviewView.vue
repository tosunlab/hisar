<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { auth, getErrorMessage, getHealth, getServer, type AdminHealth, type AdminServer, type CheckStatus } from "../../api";

const server = ref<AdminServer | null>(null);
const health = ref<AdminHealth | null>(null);
const errorMessage = ref("");

const statusMarks: Record<CheckStatus, { mark: string; label: string }> = {
  ok: { mark: "✓", label: "OK" },
  warning: { mark: "!", label: "Warning" },
  failed: { mark: "✕", label: "Failed" },
};

const numberFormat = new Intl.NumberFormat("en");
const sizeFormat = new Intl.NumberFormat("en", { maximumFractionDigits: 1 });
const sizeUnits = ["bytes", "kB", "MB", "GB", "TB"];

// formatSize writes a byte count in decimal units, as macOS does: 48.2 MB.
function formatSize(byteCount: number): string {
  let exponent = Math.min(Math.floor(Math.log10(Math.max(byteCount, 1)) / 3), sizeUnits.length - 1);
  if (Math.round((byteCount / 1000 ** exponent) * 10) >= 10_000 && exponent < sizeUnits.length - 1) {
    exponent++;
  }
  return `${sizeFormat.format(byteCount / 1000 ** exponent)} ${sizeUnits[exponent]}`;
}

const counts = computed(() => {
  const current = health.value?.counts;
  return [
    { id: "accounts", labels: ["Account", "Accounts"], value: current?.accounts },
    { id: "vaults", labels: ["Vault", "Vaults"], value: current?.vaults },
    { id: "items", labels: ["Item", "Items"], value: current?.items },
    { id: "share-links", labels: ["Share link", "Share links"], value: current?.shareLinks },
    { id: "pending-invites", labels: ["Pending invite", "Pending invites"], value: current?.pendingInvites },
  ].map(({ id, labels, value }) => ({ id, value, label: value === 1 ? labels[0] : labels[1] }));
});

const migrationSummary = computed(() => {
  const database = health.value?.database;
  const migrations = database?.migrations ?? [];
  if (database?.ok) {
    return `All ${migrations.length} migrations applied`;
  }
  const appliedCount = migrations.filter((migration) => migration.appliedAt).length;
  return `${appliedCount} of ${migrations.length} migrations applied`;
});

function showError(error: unknown): void {
  errorMessage.value = getErrorMessage(error);
}

onMounted(() => {
  getServer().then((answer) => (server.value = answer), showError);
  getHealth().then((answer) => (health.value = answer), showError);
});
</script>

<template>
  <section>
    <h1>Overview</h1>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <dl class="facts">
      <div>
        <dt>Signed in as</dt>
        <dd data-testid="admin-name">{{ auth.session?.name }}</dd>
      </div>
      <div>
        <dt>Server</dt>
        <dd data-testid="public-url">{{ server?.publicUrl ?? "…" }}</dd>
      </div>
      <div>
        <dt>Version</dt>
        <dd>
          <span data-testid="version">{{ health?.version ?? "…" }}</span>
          <span v-if="health?.latestVersion" class="update" data-testid="update">Version {{ health.latestVersion }} is available</span>
        </dd>
      </div>
      <div>
        <dt>Database</dt>
        <dd data-testid="database">
          <span v-if="health">
            {{ formatSize(health.storageBytes) }} · <span :class="{ pending: !health.database.ok }">{{ migrationSummary }}</span>
          </span>
          <template v-else>…</template>
        </dd>
      </div>
    </dl>

    <template v-if="health || !errorMessage">
      <h2>Usage</h2>
      <ul class="counts">
        <li v-for="count in counts" :key="count.id" :data-testid="`count-${count.id}`">
          <span class="count-value">{{ count.value === undefined ? "…" : numberFormat.format(count.value) }}</span>
          <span class="count-label">{{ count.label }}</span>
        </li>
      </ul>
      <p class="hint" data-testid="counts-hint">The item count may be up to a minute old.</p>

      <h2>Checks</h2>
      <ul v-if="health" class="checks">
        <li v-for="check in health.checks" :key="check.id" :data-testid="`check-${check.id}`" :data-status="check.status">
          <span class="mark" :class="check.status" role="img" :aria-label="statusMarks[check.status].label">{{ statusMarks[check.status].mark }}</span>
          <span class="check-text">
            <span class="check-title">{{ check.title }}</span>
            <span class="check-message">{{ check.message }}</span>
          </span>
        </li>
      </ul>
    </template>
  </section>
</template>

<style scoped>
h1 {
  margin: 0 0 20px;
  font-size: 22px;
  font-weight: 650;
}

h2 {
  margin: 28px 0 10px;
  font-size: 15px;
  font-weight: 650;
}

.error {
  max-width: 720px;
  margin-bottom: 16px;
}

.facts,
.checks {
  display: grid;
  gap: 1px;
  max-width: 720px;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-border);
  list-style: none;
}

.facts div {
  display: grid;
  grid-template-columns: 160px 1fr;
  gap: 16px;
  padding: 12px 16px;
  background: var(--color-surface);
}

dt {
  color: var(--color-text-muted);
}

dd {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: baseline;
  margin: 0;
  overflow-wrap: anywhere;
}

.update {
  padding: 1px 8px;
  border: 1px solid var(--color-warning-border);
  border-radius: 999px;
  background: var(--color-warning-background);
  color: var(--color-warning-text);
  font-size: 12px;
  font-weight: 600;
}

.counts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(128px, 1fr));
  gap: 10px;
  max-width: 720px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.counts li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px 14px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
}

.count-value {
  font-size: 20px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.count-label {
  color: var(--color-text-muted);
  font-size: 12px;
}

.hint {
  margin: 8px 0 0;
  color: var(--color-text-muted);
  font-size: 12px;
}

.checks li {
  display: grid;
  grid-template-columns: 22px 1fr;
  gap: 12px;
  padding: 12px 16px;
  background: var(--color-surface);
}

.mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 700;
}

.mark.ok {
  background: var(--color-success);
  color: var(--color-primary-text);
}

.mark.warning {
  background: var(--color-warning);
  color: var(--color-primary-text);
}

.mark.failed {
  background: var(--color-danger-solid);
  color: var(--color-on-danger);
}

.check-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.check-title {
  font-weight: 600;
}

.check-message {
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.pending {
  color: var(--color-warning-text);
}
</style>
