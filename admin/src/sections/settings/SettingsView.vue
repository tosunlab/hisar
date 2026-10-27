<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { getErrorMessage } from "../../api";
import {
  getSettings,
  invitePolicyLabels,
  isEmailFromEnvironment,
  isSecretSet,
  listChanges,
  needsPasswordAgain,
  saveSettings,
  sendTestEmail,
  settingGroups,
  toFormValue,
  type AdminSettings,
  type FormValues,
  type SettingGroup,
} from "./settings";

const settings = ref<AdminSettings | null>(null);
const form = reactive<FormValues>({});
const loadError = ref("");
const savingGroupId = ref("");
const saveErrors = reactive<Record<string, string>>({});
const savedGroupId = ref("");

const isTestEmailOpen = ref(false);
const testEmailRecipient = ref("");
const isSendingTestEmail = ref(false);
const testEmailResult = ref<{ isSent: boolean; message: string } | null>(null);

const changesByGroup = computed(() =>
  Object.fromEntries(settingGroups.map((group) => [group.id, settings.value ? listChanges(group, settings.value, form) : {}])),
);

function resetForm(groups: readonly SettingGroup[], loaded: AdminSettings): void {
  for (const group of groups) {
    for (const field of group.fields) {
      form[field.name] = toFormValue(field, loaded[field.name]);
    }
  }
}

onMounted(async () => {
  try {
    const loaded = await getSettings();
    resetForm(settingGroups, loaded);
    settings.value = loaded;
  } catch (error) {
    loadError.value = getErrorMessage(error);
  }
});

// save stores one group and keeps unsaved edits in the others.
async function save(group: SettingGroup): Promise<void> {
  savingGroupId.value = group.id;
  saveErrors[group.id] = "";
  savedGroupId.value = "";
  try {
    const saved = await saveSettings(changesByGroup.value[group.id]);
    resetForm([group], saved);
    settings.value = saved;
    savedGroupId.value = group.id;
  } catch (error) {
    saveErrors[group.id] = getErrorMessage(error);
  } finally {
    savingGroupId.value = "";
  }
}

async function submitTestEmail(): Promise<void> {
  isSendingTestEmail.value = true;
  testEmailResult.value = null;
  try {
    await sendTestEmail(testEmailRecipient.value);
    testEmailResult.value = { isSent: true, message: `Test email sent to ${testEmailRecipient.value}.` };
  } catch (error) {
    testEmailResult.value = { isSent: false, message: getErrorMessage(error) };
  } finally {
    isSendingTestEmail.value = false;
  }
}
</script>

