<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { loadBrowserStatus, removePairedBrowser, setExactHostMatch, store } from "../store";
import { formatLastSeen } from "../util";
import { useSettingsRowVisibility } from "../settingsSections";
import AppIcon from "./AppIcon.vue";

const isRowVisible = useSettingsRowVisibility();

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
    <div v-show="isRowVisible('browser-extension') || isRowVisible('paired-browsers')" class="settings-block">
      <p class="section-label">Browser extension</p>
      <div class="settings-group">
        <div v-show="isRowVisible('browser-extension')" class="setting">
          <div class="setting-text">
            <strong>Hisar for Chrome, Brave, Edge, Arc, Dia, Vivaldi and Opera</strong>
            <div v-if="allRegisteredBrowser.length > 0" class="registered-browsers" data-testid="registered-browsers">
              <span class="muted">Ready in</span>
              <ul class="browser-list">
                <li v-for="installation in allRegisteredBrowser" :key="installation.name" class="tag" :class="installation.isInstalled ? 'success' : 'warning'" :title="installation.errorMessage">
                  {{ installation.name }}
                </li>
              </ul>
            </div>
            <p v-else class="muted" data-testid="registered-browsers">No supported browser found. Hisar is ready for one as soon as you install it.</p>
          </div>
        </div>
        <p v-if="allPairedBrowser.length === 0" v-show="isRowVisible('paired-browsers')" class="setting muted pairing-hint" data-testid="no-paired-browser">
          Install the Hisar extension and click its icon to pair it. Hisar shows a code to enter in the extension.
        </p>
        <div v-for="pairedBrowser in allPairedBrowser" v-show="isRowVisible('paired-browsers')" :key="pairedBrowser.instanceID" class="setting" data-testid="paired-browser">
          <span class="settings-row-icon"><AppIcon name="laptop" :size="14" /></span>
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
      </div>
    </div>

    <div v-show="isRowVisible('exact-host')" class="settings-block">
      <p class="section-label">Filling</p>
      <div class="settings-group">
        <label class="setting">
          <div class="setting-text">
            <strong>Only fill on the exact host</strong>
            <p class="muted">Off: a login for google.com also fills on accounts.google.com. On: only on the host saved in the item.</p>
          </div>
          <input
            class="switch"
            type="checkbox"
            name="exactHostMatch"
            :checked="store.browserStatus?.isExactHostMatch ?? false"
            :disabled="!store.browserStatus || store.isBusy"
            @change="setExactHostMatch(($event.target as HTMLInputElement).checked)"
          />
        </label>
      </div>
    </div>
  </section>
</template>

<style scoped>
.registered-browsers {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-top: 2px;
}

.registered-browsers > .muted {
  flex: none;
}

.browser-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.pairing-hint {
  font-size: 12px;
}
</style>
