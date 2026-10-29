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
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const api = apiMock.current;
enableAutoUnmount(afterEach);

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"), allVault: [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 }],
    listVaultID: "personal", selectedVaultID: "personal", allItem: [createItem({ id: "one", title: "One" })], selectedItem: null,
    isFavoritesSelected: false, editorMode: "view", editorDraft: null, discardDraftPrompt: null,
    isAllItemsSelected: false, allItemReference: [], allItemsError: "",
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
    await flushPromises();
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
    store.listVaultID = "another";
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

describe("Repeat search", () => {
  it("selects the existing query on Command+F and retains ArrowDown entry", async () => {
    store.searchQuery = "previous";
    const wrapper = mount(MainView, { attachTo: document.body });
    const search = wrapper.get('input[name="search"]');
    const input = search.element as HTMLInputElement;
    (wrapper.get('[aria-label="New item"]').element as HTMLElement).focus();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", metaKey: true, bubbles: true }));
    expect(document.activeElement).toBe(input);
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 8]);
    input.value = input.value.slice(0, input.selectionStart ?? 0) + "One" + input.value.slice(input.selectionEnd ?? 0);
    await search.trigger("input");
    expect(store.searchQuery).toBe("One");
    await search.trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(document.activeElement).toBe(wrapper.get('[role="option"]').element);
    expect(store.selectedItem?.id).toBe("one");
  });
});

describe("Reader access", () => {
  it("explains disabled creation accessibly and keeps useful shortcut guidance", async () => {
    store.allVault[0] = { ...store.allVault[0], name: "Work", type: "shared", role: "reader" };
    const wrapper = mount(MainView, { attachTo: document.body });
    const description = "You can view items in Work. You cannot add or change items.";
    expect(wrapper.get(".reader-badge").text()).toBe("Reader");
    expect(wrapper.get("#reader-access").text()).toBe(description);
    const control = wrapper.get('[aria-label="New item"]');
    expect(control.attributes("disabled")).toBeDefined();
    expect(control.attributes("aria-describedby")).toBe("reader-access");
    expect(control.attributes("title")).toBe(description);
    const keyboardExplanation = wrapper.get('[aria-label="New item unavailable"]');
    (keyboardExplanation.element as HTMLElement).focus();
    expect(document.activeElement).toBe(keyboardExplanation.element);
    expect(keyboardExplanation.attributes("aria-describedby")).toBe("reader-access");
    expect(wrapper.get(".shortcut-list").text()).not.toContain("New item");
    expect(wrapper.get(".shortcut-list").text()).toContain("Copy username");
    expect(wrapper.get(".shortcut-list").text()).toContain("Search");
    store.allVault[0].role = "owner";
    await flushPromises();
    expect(wrapper.find(".reader-badge").exists()).toBe(false);
    expect(wrapper.get(".shortcut-list").text()).toContain("New item");
    expect(control.attributes("disabled")).toBeUndefined();
    store.isFavoritesSelected = true;
    store.allFavorite = [];
    await flushPromises();
    expect(wrapper.get(".shortcut-list").text()).not.toContain("New item");
    expect(wrapper.get(".shortcut-list").text()).toContain("Search");
    expect(wrapper.get(".shortcut-list").text()).toContain("Copy password");
  });
});

