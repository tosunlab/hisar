import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createItem({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

const { store } = await import("../store");
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const api = apiMock.current;

const githubPasskey = { credentialId: "Y3JlZC0x", relyingPartyId: "github.com", userName: "alice", userDisplayName: "Alice", createdAt: "2026-10-02T09:41:00Z" };
const workPasskey = { credentialId: "Y3JlZC0y", relyingPartyId: "github.com", userName: "", userDisplayName: "Alice at work", createdAt: "2026-10-05T10:00:00Z" };

beforeEach(() => {
  vi.clearAllMocks();
  const selectedItem = createItem({ id: "github", passkeys: [githubPasskey, workPasskey] });
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [selectedItem],
    selectedItem,
    editorMode: "view",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
});

describe("passkeys in the item view", () => {
  it("lists each passkey with its site, user name and creation date", () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    const allRow = wrapper.findAll('[data-testid="passkey-row"]');
    expect(allRow).toHaveLength(2);
    expect(allRow[0].get('[data-testid="passkey-site"]').text()).toBe("github.com");
    expect(allRow[0].get('[data-testid="passkey-user"]').text()).toBe("alice");
    expect(allRow[0].get('[data-testid="passkey-created"]').text()).toBe("· Created 2 Oct 2026");
    expect(allRow[1].get('[data-testid="passkey-user"]').text()).toBe("Alice at work");
    expect(wrapper.text()).not.toContain("Y3JlZC0x");
  });

  it("deletes a passkey after confirmation and shows the updated item", async () => {
    api.deletePasskey.mockResolvedValue(createItem({ id: "github", passkeys: [workPasskey] }));
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    await wrapper.findAll('[data-testid="delete-passkey"]')[0].trigger("click");
    expect(api.deletePasskey).not.toHaveBeenCalled();
    await wrapper.get('[data-testid="confirm-delete-passkey"]').trigger("click");
    await flushPromises();
    expect(api.deletePasskey).toHaveBeenCalledWith("personal", "github", "Y3JlZC0x");
    expect(wrapper.findAll('[data-testid="passkey-row"]')).toHaveLength(1);
    expect(store.allItem[0].passkeys).toEqual([workPasskey]);
    expect(store.noticeMessage).toBe("Passkey deleted");
  });

  it("offers no delete in read-only vaults and hides the section without passkeys", () => {
    const readOnly = mount(ItemDetailPanel, { props: { canEdit: false, vaultName: "Work", vaultType: "shared" } });
    expect(readOnly.findAll('[data-testid="passkey-row"]')).toHaveLength(2);
    expect(readOnly.find('[data-testid="delete-passkey"]').exists()).toBe(false);
    store.selectedItem = createItem({ id: "plain" });
    const plain = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    expect(plain.find('[data-testid="passkey-section"]').exists()).toBe(false);
  });
});

describe("passkeys in the editor", () => {
  it("shows them read-only and saves the item without touching them", async () => {
    store.editorMode = "edit";
    api.saveItem.mockImplementation(async (_vaultID: string, detail: unknown) => detail);
    api.listAllItem.mockResolvedValue([]);
    const wrapper = mount(ItemEditor);
    expect(wrapper.get('[data-testid="editor-passkeys"]').text()).toContain("github.com");
    expect(wrapper.find('[data-testid="editor-passkeys"] input').exists()).toBe(false);
    await wrapper.get('input[name="title"]').setValue("GitHub (personal)");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ title: "GitHub (personal)", passkeys: [githubPasskey, workPasskey] }));
  });
});
