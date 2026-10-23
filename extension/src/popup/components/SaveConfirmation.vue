<script setup lang="ts">
import { computed, ref } from "vue";
import type { SaveOfferAction, SaveOfferView } from "../../shared/messages";
import { describeError, sendToBackground } from "../api";

const props = defineProps<{ offer: SaveOfferView; tabId: number }>();
const emit = defineEmits<{ finished: []; failed: [message: string] }>();
const title = ref(props.offer.defaultTitle ?? props.offer.host);
const username = ref(props.offer.username);
const vaultID = ref(props.offer.allVault?.[0]?.vaultID ?? "");
const isBusy = ref(false);
const errorMessage = ref("");
const destination = computed(() => props.offer.kind === "update" ? props.offer.vaultName : props.offer.allVault?.find(vault => vault.vaultID === vaultID.value)?.name ?? "Personal");
const isPersonal = computed(() => props.offer.kind === "update" ? props.offer.isPersonal === true : !props.offer.allVault?.length || props.offer.allVault.find(vault => vault.vaultID === vaultID.value)?.isPersonal === true);

async function act(action: SaveOfferAction): Promise<void> {
  isBusy.value = true;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.actOnSaveOffer", tabId: props.tabId, offerID: props.offer.offerID, action,
      ...(props.offer.kind === "save" && action === "save" ? { edits: { title: title.value, username: username.value, vaultID: vaultID.value } } : {}),
    });
    emit("finished");
  } catch (error) {
    errorMessage.value = `${describeError(error)}. Submit the login again to retry.`;
    emit("failed", errorMessage.value);
  } finally {
    isBusy.value = false;
  }
}
</script>

<template>
  <section class="panel save-confirmation" data-testid="save-confirmation">
    <h2>{{ offer.kind === 'update' ? 'Update this login?' : 'Save this login?' }}</h2>
    <p>Site: <strong>{{ offer.host }}</strong></p>
    <template v-if="offer.kind === 'update'">
      <p>Login: <strong>{{ offer.title }}</strong></p>
      <p>Username: <strong>{{ offer.username || '(empty)' }}</strong></p>
    </template>
    <template v-else>
      <label>Title <input v-model="title" data-testid="save-title" :disabled="isBusy" /></label>
      <label>Username <input v-model="username" data-testid="save-username" :disabled="isBusy" /></label>
      <label v-if="offer.allVault?.length">Vault
        <select v-model="vaultID" data-testid="save-vault" :disabled="isBusy">
          <option v-for="vault in offer.allVault" :key="vault.vaultID" :value="vault.vaultID">{{ vault.name }}</option>
        </select>
      </label>
    </template>
    <p>Destination: <strong>{{ destination || 'Selected login vault' }}</strong></p>
    <p class="muted">{{ isPersonal ? 'Only you have access in your Personal vault.' : 'Everyone with access to this vault can read this password.' }}</p>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <div class="row">
      <button class="ghost" :disabled="isBusy" data-testid="save-never" @click="act('never')">Never for this site</button>
      <button class="ghost" :disabled="isBusy" data-testid="save-decline" @click="act('not-now')">Not now</button>
      <button class="primary" :disabled="isBusy" data-testid="save-confirm" @click="act('save')">{{ offer.kind === 'update' ? 'Update password' : 'Save login' }}</button>
    </div>
  </section>
</template>

<style scoped>
.save-confirmation { border-bottom: 1px solid var(--color-border); }
label { display: grid; gap: 4px; margin: 8px 0; }
.row { flex-wrap: wrap; gap: 8px; margin-top: 12px; }
</style>
