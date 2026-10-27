<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { ItemIcon } from "../api";
import { getInitials, getPresetGlyph, getTileColor } from "../util";
import { getWebsiteIcon, websiteIconState } from "../websiteicons";
import AppIcon, { type IconName } from "./AppIcon.vue";

// icon is the category's icon; itemIcon the one the user chose, which wins; websiteURL a login's website, whose icon
// replaces the initials once it loads.
const props = defineProps<{ title: string; size?: number; icon?: IconName; itemIcon?: ItemIcon | null; websiteURL?: string }>();

const tileSize = computed(() => props.size ?? 32);
const tileElement = ref<HTMLElement | null>(null);
const isVisible = ref(typeof IntersectionObserver === "undefined");
let observer: IntersectionObserver | null = null;

onMounted(() => {
  if (isVisible.value || !tileElement.value) {
    return;
  }
  observer = new IntersectionObserver((allEntry) => {
    if (allEntry.some((entry) => entry.isIntersecting)) {
      isVisible.value = true;
      observer?.disconnect();
      observer = null;
    }
  });
  observer.observe(tileElement.value);
});

onBeforeUnmount(() => observer?.disconnect());

// websiteIcon fills in once the backend has the icon; a new generation (the setting changed, or the app locked) asks again.
const websiteIcon = computed(() => {
  void websiteIconState.generation;
  return websiteIconState.isEnabled && isVisible.value && props.websiteURL && !props.itemIcon ? getWebsiteIcon(props.websiteURL) : null;
});

const imageSource = computed(() => {
  if (props.itemIcon?.image) {
    return `data:image/png;base64,${props.itemIcon.image}`;
  }
  return props.itemIcon ? "" : (websiteIcon.value?.value ?? "");
});
const presetGlyph = computed(() => (imageSource.value ? undefined : getPresetGlyph(props.itemIcon?.preset)));
const glyph = computed(() => presetGlyph.value ?? (imageSource.value ? undefined : props.icon));
</script>

<template>
  <span
    ref="tileElement"
    class="tile"
    :class="{ 'icon-tile': glyph, 'image-tile': imageSource, 'preset-tile': presetGlyph }"
    :style="{
      width: `${tileSize}px`,
      height: `${tileSize}px`,
      fontSize: tileSize > 32 ? 'var(--font-size-22)' : tileSize < 32 ? 'var(--font-size-11)' : 'var(--font-size-12)',
      ...(glyph || imageSource ? {} : getTileColor(title)),
    }"
    :data-icon="glyph"
    :data-icon-color="presetGlyph ? itemIcon?.color || undefined : undefined"
    :data-image="imageSource ? (itemIcon?.image ? 'chosen' : 'website') : undefined"
    aria-hidden="true"
    ><img v-if="imageSource" :src="imageSource" alt="" draggable="false" /><AppIcon
      v-else-if="glyph"
      :name="glyph"
      :size="Math.round(tileSize * 0.5)"
    /><template v-else>{{ getInitials(title) }}</template></span
  >
</template>

<style scoped>
.tile {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 25%;
  font-weight: 700;
  letter-spacing: 0.02em;
  box-shadow: inset 0 0 0 1px var(--color-tile-edge);
}

.icon-tile {
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
  box-shadow: inset 0 0 0 1px var(--color-border-strong);
}

.image-tile {
  background: #fff;
}

.image-tile img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

/* Preset colours: the glyph in the hue, on a tint of it that follows light and dark. */
.preset-tile[data-icon-color] {
  background: color-mix(in srgb, var(--icon-hue) 18%, var(--color-surface-raised));
  color: var(--icon-hue);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--icon-hue) 30%, transparent);
}
</style>
