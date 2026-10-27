import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import type { VaultSummary } from "../api";
import { createApiMock, createItem, createStatus, createVaultActions, createVaultMember, testBob } from "../testing";
import ReadIssuesPanel from "./ReadIssuesPanel.vue";

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

const { store, loadAllVault, selectVault, syncInBackground } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const api = apiMock.current;

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 };

function createWorkVault(role = "owner"): VaultSummary {
  return { id: "work", name: "Work", type: "shared", role, itemCount: 12 };
}

function resetStore(workRole = "owner"): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: [personalVault, createWorkVault(workRole)],
    selectedVaultID: "work",
    allItem: [createItem()],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    vaultDialog: null,
    allRemovedVaultNotice: [],
  });
}

async function openMenu(wrapper: VueWrapper): Promise<void> {
  await wrapper.find('[data-testid="vault-actions"]').trigger("click");
  await flushPromises();
}

async function chooseFromMenu(wrapper: VueWrapper, action: string): Promise<void> {
  await openMenu(wrapper);
  await wrapper.find(`[data-testid="vault-menu-${action}"]`).trigger("click");
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
  api.listAllVault.mockResolvedValue([personalVault, createWorkVault()]);
  api.listAllItem.mockResolvedValue([createItem()]);
  api.getVaultActions.mockResolvedValue(createVaultActions());
  api.listAllRemovedVault.mockResolvedValue([]);
  document.body.innerHTML = "";
});

describe("vault menu", () => {
  it("offers rename, delete and leave for an owner's shared vault", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await openMenu(wrapper);
    expect(wrapper.find('[data-testid="vault-menu-rename"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="vault-menu-delete"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="vault-menu-leave"]').exists()).toBe(true);
  });

  it("opens from a right click on a vault in the sidebar", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="vault-personal"]').trigger("contextmenu");
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-menu"]').attributes("aria-label")).toBe("Personal vault");
    expect(wrapper.find('[data-testid="vault-menu-leave"]').exists()).toBe(false);
  });

  it("hides rename for an editor and explains why an editor can't delete", async () => {
    resetStore("editor");
    api.getVaultActions.mockResolvedValue(
      createVaultActions({ vault: createWorkVault("editor"), canRename: false, canDelete: false, deleteBlockedReason: "Only an owner of this vault can delete it. You can leave it instead.", canLeave: true, leaveBlockedReason: "", targetVaults: [] }),
    );
    const wrapper = mount(MainView, { attachTo: document.body });
    await openMenu(wrapper);
    expect(wrapper.find('[data-testid="vault-menu-rename"]').exists()).toBe(false);
    await wrapper.find('[data-testid="vault-menu-delete"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-blocked-reason"]').text()).toBe("Only an owner of this vault can delete it. You can leave it instead.");
    expect(api.deleteVault).not.toHaveBeenCalled();
  });

  it("explains that the Personal vault can't be deleted", async () => {
    store.selectedVaultID = "personal";
    const reason = "Your Personal vault can't be deleted. It is the vault Hisar keeps for you alone, and logins saved from the browser and your phone go there.";
    api.getVaultActions.mockResolvedValue(createVaultActions({ vault: personalVault, canDelete: false, deleteBlockedReason: reason, canLeave: false, targetVaults: [] }));
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "delete");
    expect(api.getVaultActions).toHaveBeenCalledWith("personal");
    expect(wrapper.find('[data-testid="vault-dialog-blocked"]').text()).toContain(reason);
  });
});

