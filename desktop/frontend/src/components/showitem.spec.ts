import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createFavoriteItem, createItem, createStatus, type ApiMock } from "../testing";
import type { ItemDetail, VaultSummary } from "../api";
import { selectFavoriteItem, selectFavorites, selectItem, selectVault, store } from "../store";
import App from "../App.vue";

const apiMock = vi.hoisted(() => ({ current: {} as ApiMock }));
const allListener = vi.hoisted(() => ({ current: new Map<string, (event: { data: unknown }) => void>() }));

// vi.mock runs before this file's imports, so its factory loads the fixtures itself.
vi.mock("../api", async () => {
  const { createApiMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createApiMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      allListener.current.set(name, listener);
      return () => undefined;
    },
  },
}));

const api = apiMock.current;

enableAutoUnmount(afterEach);

const allVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 },
  { id: "work", name: "Work", type: "shared", role: "editor", itemCount: 2 },
  { id: "family", name: "Family", type: "shared", role: "reader", itemCount: 1 },
];
const allItemByVaultID: Record<string, ItemDetail[]> = {
  personal: [createItem({ id: "github", title: "GitHub" })],
  work: [createItem({ id: "deploy", title: "Deploy" }), createItem({ id: "grafana", title: "Grafana" })],
  family: [createItem({ id: "wifi", title: "Wi-Fi" })],
};

function sendEvent(name: string, data: unknown): void {
  allListener.current.get(name)?.({ data });
}