describe("Platform command modifiers", () => {
  afterEach(() => Object.defineProperty(navigator, "platform", { configurable: true, value: "MacIntel" }));
  for (const [platform, modifier, accepted] of [
    ["MacIntel", "metaKey", true], ["MacIntel", "ctrlKey", false],
    ["Linux x86_64", "ctrlKey", true], ["Linux x86_64", "metaKey", false],
  ] as const) {
    it(`${platform} ${modifier} ${accepted ? "runs" : "does not claim"} Search, Settings, Settings search, Save and Lock`, async () => {
      Object.defineProperty(navigator, "platform", { configurable: true, value: platform });
      const view = mount(MainView, { attachTo: document.body });
      const other = document.createElement("input");
      document.body.append(other);
      other.focus();
      const find = new KeyboardEvent("keydown", { key: "f", [modifier]: true, bubbles: true, cancelable: true });
      other.dispatchEvent(find);
      expect(find.defaultPrevented).toBe(accepted);
      expect(document.activeElement === view.get('input[name="search"]').element).toBe(accepted);
      if (accepted) api.showSettingsWindow.mockResolvedValueOnce(true);
      const openSettingsWindow = new KeyboardEvent("keydown", { key: ",", [modifier]: true, bubbles: true, cancelable: true });
      other.dispatchEvent(openSettingsWindow);
      await flushPromises();
      expect(openSettingsWindow.defaultPrevented).toBe(accepted);
      expect(api.showSettingsWindow).toHaveBeenCalledTimes(accepted ? 1 : 0);
      expect(store.isSettingsOpen).toBe(false);
      view.unmount();
      const settings = mount(SettingsPanel, { attachTo: document.body });
      await flushPromises();
      other.focus();
      const settingsFind = new KeyboardEvent("keydown", { key: "f", [modifier]: true, bubbles: true, cancelable: true });
      other.dispatchEvent(settingsFind);
      expect(settingsFind.defaultPrevented).toBe(accepted);
      expect(document.activeElement === settings.get('[data-testid="settings-search"]').element).toBe(accepted);
      settings.unmount();
      store.selectedItem = createItem({ title: "Valid title" });
      store.editorMode = "edit";
      api.saveItem.mockResolvedValue(store.selectedItem);
      api.listAllItem.mockResolvedValue([store.selectedItem]);
      const editor = mount(ItemEditor, { attachTo: document.body });
      await editor.get('input[name="title"]').trigger("keydown", { key: "s", [modifier]: true });
      await flushPromises();
      expect(api.saveItem).toHaveBeenCalledTimes(accepted ? 1 : 0);
      editor.unmount();
      store.isSettingsOpen = true;
      store.vaultDialog = { mode: "blocked", blockedReason: "Dialog open", actions: { vault: store.allVault[0] } } as typeof store.vaultDialog;
      api.lock.mockResolvedValue(createStatus("locked"));
      const lockedView = mount(MainView, { attachTo: document.body, global: { stubs: { VaultDialog: true } } });
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "l", [modifier]: true, bubbles: true, cancelable: true }));
      await flushPromises();
      expect(api.lock).toHaveBeenCalledTimes(accepted ? 1 : 0);
      lockedView.unmount();
      other.remove();
    });
  }
});

describe("Browse menu focus restoration", () => {
  it("returns Escape focus to keyboard triggers and pointer-invoked rows", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    const trigger = wrapper.get('[data-testid="vault-actions"]');
    await trigger.trigger("keydown", { key: "Enter" });
    await flushPromises();
    await wrapper.get('[role="menuitem"]').trigger("keydown", { key: "Escape" });
    expect(document.activeElement).toBe(trigger.element);
    const row = wrapper.get('[role="option"]');
    (wrapper.get('input[name="search"]').element as HTMLElement).focus();
    await row.trigger("contextmenu");
    await flushPromises();
    await wrapper.get('[role="menuitem"]').trigger("keydown", { key: "Escape" });
    expect(document.activeElement).toBe(row.element);
    await row.trigger("contextmenu");
    await flushPromises();
    await wrapper.get('[data-testid="context-menu-copy-username"]').trigger("click");
    expect(document.activeElement).toBe(row.element);
    expect(api.copyField).toHaveBeenCalledWith("personal", "one", "username");
  });
  it("does not restore a disconnected invoker", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    const row = wrapper.get('[role="option"]');
    await row.trigger("contextmenu");
    await flushPromises();
    store.allItem = [];
    await flushPromises();
    const focus = vi.spyOn(row.element as HTMLElement, "focus");
    await wrapper.get('[role="menuitem"]').trigger("keydown", { key: "Escape" });
    expect(focus).not.toHaveBeenCalled();
  });
  it("preserves outside pointer destinations and dialog action focus", async () => {
    const invoker = document.createElement("button");
    const destination = document.createElement("input");
    document.body.append(invoker, destination);
    const focus = vi.spyOn(invoker, "focus");
    const wrapper = mount(ContextMenu, { attachTo: document.body, props: { label: "Actions", x: 0, y: 0, invoker, allAction: [
      { id: "dialog", label: "Open dialog", icon: "edit", opensDestination: true, run: () => destination.focus() },
    ] } });
    await flushPromises();
    await wrapper.get('[role="menuitem"]').trigger("click");
    expect(document.activeElement).toBe(destination);
    expect(focus).not.toHaveBeenCalled();
    destination.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    destination.focus();
    await flushPromises();
    expect(wrapper.emitted("close")?.at(-1)).toEqual(["outside"]);
    expect(document.activeElement).toBe(destination);
    expect(focus).not.toHaveBeenCalled();
    invoker.remove();
    destination.remove();
  });
});
