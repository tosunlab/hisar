import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createItem, createStatus, type ApiMock } from "../testing";
import type { ItemDetail, VaultSummary } from "../api";
import { store } from "../store";
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
