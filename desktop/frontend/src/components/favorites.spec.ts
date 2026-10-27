import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
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

const { store, loadAllVault, syncInBackground } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const api = apiMock.current;

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 };
const workVault: VaultSummary = { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 1 };
const github = createItem({ id: "github", title: "GitHub", isFavorite: true });
const bank = createItem({ id: "bank", title: "Bank", username: "alice.bank" });
const vpn = createItem({ id: "vpn", title: "Office VPN", username: "alper", isFavorite: true });
const githubFavorite = createFavoriteItem({ item: github });
const vpnFavorite = createFavoriteItem({ vaultId: "work", vaultName: "Work", vaultType: "shared", item: vpn });

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: [personalVault, workVault],
    selectedVaultID: "personal",
    allItem: [bank, github],
    selectedItem: null,
    allFavorite: [githubFavorite, vpnFavorite],
    isFavoritesSelected: false,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
  api.listAllVault.mockResolvedValue([personalVault, workVault]);
  api.listAllItem.mockResolvedValue([bank, github]);
  api.listAllFavoriteItem.mockResolvedValue([githubFavorite, vpnFavorite]);
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  document.body.innerHTML = "";
});

describe("Favorites in the sidebar", () => {
  it("sits above the vaults with a count", () => {
    const wrapper = mount(MainView);
    const allNav = wrapper.findAll("nav");
    expect(allNav[0].attributes("aria-label")).toBe("Favorites");
    expect(allNav[1].attributes("aria-label")).toBe("Vaults");
    expect(wrapper.find('[data-testid="favorites-count"]').text()).toBe("2");
  });

  it("lists the favorites of every vault with the vault each is in", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('[data-testid="open-favorites"]').trigger("click");
    await flushPromises();
    expect(store.isFavoritesSelected).toBe(true);
    expect(wrapper.find(".list-title h2").text()).toBe("Favorites");
    expect(wrapper.find('button[aria-label="New item"]').exists()).toBe(false);
    const allRow = wrapper.findAll('[data-testid="favorites-list"] .item');
    expect(allRow.map((row) => row.find(".item-title").text())).toEqual(["GitHub", "Office VPN"]);
    expect(wrapper.findAll('[data-testid="favorite-vault"]').map((label) => label.text())).toEqual(["Personal", "Work"]);
    expect(wrapper.find('[data-testid="open-favorites"]').classes()).toContain("selected");
    expect(wrapper.find('[data-testid="vault-personal"]').classes()).not.toContain("selected");
  });

  it("opens a favorite from another vault with that vault's actions", async () => {
    api.getItem.mockResolvedValue({ ...vpn, password: "vpn-password" });
    const wrapper = mount(MainView);
    await wrapper.find('[data-testid="open-favorites"]').trigger("click");
    await flushPromises();
    await wrapper.findAll('[data-testid="favorites-list"] .item')[1].trigger("click");
    await flushPromises();
    expect(api.getItem).toHaveBeenCalledWith("work", "vpn");
    expect(store.selectedVaultID).toBe("work");
    expect(store.isFavoritesSelected).toBe(true);
    expect(wrapper.findAll('[data-testid="favorites-list"] button.item').map((item) => item.attributes("aria-current"))).toEqual([undefined, "true"]);
    expect(wrapper.find('[role="listbox"], [role="option"]').exists()).toBe(false);
    expect(wrapper.find(".item-detail .badge").text()).toContain("Work");
    expect(wrapper.find(".item-detail").text()).not.toContain("Edit");
  });

  it("filters favorites with the search field", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('[data-testid="open-favorites"]').trigger("click");
    await flushPromises();
    await wrapper.find('input[name="search"]').setValue("vpn");
    expect(wrapper.findAll('[data-testid="favorites-list"] .item-title').map((title) => title.text())).toEqual(["Office VPN"]);
  });

  it("says how to add a favorite when there are none", async () => {
    api.listAllFavoriteItem.mockResolvedValue([]);
    const wrapper = mount(MainView);
    await wrapper.find('[data-testid="open-favorites"]').trigger("click");
    await flushPromises();
    const empty = wrapper.find('[data-testid="favorites-empty"]');
    expect(empty.text()).toContain("No favorites yet");
    expect(empty.text()).toContain("Only you see your favorites.");
    expect(wrapper.find('[data-testid="favorites-count"]').exists()).toBe(false);
  });

  it("leaves Favorites when a vault is chosen", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('[data-testid="open-favorites"]').trigger("click");
    await flushPromises();
    await wrapper.find('[data-testid="vault-personal"]').trigger("click");
    await flushPromises();
    expect(store.isFavoritesSelected).toBe(false);
    expect(wrapper.find(".list-title h2").text()).toBe("Personal");
  });

  it("marks favorites in a vault's list", () => {
    const wrapper = mount(MainView);
    const allRow = wrapper.findAll(".list ul .item");
    expect(allRow.map((row) => row.find(".item-star").exists())).toEqual([false, true]);
  });
});

