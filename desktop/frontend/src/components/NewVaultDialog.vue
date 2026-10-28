<script setup lang="ts">
import { ref } from "vue";
import { useModalFocus } from "../modalFocus";

export interface TeamChoice {
  key: string;
  organizationID: string;
  teamID: string;
  teamName: string;
  label: string;
}

defineProps<{ allTeamChoice: TeamChoice[]; isLoadingTeams: boolean; teamsError: string; errorMessage: string; canSubmit: boolean; isBusy: boolean }>();
const name = defineModel<string>("name", { required: true });
const where = defineModel<string>("where", { required: true });
const emit = defineEmits<{ close: []; retry: []; submit: [] }>();
const panel = ref<HTMLElement | null>(null);
useModalFocus(panel, () => emit("close"));
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div ref="panel" class="card new-vault-dialog" role="dialog" aria-modal="true" aria-labelledby="new-vault-title" tabindex="-1" data-testid="new-vault-dialog">
      <h2 id="new-vault-title">New vault</h2>
      <form data-testid="new-vault-form" @submit.prevent="emit('submit')">
        <label for="new-vault-name">Name</label>
        <input id="new-vault-name" v-model="name" name="vaultName" placeholder="Vault name" autofocus />
        <div class="destination-area">
          <fieldset data-testid="new-vault-where" :aria-busy="isLoadingTeams" :disabled="isLoadingTeams || !!teamsError">
            <legend>Where</legend>
            <div :class="{ 'destinations-pending': isLoadingTeams || !!teamsError }">
              <label class="destination">
                <input v-model="where" type="radio" name="vaultWhere" value="" />
                <span><span class="destination-title">Your own vault</span><small class="muted">Share it later</small></span>
              </label>
              <label v-for="choice in allTeamChoice" :key="choice.key" class="destination">
                <input v-model="where" type="radio" name="vaultWhere" :value="choice.key" />
                <span><span class="destination-title">{{ choice.label }}</span><small class="muted">{{ choice.teamName }}'s members get this vault. Manage it in Settings, Organizations.</small></span>
              </label>
            </div>
          </fieldset>
          <p v-if="isLoadingTeams" class="destination-status" role="status" data-testid="new-vault-teams-loading"><progress aria-label="Loading teams" /> Loading teams…</p>
          <p v-else-if="teamsError" class="destination-status" role="alert" data-testid="new-vault-teams-error">Couldn't load teams. <button type="button" class="ghost" @click="emit('retry')">Retry</button></p>
        </div>
        <p v-if="errorMessage" class="error" role="alert" data-testid="new-vault-error">{{ errorMessage }}</p>
        <div class="row actions">
          <button type="button" class="ghost" @click="emit('close')">Cancel</button>
          <button class="primary" type="submit" data-testid="create-vault" :disabled="isBusy || !canSubmit">Create</button>
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

.new-vault-dialog {
  width: min(460px, 100%);
  max-height: 100%;
  overflow-y: auto;
  padding: 20px 22px;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

h2 {
  margin: 0 0 18px;
  font-size: var(--font-size-15);
}

.destination-area {
  display: grid;
  margin: 16px 0;
}

.destination-status {
  grid-area: 1 / 1;
  align-self: start;
  z-index: 1;
  margin: 42px 12px 12px;
}

.destinations-pending {
  visibility: hidden;
}

fieldset {
  min-width: 0;
  grid-area: 1 / 1;
  height: 280px;
  overflow-y: auto;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.destination {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 0;
  padding: 10px 0;
  cursor: pointer;
}

.destination input {
  flex: none;
  margin-top: 3px;
}

.destination > span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.destination-title,
small {
  display: block;
}

.destination-title {
  margin-bottom: 4px;
}

.actions {
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
