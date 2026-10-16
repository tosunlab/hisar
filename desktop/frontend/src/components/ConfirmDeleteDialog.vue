<script setup lang="ts">
import { computed } from "vue";
import { deleteItem, store } from "../store";
import AccountDialog from "./AccountDialog.vue";

const props = defineProps<{ vaultID: string; itemID: string; title: string }>();
const emit = defineEmits<{ close: [] }>();

const vault = computed(() => store.allVault.find((candidate) => candidate.id === props.vaultID));
const subtitle = computed(() =>
  vault.value && vault.value.type !== "personal"
    ? `It is removed from ${vault.value.name} for everyone with access to the vault. It can't be undone.`
    : "It is removed from all your devices. It can't be undone.",
);

async function confirmDeletion(): Promise<void> {
  if (await deleteItem(props.itemID, props.vaultID)) {
    emit("close");
  }
}
</script>

<template>
  <AccountDialog :title="`Delete “${title}”?`" :subtitle="subtitle" icon="trash" is-danger :is-closable="!store.isBusy" data-testid="confirm-delete-dialog" @close="emit('close')">
    <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
    <div class="row actions">
      <button type="button" class="ghost" :disabled="store.isBusy" data-testid="confirm-delete-cancel" @click="emit('close')">Cancel</button>
      <button type="button" class="danger-solid" :disabled="store.isBusy" data-testid="confirm-delete-item" autofocus @click="confirmDeletion">Delete item</button>
    </div>
  </AccountDialog>
</template>
