<script setup lang="ts">
import { onMounted } from "vue";
import { useSettingsRowVisibility } from "../settingsSections";
import { allAppearanceOption, allThemeColorOption, chooseTheme, themeStore } from "../theme";
import { loadWebsiteIconSetting, setWebsiteIconsEnabled, websiteIconState } from "../websiteicons";
import AppIcon from "./AppIcon.vue";
import { getThisDeviceName } from "../util";

const isRowVisible = useSettingsRowVisibility();

onMounted(loadWebsiteIconSetting);
</script>

<template>
  <section v-show="isRowVisible('appearance') || isRowVisible('theme-color') || isRowVisible('website-icons')" class="settings-block" data-testid="appearance-section">
    <p class="section-label">Appearance</p>
    <div class="settings-group">
      <div v-show="isRowVisible('appearance')" class="setting">
        <div class="setting-text">
          <strong id="appearance-label">Appearance</strong>
          <p class="muted">Choose System to follow the system's light or dark appearance.</p>
        </div>
        <div class="segmented" role="radiogroup" aria-labelledby="appearance-label">
          <button
            v-for="option in allAppearanceOption"
            :key="option.id"
            type="button"
            role="radio"
            :aria-checked="themeStore.appearance === option.id"
            :data-testid="`appearance-${option.id}`"
            @click="chooseTheme(option.id, themeStore.color)"
          >
            {{ option.label }}
          </button>
        </div>
      </div>
      <div v-show="isRowVisible('theme-color')" class="setting color-setting">
        <div class="setting-text">
          <strong id="theme-color-label">Color</strong>
          <p class="muted">Tints the sidebar, selection, buttons and approval dialogs on {{ getThisDeviceName() }}.</p>
        </div>
        <div class="swatch-list" role="radiogroup" aria-labelledby="theme-color-label">
          <button
            v-for="option in allThemeColorOption"
            :key="option.id"
            type="button"
            role="radio"
            class="swatch-button"
            :aria-checked="themeStore.color === option.id"
            :data-testid="`theme-color-${option.id}`"
            @click="chooseTheme(themeStore.appearance, option.id)"
          >
            <span class="swatch" :data-color="option.id" :data-appearance="themeStore.appearance" aria-hidden="true">
              <span class="swatch-sidebar">
                <span class="swatch-selection"></span>
                <span class="swatch-line short"></span>
              </span>
              <span class="swatch-content">
                <span class="swatch-line strong"></span>
                <span class="swatch-line short"></span>
                <span class="swatch-accent"></span>
              </span>
              <span v-if="themeStore.color === option.id" class="swatch-check"><AppIcon name="check" :size="10" /></span>
            </span>
            <span class="swatch-name">{{ option.label }}</span>
          </button>
        </div>
      </div>
      <label v-show="isRowVisible('website-icons')" class="setting">
        <div class="setting-text">
          <strong>Show website icons</strong>
          <p class="muted">Logins without an icon of their own show their website's. Hisar on {{ getThisDeviceName() }} asks each website for it directly, never a third party.</p>
        </div>
        <input
          class="switch"
          type="checkbox"
          name="websiteIcons"
          :checked="websiteIconState.isEnabled"
          data-testid="website-icons-switch"
          @change="setWebsiteIconsEnabled(($event.target as HTMLInputElement).checked)"
        />
      </label>
    </div>
    <p v-if="themeStore.errorMessage" class="error settings-note" role="alert" data-testid="theme-error">{{ themeStore.errorMessage }}</p>
    <p v-if="websiteIconState.errorMessage" class="error settings-note" role="alert" data-testid="website-icons-error">{{ websiteIconState.errorMessage }}</p>
  </section>
</template>

<style scoped>
.segmented {
  display: inline-flex;
  flex: none;
  padding: 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
}

.segmented button {
  min-width: 64px;
  padding: 4px 12px;
  border: none;
  border-radius: var(--radius-small);
  background: transparent;
  color: var(--color-text-secondary);
}

.segmented button:hover:not([aria-checked="true"]) {
  background: var(--color-hover);
  color: var(--color-text);
}

.segmented button[aria-checked="true"] {
  background: var(--color-surface);
  box-shadow: 0 1px 2px var(--color-shadow), inset 0 0 0 1px var(--color-border-strong);
  color: var(--color-text);
  font-weight: 600;
}

.setting.color-setting {
  flex-direction: column;
  align-items: stretch;
}

.swatch-list {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  padding-bottom: 4px;
}

.swatch-button {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 7px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
}

.swatch-button:hover:not(:disabled) {
  background: transparent;
  color: var(--color-text);
}

.swatch {
  position: relative;
  display: flex;
  height: 62px;
  overflow: hidden;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  outline: 2px solid transparent;
  outline-offset: 2px;
  background: var(--color-background);
}

.swatch-button[aria-checked="true"] .swatch {
  outline-color: var(--color-primary);
}

.swatch-sidebar {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 32%;
  padding: 10px 5px;
  border-right: 1px solid var(--color-border);
  background: var(--color-sidebar);
}

.swatch-selection {
  height: 7px;
  border-radius: 3px;
  background: var(--color-selected);
}

.swatch-content {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 4px;
  padding: 10px 7px;
}

.swatch-line {
  height: 4px;
  width: 55%;
  border-radius: 2px;
  background: var(--color-border-hover);
}

.swatch-line.strong {
  width: 80%;
  background: var(--color-text);
}

.swatch-sidebar .swatch-line.short {
  width: 70%;
}

.swatch-accent {
  width: 42%;
  height: 9px;
  margin-top: 6px;
  border-radius: 3px;
  background: var(--color-primary);
}

.swatch-check {
  position: absolute;
  top: 5px;
  right: 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--color-primary);
  color: var(--color-primary-text);
}

.swatch-name {
  font-size: var(--font-size-12);
  text-align: center;
}

.swatch-button[aria-checked="true"] .swatch-name {
  color: var(--color-text);
  font-weight: 600;
}
</style>
