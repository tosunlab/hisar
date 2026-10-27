<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { copyField, copyText, openWebsite, store } from "../store";
import { buildCommitSigningCommandList, gitHubSSHKeyURL, gitLabSSHKeyURL } from "../util";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ publicKey: string }>();

const isSigningShown = ref(false);
const allCommand = computed(() => buildCommitSigningCommandList(props.publicKey));

watch(
  () => store.selectedItem?.id,
  () => {
    isSigningShown.value = false;
  },
);

async function addToHost(link: string): Promise<void> {
  await copyField("public key", "Public key");
  await openWebsite(link);
}

async function copyAllCommand(): Promise<void> {
  await copyText(allCommand.value.map((entry) => entry.command).join("\n"), "Commands");
}
</script>

<template>
  <section class="card commit-signing" data-testid="commit-signing-panel">
    <div class="row host-row">
      <div class="host-text">
        <strong>Use this key for Git</strong>
        <p class="muted">Copies the public key and opens the page to paste it; on GitHub add it as a Signing Key too.</p>
      </div>
    </div>
    <div class="row actions">
      <button data-testid="add-to-github" @click="addToHost(gitHubSSHKeyURL)"><AppIcon name="globe" /> Add to GitHub</button>
      <button data-testid="add-to-gitlab" @click="addToHost(gitLabSSHKeyURL)"><AppIcon name="globe" /> Add to GitLab</button>
      <div class="spacer"></div>
      <button
        v-if="allCommand.length > 0"
        :class="{ primary: !isSigningShown }"
        :aria-expanded="isSigningShown"
        data-testid="set-up-commit-signing"
        @click="isSigningShown = !isSigningShown"
      >
        <AppIcon name="edit" /> Set up commit signing
      </button>
    </div>

    <div v-if="isSigningShown" class="signing-steps" data-testid="commit-signing-steps">
      <p class="muted">
        Run these in a terminal; Hisar does not change your Git config. Git then signs every commit with this key through Hisar's SSH agent, and the
        <span class="monospace">allowed_signers</span> line lets <span class="monospace">git log --show-signature</span> verify your own commits.
      </p>
      <ol class="command-list">
        <li v-for="(entry, index) in allCommand" :key="index" class="command-row" data-testid="commit-signing-command">
          <div class="command-text">
            <span class="command-description">{{ entry.description }}</span>
            <code class="monospace">{{ entry.command }}</code>
          </div>
          <button class="ghost icon-button" :aria-label="`Copy: ${entry.description}`" title="Copy" @click="copyText(entry.command, 'Command')">
            <AppIcon name="copy" />
          </button>
        </li>
      </ol>
      <div class="row">
        <div class="spacer"></div>
        <button class="primary" data-testid="copy-all-commit-signing" @click="copyAllCommand"><AppIcon name="copy" /> Copy all</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.commit-signing {
  margin-top: 12px;
  padding: 14px 16px;
}

.host-text p {
  margin: 4px 0 0;
  font-size: var(--font-size-12);
}

.actions {
  margin-top: 10px;
  gap: 8px;
  flex-wrap: wrap;
}

.signing-steps {
  margin-top: 14px;
  border-top: 1px solid var(--color-border);
  padding-top: 12px;
}

.signing-steps > p {
  margin: 0 0 10px;
  font-size: var(--font-size-12);
}

.command-list {
  margin: 0 0 10px;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.command-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 6px 8px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.command-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.command-description {
  font-size: var(--font-size-11);
  color: var(--color-text-secondary);
}

.command-text code {
  font-size: var(--font-size-12);
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}
</style>
