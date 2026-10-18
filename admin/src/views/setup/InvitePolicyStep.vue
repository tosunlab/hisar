<script setup lang="ts">
import { ref } from "vue";
import { getErrorMessage } from "../../api";
import { invitePolicyLabels } from "../../sections/settings/settings";
import { finishInvitePolicyStep, type InvitePolicy } from "./setup";

const emit = defineEmits<{ done: [] }>();

const invitePolicy = ref<InvitePolicy>("anyone");
const isSaving = ref(false);
const errorMessage = ref("");

async function submit(): Promise<void> {
  isSaving.value = true;
  errorMessage.value = "";
  try {
    await finishInvitePolicyStep(invitePolicy.value);
    emit("done");
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isSaving.value = false;
  }
}
</script>

<template>
  <form class="step" @submit.prevent="submit">
    <h1>Choose who can invite people</h1>
    <p class="subtitle">Who can invite people to the Hisar server from the apps. You can change it later in Settings → Sign-up.</p>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <fieldset class="choices">
      <legend>Who can invite people</legend>
      <label v-for="(label, policy) in invitePolicyLabels" :key="policy" class="choice">
        <input v-model="invitePolicy" type="radio" name="invitePolicy" :value="policy" />
        <span>{{ label }}</span>
      </label>
    </fieldset>
    <button class="primary submit" type="submit" :disabled="isSaving">{{ isSaving ? "Finishing…" : "Finish setup" }}</button>
  </form>
</template>