<template>
  <section class="settings">
    <h1>Settings</h1>
    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p>
    <p v-else-if="!settings" class="muted">Loading…</p>
    <template v-else>
      <section v-for="group in settingGroups" :key="group.id" class="group" :aria-labelledby="`group-${group.id}`">
        <form class="group-form" @submit.prevent="save(group)">
          <h2 :id="`group-${group.id}`">{{ group.title }}</h2>
          <p class="description">{{ group.description }}</p>
          <p v-if="group.id === 'email' && isEmailFromEnvironment(settings)" class="hint locked">
            Email is set by HISAR_SMTP_* variables on the server. Change them there and restart the server.
          </p>
          <template v-for="field in group.fields" :key="field.name">
            <fieldset v-if="field.kind === 'invitePolicy'" class="choices">
              <legend>{{ field.label }}</legend>
              <label v-for="(label, policy) in invitePolicyLabels" :key="policy" class="choice">
                <input v-model="form[field.name]" type="radio" :name="field.name" :value="policy" />
                <span>{{ label }}</span>
              </label>
            </fieldset>
            <label v-else-if="field.kind === 'checkbox'" class="choice checkbox">
              <input v-model="form[field.name]" :name="field.name" type="checkbox" :disabled="settings[field.name].source === 'environment'" />
              <span class="checkbox-text">
                <span>{{ field.label }}</span>
                <span v-if="settings[field.name].source === 'environment'" class="hint locked">Set by {{ settings[field.name].environmentVariable }}</span>
                <span v-else-if="field.hint" class="hint">{{ field.hint }}</span>
              </span>
            </label>
            <label v-else class="field">
              <span>{{ field.label }}</span>
              <input
                v-model="form[field.name]"
                :name="field.name"
                :type="field.kind"
                :readonly="settings[field.name].source === 'environment'"
                :placeholder="field.kind === 'password' && isSecretSet(settings[field.name]) ? 'Saved; type to replace' : field.placeholder"
                :autocomplete="field.kind === 'password' ? 'new-password' : 'off'"
                spellcheck="false"
              />
              <span v-if="settings[field.name].source === 'environment'" class="hint locked">Set by {{ settings[field.name].environmentVariable }}</span>
              <span v-else-if="field.kind === 'password' && needsPasswordAgain(settings, form)" class="hint locked">
                Enter the password again: changing the host, port or username clears the saved one.
              </span>
              <span v-else-if="field.hint" class="hint">{{ field.hint }}</span>
            </label>
          </template>
          <p v-if="saveErrors[group.id]" class="error" role="alert">{{ saveErrors[group.id] }}</p>
          <div class="actions">
            <button v-if="group.id === 'email'" type="button" @click="isTestEmailOpen = true">Send test email…</button>
            <span v-if="Object.keys(changesByGroup[group.id]).length > 0" class="status">Unsaved changes</span>
            <span v-else-if="savedGroupId === group.id" class="status" role="status">Saved</span>
            <button type="submit" class="primary" :disabled="Object.keys(changesByGroup[group.id]).length === 0 || savingGroupId !== ''">
              {{ savingGroupId === group.id ? "Saving…" : "Save" }}
            </button>
          </div>
        </form>
        <form v-if="group.id === 'email' && isTestEmailOpen" class="test-email" role="dialog" aria-label="Send test email" @submit.prevent="submitTestEmail">
          <p class="description">Hisar sends one email with the saved settings.</p>
          <label class="field">
            <span>Send to</span>
            <input v-model="testEmailRecipient" name="testEmailRecipient" type="email" placeholder="you@example.com" required />
          </label>
          <p v-if="testEmailResult" :class="testEmailResult.isSent ? 'notice' : 'error'" role="alert">{{ testEmailResult.message }}</p>
          <div class="actions">
            <button type="button" class="ghost" @click="isTestEmailOpen = false">Close</button>
            <button type="submit" class="primary" :disabled="isSendingTestEmail">{{ isSendingTestEmail ? "Sending…" : "Send" }}</button>
          </div>
        </form>
      </section>
    </template>
  </section>
</template>

<style scoped>
h1 {
  margin: 0 0 20px;
  font-size: 22px;
  font-weight: 650;
}

h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 650;
}

.settings {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 640px;
}

.group,
.group-form,
.test-email {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.group {
  padding: 20px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface);
}

.test-email {
  padding: 16px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-background);
}

.description,
.muted {
  margin: 0;
  color: var(--color-text-muted);
}

.hint {
  display: block;
  margin-top: 6px;
  color: var(--color-text-muted);
  font-size: 12px;
}

.hint.locked {
  font-weight: 500;
}

input[readonly] {
  background: var(--color-background);
  color: var(--color-text-secondary);
  cursor: default;
}

.choices {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  border: none;
}

legend {
  margin-bottom: 6px;
  padding: 0;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 500;
}

.choice {
  display: flex;
  align-items: center;
  gap: 8px;
}

.choice input {
  width: auto;
}

.checkbox {
  align-items: flex-start;
}

.checkbox input {
  margin-top: 3px;
}

.checkbox-text {
  display: flex;
  flex-direction: column;
}

.checkbox-text .hint {
  margin-top: 2px;
}

.actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.actions > button:first-child:not(.primary) {
  margin-right: auto;
}

.status {
  color: var(--color-text-muted);
  font-size: 12px;
}

.notice {
  margin: 0;
  padding: 9px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}
</style>
