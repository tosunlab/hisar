import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import type { VaultSummary } from "../api";
import { createApiMock, createFavoriteItem, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", category: "", title: "", username: "", urls: [], password: "", notes: "", fields: [], publicKey: "", fingerprint: "", keyType: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const api = apiMock.current;

enableAutoUnmount(afterEach);

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 };
const workVault: VaultSummary = { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 0 };
const bank = createItem({ id: "bank", title: "Bank", username: "alice.bank", password: "b4nk" });
const github = createItem({ id: "github", title: "GitHub", password: "s3cret", isFavorite: true });

function pressKey(init: KeyboardEventInit): void {
  window.dispatchEvent(new KeyboardEvent("keydown", { cancelable: true, ...init }));
}

function mountView() {
  return mount(MainView, { attachTo: document.body });
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: [personalVault, workVault],
    selectedVaultID: "personal",
    listVaultID: "personal",
    allItem: [bank, github],
    selectedItem: null,
    allFavorite: [createFavoriteItem({ item: github })],
    isFavoritesSelected: false,
    editorMode: "view",
    editorDraft: null,
    discardDraftPrompt: null,
    searchQuery: "",
    selectedTag: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isSharingOpen: false,
  });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue([personalVault, workVault]);
  api.listAllItem.mockImplementation((vaultID: string) => Promise.resolve(vaultID === "personal" ? [bank, github] : []));
  api.getItem.mockImplementation((_vaultID: string, itemID: string) => Promise.resolve(itemID === "github" ? github : bank));
  api.listAllFavoriteItem.mockResolvedValue([createFavoriteItem({ item: github })]);
  api.copyField.mockResolvedValue(undefined);
});

describe("Keyboard navigation in the item list", () => {
  it("is a listbox whose rows are options marking the selected one", async () => {
    store.selectedItem = github;
    const wrapper = mountView();
    const list = wrapper.find('[role="listbox"]');
    expect(list.attributes("aria-label")).toBe("Personal");
    const allRow = list.findAll('[role="option"]');
    expect(allRow.map((row) => row.attributes("aria-selected"))).toEqual(["false", "true"]);
    expect(allRow.map((row) => row.attributes("tabindex"))).toEqual(["-1", "0"]);
  });

  it("makes the first item the only Tab entry when no item is selected and selects it on focus", async () => {
    const wrapper = mountView();
    const rows = wrapper.findAll('[role="option"]');
    expect(rows.map((row) => row.attributes("tabindex"))).toEqual(["0", "-1"]);
    (rows[0].element as HTMLElement).focus();
    await flushPromises();
    expect(store.selectedItem?.id).toBe("bank");
    expect(rows[0].attributes("aria-selected")).toBe("true");
    await rows[0].trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(rows.map((row) => row.attributes("tabindex"))).toEqual(["-1", "0"]);
    const event = new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true });
    rows[1].element.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
  });

  it("keeps one Tab entry in Favorites and copies the focused favorite from its own vault", async () => {
    store.isFavoritesSelected = true;
    store.allFavorite = [
      createFavoriteItem({ item: bank }),
      createFavoriteItem({ vaultId: "work", vaultName: "Work", item: github }),
    ];
    store.selectedVaultID = "work";
    store.selectedItem = github;
    const wrapper = mountView();
    const rows = wrapper.findAll('[role="option"]');
    expect(rows.map((row) => row.attributes("tabindex"))).toEqual(["-1", "0"]);
    (rows[1].element as HTMLElement).focus();
    pressKey({ key: "c", metaKey: true });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("work", "github", "username");
    await rows[1].trigger("keydown", { key: "ArrowUp" });
    await flushPromises();
    expect(store.selectedItem?.id).toBe("bank");
    expect(rows.map((row) => row.attributes("tabindex"))).toEqual(["0", "-1"]);
  });

  it("↓ from the search field enters the list at the first item", async () => {
    const wrapper = mountView();
    await wrapper.find('input[name="search"]').trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    const allRow = wrapper.findAll('[role="option"]');
    expect(document.activeElement).toBe(allRow[0].element);
    expect(store.selectedItem?.id).toBe("bank");
  });

  it("↓ and ↑ move the selection, and ↑ from the first row returns to the search field", async () => {
    const wrapper = mountView();
    const allRow = wrapper.findAll('[role="option"]');
    (allRow[0].element as HTMLElement).focus();
    await allRow[0].trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(document.activeElement).toBe(allRow[1].element);
    expect(store.selectedItem?.id).toBe("github");
    await allRow[1].trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(document.activeElement).toBe(allRow[1].element);
    await allRow[1].trigger("keydown", { key: "ArrowUp" });
    await flushPromises();
    expect(document.activeElement).toBe(allRow[0].element);
    expect(store.selectedItem?.id).toBe("bank");
    await allRow[0].trigger("keydown", { key: "ArrowUp" });
    expect(document.activeElement).toBe(wrapper.find('input[name="search"]').element);
  });

  it("Return opens the focused item in the editor", async () => {
    const wrapper = mountView();
    const row = wrapper.findAll('[role="option"]')[1];
    (row.element as HTMLElement).focus();
    await row.trigger("keydown", { key: "Enter" });
    await flushPromises();
    expect(store.selectedItem?.id).toBe("github");
    expect(store.editorMode).toBe("edit");
  });

  it("Return in a read-only vault only shows the item", async () => {
    store.allItem = [github];
    store.selectedVaultID = "work";
    store.listVaultID = "work";
    const wrapper = mountView();
    const row = wrapper.find('[role="option"]');
    (row.element as HTMLElement).focus();
    await row.trigger("keydown", { key: "Enter" });
    await flushPromises();
    expect(store.selectedItem?.id).toBe("github");
    expect(store.editorMode).toBe("view");
  });

  it("walks the favorites list the same way", async () => {
    store.isFavoritesSelected = true;
    const wrapper = mountView();
    expect(wrapper.find('[role="listbox"]').attributes("aria-label")).toBe("Favorites");
    await wrapper.find('input[name="search"]').trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(document.activeElement).toBe(wrapper.find('[role="option"]').element);
    expect(store.selectedItem?.id).toBe("github");
  });
});

