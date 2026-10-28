<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from "vue";
import { countPendingInvitations, isSettingsWindow, loadSettingsData, loadSSHAgentStatus, store } from "../store";
import {
  allSettingsSection,
  findAllMatchingRowID,
  loadSelectedSectionID,
  saveSelectedSectionID,
  settingsRowVisibilityKey,
  type SettingsSection,
  type SettingsSectionID,
} from "../settingsSections";
import { isApplicationCommand } from "../util";
import AccountSection from "./AccountSection.vue";
import AppIcon from "./AppIcon.vue";
import AppearanceSection from "./AppearanceSection.vue";
import BrowserExtensionSection from "./BrowserExtensionSection.vue";
import CodingAgentsSection from "./CodingAgentsSection.vue";
import CommandLineSection from "./CommandLineSection.vue";
import DevicesSection from "./DevicesSection.vue";
import ExportSection from "./ExportSection.vue";
import HelpSection from "./HelpSection.vue";
import ImportSection from "./ImportSection.vue";
import OpenAtLoginSection from "./OpenAtLoginSection.vue";
import OrganizationsSection from "./OrganizationsSection.vue";
import PeopleSection from "./PeopleSection.vue";
import SecuritySection from "./SecuritySection.vue";
import SSHAgentSection from "./SSHAgentSection.vue";
import UpdatesSection from "./UpdatesSection.vue";

const searchQuery = ref("");
const selectedSectionID = ref<SettingsSectionID>(loadSelectedSectionID());
const isInvitingPerson = ref(false);
const isSavingEmergencyKit = ref(false);
const searchInput = ref<HTMLInputElement | null>(null);
const sectionList = ref<HTMLElement | null>(null);
const sectionContent = ref<HTMLElement | null>(null);

const isSearching = computed(() => searchQuery.value.trim() !== "");
const allMatchingRowID = computed(() => findAllMatchingRowID(searchQuery.value));


function isRowAvailable(rowID: string): boolean {
  if (rowID === "received-invitations") {
    return store.shareInvitations.received.length > 0 || (store.shareInvitations.waiting ?? []).length > 0;
  }
  if (rowID === "sent-invitations") {
    return store.shareInvitations.sent.length > 0;
  }
  if (rowID === "organization-invitations") {
    return store.organizations.invitations.length > 0;
  }
  return true;
}

function isRowVisible(rowID: string): boolean {
  return !isSearching.value || allMatchingRowID.value.has(rowID);
}

provide(settingsRowVisibilityKey, isRowVisible);

const allVisibleSection = computed<SettingsSection[]>(() =>
  isSearching.value
    ? allSettingsSection.filter((section) => section.allRow.some((row) => allMatchingRowID.value.has(row.id) && isRowAvailable(row.id)))
    : allSettingsSection,
);
const displayedSection = computed(() => allVisibleSection.value.find((section) => section.id === selectedSectionID.value) ?? null);

function selectSection(sectionID: SettingsSectionID): void {
  if (sectionID !== selectedSectionID.value) {
    isInvitingPerson.value = false;
    if (sectionContent.value) {
      sectionContent.value.scrollTop = 0;
    }
  }
  selectedSectionID.value = sectionID;
  saveSelectedSectionID(sectionID);
}

async function reviewAgentApprovals(destination: "agent-sessions" | "agent-grants"): Promise<void> {
  searchQuery.value = "";
  selectSection("developer");
  await nextTick();
  const section = sectionContent.value?.querySelector<HTMLElement>(`#${destination}`);
  section?.scrollIntoView?.({ block: "start" });
  section?.focus();
}

// The form opens once Security is active: activating a section clears the Security forms it left behind.
async function showEmergencyKit(): Promise<void> {
  searchQuery.value = "";
  selectSection("security");
  await nextTick();
  isSavingEmergencyKit.value = true;
}

watch(allVisibleSection, (allSection) => {
  if (allSection.length > 0 && !allSection.some((section) => section.id === selectedSectionID.value)) {
    selectSection(allSection[0].id);
  }
});

async function focusSelectedSection(): Promise<void> {
  await nextTick();
  sectionList.value?.querySelector<HTMLButtonElement>('[aria-current="page"]')?.focus();
}

