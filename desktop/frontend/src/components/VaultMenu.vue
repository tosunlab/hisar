<script setup lang="ts">
import { ref } from "vue";
import type { VaultSummary } from "../api";
import { openVaultDialog, type VaultDialogMode } from "../store";
import AppIcon from "./AppIcon.vue";
import { useMenu } from "../menu";

const props = defineProps<{ vault: VaultSummary; x: number; y: number }>();
const emit = defineEmits<{ close: [] }>();
const menu = ref<HTMLElement | null>(null);
useMenu(menu, () => emit("close"));

function choose(mode: Exclude<VaultDialogMode, "blocked">): void {
  emit("close");
  openVaultDialog(props.vault.id, mode);
}

</script>

<template>
  <div ref="menu" class="card vault-menu" role="menu" :aria-label="`${vault.name} vault`" :style="{ left: `${x}px`, top: `${y}px` }" data-testid="vault-menu">
    <button v-if="vault.role === 'owner'" class="ghost" role="menuitem" data-testid="vault-menu-rename" @click="choose('rename')"><AppIcon name="edit" /> Rename…</button>
    <button class="ghost" role="menuitem" data-testid="vault-menu-delete" @click="choose('delete')"><AppIcon name="trash" /> Delete vault…</button>
    <button v-if="vault.type !== 'personal'" class="ghost" role="menuitem" data-testid="vault-menu-leave" @click="choose('leave')"><AppIcon name="signOut" /> Leave vault…</button>
  </div>
</template>

<style scoped>
.vault-menu {
  position: fixed;
  z-index: 9;
  display: flex;
  flex-direction: column;
  min-width: 168px;
  padding: 4px;
  background: var(--color-surface-raised);
  box-shadow: 0 12px 32px var(--color-shadow);
}

.vault-menu button {
  justify-content: flex-start;
  gap: 10px;
  padding: 7px 8px;
  color: var(--color-text);
}
</style>
