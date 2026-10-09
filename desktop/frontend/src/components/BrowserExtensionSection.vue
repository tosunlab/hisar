<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { loadBrowserStatus, removePairedBrowser, setExactHostMatch, store } from "../store";
import { formatLastSeen } from "../util";
import AppIcon from "./AppIcon.vue";

const removingInstanceID = ref("");

const allRegisteredBrowser = computed(() => store.browserStatus?.allInstallation ?? []);
const allPairedBrowser = computed(() => store.browserStatus?.allPairedBrowser ?? []);

function describeUse(lastUsedAt: string): string {
  const lastUsed = formatLastSeen(lastUsedAt);
  return lastUsed ? `Last used ${lastUsed === "Active now" ? "just now" : lastUsed.toLowerCase()}` : "";
}

async function confirmRemove(instanceID: string): Promise<void> {
  if (await removePairedBrowser(instanceID)) {
    removingInstanceID.value = "";
  }
}

onMounted(loadBrowserStatus);
</script>

<template>
  <section data-testid="browser-extension-section">
    <p class="section-label">Browser extension</p>
    <div class="setting">
      <span class="setting-icon"><AppIcon name="globe" /></span>
      <div class="setting-text">
        <strong>Hisar for Chrome, Brave, Edge, Arc and Vivaldi</strong>
        <p v-if="allRegisteredBrowser.length > 0" class="muted" data-testid="registered-browsers">
          Ready in
          <span v-for="installation in allRegisteredBrowser" :key="installation.name" class="tag" :class="installation.isInstalled ? 'success' : 'warning'" :title="installation.errorMessage">
            {{ installation.name }}
          </span>
        </p>
        <p v-else class="muted" data-testid="registered-browsers">No supported browser found. Install one, then restart Hisar.</p>
      </div>
    </div>

    <p v-if="allPairedBrowser.length === 0" class="empty-note muted" data-testid="no-paired-browser">
      Install the Hisar extension and click its icon to pair it. Hisar shows a code to enter in the extension.
    </p>
    <div v-for="pairedBrowser in allPairedBrowser" :key="pairedBrowser.instanceID" class="setting paired-browser" data-testid="paired-browser">
      <span class="setting-icon"><AppIcon name="laptop" /></span>
      <div class="setting-text">
        <strong>{{ pairedBrowser.browser }}</strong>
        <p class="muted">{{ describeUse(pairedBrowser.lastUsedAt) }}</p>
      </div>
      <template v-if="removingInstanceID === pairedBrowser.instanceID">
        <button class="ghost" @click="removingInstanceID = ''">Cancel</button>
        <button class="danger" data-testid="confirm-remove-browser" :disabled="store.isBusy" @click="confirmRemove(pairedBrowser.instanceID)">Remove</button>
      </template>
      <button v-else data-testid="remove-browser" @click="removingInstanceID = pairedBrowser.instanceID">Remove</button>
    </div>

    <label class="setting exact-host">
      <span class="setting-icon"><AppIcon name="shield" /></span>
      <div class="setting-text">
        <strong>Only fill on the exact host</strong>
        <p class="muted">
          Off: a login for google.com also fills on accounts.google.com. On: only on the host saved in the item.
        </p>
      </div>
      <input
        type="checkbox"
        name="exactHostMatch"
        :checked="store.browserStatus?.isExactHostMatch ?? false"
        :disabled="!store.browserStatus || store.isBusy"
        @change="setExactHostMatch(($event.target as HTMLInputElement).checked)"
      />
    </label>
  </section>
</template>

<style scoped>
.tag {
  margin-left: 4px;
}

.exact-host {
  cursor: pointer;
}

.exact-host input {
  width: auto;
}
</style>