function moveSelection(event: KeyboardEvent): void {
  const allSection = allVisibleSection.value;
  if ((event.key !== "ArrowDown" && event.key !== "ArrowUp") || allSection.length === 0) {
    return;
  }
  event.preventDefault();
  const currentIndex = allSection.findIndex((section) => section.id === selectedSectionID.value);
  const nextIndex = Math.min(allSection.length - 1, Math.max(0, currentIndex + (event.key === "ArrowDown" ? 1 : -1)));
  selectSection(allSection[nextIndex].id);
  focusSelectedSection();
}

function moveFromSearch(event: KeyboardEvent): void {
  if (event.key === "ArrowDown" && displayedSection.value) {
    event.preventDefault();
    focusSelectedSection();
  }
}

function startInvitingPerson(): void {
  isInvitingPerson.value = true;
}

function closeSettings(): void {
  store.isSettingsOpen = false;
  store.selectedOrganizationID = "";
  store.errorMessage = "";
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    if (event.target === searchInput.value && searchQuery.value !== "") {
      searchQuery.value = "";
      return;
    }
    closeSettings();
  } else if (isApplicationCommand(event) && !event.altKey && !event.shiftKey && event.key.toLowerCase() === "f") {
    event.preventDefault();
    searchInput.value?.focus();
    searchInput.value?.select();
  }
}

onMounted(() => {
  window.addEventListener("keydown", handleKeydown);
  loadSSHAgentStatus();
  loadSettingsData();
});
onBeforeUnmount(() => window.removeEventListener("keydown", handleKeydown));
</script>

<template>
  <div class="overlay" :class="{ window: isSettingsWindow }" @click.self="closeSettings">
    <div class="card settings" :role="isSettingsWindow ? undefined : 'dialog'" :aria-modal="isSettingsWindow ? undefined : 'true'" aria-labelledby="settings-title">
      <nav class="settings-sidebar" aria-label="Settings sections">
        <div class="settings-search">
          <AppIcon name="search" :size="14" />
          <input
            ref="searchInput"
            v-model="searchQuery"
            type="search"
            name="settingsSearch"
            aria-label="Search settings"
            placeholder="Search settings"
            autocomplete="off"
            spellcheck="false"
            data-testid="settings-search"
            @keydown="moveFromSearch"
          />
        </div>
        <div ref="sectionList" class="section-list" @keydown="moveSelection">
          <button
            v-for="section in allVisibleSection"
            :key="section.id"
            type="button"
            class="section-button"
            :aria-current="section.id === displayedSection?.id ? 'page' : undefined"
            :tabindex="section.id === displayedSection?.id ? 0 : -1"
            :data-testid="`settings-section-${section.id}`"
            @click="selectSection(section.id)"
          >
            <AppIcon :name="section.icon" :size="16" />
            <span class="section-name">{{ section.label }}</span>
            <span
              v-if="countPendingInvitations(section.id) > 0"
              class="badge"
              :data-testid="`${section.id}-invitation-badge`"
              :aria-label="countPendingInvitations(section.id) === 1 ? '1 invitation' : `${countPendingInvitations(section.id)} invitations`"
              >{{ countPendingInvitations(section.id) }}</span
            >
          </button>
          <p v-if="allVisibleSection.length === 0" class="muted no-section">No results</p>
        </div>
      </nav>

      <div class="settings-main">
        <header class="settings-header">
          <h2 id="settings-title">{{ displayedSection?.label ?? "Settings" }}</h2>
          <button v-if="displayedSection?.id === 'people' && !isInvitingPerson" v-show="isRowVisible('invite')" data-testid="start-invite" @click="startInvitingPerson">
            <AppIcon name="plus" :size="14" />Invite to Hisar
          </button>
          <button v-if="!isSettingsWindow" class="ghost icon-button" aria-label="Close settings" @click="closeSettings"><AppIcon name="close" /></button>
        </header>

        <div ref="sectionContent" class="settings-content">
          <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

          <div v-if="!displayedSection" class="no-results" data-testid="settings-no-results">
            <AppIcon name="search" :size="20" />
            <p>No settings match “{{ searchQuery.trim() }}”.</p>
          </div>

          <div v-show="displayedSection?.id === 'general'" data-testid="settings-page-general">
            <AppearanceSection />
            <OpenAtLoginSection />
            <UpdatesSection />
          </div>
          <div v-show="displayedSection?.id === 'security'" data-testid="settings-page-security">
            <SecuritySection v-model:is-saving-emergency-kit="isSavingEmergencyKit" :is-active="displayedSection?.id === 'security'" />
          </div>
          <div v-show="displayedSection?.id === 'ssh'" data-testid="settings-page-ssh">
            <SSHAgentSection />
          </div>
          <div v-show="displayedSection?.id === 'developer'" data-testid="settings-page-developer">
            <CommandLineSection />
            <CodingAgentsSection @review-approvals="reviewAgentApprovals" />
          </div>
          <div v-show="displayedSection?.id === 'browser'" data-testid="settings-page-browser">
            <BrowserExtensionSection />
          </div>
          <div v-show="displayedSection?.id === 'people'" data-testid="settings-page-people">
            <PeopleSection v-model:is-inviting="isInvitingPerson" />
          </div>
          <div v-show="displayedSection?.id === 'organizations'" data-testid="settings-page-organizations">
            <OrganizationsSection />
          </div>
          <div v-show="displayedSection?.id === 'devices'" data-testid="settings-page-devices">
            <DevicesSection />
          </div>
          <div v-show="displayedSection?.id === 'export'" data-testid="settings-page-export">
            <ImportSection />
            <ExportSection />
          </div>
          <div v-show="displayedSection?.id === 'account'" data-testid="settings-page-account">
            <AccountSection @save-emergency-kit="showEmergencyKit" />
          </div>
          <div v-show="displayedSection?.id === 'help'" data-testid="settings-page-help">
            <HelpSection />
          </div>
        </div>
      </div>
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

