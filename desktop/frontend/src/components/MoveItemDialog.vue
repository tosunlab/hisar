<script setup lang="ts">
import { computed, ref } from "vue";
import { moveItem, store } from "../store";
import { groupVaultDestinations } from "../vaultDestinations";
import AccountDialog from "./AccountDialog.vue";

const props = defineProps<{ vaultID: string; itemID: string; title: string }>();
const emit = defineEmits<{ close: [] }>();

// allTargetVault are the other vaults the user may add items to.
const allTargetVault = computed(() => store.allVault.filter((vault) => vault.id !== props.vaultID && vault.role !== "reader"));
const targetVaultID = ref(allTargetVault.value[0]?.id ?? "");
const sourceVaultName = computed(() => store.allVault.find((vault) => vault.id === props.vaultID)?.name ?? "this vault");
const destinationGroups = computed(() => groupVaultDestinations(allTargetVault.value, store.organizations.organizations));
const selectedDestination = computed(() => destinationGroups.value.flatMap((group) => group.destinations).find((destination) => destination.id === targetVaultID.value));

async function submitMove(): Promise<void> {
  if (targetVaultID.value && (await moveItem(props.vaultID, props.itemID, targetVaultID.value))) {
    emit("close");
  }
}
</script>

<template>
  <AccountDialog
    :title="`Move “${title}”`"
    :subtitle="`Everyone with access to the vault you choose can see it. hisar:// references that name ${sourceVaultName} need the new vault's name.`"
    icon="vault"
    :is-closable="!store.isBusy"
    data-testid="move-item-dialog"
    @close="emit('close')"
  >
    <form @submit.prevent="submitMove">
      <label>
        <span>Move to</span>
        <select v-model="targetVaultID" name="targetVault" data-testid="move-item-target">
          <optgroup v-for="group in destinationGroups" :key="group.id" :label="group.name">
            <option v-for="destination in group.destinations" :key="destination.id" :value="destination.id">{{ destination.label }}</option>
          </optgroup>
        </select>
      </label>
      <p data-testid="move-item-context">{{ selectedDestination?.context }}</p>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <div class="row actions">
        <button type="button" class="ghost" :disabled="store.isBusy" @click="emit('close')">Cancel</button>
        <button class="primary" type="submit" :disabled="store.isBusy || !targetVaultID" data-testid="move-item-confirm">Move</button>
      </div>
    </form>
  </AccountDialog>
</template>
