<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { api, getErrorMessage, type ItemIcon } from "../api";
import { allIconColor, allIconPreset } from "../util";
import AppIcon from "./AppIcon.vue";
import ItemTile from "./ItemTile.vue";

// IconPicker chooses an item's icon: Automatic (null), a preset in a color, or an uploaded image. The choice is saved
// with the item.
const props = defineProps<{ modelValue: ItemIcon | null | undefined; title: string; websiteURL: string }>();
const emit = defineEmits<{ "update:modelValue": [icon: ItemIcon | null]; close: [] }>();

// Matches itemicon.MaxSourceSize: the backend refuses larger files anyway.
const maxUploadSize = 5 << 20;

const picker = ref<HTMLElement | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
const isPreparing = ref(false);
const errorMessage = ref("");
// color is kept while the user tries presets, so a preset keeps the color chosen before it.
const color = ref(props.modelValue?.color ?? "");

function formatName(id: string): string {
  return id.charAt(0).toUpperCase() + id.slice(1);
}

function choosePreset(preset: string): void {
  emit("update:modelValue", { preset, color: color.value });
}

function chooseColor(chosen: string): void {
  color.value = chosen;
  if (props.modelValue?.preset && !props.modelValue.image) {
    emit("update:modelValue", { preset: props.modelValue.preset, color: chosen });
  }
}

function chooseAutomatic(): void {
  errorMessage.value = "";
  emit("update:modelValue", null);
}

function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, ""));
    reader.onerror = () => reject(new Error("could not read the file"));
    reader.readAsDataURL(file);
  });
}

async function uploadImage(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) {
    return;
  }
  errorMessage.value = "";
  if (file.size > maxUploadSize) {
    errorMessage.value = "Choose an image under 5 MB.";
    return;
  }
  isPreparing.value = true;
  try {
    const image = await api.prepareItemIcon(await readFileAsBase64(file));
    emit("update:modelValue", { image });
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isPreparing.value = false;
  }
}

function closeOnOutside(event: PointerEvent): void {
  const target = event.target as Node | null;
  if (target && !picker.value?.contains(target) && !(target as Element).closest?.("[data-icon-picker-anchor]")) {
    emit("close");
  }
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    event.stopPropagation();
    emit("close");
  }
}

onMounted(() => {
  window.addEventListener("pointerdown", closeOnOutside);
  picker.value?.querySelector<HTMLButtonElement>("button[aria-pressed='true']")?.focus();
});
onBeforeUnmount(() => window.removeEventListener("pointerdown", closeOnOutside));
</script>

<template>
  <div ref="picker" class="icon-picker" role="dialog" aria-label="Item icon" data-testid="icon-picker" @keydown="closeOnEscape">
    <div class="picker-row">
      <button
        type="button"
        class="automatic"
        :aria-pressed="!modelValue"
        data-testid="icon-automatic"
        @click="chooseAutomatic"
      >
        <ItemTile :title="title || '?'" :size="28" :websiteURL="websiteURL" />
        <span>
          <strong>Automatic</strong>
          <span class="muted">{{ websiteURL ? "The website's icon, or the initials" : "The initials or the item type's icon" }}</span>
        </span>
      </button>
      <button type="button" class="upload" :disabled="isPreparing" data-testid="icon-upload" @click="fileInput?.click()">
        <AppIcon name="import" :size="14" /> {{ isPreparing ? "Preparing…" : "Upload image…" }}
      </button>
      <input
        ref="fileInput"
        type="file"
        accept="image/png,image/jpeg,image/gif,image/webp"
        hidden
        data-testid="icon-file"
        @change="uploadImage"
      />
    </div>
    <p class="section-label">Icons</p>
    <div class="preset-grid" role="group" aria-label="Preset icons">
      <button
        v-for="preset in allIconPreset"
        :key="preset"
        type="button"
        class="preset"
        :aria-pressed="!modelValue?.image && modelValue?.preset === preset"
        :aria-label="formatName(preset)"
        :title="formatName(preset)"
        :data-testid="`icon-preset-${preset}`"
        @click="choosePreset(preset)"
      >
        <ItemTile :title="preset" :size="28" :item-icon="{ preset, color }" />
      </button>
    </div>
    <p class="section-label">Color</p>
    <div class="color-row" role="group" aria-label="Icon color">
      <button
        type="button"
        class="swatch neutral"
        :aria-pressed="color === ''"
        aria-label="No color"
        title="No color"
        data-testid="icon-color-none"
        @click="chooseColor('')"
      ></button>
      <button
        v-for="hue in allIconColor"
        :key="hue"
        type="button"
        class="swatch"
        :data-icon-color="hue"
        :aria-pressed="color === hue"
        :aria-label="formatName(hue)"
        :title="formatName(hue)"
        :data-testid="`icon-color-${hue}`"
        @click="chooseColor(hue)"
      ></button>
    </div>
    <p v-if="errorMessage" class="error" role="alert" data-testid="icon-error">{{ errorMessage }}</p>
  </div>
</template>

<style scoped>
.icon-picker {
  position: absolute;
  z-index: 30;
  top: 60px;
  left: 0;
  width: 352px;
  max-width: calc(100vw - 32px);
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  box-shadow: 0 12px 32px var(--color-shadow);
}

.picker-row {
  display: flex;
  gap: 8px;
  align-items: stretch;
}

.automatic {
  display: flex;
  flex: 1;
  gap: 8px;
  align-items: center;
  min-width: 0;
  padding: 6px 8px;
  text-align: left;
}

.automatic > span:last-child {
  display: flex;
  flex-direction: column;
  min-width: 0;
  font-size: var(--font-size-12);
}

.automatic .muted {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.upload {
  display: inline-flex;
  flex: none;
  gap: 6px;
  align-items: center;
  font-size: var(--font-size-12);
}

.section-label {
  margin: 12px 0 6px;
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 4px;
}

.preset {
  display: inline-flex;
  width: 100%;
  min-width: 0;
  min-height: 0;
  align-items: center;
  justify-content: center;
  padding: 3px;
  border: 1px solid transparent;
  background: none;
}

.preset[aria-pressed="true"],
.automatic[aria-pressed="true"] {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 1px var(--color-accent);
}

.color-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.swatch {
  width: 22px;
  height: 22px;
  padding: 0;
  border: 2px solid var(--color-surface);
  border-radius: 50%;
  background: var(--icon-hue);
  box-shadow: 0 0 0 1px var(--color-border-strong);
}

.swatch.neutral {
  background: var(--color-surface-raised);
}

.swatch[aria-pressed="true"] {
  box-shadow: 0 0 0 2px var(--color-accent);
}

.error {
  margin: 10px 0 0;
}
</style>
