import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
vi.mock("../api", async () => {
  const { createApiMock } = await import("../testing");
  apiMock.current = createApiMock();
  return { api: apiMock.current, getErrorMessage: (e: unknown) => String(e), createEmptyItem: () => ({ id: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }) };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));
const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const ContextMenu = (await import("./ContextMenu.vue")).default;
const api = apiMock.current;
enableAutoUnmount(afterEach);

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"), allVault: [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 }],
    selectedVaultID: "personal", allItem: [createItem({ id: "one", title: "One" })], selectedItem: null,
    isFavoritesSelected: false, editorMode: "view", editorDraft: null, discardDraftPrompt: null,
    searchQuery: "", selectedTag: "", isBusy: false, isSettingsOpen: false, isSharingOpen: false,
    isImportOpen: false, isShareLinksOpen: false, vaultDialog: null, itemDetailDialog: null, shareLinkDialog: null,
  });
  api.getItem.mockResolvedValue(createItem({ id: "one", title: "One" }));
});

describe("Browse menu keyboard navigation", () => {
  for (const trigger of ['[aria-label="New item"]', '[data-testid="vault-actions"]']) {
    for (const key of ["Enter", " ", "ArrowDown"]) {
      it(`opens ${trigger} with ${key} and supports arrows, Escape and Tab`, async () => {
        const wrapper = mount(MainView, { attachTo: document.body });
        await wrapper.get(trigger).trigger("keydown", { key });
        await flushPromises();
        const buttons = wrapper.findAll('[role="menu"] [role="menuitem"]');
        expect(document.activeElement).toBe(buttons[0].element);
        await buttons[0].trigger("keydown", { key: "ArrowDown" });
        expect(document.activeElement).toBe(buttons[1].element);
        await buttons[1].trigger("keydown", { key: "ArrowUp" });
        expect(document.activeElement).toBe(buttons[0].element);
        await buttons[0].trigger("keydown", { key: "Escape" });
        expect(wrapper.find('[role="menu"]').exists()).toBe(false);
        await wrapper.get(trigger).trigger("keydown", { key: "ArrowDown" });
        await flushPromises();
        await wrapper.get('[role="menuitem"]').trigger("keydown", { key: "Tab" });
        expect(wrapper.find('[role="menu"]').exists()).toBe(false);
      });
    }
  }
  it("skips disabled actions when moving focus", async () => {
    const wrapper = mount(ContextMenu, { attachTo: document.body, props: { label: "Actions", x: 0, y: 0, allAction: [
      { id: "one", label: "One", icon: "copy", run: vi.fn() },
      { id: "two", label: "Two", icon: "copy", disabled: true, run: vi.fn() },
      { id: "three", label: "Three", icon: "copy", run: vi.fn() },
    ] } });
    await wrapper.get('[data-testid="context-menu-one"]').trigger("keydown", { key: "ArrowDown" });
    expect(document.activeElement).toBe(wrapper.get('[data-testid="context-menu-three"]').element);
  });
});

describe("Type filtering", () => {
  it("counts non-empty types, combines search and tags, clears and resets on scope changes", async () => {
    store.allItem = [
      createItem({ id: "login", title: "Work login", tags: ["work"] }),
      createItem({ id: "card", title: "Work card", category: "credit-card", tags: ["work"] }),
      createItem({ id: "other-card", title: "Personal card", category: "credit-card", tags: ["personal"] }),
    ];
    const wrapper = mount(MainView);
    const filter = wrapper.get('select[aria-label="Type"]');
    expect(filter.findAll("option").map((option) => option.text())).toEqual(["All types (3)", "Logins (1)", "Credit cards (2)"]);
    await filter.setValue("credit-card");
    expect(wrapper.findAll('[role="option"]')).toHaveLength(2);
    store.searchQuery = "Work";
    store.selectedTag = "work";
    await flushPromises();
    expect(wrapper.findAll('[role="option"]').map((row) => row.find(".item-title").text())).toEqual(["Work card"]);
    await wrapper.get('[aria-label="Clear type filter"]').trigger("click");
    expect(wrapper.find('[data-testid="type-filter"]').exists()).toBe(false);
    expect(wrapper.findAll('[role="option"]')).toHaveLength(2);
    await filter.setValue("credit-card");
    store.selectedVaultID = "another";
    store.allItem = [];
    await flushPromises();
    expect((filter.element as HTMLSelectElement).value).toBe("");
    expect(filter.findAll("option").map((option) => option.text())).toEqual(["All types (0)"]);
  });
});

describe("Persistent sidebar controls", () => {
  it("keeps account actions outside the scrolling vault and tag navigation", async () => {
    store.allVault = Array.from({ length: 30 }, (_, index) => ({ id: `vault-${index}`, name: `Vault ${index}`, type: "shared", role: "owner", itemCount: 1 }));
    store.allItem = Array.from({ length: 30 }, (_, index) => createItem({ id: `item-${index}`, tags: [`tag-${index}`] }));
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.get('[data-testid="sidebar-navigation"]').findAll(".vault")).toHaveLength(63);
    for (const label of ["Settings", "Lock"]) {
      const control = wrapper.get(`[aria-label="${label}"]`);
      expect(control.element.closest(".sidebar-navigation")).toBeNull();
      (control.element as HTMLElement).focus();
      expect(document.activeElement).toBe(control.element);
    }
    expect(wrapper.get(".account-text").element.closest(".sidebar-navigation")).toBeNull();
  });
});
