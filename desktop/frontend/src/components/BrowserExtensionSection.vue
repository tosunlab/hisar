<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { loadBrowserStatus, removePairedBrowser, setExactHostMatch, store } from "../store";
import { formatLastUse } from "../util";
import { useSettingsRowVisibility } from "../settingsSections";
import AppIcon from "./AppIcon.vue";

const isRowVisible = useSettingsRowVisibility();

// The Chrome Web Store item serves every Chromium browser; Firefox installs the signed .xpi from the newest extension release.
const chromeWebStoreURL = "https://chromewebstore.google.com/detail/hisar/mmipiffoogblblocokimpemdopmdjghf";
const firefoxReleasesURL = "https://github.com/tosunlab/hisar/releases?q=%22Hisar+extension%22&expanded=true";

const removingInstanceID = ref("");

const allRegisteredBrowser = computed(() => (store.browserStatus?.allInstallation ?? []).filter((installation) => installation.isInstalled));
const allFailedBrowser = computed(() => (store.browserStatus?.allInstallation ?? []).filter((installation) => !installation.isInstalled));
const allPairedBrowser = computed(() => store.browserStatus?.allPairedBrowser ?? []);

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
            <strong>Browser extension</strong>
            <p class="muted">Works in Chrome, Firefox, Brave, Edge, Arc, Dia, Vivaldi and Opera.</p>
            <p class="muted" data-testid="install-extension">
              Install it from the <a :href="chromeWebStoreURL" target="_blank" data-testid="chrome-web-store-link">Chrome Web Store</a>, or get the
              <a :href="firefoxReleasesURL" target="_blank" data-testid="firefox-add-on-link">Firefox add-on</a> from GitHub.
            </p>
            <div v-if="allRegisteredBrowser.length > 0" class="registered-browsers" data-testid="registered-browsers">
              <span class="muted">Ready in</span>
              <ul class="browser-list">
                <li v-for="installation in allRegisteredBrowser" :key="installation.name" class="tag success">
                  {{ installation.name }}
                </li>
              </ul>
            </div>
            <p v-else-if="allFailedBrowser.length === 0" class="muted" data-testid="registered-browsers">No supported browser found. Hisar is ready for one as soon as you install it.</p>
            <div v-for="installation in allFailedBrowser" :key="installation.name" class="registration-failure" data-testid="browser-registration-failure">
              <p class="warning-text"><AppIcon name="warning" :size="14" /> <strong>{{ installation.name }} registration failed</strong></p>
              <p data-testid="browser-registration-error">{{ installation.errorMessage }}</p>
              <label>
                <span>Manifest location</span>
                <input class="manifest-path monospace" type="text" readonly :value="installation.manifestPath" :aria-label="`${installation.name} manifest location`" data-testid="browser-manifest-path" />
              </label>
              <p v-if="/permission|denied|not permitted|read-only/i.test(installation.errorMessage)" class="muted">Check that Hisar can write to this location.</p>
              <p class="muted">Quit Hisar Completely from the menu bar, then reopen it to retry registration.</p>
            </div>
          </div>
        </div>
        <p v-if="allPairedBrowser.length === 0" v-show="isRowVisible('paired-browsers')" class="setting muted pairing-hint" data-testid="no-paired-browser">
          Click the extension's icon to pair it. Hisar shows a code to enter in the extension.
        </p>
        <div v-for="pairedBrowser in allPairedBrowser" v-show="isRowVisible('paired-browsers')" :key="pairedBrowser.instanceID" class="setting" data-testid="paired-browser">
          <span class="settings-row-icon"><AppIcon name="laptop" :size="14" /></span>
          <div class="setting-text">
            <strong>{{ pairedBrowser.browser }}</strong>
            <p class="muted">{{ formatLastUse(pairedBrowser.lastUsedAt) }}</p>
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
  font-size: var(--font-size-12);
}

.registration-failure {
  margin-top: 12px;
}

.registration-failure p {
  overflow-wrap: anywhere;
  white-space: normal;
}

.registration-failure .warning-text {
  display: flex;
  align-items: baseline;
  gap: 5px;
}

.manifest-path {
  width: 100%;
  user-select: text;
}
</style>
