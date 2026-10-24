<script setup lang="ts">
import { onMounted, ref } from "vue";
import { defaultPageSettings, readPageSettings, writePageSetting, type PageSettings } from "../../shared/settings";

const emit = defineEmits<{ close: [] }>();

const settings = ref<PageSettings>({ ...defaultPageSettings });
const errorMessage = ref("");

async function updateSetting(key: keyof PageSettings, event: Event): Promise<void> {
  const value = (event.target as HTMLInputElement).checked;
  settings.value = { ...settings.value, [key]: value };
  try {
    await writePageSetting(chrome.storage.local, key, value);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error);
  }
}

onMounted(async () => {
  settings.value = await readPageSettings(chrome.storage.local);
});
</script>

<template>
  <section class="panel" data-testid="settings-view">
    <h1>Settings</h1>
    <p class="subtitle">These settings apply to this browser only.</p>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <label class="setting">
      <input
        type="checkbox"
        :checked="settings.isInlineMenuEnabled"
        data-testid="setting-inline-menu"
        @change="updateSetting('isInlineMenuEnabled', $event)"
      />
      <span>
        <span class="label">Show Hisar in login fields</span>
        <span class="hint muted">An icon in username and password fields opens your matching logins.</span>
      </span>
    </label>
    <label class="setting" :class="{ disabled: !settings.isInlineMenuEnabled }">
      <input
        type="checkbox"
        :checked="settings.isInlineMenuOpenedOnFocus"
        :disabled="!settings.isInlineMenuEnabled"
        data-testid="setting-open-on-focus"
        @change="updateSetting('isInlineMenuOpenedOnFocus', $event)"
      />
      <span>
        <span class="label">Open the menu when a login field is focused</span>
        <span class="hint muted">Otherwise click the icon or press the down arrow.</span>
      </span>
    </label>
    <label class="setting">
      <input
        type="checkbox"
        :checked="settings.isFillDiagnosticsEnabled"
        data-testid="setting-fill-diagnostics"
        @change="updateSetting('isFillDiagnosticsEnabled', $event)"
      />
      <span>
        <span class="label">Fill diagnostics</span>
        <span class="hint muted">Writes the fields Hisar found and what each fill did to the page's console, without their values. For reporting a fill that did not work.</span>
      </span>
    </label>
    <button type="button" class="full-width" data-testid="settings-done" @click="emit('close')">Done</button>
  </section>
</template>

<style scoped>
.setting {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin-bottom: 14px;
  cursor: pointer;
}

.setting input {
  margin-top: 2px;
  flex: none;
}

.setting.disabled {
  opacity: 0.5;
  cursor: default;
}

.label {
  display: block;
  font-weight: 500;
}

.hint {
  display: block;
  font-size: 12px;
}
</style>