describe("starring an item", () => {
  it("stars the item from the detail and updates the list and Favorites", async () => {
    store.selectedItem = { ...bank };
    api.listAllFavoriteItem.mockResolvedValue([createFavoriteItem({ item: { ...bank, isFavorite: true } }), githubFavorite, vpnFavorite]);
    const wrapper = mount(MainView);
    const toggle = wrapper.find('[data-testid="favorite-toggle"]');
    expect(toggle.attributes("aria-label")).toBe("Add to Favorites");
    await toggle.trigger("click");
    await flushPromises();
    expect(api.setItemFavorite).toHaveBeenCalledWith("personal", "bank", true);
    expect(store.selectedItem?.isFavorite).toBe(true);
    expect(store.allItem.find((vaultItem) => vaultItem.id === "bank")?.isFavorite).toBe(true);
    expect(wrapper.find('[data-testid="favorite-toggle"]').attributes("aria-label")).toBe("Remove from Favorites");
    expect(wrapper.find('[data-testid="favorites-count"]').text()).toBe("3");
    expect(store.noticeMessage).toBe("Added to Favorites");
  });

  it("lets a reader star an item they can't edit", async () => {
    store.selectedVaultID = "work";
    store.allItem = [vpn];
    store.selectedItem = { ...vpn, isFavorite: false };
    const wrapper = mount(MainView);
    expect(wrapper.find(".item-detail").text()).not.toContain("Edit");
    await wrapper.find('[data-testid="favorite-toggle"]').trigger("click");
    await flushPromises();
    expect(api.setItemFavorite).toHaveBeenCalledWith("work", "vpn", true);
  });

  it("toggles the selected item with ⌘D", async () => {
    store.selectedItem = { ...github };
    const wrapper = mount(MainView, { attachTo: document.body });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "d", metaKey: true }));
    await flushPromises();
    expect(api.setItemFavorite).toHaveBeenCalledWith("personal", "github", false);
    expect(store.noticeMessage).toBe("Removed from Favorites");
    wrapper.unmount();
  });

  it("does not toggle while an item is being edited", async () => {
    store.selectedItem = { ...github };
    store.editorMode = "edit";
    const wrapper = mount(MainView, { attachTo: document.body });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "d", metaKey: true }));
    await flushPromises();
    expect(api.setItemFavorite).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("leaves ⌘D, ⌘N and ⌘F to the panel in front, but still locks with ⌘L", async () => {
    store.selectedItem = { ...github };
    store.isShareLinksOpen = true;
    const wrapper = mount(MainView, { attachTo: document.body });
    for (const key of ["d", "n", "f", ","]) {
      window.dispatchEvent(new KeyboardEvent("keydown", { key, metaKey: true }));
    }
    await flushPromises();
    expect(api.setItemFavorite).not.toHaveBeenCalled();
    expect(store.editorMode).toBe("view");
    expect(store.isSettingsOpen).toBe(false);
    expect(document.activeElement).not.toBe(wrapper.find('input[name="search"]').element);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "l", metaKey: true }));
    await flushPromises();
    expect(api.lock).toHaveBeenCalled();
    store.isShareLinksOpen = false;
    wrapper.unmount();
  });

  it("stars from the item's context menu", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.findAll(".list ul .item")[0].trigger("contextmenu");
    await flushPromises();
    const menuItem = wrapper.find('[data-testid="context-menu-favorite"]');
    expect(menuItem.text()).toBe("Add to Favorites");
    await menuItem.trigger("click");
    await flushPromises();
    expect(api.setItemFavorite).toHaveBeenCalledWith("personal", "bank", true);
    expect(wrapper.find('[data-testid="context-menu"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows the error and keeps the star as it was when saving fails", async () => {
    store.selectedItem = { ...bank };
    api.setItemFavorite.mockRejectedValueOnce(new Error("cannot reach the Hisar server; changes can be saved once you are back online"));
    const wrapper = mount(MainView);
    await wrapper.find('[data-testid="favorite-toggle"]').trigger("click");
    await flushPromises();
    expect(store.selectedItem?.isFavorite).toBe(false);
    expect(wrapper.find(".error").text()).toContain("cannot reach the Hisar server");
  });
});

describe("Favorites stay current", () => {
  it("reloads on unlock and in the background sync, as after another device starred an item", async () => {
    await loadAllVault();
    expect(api.listAllFavoriteItem).toHaveBeenCalledTimes(1);
    api.listAllFavoriteItem.mockResolvedValue([githubFavorite]);
    await syncInBackground();
    expect(store.allFavorite.map((favorite) => favorite.item.id)).toEqual(["github"]);
  });

  it("drops a deleted item from Favorites at once", async () => {
    const { deleteItem } = await import("../store");
    api.deleteItem.mockResolvedValue(undefined);
    await deleteItem("github");
    expect(store.allFavorite.map((favorite) => favorite.item.id)).toEqual(["vpn"]);
  });

  it("forgets favorites when Hisar locks", async () => {
    const { handleLocked } = await import("../store");
    handleLocked();
    expect(store.allFavorite).toEqual([]);
    expect(store.isFavoritesSelected).toBe(false);
  });
});
