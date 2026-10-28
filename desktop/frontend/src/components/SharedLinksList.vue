<script setup lang="ts">
import { ref } from "vue";
import type { ShareLink } from "../api";
import { copyShareLink, openShareLinkSource, revokeShareLink, store } from "../store";
import { describeShareLinkContent, formatDateTime, formatExpiresIn, formatShareLinkViews } from "../util";
import AppIcon from "./AppIcon.vue";

withDefaults(defineProps<{ allShareLink: ShareLink[]; isTitleShown?: boolean; emptyText?: string }>(), { isTitleShown: true, emptyText: "No live share links." });

const confirmingShareLinkID = ref("");

async function confirmRevoke(shareLink: ShareLink): Promise<void> {
  if (await revokeShareLink(shareLink)) {
    confirmingShareLinkID.value = "";
  }
}

</script>

<template>
  <p v-if="allShareLink.length === 0" class="muted empty" data-testid="share-links-empty">{{ emptyText }}</p>
  <ul v-else class="share-link-list" data-testid="share-link-list">
    <li v-for="shareLink in allShareLink" :key="shareLink.id" class="share-link" data-testid="share-link-row">
      <div class="share-link-main">
        <span class="share-link-icon"><AppIcon :name="shareLink.isText ? 'note' : 'link'" :size="15" /></span>
        <div class="share-link-text">
          <strong v-if="isTitleShown" class="share-link-title">{{ shareLink.title }}</strong>
          <span class="share-link-fields"><template v-if="isTitleShown && !shareLink.isText && store.allVault.find(vault => vault.id === shareLink.vaultID)">{{ store.allVault.find(vault => vault.id === shareLink.vaultID)?.name }} · </template>{{ describeShareLinkContent(shareLink) }}</span>
          <span class="share-link-meta muted">
            <span data-testid="share-link-views">{{ formatShareLinkViews(shareLink.maxViews, shareLink.viewCount) }}</span>
            <span aria-hidden="true">·</span>
            <span :title="formatDateTime(shareLink.expiresAt)">{{ formatExpiresIn(shareLink.expiresAt) }}</span>
            <span v-if="shareLink.hasPassphrase" class="badge" title="Needs a passphrase"><AppIcon name="lock" :size="11" /> Passphrase</span>
          </span>
        </div>
        <div class="share-link-actions">
          <button v-if="isTitleShown && !shareLink.isText" class="ghost" :disabled="store.isBusy" :aria-label="`Open source item for ${shareLink.title}`" @click="openShareLinkSource(shareLink)">Open source item</button>
          <button class="ghost icon-button" :aria-label="`Copy link to ${shareLink.title}`" title="Copy link" @click="copyShareLink(shareLink)"><AppIcon name="copy" /></button>
          <button
            v-if="confirmingShareLinkID !== shareLink.id"
            class="ghost danger"
            :data-testid="`revoke-share-link-${shareLink.id}`"
            @click="confirmingShareLinkID = shareLink.id"
          >
            Revoke
          </button>
        </div>
      </div>
      <div v-if="confirmingShareLinkID === shareLink.id" class="revoke-confirm" role="alertdialog" :aria-label="`Revoke the link to ${shareLink.title}?`">
        <span>Revoke this link? It stops working at once.</span>
        <div class="row">
          <button class="ghost" @click="confirmingShareLinkID = ''">Cancel</button>
          <button class="danger-solid" :disabled="store.isBusy" data-testid="confirm-revoke-share-link" @click="confirmRevoke(shareLink)">Revoke link</button>
        </div>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.empty {
  margin: 0;
  padding: 12px 14px;
  font-size: var(--font-size-12);
}

.share-link-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.share-link {
  padding: 10px 10px 10px 14px;
  border-bottom: 1px solid var(--color-border);
}

.share-link:last-child {
  border-bottom: none;
}

.share-link-main {
  display: flex;
  align-items: center;
  gap: 12px;
}

.share-link-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.share-link-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.share-link-title,
.share-link-fields {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-link-fields {
  font-size: var(--font-size-12);
  color: var(--color-text-secondary);
}

.share-link-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: var(--font-size-11);
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 0 6px;
  border: 1px solid var(--color-border-strong);
  border-radius: 10px;
}

.share-link-actions {
  display: flex;
  gap: 2px;
}

.revoke-confirm {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 10px;
  padding: 8px 10px;
  border: 1px solid var(--color-danger-border);
  border-radius: var(--radius);
  background: var(--color-danger-background);
  font-size: var(--font-size-12);
}
</style>
