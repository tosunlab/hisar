<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { closeShareLinks, openTextShareLinkDialog, store } from "../store";
import AppIcon from "./AppIcon.vue";
import SharedLinksList from "./SharedLinksList.vue";

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape" && !store.shareLinkDialog) {
    closeShareLinks();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeShareLinks">
    <div class="card panel" role="dialog" aria-modal="true" aria-labelledby="shared-links-title" data-testid="shared-links-panel">
      <header class="panel-header">
        <span class="panel-icon"><AppIcon name="link" :size="18" /></span>
        <div class="panel-heading">
          <h2 id="shared-links-title">Shared links</h2>
          <p class="muted">Live links to your items and text. Each one stops working when it expires, runs out of views or is revoked.</p>
        </div>
        <button class="ghost icon-button" aria-label="Close" @click="closeShareLinks"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <div class="card list-card">
        <SharedLinksList :all-share-link="store.allShareLink" empty-text="No live share links. Open an item and choose Share link…, or share a piece of text." />
      </div>

      <div class="row actions">
        <button class="ghost" data-testid="share-text-from-panel" @click="openTextShareLinkDialog"><AppIcon name="note" /> Share text…</button>
        <div class="spacer"></div>
        <button class="primary" @click="closeShareLinks">Done</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--color-backdrop);
}

.panel {
  width: min(580px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

.panel-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}

.panel-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.panel-heading {
  flex: 1;
  min-width: 0;
}

h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
}

.panel-heading p {
  margin: 3px 0 0;
  font-size: 12px;
}

.list-card {
  background: var(--color-surface);
}

.actions {
  margin-top: 16px;
}
</style>
