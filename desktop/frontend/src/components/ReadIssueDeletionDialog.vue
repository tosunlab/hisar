<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import type { VaultSummary } from "../api";
import { lockOnShortcut } from "../store";

const props = defineProps<{ vault: VaultSummary; issue: NonNullable<VaultSummary["readIssues"]>[number] }>();
const emit = defineEmits<{ cancel: []; confirm: [] }>();
const typed = ref("");
const dialog = ref<HTMLElement | null>(null);
const previousFocus = document.activeElement as HTMLElement | null;
const isItem = computed(() => !!props.issue.itemId);
const nameUnreadable = computed(() => props.vault.readIssues?.some((issue) => issue.kind === "vault-name" || issue.kind === "vault-key"));
const confirmation = computed(() => nameUnreadable.value ? props.vault.id.slice(0, 8) : props.vault.name);
const canDelete = computed(() => isItem.value || typed.value === confirmation.value);
const message = computed(() => {
  if (isItem.value) return `Delete unreadable item ${props.issue.itemId} from “${props.vault.name}” on every device. Everyone who has the vault loses this item. This cannot be undone.`;
  const items = props.vault.readIssues?.some((issue) => issue.kind === "vault-key") ? "all its items" : `its ${props.vault.itemCount} items, including the readable ones`;
  return `Delete the vault and ${items}. Everyone who has the vault loses them. This cannot be undone.`;
});

function handleKey(event: KeyboardEvent): void {
  if (lockOnShortcut(event)) return;
  if (event.key === "Escape") {
    event.preventDefault();
    emit("cancel");
  } else if (event.key === "Tab") {
    const controls = [...(dialog.value?.querySelectorAll<HTMLElement>("input, button:not(:disabled)") ?? [])];
    const index = controls.indexOf(document.activeElement as HTMLElement);
    if (event.shiftKey && index <= 0) {
      event.preventDefault();
      controls.at(-1)?.focus();
    } else if (!event.shiftKey && index === controls.length - 1) {
      event.preventDefault();
      controls[0]?.focus();
    }
  }
}

onMounted(() => { void nextTick(() => dialog.value?.querySelector<HTMLElement>("input, button")?.focus()); });
onBeforeUnmount(() => previousFocus?.focus());
</script>

<template>
  <div class="overlay" @click.self="emit('cancel')" @keydown.stop="handleKey">
    <form ref="dialog" class="card recovery-dialog" role="alertdialog" aria-modal="true" aria-labelledby="recovery-delete-title" aria-describedby="recovery-delete-description" @submit.prevent="canDelete && emit('confirm')">
      <h2 id="recovery-delete-title">{{ isItem ? 'Delete unreadable item?' : `Delete “${vault.name}”?` }}</h2>
      <p id="recovery-delete-description">{{ message }}</p>
      <p>Vault ID: {{ vault.id }}</p>
      <template v-if="!isItem">
        <label for="recovery-confirmation">Type {{ confirmation }} to confirm</label>
        <input id="recovery-confirmation" v-model="typed" autocomplete="off" spellcheck="false" data-testid="delete-unreadable-input" />
      </template>
      <div class="actions">
        <button type="button" class="ghost" data-testid="delete-unreadable-cancel" @click="emit('cancel')">Cancel</button>
        <button type="submit" class="danger-solid" :disabled="!canDelete" data-testid="delete-unreadable-confirm">Permanently delete</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.overlay { position: fixed; inset: 0; z-index: 10; display: flex; align-items: center; justify-content: center; padding: 24px; background: var(--color-backdrop); }
.recovery-dialog { width: min(460px, 100%); max-height: 100%; padding: 20px 22px; overflow-y: auto; overflow-wrap: anywhere; color: var(--color-text); background: var(--color-panel); box-shadow: 0 24px 64px var(--color-shadow); }
h2 { margin-top: 0; font-size: var(--font-size-15); }
input { display: block; width: 100%; margin-top: 8px; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