beforeEach(() => {
  vi.clearAllMocks();
  allListener.current.clear();
  Object.assign(store, {
    status: createStatus("signed-out"),
    allVault: [],
    selectedVaultID: "",
    allItem: [],
    selectedItem: null,
    isFavoritesSelected: false,
    editorMode: "view",
    searchQuery: "",
    selectedTag: "",
    errorMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue(allVault);
  api.listAllItem.mockImplementation((vaultID: string) => Promise.resolve(allItemByVaultID[vaultID]));
  api.getItem.mockImplementation((vaultID: string, itemID: string) => Promise.resolve(allItemByVaultID[vaultID].find((vaultItem) => vaultItem.id === itemID)));
});

describe("Open in Hisar from a launcher", () => {
  it("shows the item in its vault, over Settings", async () => {
    const wrapper = mount(App);
    await flushPromises();
    store.isSettingsOpen = true;
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: false });
    await flushPromises();
    expect(wrapper.find(".heading h2").text()).toBe("Deploy");
    expect(wrapper.find('[data-testid="item-vault"]').text()).toBe("Work");
    expect(wrapper.findAll(".item .item-title").map((title) => title.text())).toEqual(["Deploy", "Grafana"]);
    expect(wrapper.find(".item-editor").exists()).toBe(false);
    expect(store.isSettingsOpen).toBe(false);
  });

  it("opens the item's editor when asked to edit", async () => {
    const wrapper = mount(App);
    await flushPromises();
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    await flushPromises();
    expect((wrapper.find('.item-editor input[name="title"]').element as HTMLInputElement).value).toBe("Deploy");
  });

  it("shows a read-only vault's item without its editor", async () => {
    const wrapper = mount(App);
    await flushPromises();
    sendEvent("show-item", { vaultID: "family", itemID: "wifi", isEditing: true });
    await flushPromises();
    expect(wrapper.find(".heading h2").text()).toBe("Wi-Fi");
    expect(wrapper.find(".item-editor").exists()).toBe(false);
  });

  it("keeps the second show-item when the first vault loads last", async () => {
    const wrapper = mount(App);
    await flushPromises();
    const workItems = Promise.withResolvers<ItemDetail[]>();
    api.listAllItem.mockImplementation((vaultID: string) => (vaultID === "work" ? workItems.promise : Promise.resolve(allItemByVaultID[vaultID])));
    // The same item ID in another vault must not inherit Work's editing permission.
    api.getItem.mockImplementation((vaultID: string, itemID: string) => Promise.resolve(
      vaultID === "family" && itemID === "deploy" ? createItem({ id: "deploy", title: "Family deploy" }) : allItemByVaultID[vaultID].find((item) => item.id === itemID),
    ));
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    sendEvent("show-item", { vaultID: "family", itemID: "wifi", isEditing: false });
    await flushPromises();
    workItems.resolve(allItemByVaultID.work);
    await flushPromises();
    expect(store.selectedVaultID).toBe("family");
    expect(wrapper.find(".heading h2").text()).toBe("Wi-Fi");
    expect(wrapper.find(".item-editor").exists()).toBe(false);
  });

  it("keeps the second show-item when the first item loads last in the same vault", async () => {
    const wrapper = mount(App);
    await flushPromises();
    const deploy = Promise.withResolvers<ItemDetail>();
    api.getItem.mockImplementation((_vaultID: string, itemID: string) => itemID === "deploy" ? deploy.promise : Promise.resolve(allItemByVaultID.work[1]));
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    await flushPromises();
    sendEvent("show-item", { vaultID: "work", itemID: "grafana", isEditing: false });
    await flushPromises();
    deploy.resolve(allItemByVaultID.work[0]);
    await flushPromises();
    expect(wrapper.find(".heading h2").text()).toBe("Grafana");
    expect(wrapper.find(".item-editor").exists()).toBe(false);
  });

  it.each(["vault", "item"])("keeps the user's vault selection while a launcher's %s loads", async (pendingStep) => {
    const wrapper = mount(App);
    await flushPromises();
    const workItems = Promise.withResolvers<ItemDetail[]>();
    const deploy = Promise.withResolvers<ItemDetail>();
    if (pendingStep === "vault") {
      api.listAllItem.mockImplementation((vaultID: string) => vaultID === "work" ? workItems.promise : Promise.resolve(allItemByVaultID[vaultID]));
    }
    api.getItem.mockImplementation(() => deploy.promise);
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    await flushPromises();
    await selectVault("family");
    workItems.resolve(allItemByVaultID.work);
    deploy.resolve(allItemByVaultID.work[0]);
    await flushPromises();
    expect(store.selectedVaultID).toBe("family");
    expect(store.selectedItem).toBeNull();
    expect(wrapper.find(".item-editor").exists()).toBe(false);
  });

  it("keeps the user's item selection while a launcher item loads", async () => {
    const wrapper = mount(App);
    await flushPromises();
    const deploy = Promise.withResolvers<ItemDetail>();
    api.getItem.mockImplementation((_vaultID: string, itemID: string) => itemID === "deploy" ? deploy.promise : Promise.resolve(allItemByVaultID.work[1]));
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    await flushPromises();
    await selectItem("grafana");
    deploy.resolve(allItemByVaultID.work[0]);
    await flushPromises();
    expect(wrapper.find(".heading h2").text()).toBe("Grafana");
    expect(wrapper.find(".item-editor").exists()).toBe(false);
  });

  it.each([false, true])("keeps the user's Favorites navigation while a launcher item loads (item: %s)", async (isSelectingItem) => {
    const wrapper = mount(App);
    await flushPromises();
    const deploy = Promise.withResolvers<ItemDetail>();
    api.getItem.mockImplementation((_vaultID: string, itemID: string) => itemID === "deploy" ? deploy.promise : Promise.resolve(allItemByVaultID.work[1]));
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    await flushPromises();
    const favorite = createFavoriteItem({ vaultId: "work", item: allItemByVaultID.work[1] });
    api.listAllFavoriteItem.mockResolvedValue([favorite]);
    await selectFavorites();
    if (isSelectingItem) {
      await selectFavoriteItem(favorite);
    }
    deploy.resolve(allItemByVaultID.work[0]);
    await flushPromises();
    expect(store.isFavoritesSelected).toBe(true);
    expect(store.selectedItem?.id ?? null).toBe(isSelectingItem ? "grafana" : null);
    expect(wrapper.find(".item-editor").exists()).toBe(false);
  });

  it("keeps a launcher selection when an earlier favorite loads last", async () => {
    const wrapper = mount(App);
    await flushPromises();
    const grafana = Promise.withResolvers<ItemDetail>();
    api.getItem.mockImplementation((_vaultID: string, itemID: string) => itemID === "grafana" ? grafana.promise : Promise.resolve(allItemByVaultID.work[0]));
    const favoriteSelection = selectFavoriteItem(createFavoriteItem({ vaultId: "work", item: allItemByVaultID.work[1] }));
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: false });
    await flushPromises();
    grafana.resolve(allItemByVaultID.work[1]);
    await favoriteSelection;
    await flushPromises();
    expect(wrapper.find(".heading h2").text()).toBe("Deploy");
  });

  it("discards an item still loading when Hisar locks", async () => {
    mount(App);
    await flushPromises();
    const deploy = Promise.withResolvers<ItemDetail>();
    api.getItem.mockImplementation(() => deploy.promise);
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: true });
    await flushPromises();
    api.getStatus.mockResolvedValue(createStatus("locked"));
    sendEvent("locked", "");
    deploy.resolve(allItemByVaultID.work[0]);
    await flushPromises();
    expect(store.status.state).toBe("locked");
    expect(store.selectedItem).toBeNull();
    expect(store.selectedVaultID).toBe("");
  });

  it("shows the item after the unlock that came with the approval, even when Personal loads last", async () => {
    api.getStatus.mockResolvedValueOnce(createStatus("locked"));
    const personalItems = Promise.withResolvers<ItemDetail[]>();
    api.listAllItem.mockImplementation((vaultID: string) => (vaultID === "personal" ? personalItems.promise : Promise.resolve(allItemByVaultID[vaultID])));
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.find(".heading h2").exists()).toBe(false);
    sendEvent("unlocked", "approval");
    sendEvent("show-item", { vaultID: "work", itemID: "deploy", isEditing: false });
    await flushPromises();
    personalItems.resolve(allItemByVaultID.personal);
    await flushPromises();
    expect(store.selectedVaultID).toBe("work");
    expect(wrapper.find(".heading h2").text()).toBe("Deploy");
    expect(wrapper.findAll(".item .item-title").map((title) => title.text())).toEqual(["Deploy", "Grafana"]);
  });
});

describe("Selection for assistive technology", () => {
  it("marks the selected item and the current vault", async () => {
    const wrapper = mount(App);
    await flushPromises();
    sendEvent("show-item", { vaultID: "work", itemID: "grafana", isEditing: false });
    await flushPromises();
    const list = wrapper.get('.list ul[role="listbox"][aria-label="Work"]');
    expect(list.findAll('[role="option"]').map((item) => [item.find(".item-title").text(), item.attributes("aria-selected")])).toEqual([
      ["Deploy", "false"],
      ["Grafana", "true"],
    ]);
    expect(wrapper.find('[data-testid="vault-work"]').attributes("aria-current")).toBe("true");
  });
});