.settings {
  display: flex;
  width: min(880px, 100%);
  height: min(620px, 100%);
  overflow: hidden;
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
}

/* In its own window (macOS), Settings fills the window, which has its own title bar and close button. */
.overlay.window {
  padding: 0;
  background: var(--color-panel);
}

.overlay.window .settings {
  width: 100%;
  height: 100%;
  border: none;
  border-radius: 0;
  box-shadow: none;
}

.settings-sidebar {
  display: flex;
  flex: none;
  flex-direction: column;
  gap: 14px;
  width: 208px;
  padding: 16px 12px;
  overflow-y: auto;
  border-right: 1px solid var(--color-border);
  background: var(--color-sidebar);
}

.settings-search {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 30px;
  flex: none;
  padding: 0 9px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-muted);
}

.settings-search:focus-within {
  border-color: var(--color-focus);
}

.settings-search input {
  flex: 1;
  min-width: 0;
  padding: 0;
  border: none;
  background: transparent;
  box-shadow: none;
  color: var(--color-text);
}

.settings-search input::-webkit-search-cancel-button {
  filter: grayscale(1);
}

.section-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.section-button {
  flex: none;
  justify-content: flex-start;
  gap: 9px;
  width: 100%;
  padding: 6px 9px;
  border: none;
  border-radius: var(--radius-small);
  background: transparent;
  color: var(--color-text-secondary);
  font-weight: 500;
  text-align: left;
}

.section-button:hover {
  background: var(--color-hover);
  color: var(--color-text);
}

.section-button[aria-current="page"] {
  background: var(--color-selected);
  color: var(--color-text);
}

.section-button[aria-current="page"] :deep(svg) {
  color: var(--color-accent-text);
}

.section-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.badge {
  min-width: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--color-primary);
  color: var(--color-primary-text);
  font-size: var(--font-size-11);
  font-weight: 600;
  line-height: 18px;
  text-align: center;
}

.no-section {
  margin: 4px 9px;
  font-size: var(--font-size-12);
}

.settings-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.settings-header {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
  min-height: 58px;
  padding: 14px 16px 10px 24px;
}

.settings-header h2 {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: var(--font-size-15);
  font-weight: 600;
}

.settings-header button {
  flex: none;
}

.settings-content {
  flex: 1;
  min-height: 0;
  padding: 6px 24px 24px;
  overflow-y: auto;
}

.settings-content > .error {
  margin: 0 0 12px;
}

.no-results {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 80px 0;
  color: var(--color-text-muted);
}

.no-results p {
  margin: 0;
}
</style>
