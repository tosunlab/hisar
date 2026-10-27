<script setup lang="ts">
import { computed } from "vue";
import type { VaultSummary } from "../api";
import { openVaultDialog, type VaultDialogMode } from "../store";
import ContextMenu, { type ContextMenuAction } from "./ContextMenu.vue";

const props = defineProps<{ vault: VaultSummary; x: number; y: number }>();
const emit = defineEmits<{ close: [] }>();

function choose(mode: Exclude<VaultDialogMode, "blocked">): void {
  openVaultDialog(props.vault.id, mode);
}

const allAction = computed<ContextMenuAction[]>(() => [
  ...(props.vault.role === "owner" ? [{ id: "rename", label: "Rename…", icon: "edit" as const, run: () => choose("rename") }] : []),
  { id: "delete", label: "Delete vault…", icon: "trash", run: () => choose("delete") },
  ...(props.vault.type !== "personal" ? [{ id: "leave", label: "Leave vault…", icon: "signOut" as const, run: () => choose("leave") }] : []),
]);
</script>

<template>
  <ContextMenu :label="`${vault.name} vault`" :x="x" :y="y" :all-action="allAction" test-id="vault-menu" @close="emit('close')" />
</template>