describe("Copy shortcuts", () => {
  it("⌘⇧C copies the selected item's password from anywhere in the window", async () => {
    store.selectedItem = github;
    mountView();
    pressKey({ key: "C", metaKey: true, shiftKey: true });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "github", "password");
    expect(store.noticeMessage).toBe("Password copied. It clears in 90 seconds.");
  });

  it("⌘C copies the username while the list has the focus, and leaves text fields alone", async () => {
    store.selectedItem = bank;
    const wrapper = mountView();
    (wrapper.find('[role="option"]').element as HTMLElement).focus();
    pressKey({ key: "c", metaKey: true });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "bank", "username");
    (wrapper.find('input[name="search"]').element as HTMLElement).focus();
    pressKey({ key: "c", metaKey: true });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledTimes(1);
  });

  it("copies the focused row even before its detail selection finishes loading", async () => {
    store.selectedItem = github;
    api.getItem.mockReturnValue(new Promise(() => undefined));
    const wrapper = mountView();
    (wrapper.find('[role="option"]').element as HTMLElement).focus();
    pressKey({ key: "c", metaKey: true });
    pressKey({ key: "C", metaKey: true, shiftKey: true });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "bank", "username");
    expect(api.copyField).toHaveBeenCalledWith("personal", "bank", "password");
    expect(api.copyField).not.toHaveBeenCalledWith("personal", "github", expect.anything());
  });

  it("does nothing while an item is being edited", async () => {
    store.selectedItem = github;
    store.editorMode = "edit";
    mountView();
    pressKey({ key: "C", metaKey: true, shiftKey: true });
    await flushPromises();
    expect(api.copyField).not.toHaveBeenCalled();
  });

  it.each(["Bank", "no matches"])("clears a filtered-out selection without selecting a replacement for %s", async (query) => {
    store.selectedItem = github;
    const wrapper = mountView();
    await wrapper.find('input[name="search"]').setValue(query);
    pressKey({ key: "C", metaKey: true, shiftKey: true });
    await flushPromises();
    expect(store.selectedItem).toBeNull();
    expect(wrapper.text()).toContain("Select an item to see its details.");
    expect(api.copyField).not.toHaveBeenCalled();
    if (query === "Bank") {
      await wrapper.find('input[name="search"]').trigger("keydown", { key: "ArrowDown" });
      await flushPromises();
      expect(store.selectedItem?.id).toBe("bank");
    }
  });

  it("preserves the editor when a background filter changes and reconciles on return", async () => {
    store.selectedItem = github;
    store.editorMode = "edit";
    const wrapper = mountView();
    await wrapper.find('input[name="search"]').setValue("Bank");
    expect(store.selectedItem?.id).toBe("github");
    expect(store.editorMode).toBe("edit");
    expect(wrapper.findComponent({ name: "ItemEditor" }).exists()).toBe(true);
    store.editorMode = "view";
    await flushPromises();
    expect(store.selectedItem).toBeNull();
    expect(wrapper.text()).toContain("Select an item to see its details.");
  });

  it("lists both shortcuts when no item is selected", () => {
    const wrapper = mountView();
    const allShortcut = wrapper.findAll(".shortcut-list div").map((row) => `${row.find("kbd").text()} ${row.find("dd").text()}`);
    expect(allShortcut).toContain("⌘C Copy username");
    expect(allShortcut).toContain("⌘⇧C Copy password");
  });
});

describe("Vault shortcuts", () => {
  it("⌥↓ and ⌥↑ step through Favorites and the vaults", async () => {
    mountView();
    pressKey({ key: "ArrowDown", altKey: true });
    await flushPromises();
    expect(store.selectedVaultID).toBe("work");
    pressKey({ key: "ArrowDown", altKey: true });
    await flushPromises();
    expect(store.selectedVaultID).toBe("work");
    pressKey({ key: "ArrowUp", altKey: true });
    pressKey({ key: "ArrowUp", altKey: true });
    await flushPromises();
    expect(store.isFavoritesSelected).toBe(true);
  });

  it("stays put while an item is being edited", async () => {
    store.selectedItem = bank;
    store.editorMode = "edit";
    mountView();
    pressKey({ key: "ArrowDown", altKey: true });
    await flushPromises();
    expect(store.selectedVaultID).toBe("personal");
  });

  it("keeps ⌘N and ⌘F out of an open panel", async () => {
    store.isSharingOpen = true;
    const wrapper = mountView();
    pressKey({ key: "n", metaKey: true });
    pressKey({ key: "f", metaKey: true });
    await flushPromises();
    expect(store.editorMode).toBe("view");
    expect(document.activeElement).not.toBe(wrapper.find('input[name="search"]').element);
  });
});
