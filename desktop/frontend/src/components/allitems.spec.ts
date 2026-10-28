import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createItem, createStatus } from "../testing";
import { api } from "../api";
import { filterAllItem, selectAllItems, selectVault, store } from "../store";
import MainView from "./MainView.vue";

vi.mock("../api", async () => {
  const { createApiMock } = await import("../testing");
  return { api: createApiMock(), getErrorMessage: (e: unknown) => String(e), createEmptyItem: () => ({ id: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }) };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));
const mock = vi.mocked(api);
enableAutoUnmount(afterEach);
const personal = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 };
const work = { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 1 };
const one = createItem({ id: "same", title: "Personal credential", password: "", notes: "" });
const two = createItem({ id: "same", title: "Work credential", password: "", notes: "" });
beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createStatus("unlocked"), allVault: [{ ...personal }, { ...work }], allItem: [one],
    listVaultID: "personal", selectedVaultID: "personal", selectedItem: null, isAllItemsSelected: false, allItemReference: [], allItemsError: "", isFavoritesSelected: false,
    editorMode: "view", editorDraft: null, discardDraftPrompt: null, searchQuery: "credential", selectedTag: "", isBusy: false,
    isSettingsOpen: false, isSharingOpen: false, isImportOpen: false, isShareLinksOpen: false, shareLinkDialog: null, vaultDialog: null, itemDetailDialog: null });
  mock.listAllItem.mockImplementation(async (id) => id === "personal" ? [one] : [two]);
  mock.getItem.mockResolvedValue({ ...two, password: "secret" });
});

describe("All-vault browsing", () => {
  it("acquires only summaries, retains source permissions and query, and selects from the right vault", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.get('[data-testid="open-all-items"]').trigger("click");
    await flushPromises();
    expect(mock.listAllItem.mock.calls).toEqual([["personal"], ["work"]]);
    expect(mock.getItem).not.toHaveBeenCalled();
    expect(store.allItemReference.map((reference) => [reference.vaultId, reference.vaultName, reference.vaultRole])).toEqual([["personal", "Personal", "owner"], ["work", "Work", "reader"]]);
    expect(wrapper.get('input[name="search"]').attributes("aria-label")).toBe("Search all vaults");
    const rows = wrapper.findAll('[role="option"]');
    expect(rows.map((row) => row.find(".item-vault").text())).toEqual(["Personal", "Work"]);
    await rows[1].trigger("click");
    await flushPromises();
    expect(mock.getItem).toHaveBeenCalledWith("work", "same");
    expect(store.selectedVaultID).toBe("work");
    expect(wrapper.find('[data-testid="edit-item"]').exists()).toBe(false);
    await selectVault("personal");
    expect(store.isAllItemsSelected).toBe(false);
    expect(store.searchQuery).toBe("credential");
    expect(store.allItem).toEqual([one]);
  });
  it("does not search passwords, notes or concealed fields", () => {
    expect(filterAllItem([createItem({ title: "Safe", username: "", urls: [], password: "hiddenneedle", notes: "hiddenneedle", fields: [{ name: "Other", value: "hiddenneedle", concealed: true }] })], "hiddenneedle")).toEqual([]);
  });
  it("preserves unreadable and offline disclosures rather than calling partial results complete", async () => {
    store.status.isOffline = true;
    const issue = { kind: "item", vaultId: "work", itemId: "broken", revision: "r1", canDelete: false, canRename: false };
    mock.listAllItem.mockImplementation(async (id) => id === "work" ? [two, createItem({ id: "broken", readIssue: issue })] : [one]);
    await selectAllItems();
    store.searchQuery = "not found";
    const wrapper = mount(MainView);
    expect(wrapper.get('[data-testid="offline-notice"]').text()).toContain("copy saved on this device");
    expect(wrapper.get('[data-testid="read-issues"]').text()).toContain("1 known unreadable item in 1 affected vault.");
    expect(wrapper.get(".list-empty").text()).toContain("No readable matches");
    expect(store.allItemReference).toHaveLength(2);
  });
  it("shows an ordinary no-results state only after all vaults load", async () => {
    await selectAllItems();
    store.searchQuery = "absent";
    const wrapper = mount(MainView);
    expect(wrapper.get(".list-empty").text()).toBe("No items match your search.");
    mock.listAllItem.mockRejectedValue(new Error("Cannot open vault"));
    await selectAllItems();
    await flushPromises();
    expect(wrapper.get(".list-empty").text()).toContain("Search is incomplete");
  });
});
