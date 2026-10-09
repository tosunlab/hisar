import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createStatus } from "./testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("./api", async () => {
  const { createApiMock: createMock } = await import("./testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }),
  };
});

const { store, syncInBackground, syncInterval } = await import("./store");
const MainView = (await import("./components/MainView.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault,
    selectedVaultID: "personal",
    allItem: [createItem({ id: "a", title: "Alpha" })],
    selectedItem: createItem({ id: "a", title: "Alpha" }),
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isSharingOpen: false,
  });
  api.listAllVault.mockResolvedValue(allTestVault);
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
});

describe("syncInBackground", () => {
  it("refreshes the items and the open item", async () => {
    api.listAllItem.mockResolvedValue([createItem({ id: "b", title: "Bank" }), createItem({ id: "a", title: "Alpha", notes: "changed elsewhere" })]);
    api.getItem.mockResolvedValue(createItem({ id: "a", title: "Alpha", notes: "changed elsewhere" }));
    await syncInBackground();
    expect(store.allItem.map((vaultItem) => vaultItem.title)).toEqual(["Alpha", "Bank"]);
    expect(store.selectedItem?.notes).toBe("changed elsewhere");
  });

  it("closes the open item when it was deleted on another device", async () => {
    api.listAllItem.mockResolvedValue([createItem({ id: "b", title: "Bank" })]);
    await syncInBackground();
    expect(store.selectedItem).toBeNull();
    expect(api.getItem).not.toHaveBeenCalled();
  });

  it("does not touch the list while an item is being edited", async () => {
    store.editorMode = "edit";
    await syncInBackground();
    expect(api.listAllVault).not.toHaveBeenCalled();
    store.editorMode = "view";
    store.isBusy = true;
    await syncInBackground();
    expect(api.listAllVault).not.toHaveBeenCalled();
  });

  it("stays quiet when the backend fails", async () => {
    api.listAllVault.mockRejectedValue(new Error("cannot reach the Hisar server"));
    await syncInBackground();
    expect(store.errorMessage).toBe("");
    expect(store.allItem).toHaveLength(1);
  });

  it("picks up the offline state from the backend", async () => {
    api.listAllItem.mockResolvedValue([createItem({ id: "a", title: "Alpha" })]);
    api.getItem.mockResolvedValue(createItem({ id: "a", title: "Alpha" }));
    api.getStatus.mockResolvedValue({ ...createStatus("unlocked"), isOffline: true });
    await syncInBackground();
    expect(store.status.isOffline).toBe(true);
  });

  it("selects another vault when the open one was removed", async () => {
    store.selectedVaultID = "gone";
    api.listAllItem.mockResolvedValue([]);
    await syncInBackground();
    expect(store.selectedVaultID).toBe("personal");
  });
});

describe("MainView", () => {
  it("shows the offline notice and syncs every minute and on focus", async () => {
    vi.useFakeTimers();
    api.listAllItem.mockResolvedValue([createItem({ id: "a", title: "Alpha" })]);
    api.getItem.mockResolvedValue(createItem({ id: "a", title: "Alpha" }));
    store.status = { ...createStatus("unlocked"), isOffline: true };
    const wrapper = mount(MainView);
    expect(wrapper.find('[data-testid="offline-notice"]').text()).toContain("Offline");
    await vi.advanceTimersByTimeAsync(syncInterval);
    expect(api.listAllVault).toHaveBeenCalledTimes(1);
    window.dispatchEvent(new Event("focus"));
    await vi.advanceTimersByTimeAsync(0);
    expect(api.listAllVault).toHaveBeenCalledTimes(2);
    expect(wrapper.find('[data-testid="offline-notice"]').exists()).toBe(false);
    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(syncInterval);
    expect(api.listAllVault).toHaveBeenCalledTimes(2);
  });
});
