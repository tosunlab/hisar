<script setup lang="ts">
import { computed, onMounted } from "vue";
import { installCommandLineTool, loadCommandLineStatus, setCommandLineIntegration, store, uninstallCommandLineTool } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import AppIcon from "./AppIcon.vue";

const isRowVisible = useSettingsRowVisibility();

const commandLineStatus = computed(() => store.commandLineStatus);
const isInstalled = computed(() => (commandLineStatus.value?.installedPath ?? "") !== "");
const allConflict = computed(() => (commandLineStatus.value?.allInstallation ?? []).filter((installation) => installation.kind === "other"));
const isOtherCommandOnPath = computed(() => {
  const status = commandLineStatus.value;
  return status !== null && status.pathCommandPath !== "" && !status.isPathCommandFromThisApp;
});
const installedDirectory = computed(() => {
  const installedPath = commandLineStatus.value?.installedPath ?? "";
  return installedPath.slice(0, installedPath.lastIndexOf("/"));
});
const otherCommandTarget = computed(() => {
  const status = commandLineStatus.value;
  return status && status.pathCommandTarget && status.pathCommandTarget !== status.pathCommandPath ? status.pathCommandTarget : "";
});

onMounted(loadCommandLineStatus);
</script>

<template>
  <section v-show="isRowVisible('command-line-tool') || isRowVisible('command-line-integration')" class="settings-block" data-testid="command-line-section">
    <p class="section-label">Command-line tool</p>
    <div class="settings-group">
      <div v-show="isRowVisible('command-line-tool')" class="setting">
        <div class="setting-text">
          <strong><span class="monospace">hisar</span> command-line tool</strong>
          <p v-if="!commandLineStatus" class="muted">Checking the command-line tool…</p>
          <p v-else-if="!commandLineStatus.isAvailable" class="muted" data-testid="command-line-state">
            The command-line tool comes with the packaged Hisar app; this development build has none.
          </p>
          <p v-else-if="isInstalled && commandLineStatus.isInstalledByPackage" class="muted" data-testid="command-line-state">
            <span class="state-dot running"></span>Installed with the Hisar package at
            <span class="monospace">{{ commandLineStatus.installedPath }}</span>
          </p>
          <p v-else-if="isInstalled" class="muted" data-testid="command-line-state">
            <span class="state-dot running"></span>Installed at <span class="monospace">{{ commandLineStatus.installedPath }}</span>
          </p>
          <p v-else class="muted" data-testid="command-line-state"><span class="state-dot"></span>Not installed</p>
          <template v-if="commandLineStatus?.isAvailable">
            <p v-if="commandLineStatus.isPathCommandFromThisApp" class="muted" data-testid="command-line-path">
              <span class="monospace">hisar</span> in your terminal is this app's.
            </p>
            <p v-else-if="isOtherCommandOnPath" class="warning-text" data-testid="command-line-path">
              <span class="monospace">hisar</span> in your terminal is <span class="monospace">{{ commandLineStatus.pathCommandPath }}</span
              ><template v-if="otherCommandTarget"> (<span class="monospace">{{ otherCommandTarget }}</span>)</template>, not this app's.
            </p>
            <p v-else-if="isInstalled && commandLineStatus.pathHint" class="warning-text" data-testid="command-line-path">
              <span class="monospace">{{ installedDirectory }}</span> is not on your PATH yet. Add it with:
            </p>
            <p v-else-if="isInstalled" class="warning-text" data-testid="command-line-path">
              <span class="monospace">hisar</span> is not on your PATH yet; open a new terminal.
            </p>
          </template>
        </div>
        <template v-if="commandLineStatus?.isAvailable && !commandLineStatus.isInstalledByPackage">
          <button v-if="isInstalled" data-testid="uninstall-command-line" :disabled="store.isBusy" @click="uninstallCommandLineTool">Uninstall</button>
          <button v-else class="primary" data-testid="install-command-line" :disabled="store.isBusy" @click="installCommandLineTool">
            Install command-line tool
          </button>
        </template>
      </div>
      <div v-if="allConflict.length > 0 || commandLineStatus?.pathHint" v-show="isRowVisible('command-line-tool')" class="settings-group-detail">
        <p v-for="conflict in allConflict" :key="conflict.path" class="muted" data-testid="command-line-conflict">
          <AppIcon name="warning" :size="13" /> <span class="monospace">{{ conflict.path }}</span> is {{ conflict.description }}; Hisar leaves it alone.
        </p>
        <p v-if="commandLineStatus?.pathHint" class="code-block monospace" data-testid="command-line-path-hint">{{ commandLineStatus.pathHint }}</p>
      </div>
      <label v-show="isRowVisible('command-line-integration')" class="setting">
        <div class="setting-text">
          <strong>Let the command-line tool use Hisar</strong>
          <p class="muted" data-testid="command-line-integration">
            <template v-if="commandLineStatus?.errorMessage">Not available: {{ commandLineStatus.errorMessage }}</template>
            <template v-else>
              <span class="monospace">hisar</span> uses this app after you approve each terminal{{ store.status.biometricAvailable ? " with Touch ID" : "" }}, for
              10 minutes after its last command or until Hisar locks. Off: <span class="monospace">hisar</span> signs in on its own.
            </template>
          </p>
        </div>
        <input
          class="switch"
          type="checkbox"
          name="commandLineIntegration"
          :checked="commandLineStatus?.isIntegrationEnabled ?? true"
          :disabled="!commandLineStatus || store.isBusy"
          @change="setCommandLineIntegration(($event.target as HTMLInputElement).checked)"
        />
      </label>
    </div>
    <p v-if="store.commandLineErrorMessage" class="error settings-note" role="alert" data-testid="command-line-error">{{ store.commandLineErrorMessage }}</p>
  </section>
</template>

<style scoped>
.setting-text .warning-text {
  margin-top: 2px;
}
</style>
