<script setup lang="ts">
import { ref } from "vue";
import type { ItemAddress } from "../api";
import AppIcon from "./AppIcon.vue";

// AddressFields edits an address in place: an address item's, an identity's or a card's billing address. oldText is the
// one-line address an identity or card saved before it had these fields; it shows only when the item has one.
const props = defineProps<{
  address: ItemAddress;
  oldText?: string;
  oldLabel?: string;
}>();
const emit = defineEmits<{ "update:oldText": [value: string] }>();
// The old address stays on screen while it is edited, even when emptied, until the item is saved.
const isOldTextShown = ref(Boolean(props.oldText));

// moveOldTextToStreet keeps every character of the old address: it becomes the street, after one already typed.
function moveOldTextToStreet(): void {
  props.address.street = [props.address.street, props.oldText ?? ""]
    .filter((part) => part.trim() !== "")
    .join("\n");
  emit("update:oldText", "");
  isOldTextShown.value = false;
}
</script>

<template>
  <div class="address-fields" data-testid="address-fields">
    <label>
      <span>Street</span>
      <textarea
        v-model="address.street"
        name="street"
        rows="2"
        autocomplete="off"
        placeholder="Street and number, then an apartment or suite"
      ></textarea>
    </label>
    <div class="row card-row">
      <label>
        <span>City</span>
        <input v-model="address.city" name="city" autocomplete="off" />
      </label>
      <label>
        <span>Region</span>
        <input
          v-model="address.region"
          name="region"
          autocomplete="off"
          placeholder="State or province"
        />
      </label>
    </div>
    <div class="row card-row">
      <label :class="{ last: !isOldTextShown }">
        <span>Postal code</span>
        <input
          v-model="address.postalCode"
          name="postalCode"
          autocomplete="off"
        />
      </label>
      <label :class="{ last: !isOldTextShown }">
        <span>Country</span>
        <input
          v-model="address.country"
          name="country"
          autocomplete="off"
          placeholder="US, or United States"
        />
      </label>
    </div>
    <label v-if="isOldTextShown" class="last" data-testid="old-address">
      <span>{{ oldLabel ?? "Address" }} (old format)</span>
      <textarea
        :value="oldText"
        name="oldAddress"
        rows="2"
        @input="
          emit('update:oldText', ($event.target as HTMLTextAreaElement).value)
        "
      ></textarea>
      <button
        type="button"
        class="ghost use-as-street"
        data-testid="use-as-street"
        @click="moveOldTextToStreet"
      >
        <AppIcon name="edit" :size="12" /> Use as street
      </button>
    </label>
  </div>
</template>

<style scoped>
label.last {
  margin-bottom: 0;
}

.card-row {
  align-items: flex-start;
  gap: 12px;
}

.card-row label {
  flex: 1;
}

.use-as-street {
  margin-top: 4px;
  padding-left: 0;
  font-size: 12px;
  color: var(--color-text-muted);
}
</style>
