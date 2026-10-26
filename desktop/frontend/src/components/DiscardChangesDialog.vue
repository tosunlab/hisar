<script setup lang="ts">
import type { DiscardDraftPrompt } from "../store";
import AccountDialog from "./AccountDialog.vue";

const props = defineProps<{ prompt: DiscardDraftPrompt }>();
</script>

<template>
  <AccountDialog
    :title="props.prompt.title ? `Discard changes to “${props.prompt.title}”?` : 'Discard the new item?'"
    subtitle="What you typed is not saved."
    icon="edit"
    data-testid="discard-draft-dialog"
    @close="props.prompt.answer(false)"
  >
    <div class="row actions">
      <button type="button" class="ghost" data-testid="keep-editing" autofocus @click="props.prompt.answer(false)">Keep editing</button>
      <button type="button" class="danger-solid" data-testid="discard-draft" @click="props.prompt.answer(true)">Discard</button>
    </div>
  </AccountDialog>
</template>
