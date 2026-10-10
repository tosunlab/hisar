<script setup lang="ts">
import { computed, onMounted } from "vue";
import { installCommandLineTool, loadCommandLineStatus, setCommandLineIntegration, store, uninstallCommandLineTool } from "../store";
import AppIcon from "./AppIcon.vue";

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
  <section data-testid="command-line-section">
    <p class="section-label">Command-line tool</p>
    <div class="setting">
      <span class="setting-icon"><AppIcon name="terminal" /></span>
      <div class="setting-text">
        <strong><span class="monospace">hisar</span> command-line tool</strong>
        <p v-if="!commandLineStatus" class="muted">Checking the command-line tool…</p>
        <p v-else-if="!commandLineStatus.isAvailable" class="muted" data-testid="command-line-state">
          The command-line tool comes with the packaged Hisar app; this development build has none.
        </p>
        <p v-else-if="isInstalled" class="muted" data-testid="command-line-state">
          <span class="state-dot running"></span>Installed at <span class="monospace">{{ commandLineStatus.installedPath }}</span>
        </p>
        <p v-else class="muted" data-testid="command-line-state"><span class="state-dot stopped"></span>Not installed</p>
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
      <template v-if="commandLineStatus?.isAvailable">
        <button v-if="isInstalled" data-testid="uninstall-command-line" :disabled="store.isBusy" @click="uninstallCommandLineTool">Uninstall</button>
        <button v-else class="primary" data-testid="install-command-line" :disabled="store.isBusy" @click="installCommandLineTool">
          Install command-line tool
        </button>
      </template>
    </div>
    <div v-if="allConflict.length > 0 || commandLineStatus?.pathHint" class="command-line-notes">
      <p v-for="conflict in allConflict" :key="conflict.path" class="muted" data-testid="command-line-conflict">
        <AppIcon name="warning" :size="13" /> <span class="monospace">{{ conflict.path }}</span> is {{ conflict.description }}; Hisar leaves it alone.
      </p>
      <p v-if="commandLineStatus?.pathHint" class="path-hint monospace" data-testid="command-line-path-hint">{{ commandLineStatus.pathHint }}</p>
    </div>
    <p v-if="store.commandLineErrorMessage" class="error" role="alert" data-testid="command-line-error">{{ store.commandLineErrorMessage }}</p>

    <label class="setting integration">
      <span class="setting-icon"><AppIcon name="shield" /></span>
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
        type="checkbox"
        name="commandLineIntegration"
        :checked="commandLineStatus?.isIntegrationEnabled ?? true"
        :disabled="!commandLineStatus || store.isBusy"
        @change="setCommandLineIntegration(($event.target as HTMLInputElement).checked)"
      />
    </label>
  </section>
</template>

<style scoped>
.state-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  margin-right: 6px;
  border-radius: 50%;
  vertical-align: 1px;
}

.state-dot.running {
  background: var(--color-success);
}

.state-dot.stopped {
  background: var(--color-text-muted);
}

.setting-text .warning-text {
  margin-top: 2px;
}

.command-line-notes {
  padding: 0 0 8px 44px;
}

.command-line-notes p {
  margin: 0 0 6px;
  font-size: 12px;
}

.path-hint {
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.integration {
  cursor: pointer;
}

.integration input {
  width: auto;
}
</style>
