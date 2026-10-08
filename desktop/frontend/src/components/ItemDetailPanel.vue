<script setup lang="ts">
import { ref, watch } from "vue";
import { copyField, deleteItem, startEditingItem, store } from "../store";

defineProps<{ canEdit: boolean }>();

const isPasswordRevealed = ref(false);
const allRevealedFieldName = ref<string[]>([]);
const isConfirmingDelete = ref(false);

watch(
  () => store.selectedItem?.id,
  () => {
    isPasswordRevealed.value = false;
    allRevealedFieldName.value = [];
    isConfirmingDelete.value = false;
  },
);

function toggleFieldReveal(fieldName: string): void {
  allRevealedFieldName.value = allRevealedFieldName.value.includes(fieldName)
    ? allRevealedFieldName.value.filter((name) => name !== fieldName)
    : [...allRevealedFieldName.value, fieldName];
}

function maskSecret(secret: string): string {
  return "•".repeat(Math.min(Math.max(secret.length, 8), 24));
}
</script>

<template>
  <article v-if="store.selectedItem" class="item-detail">
    <header class="row">
      <h2>{{ store.selectedItem.title }}</h2>
      <div class="spacer"></div>
      <template v-if="canEdit">
        <button @click="startEditingItem">Edit</button>
        <button v-if="!isConfirmingDelete" class="danger" @click="isConfirmingDelete = true">Delete</button>
        <template v-else>
          <button @click="isConfirmingDelete = false">Cancel</button>
          <button class="danger" data-testid="confirm-delete" @click="deleteItem(store.selectedItem.id)">Delete item</button>
        </template>
      </template>
    </header>

    <dl>
      <div v-if="store.selectedItem.username" class="field">
        <dt>Username</dt>
        <dd class="row">
          <span class="value">{{ store.selectedItem.username }}</span>
          <button @click="copyField('username', 'Username')">Copy</button>
        </dd>
      </div>

      <div v-if="store.selectedItem.password" class="field">
        <dt>Password</dt>
        <dd class="row">
          <span class="value monospace" data-testid="password-value">
            {{ isPasswordRevealed ? store.selectedItem.password : maskSecret(store.selectedItem.password) }}
          </span>
          <button @click="isPasswordRevealed = !isPasswordRevealed">{{ isPasswordRevealed ? "Hide" : "Reveal" }}</button>
          <button @click="copyField('password', 'Password')">Copy</button>
        </dd>
      </div>

      <div v-for="link in store.selectedItem.urls" :key="link" class="field">
        <dt>Website</dt>
        <dd class="row">
          <span class="value">{{ link }}</span>
        </dd>
      </div>

      <div v-for="field in store.selectedItem.fields" :key="field.name" class="field">
        <dt>{{ field.name }}</dt>
        <dd class="row">
          <span class="value" :class="{ monospace: field.concealed }">
            {{ field.concealed && !allRevealedFieldName.includes(field.name) ? maskSecret(field.value) : field.value }}
          </span>
          <button v-if="field.concealed" @click="toggleFieldReveal(field.name)">
            {{ allRevealedFieldName.includes(field.name) ? "Hide" : "Reveal" }}
          </button>
          <button @click="copyField(field.name, field.name)">Copy</button>
        </dd>
      </div>

      <div v-if="store.selectedItem.notes" class="field">
        <dt>Notes</dt>
        <dd class="notes">{{ store.selectedItem.notes }}</dd>
      </div>
    </dl>
  </article>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
}

header {
  margin-bottom: 20px;
}

dl {
  margin: 0;
}

.field {
  padding: 10px 0;
  border-bottom: 1px solid var(--color-border);
}

dt {
  margin-bottom: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-muted);
}

dd {
  margin: 0;
}

.value {
  flex: 1;
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.notes {
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

dd button {
  padding: 4px 10px;
  font-size: 12px;
}
</style>