describe("delete vault dialog", () => {
  it("lists who loses access and moves the items to the first writable vault by default", async () => {
    api.getVaultActions.mockResolvedValue(createVaultActions({ membersLosingAccess: [createVaultMember({ ...testBob, role: "editor", isCurrentUser: false })] }));
    api.deleteVault.mockResolvedValue({ movedCount: 12, targetVaultName: "Personal" });
    api.listAllVault.mockResolvedValue([personalVault]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "delete");

    const dialog = wrapper.find('[data-testid="vault-dialog-delete"]');
    expect(dialog.find("h2").text()).toBe("Delete “Work”?");
    expect(dialog.find('[data-testid="vault-members-losing-access"]').text()).toContain(testBob.email);
    expect(dialog.text()).toContain("Move its 12 items to");
    expect((dialog.find('[data-testid="vault-delete-move"]').element as HTMLInputElement).checked).toBe(true);
    expect(dialog.find('[data-testid="vault-delete-confirm-name"]').exists()).toBe(false);

    await dialog.find('[data-testid="vault-delete-confirm"]').trigger("click");
    await flushPromises();
    expect(api.deleteVault).toHaveBeenCalledWith("work", { moveToVaultId: "personal", isDeletingItems: false, confirmedName: "" });
    expect(store.vaultDialog).toBeNull();
    expect(store.selectedVaultID).toBe("personal");
    expect(store.noticeMessage).toBe("Moved 12 items to Personal and deleted Work.");
  });

  it("deletes the items only after the vault's name is typed exactly", async () => {
    api.deleteVault.mockResolvedValue({ movedCount: 0, targetVaultName: "" });
    api.listAllVault.mockResolvedValue([personalVault]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "delete");
    await wrapper.find('[data-testid="vault-delete-items"]').setValue(true);

    const confirmButton = wrapper.find('[data-testid="vault-delete-confirm"]');
    expect(confirmButton.attributes("disabled")).toBeDefined();
    await wrapper.find('[data-testid="vault-delete-confirm-name"]').setValue("work");
    expect(confirmButton.attributes("disabled")).toBeDefined();
    await wrapper.find('[data-testid="vault-delete-confirm-name"]').setValue("Work");
    expect(confirmButton.attributes("disabled")).toBeUndefined();

    await confirmButton.trigger("click");
    await flushPromises();
    expect(api.deleteVault).toHaveBeenCalledWith("work", { moveToVaultId: "", isDeletingItems: true, confirmedName: "Work" });
    expect(store.noticeMessage).toBe("Deleted Work.");
  });

  it("offers only deleting with the items when no other vault can take them", async () => {
    api.getVaultActions.mockResolvedValue(createVaultActions({ targetVaults: [] }));
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "delete");
    expect(wrapper.find('[data-testid="vault-delete-move"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="vault-delete-confirm-name"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("There is no other vault you can add items to");
  });

  it("deletes an empty vault after one confirmation and shows backend errors", async () => {
    api.getVaultActions.mockResolvedValue(createVaultActions({ vault: { ...createWorkVault(), itemCount: 0 } }));
    api.deleteVault.mockRejectedValueOnce(new Error("the vault changed on another device; try again"));
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "delete");
    expect(wrapper.text()).toContain("The vault is empty.");
    await wrapper.find('[data-testid="vault-delete-confirm"]').trigger("click");
    await flushPromises();
    expect(api.deleteVault).toHaveBeenCalledWith("work", { moveToVaultId: "", isDeletingItems: false, confirmedName: "" });
    expect(wrapper.find('[data-testid="vault-dialog-delete"] .error').text()).toBe("the vault changed on another device; try again");
  });
});

describe("rename and leave", () => {
  it("renames the vault", async () => {
    api.renameVault.mockResolvedValue({ ...createWorkVault(), name: "Work 2026" });
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "rename");
    const input = wrapper.find('[data-testid="vault-rename-input"]');
    expect((input.element as HTMLInputElement).value).toBe("Work");
    expect(wrapper.find('[data-testid="vault-rename-save"]').attributes("disabled")).toBeDefined();
    await input.setValue("  Work 2026 ");
    await wrapper.find('[data-testid="vault-rename-save"]').trigger("click");
    await flushPromises();
    expect(api.renameVault).toHaveBeenCalledWith("work", "Work 2026");
    expect(store.allVault.find((vault) => vault.id === "work")?.name).toBe("Work 2026");
  });

  it("leaves a vault shared with the user", async () => {
    resetStore("editor");
    api.getVaultActions.mockResolvedValue(createVaultActions({ vault: createWorkVault("editor"), canRename: false, canDelete: false, canLeave: true, leaveBlockedReason: "" }));
    api.listAllVault.mockResolvedValue([personalVault]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "leave");
    expect(wrapper.find('[data-testid="vault-dialog-leave"]').text()).toContain("An owner can invite you again.");
    await wrapper.find('[data-testid="vault-leave-confirm"]').trigger("click");
    await flushPromises();
    expect(api.leaveVault).toHaveBeenCalledWith("work");
    expect(store.selectedVaultID).toBe("personal");
    expect(store.noticeMessage).toBe("You left Work.");
  });

  it("explains why the only owner can't leave", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await chooseFromMenu(wrapper, "leave");
    expect(wrapper.find('[data-testid="vault-blocked-reason"]').text()).toContain("You are its only owner.");
    expect(api.leaveVault).not.toHaveBeenCalled();
  });
});

