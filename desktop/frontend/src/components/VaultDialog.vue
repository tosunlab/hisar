<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { closeVaultDialog, deleteVault, leaveVault, renameVault, store, type VaultDialog } from "../store";
import { formatRole } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";

const props = defineProps<{ dialog: VaultDialog }>();

const vault = computed(() => props.dialog.actions.vault);
const allTargetVault = computed(() => props.dialog.actions.targetVaults);
const allMemberLosingAccess = computed(() => props.dialog.actions.membersLosingAccess);
const itemCountText = computed(() => (vault.value.itemCount === 1 ? "1 item" : `${vault.value.itemCount} items`));
const hasItems = computed(() => vault.value.itemCount > 0);

const newName = ref(vault.value.name);
const itemAction = ref<"move" | "delete">(allTargetVault.value.length > 0 ? "move" : "delete");
const targetVaultID = ref(allTargetVault.value[0]?.id ?? "");
const confirmedName = ref("");

const title = computed(() => {
  switch (props.dialog.mode) {
    case "rename":
      return `Rename “${vault.value.name}”`;
    case "leave":
      return `Leave “${vault.value.name}”?`;
    case "delete":
      return `Delete “${vault.value.name}”?`;
  }
  return `“${vault.value.name}”`;
});

const canSubmitDeletion = computed(() => {
  if (!hasItems.value) {
    return true;
  }
  if (itemAction.value === "move") {
    return targetVaultID.value !== "";
  }
  return confirmedName.value === vault.value.name;
});

function submitRename(): void {
  renameVault(newName.value.trim());
}

function submitDeletion(): void {
  if (!canSubmitDeletion.value) {
    return;
  }
  const isMoving = hasItems.value && itemAction.value === "move";
  const isDeletingItems = hasItems.value && itemAction.value === "delete";
  deleteVault({
    moveToVaultId: isMoving ? targetVaultID.value : "",
    isDeletingItems,
    confirmedName: isDeletingItems ? confirmedName.value : "",
  });
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeVaultDialog();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeVaultDialog">
    <div class="card vault-dialog" role="dialog" aria-modal="true" aria-labelledby="vault-dialog-title" :data-testid="`vault-dialog-${dialog.mode}`">
      <header class="vault-dialog-header">
        <span class="vault-dialog-icon" :class="{ danger: dialog.mode === 'delete' }">
          <AppIcon :name="dialog.mode === 'rename' ? 'edit' : dialog.mode === 'leave' ? 'signOut' : dialog.mode === 'delete' ? 'trash' : 'lock'" :size="18" />
        </span>
        <h2 id="vault-dialog-title">{{ title }}</h2>
        <button class="ghost icon-button" aria-label="Close" @click="closeVaultDialog"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <template v-if="dialog.mode === 'blocked'">
        <p class="body-text" data-testid="vault-blocked-reason">{{ dialog.blockedReason }}</p>
        <div class="actions">
          <button class="primary" @click="closeVaultDialog">OK</button>
        </div>
      </template>

      <form v-else-if="dialog.mode === 'rename'" @submit.prevent="submitRename">
        <label class="field-label" for="vault-new-name">Name</label>
        <input id="vault-new-name" v-model="newName" name="vaultName" autocomplete="off" autofocus data-testid="vault-rename-input" />
        <p v-if="vault.type === 'shared'" class="muted hint">Everyone who shares this vault sees the new name.</p>
        <div class="actions">
          <button type="button" class="ghost" @click="closeVaultDialog">Cancel</button>
          <button class="primary" type="submit" data-testid="vault-rename-save" :disabled="store.isBusy || !newName.trim() || newName.trim() === vault.name">Save</button>
        </div>
      </form>

      <template v-else-if="dialog.mode === 'leave'">
        <p class="body-text">You lose access to its {{ itemCountText }} on all your devices. An owner can invite you again.</p>
        <div class="actions">
          <button class="ghost" @click="closeVaultDialog">Cancel</button>
          <button class="danger-solid" data-testid="vault-leave-confirm" :disabled="store.isBusy" @click="leaveVault">Leave vault</button>
        </div>
      </template>

      <form v-else @submit.prevent="submitDeletion">
        <template v-if="allMemberLosingAccess.length > 0">
          <p class="body-text">These people lose access:</p>
          <ul class="member-list" data-testid="vault-members-losing-access">
            <li v-for="member in allMemberLosingAccess" :key="member.accountId" class="member-row">
              <MemberAvatar :name="member.name || member.email" />
              <span class="member-text">
                <span class="member-name">{{ member.name || member.email }}</span>
                <span class="muted member-email">{{ member.email }}</span>
              </span>
              <span class="muted role">{{ formatRole(member.role) }}</span>
            </li>
          </ul>
        </template>

        <template v-if="hasItems">
          <div class="choice-list">
            <label v-if="allTargetVault.length > 0" class="choice">
              <input v-model="itemAction" type="radio" name="itemAction" value="move" data-testid="vault-delete-move" />
              <span class="choice-text">
                Move its {{ itemCountText }} to
                <select v-model="targetVaultID" aria-label="Vault to move the items to" data-testid="vault-delete-target" @focus="itemAction = 'move'">
                  <option v-for="target in allTargetVault" :key="target.id" :value="target.id">{{ target.name }}</option>
                </select>
              </span>
            </label>
            <label class="choice">
              <input v-model="itemAction" type="radio" name="itemAction" value="delete" data-testid="vault-delete-items" />
              <span class="choice-text">Delete the vault and its {{ itemCountText }}</span>
            </label>
          </div>
          <p v-if="allTargetVault.length === 0" class="muted hint">There is no other vault you can add items to, so its items are deleted with it.</p>
          <div v-if="itemAction === 'delete'" class="confirm-name">
            <label class="field-label" for="vault-confirm-name">Type {{ vault.name }} to confirm</label>
            <input id="vault-confirm-name" v-model="confirmedName" name="confirmedName" autocomplete="off" spellcheck="false" data-testid="vault-delete-confirm-name" />
            <p class="muted hint">Deleted items can't be restored.</p>
          </div>
        </template>
        <p v-else class="body-text">The vault is empty.</p>

        <div class="actions">
          <button type="button" class="ghost" @click="closeVaultDialog">Cancel</button>
          <button class="danger-solid" type="submit" data-testid="vault-delete-confirm" :disabled="store.isBusy || !canSubmitDeletion">Delete vault</button>
        </div>
      </form>
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

.vault-dialog {
  width: min(460px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

.vault-dialog-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.vault-dialog-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.vault-dialog-icon.danger {
  border-color: var(--color-danger-border);
  background: var(--color-danger-background);
  color: var(--color-danger-text);
}

h2 {
  flex: 1;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  font-size: var(--font-size-15);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.body-text {
  margin: 0 0 12px;
  line-height: 1.45;
  color: var(--color-text-secondary);
}

.field-label {
  display: block;
  margin-bottom: 6px;
  font-size: var(--font-size-12);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.hint {
  margin: 6px 0 0;
  font-size: var(--font-size-12);
}

.member-list {
  margin: 0 0 16px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  list-style: none;
}

.member-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
}

.member-row + .member-row {
  border-top: 1px solid var(--color-border);
}

.member-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.member-name,
.member-email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-email,
.role {
  font-size: var(--font-size-12);
}

.choice-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 4px;
}

.choice {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  cursor: pointer;
}

.choice input[type="radio"] {
  flex: none;
  width: auto;
  margin: 0;
}

.choice-text {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.choice-text select {
  width: auto;
  min-width: 140px;
}

.confirm-name {
  margin-top: 14px;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 18px;
}
</style>
