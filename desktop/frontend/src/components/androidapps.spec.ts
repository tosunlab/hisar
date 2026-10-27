import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { store } from "../store";
import { allTestVault, createItem, createStatus } from "../testing";
import type { ApiMock } from "../testing";
import ItemDetailPanel from "./ItemDetailPanel.vue";
import ItemEditor from "./ItemEditor.vue";

const apiMock = vi.hoisted(() => ({ current: {} as ApiMock }));

vi.mock("../api", async () => {
  // vi.mock runs before this file's imports, so the factory loads the mock builder itself.
  const { createApiMock } = await import("../testing");
  apiMock.current = createApiMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createItem({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

const api = apiMock.current;

const githubApp = {
  packageName: "com.github.android",
  certificateSha256: "14:6D:E9:83:C5:73:06:50:D8:EE:B9:95:2F:34:FC:64:16:A0:83:42:E6:1D:BE:A8:8A:04:96:B2:3F:CF:44:E5",
};

beforeEach(() => {
  vi.clearAllMocks();
  const selectedItem = createItem({ id: "github", androidApps: [githubApp] });
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    listVaultID: "personal",
    allItem: [selectedItem],
    selectedItem,
    editorMode: "view",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
});

describe("Android apps in the item view", () => {
  it("shows each linked app with its certificate and offers no action on it", () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    const allRow = wrapper.findAll('[data-testid="android-app-row"]');
    expect(allRow).toHaveLength(1);
    expect(allRow[0].text()).toContain("Android app");
    expect(allRow[0].text()).toContain("com.github.android");
    expect(allRow[0].text()).toContain(githubApp.certificateSha256);
    expect(allRow[0].find("button").exists()).toBe(false);
  });

  it("shows none for an item without links", () => {
    store.selectedItem = createItem({ id: "plain" });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    expect(wrapper.find('[data-testid="android-app-row"]').exists()).toBe(false);
  });
});

describe("Android apps in the editor", () => {
  it("shows them read-only and saves the item with them", async () => {
    store.editorMode = "edit";
    api.saveItem.mockImplementation(async (_vaultID: string, detail: unknown) => detail);
    api.listAllItem.mockResolvedValue([]);
    const wrapper = mount(ItemEditor);
    expect(wrapper.get('[data-testid="editor-android-apps"]').text()).toContain("com.github.android");
    expect(wrapper.find('[data-testid="editor-android-apps"] input').exists()).toBe(false);
    await wrapper.get('input[name="title"]').setValue("GitHub (personal)");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ title: "GitHub (personal)", androidApps: [githubApp], urls: ["https://github.com"] }));
  });
});