describe("removed vault notice", () => {
  it("shows a vault someone else deleted after a refresh, selects Personal, and can be dismissed", async () => {
    api.listAllVault.mockResolvedValue([personalVault]);
    api.listAllRemovedVault.mockResolvedValueOnce([{ id: "work", name: "Work", message: "“Work” is no longer available: it was deleted, or you no longer have access to it." }]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await loadAllVault();
    await flushPromises();
    expect(store.selectedVaultID).toBe("personal");
    const banner = wrapper.find('[data-testid="removed-vault-work"]');
    expect(banner.text()).toContain("“Work” is no longer available");
    await wrapper.find('[data-testid="dismiss-removed-vault-work"]').trigger("click");
    expect(wrapper.find('[data-testid="removed-vault-banner"]').exists()).toBe(false);
  });

  it("also asks after a background sync", async () => {
    api.getStatus.mockResolvedValue(createStatus("unlocked"));
    api.listAllRemovedVault.mockResolvedValueOnce([{ id: "infra", name: "", message: "A vault is no longer available: it was deleted, or you no longer have access to it." }]);
    await syncInBackground();
    expect(store.allRemovedVaultNotice.map((notice) => notice.id)).toEqual(["infra"]);
  });
});

describe("unreadable recovery", () => {
  it("keeps healthy data visible and deletes only the reviewed unreadable revision", async () => {
    store.allVault = [{ ...personalVault, readIssues: [{ vaultId: "personal", itemId: "bad", revision: "bad-r1", kind: "item", canRename: false, canDelete: true }] }];
    store.allItem = [createItem({ id: "healthy", title: "Still available" })];
    api.deleteUnreadableItem.mockResolvedValue(undefined);
    const wrapper = mount(ReadIssuesPanel);
    expect(wrapper.text()).toContain("1 unreadable vault or item records");
    expect(store.allItem[0].id).toBe("healthy");
    await wrapper.findAll("button").find((button) => button.text() === "Delete unreadable data…")!.trigger("click");
    expect(api.deleteUnreadableItem).not.toHaveBeenCalled();
    await wrapper.findAll("button").find((button) => button.text() === "Permanently delete")!.trigger("click");
    await flushPromises();
    expect(api.deleteUnreadableItem).toHaveBeenCalledWith("personal", "bad", "bad-r1");
    expect(api.deleteUnreadableVault).not.toHaveBeenCalled();
  });

  it("discloses a record that stopped opening when only the vault's items are listed again", async () => {
    const issue = { vaultId: "personal", itemId: "late-bad", revision: "late-r1", kind: "item", canRename: false, canDelete: true };
    api.listAllItem.mockResolvedValue([createItem({ id: "healthy", title: "Still available" }), createItem({ id: "late-bad", revision: "late-r1", title: "Unreadable item late-bad", readIssue: issue })]);
    api.listAllVault.mockResolvedValue([{ ...personalVault }, createWorkVault()]);
    store.allVault = [{ ...personalVault }, createWorkVault()];
    const wrapper = mount(MainView, { attachTo: document.body });
    await flushPromises();
    await selectVault("personal");
    await flushPromises();
    const personal = store.allVault.find((vault) => vault.id === "personal");
    expect(personal?.itemCount).toBe(1);
    expect(personal?.readIssues).toEqual([issue]);
    expect(wrapper.find('[data-testid="read-issues"]').text()).toContain("1 unreadable vault or item records");
    expect(wrapper.findAll("button.item").map((button) => button.text()).join()).not.toContain("Unreadable item late-bad");
    wrapper.unmount();
  });

  it("names the reviewed revision when deleting or repairing an unreadable vault", async () => {
    const nameIssue = { vaultId: "work", revision: "work-review-1", kind: "vault-name", canRename: true, canDelete: true };
    store.allVault = [personalVault, { ...createWorkVault(), name: "Unreadable vault work", readIssues: [nameIssue] }];
    api.repairVaultName.mockRejectedValue(new Error("that unreadable record changed; retry and review it again"));
    api.deleteUnreadableVault.mockResolvedValue(undefined);
    const wrapper = mount(ReadIssuesPanel);
    await wrapper.find('input[aria-label="New name for work"]').setValue("Work");
    await wrapper.findAll("button").find((button) => button.text() === "Repair name")!.trigger("click");
    await flushPromises();
    expect(api.repairVaultName).toHaveBeenCalledWith("work", "work-review-1", "Work");
    expect(api.renameVault).not.toHaveBeenCalled();
    expect(api.listAllVault).toHaveBeenCalled();
    store.allVault = [personalVault, { ...createWorkVault(), name: "Unreadable vault work", readIssues: [nameIssue] }];
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "Delete unreadable data…")!.trigger("click");
    await wrapper.findAll("button").find((button) => button.text() === "Permanently delete")!.trigger("click");
    await flushPromises();
    expect(api.deleteUnreadableVault).toHaveBeenCalledWith("work", "work-review-1");
  });
});
