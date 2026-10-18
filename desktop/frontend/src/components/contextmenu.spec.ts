import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import type { ItemDetail, VaultSummary } from "../api";
import {
  createAPICredentialItem,
  createApiMock,
  createCardItem,
  createDatabaseItem,
  createFavoriteItem,
  createItem,
  createSecureNoteItem,
  createSoftwareLicenseItem,
  createSSHKeyItem,
  createStatus,
} from "../testing";

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
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const api = apiMock.current;

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 };
const workVault: VaultSummary = { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 1 };
const teamVault: VaultSummary = { id: "team", name: "Team", type: "shared", role: "editor", itemCount: 1 };
const github = createItem({ id: "github", title: "GitHub", hasOneTimePassword: true });
const githubDetail = { ...github, password: "github-password", fields: [{ name: "Recovery code", value: "abcd", concealed: true }] };
const bank = createItem({ id: "bank", title: "Bank", username: "", urls: [] });

function resetStore(allItem: ItemDetail[] = [github, bank]): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: [personalVault, workVault, teamVault].map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem,
    selectedItem: null,
    allFavorite: [],
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
  api.getItem.mockImplementation(async (_vaultID: string, itemID: string) => (itemID === "github" ? githubDetail : store.allItem.find((vaultItem) => vaultItem.id === itemID)));
  api.listAllFavoriteItem.mockResolvedValue([]);
  document.body.innerHTML = "";
});

async function openItemMenu(wrapper: VueWrapper, title: string, point = { clientX: 40, clientY: 60 }): Promise<string[]> {
  const row = wrapper.findAll(".list .item").find((candidate) => candidate.text().includes(title));
  await row!.trigger("contextmenu", point);
  await flushPromises();
  return wrapper.findAll('[data-testid="context-menu"] button').map((button) => button.text());
}

describe("The item list's context menu", () => {
  it("lists what applies to a login once its details load, and copies without selecting it", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    const allLabel = await openItemMenu(wrapper, "GitHub");
    expect(allLabel).toEqual([
      "Open",
      "Copy username",
      "Copy password",
      "Copy one-time code",
      "Open website",
      "Add to Favorites",
      "Share link…",
      "Edit",
      "Duplicate",
      "Move to…",
      "Delete…",
    ]);
    expect(wrapper.findAll('[data-testid="context-menu"] hr')).toHaveLength(2);
    expect(api.getItem).toHaveBeenCalledWith("personal", "github");

    await wrapper.find('[data-testid="context-menu-copy-password"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "github", "password");
    expect(store.noticeMessage).toBe("Password copied, clears in 90 seconds");
    expect(store.selectedItem).toBeNull();
    expect(wrapper.find('[data-testid="context-menu"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("offers each type's main value and nothing to copy on a secure note", async () => {
    resetStore([
      createCardItem({ id: "card", title: "Visa" }),
      createDatabaseItem({ id: "db", title: "Prod db" }),
      createSSHKeyItem({ id: "key", title: "Deploy key", publicKey: "ssh-ed25519 AAAA deploy" }),
      createSecureNoteItem({ id: "note", title: "Door code" }),
    ]);
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(await openItemMenu(wrapper, "Visa")).toContain("Copy card number");
    expect(await openItemMenu(wrapper, "Prod db")).toContain("Copy connection string");
    expect(await openItemMenu(wrapper, "Deploy key")).toContain("Copy public key");
    expect((await openItemMenu(wrapper, "Door code")).filter((label) => label.startsWith("Copy"))).toEqual([]);
    wrapper.unmount();
  });

  it("copies a license key and an API secret once the details load", async () => {
    resetStore([createSoftwareLicenseItem({ id: "sketch", title: "Sketch" }), createAPICredentialItem({ id: "stripe", title: "Stripe" })]);
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(await openItemMenu(wrapper, "Sketch")).toContain("Copy license key");
    await wrapper.find('[data-testid="context-menu-copy-license-key"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "sketch", "License key");
    expect(await openItemMenu(wrapper, "Stripe")).toContain("Copy secret");
    await wrapper.find('[data-testid="context-menu-copy-secret"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "stripe", "Secret");
    wrapper.unmount();
  });

  it("leaves out Edit and Delete in a vault the user can only read", async () => {
    store.selectedVaultID = "work";
    const wrapper = mount(MainView, { attachTo: document.body });
    const allLabel = await openItemMenu(wrapper, "GitHub");
    for (const label of ["Edit", "Duplicate", "Move to…", "Delete…"]) {
      expect(allLabel).not.toContain(label);
    }
    expect(allLabel).toContain("Share link…");
    wrapper.unmount();
  });

  it("copies a favorite from another vault with that vault's ID", async () => {
    const vpn = createItem({ id: "vpn", title: "Office VPN", username: "alper", urls: [], isFavorite: true });
    store.allFavorite = [createFavoriteItem({ vaultId: "team", vaultName: "Team", vaultType: "shared", item: vpn })];
    store.isFavoritesSelected = true;
    api.getItem.mockResolvedValue({ ...vpn, password: "vpn-password" });
    const wrapper = mount(MainView, { attachTo: document.body });
    const allLabel = await openItemMenu(wrapper, "Office VPN");
    expect(allLabel).toEqual(["Open", "Copy username", "Copy password", "Remove from Favorites", "Share link…", "Edit", "Duplicate", "Move to…", "Delete…"]);
    await wrapper.find('[data-testid="context-menu-copy-username"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("team", "vpn", "username");
    wrapper.unmount();
  });

  it("opens the editor for the item from Edit", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-edit"]').trigger("click");
    await flushPromises();
    expect(store.selectedItem?.id).toBe("github");
    expect(store.editorMode).toBe("edit");
    wrapper.unmount();
  });

  it("deletes only after the confirmation, keeping another selected item", async () => {
    store.selectedItem = bank;
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-delete"]').trigger("click");
    const dialog = () => wrapper.find('[data-testid="confirm-delete-dialog"]');
    expect(dialog().text()).toContain("Delete “GitHub”?");
    expect(dialog().text()).toContain("It is removed from all your devices.");
    await wrapper.find('[data-testid="confirm-delete-cancel"]').trigger("click");
    expect(dialog().exists()).toBe(false);
    expect(api.deleteItem).not.toHaveBeenCalled();

    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-delete"]').trigger("click");
    await wrapper.find('[data-testid="confirm-delete-item"]').trigger("click");
    await flushPromises();
    expect(api.deleteItem).toHaveBeenCalledWith("personal", "github");
    expect(store.allItem.map((vaultItem) => vaultItem.id)).toEqual(["bank"]);
    expect(store.selectedItem?.id).toBe("bank");
    expect(dialog().exists()).toBe(false);
    wrapper.unmount();
  });

  it("keeps the vault counts right when a favorite opened from Favorites is deleted", async () => {
    const vpn = createItem({ id: "vpn", title: "Office VPN", urls: [], isFavorite: true });
    const vpnFavorite = createFavoriteItem({ vaultId: "team", vaultName: "Team", vaultType: "shared", item: vpn });
    store.allFavorite = [vpnFavorite];
    store.isFavoritesSelected = true;
    api.getItem.mockResolvedValue(vpn);
    const wrapper = mount(MainView, { attachTo: document.body });
    // Opening the favorite makes Team the selected vault while allItem still lists Personal.
    await wrapper.findAll(".list .item").find((row) => row.text().includes("Office VPN"))!.trigger("click");
    await flushPromises();
    expect(store.selectedVaultID).toBe("team");
    await openItemMenu(wrapper, "Office VPN");
    await wrapper.find('[data-testid="context-menu-delete"]').trigger("click");
    expect(wrapper.find('[data-testid="confirm-delete-dialog"]').text()).toContain("It is removed from Team for everyone with access to the vault.");
    await wrapper.find('[data-testid="confirm-delete-item"]').trigger("click");
    await flushPromises();
    expect(api.deleteItem).toHaveBeenCalledWith("team", "vpn");
    expect(store.allVault.find((vault) => vault.id === "team")?.itemCount).toBe(0);
    expect(store.allVault.find((vault) => vault.id === "personal")?.itemCount).toBe(2);
    expect(store.allItem.map((vaultItem) => vaultItem.id)).toEqual(["github", "bank"]);
    expect(store.allFavorite).toEqual([]);
    expect(store.selectedItem).toBeNull();
    wrapper.unmount();
  });

  it("duplicates an item and opens the copy", async () => {
    const copy = createItem({ id: "github-copy", title: "GitHub copy" });
    api.duplicateItem.mockResolvedValue(copy);
    api.listAllItem.mockResolvedValue([bank, github, copy]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-duplicate"]').trigger("click");
    await flushPromises();
    expect(api.duplicateItem).toHaveBeenCalledWith("personal", "github");
    expect(store.selectedItem?.id).toBe("github-copy");
    expect(store.allItem.map((vaultItem) => vaultItem.title)).toEqual(["Bank", "GitHub", "GitHub copy"]);
    expect(store.noticeMessage).toBe("Duplicated as GitHub copy");
    expect(store.allVault.find((vault) => vault.id === "personal")?.itemCount).toBe(3);
    wrapper.unmount();
  });

  it("moves an item to a vault the user may add items to", async () => {
    store.selectedItem = github;
    api.moveItem.mockResolvedValue({ ...github });
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-move"]').trigger("click");
    const dialog = wrapper.find('[data-testid="move-item-dialog"]');
    expect(dialog.text()).toContain("Move “GitHub”");
    // Work is read-only for this user, and Personal is where the item already is.
    expect(dialog.findAll('[data-testid="move-item-target"] option').map((option) => option.text())).toEqual(["Team"]);
    await wrapper.find('[data-testid="move-item-confirm"]').trigger("submit");
    await flushPromises();
    expect(api.moveItem).toHaveBeenCalledWith("personal", "github", "team");
    expect(store.allItem.map((vaultItem) => vaultItem.id)).toEqual(["bank"]);
    expect(store.selectedItem).toBeNull();
    expect(store.allVault.find((vault) => vault.id === "team")?.itemCount).toBe(2);
    expect(store.noticeMessage).toBe("Moved GitHub to Team");
    expect(wrapper.find('[data-testid="move-item-dialog"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("moves into organization vaults by the role the user's teams give", async () => {
    const deployVault: VaultSummary = { id: "deploy", name: "Deploy", type: "organization", role: "editor", itemCount: 0, organizationId: "acme" };
    const auditVault: VaultSummary = { id: "audit", name: "Audit", type: "organization", role: "reader", itemCount: 0, organizationId: "acme" };
    store.allVault.push({ ...deployVault }, { ...auditVault });
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-move"]').trigger("click");
    expect(wrapper.findAll('[data-testid="move-item-target"] option').map((option) => option.text())).toEqual(["Team", "Deploy"]);
    wrapper.unmount();

    // A reader organization vault is no target, so with nothing else writable the menu has no Move to….
    store.allVault = [{ ...personalVault }, { ...auditVault }];
    const onlyReader = mount(MainView, { attachTo: document.body });
    expect(await openItemMenu(onlyReader, "GitHub")).not.toContain("Move to…");
    onlyReader.unmount();
  });

  it("opens the share link dialog for the item", async () => {
    api.listAllShareLinkField.mockResolvedValue([{ key: "password", label: "password", isDefault: true, isConcealed: true }]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "GitHub");
    await wrapper.find('[data-testid="context-menu-share-link"]').trigger("click");
    await flushPromises();
    expect(store.selectedItem?.id).toBe("github");
    expect(api.listAllShareLinkField).toHaveBeenCalledWith("personal", "github");
    expect(store.shareLinkDialog?.itemID).toBe("github");
    wrapper.unmount();
  });

  it("moves the focus with the arrow keys and closes on Escape", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "Bank");
    const allButton = wrapper.findAll('[data-testid="context-menu"] button');
    expect(document.activeElement).toBe(allButton[0].element);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown" }));
    expect(document.activeElement).toBe(allButton[1].element);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp" }));
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp" }));
    expect(document.activeElement).toBe(allButton[allButton.length - 1].element);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(wrapper.find('[data-testid="context-menu"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("stays inside the window when opened near its edge", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ width: 220, height: 300, top: 0, left: 0, right: 220, bottom: 300, x: 0, y: 0, toJSON: () => ({}) });
    const wrapper = mount(MainView, { attachTo: document.body });
    await openItemMenu(wrapper, "Bank", { clientX: window.innerWidth - 10, clientY: window.innerHeight - 10 });
    const style = (wrapper.find('[data-testid="context-menu"]').element as HTMLElement).style;
    expect(parseFloat(style.left)).toBe(window.innerWidth - 220 - 8);
    expect(parseFloat(style.top)).toBe(window.innerHeight - 300 - 8);
    vi.restoreAllMocks();
    wrapper.unmount();
  });
});

describe("A detail field's context menu", () => {
  it("offers the row's own actions: reveal, copy and copy the secret reference", async () => {
    store.selectedItem = githubDetail;
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    const passwordRow = wrapper.find('[data-testid="password-value"]');
    await passwordRow.trigger("contextmenu");
    const allLabel = wrapper.findAll('[data-testid="context-menu"] button').map((button) => button.text());
    expect(allLabel).toEqual(["Reveal password", "Copy password", "Copy secret reference for password"]);
    await wrapper.find('[data-testid="context-menu-reveal-password"]').trigger("click");
    expect(passwordRow.text()).toBe("github-password");
    expect(wrapper.find('[data-testid="context-menu"]').exists()).toBe(false);

    await passwordRow.trigger("contextmenu");
    expect(wrapper.find('[data-testid="context-menu-hide-password"]').exists()).toBe(true);
    await wrapper.find('[data-testid="context-menu-copy-secret-reference-for-password"]').trigger("click");
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "github", "password");
    wrapper.unmount();
  });

  it("opens a website from its row and reveals a concealed custom field", async () => {
    store.selectedItem = githubDetail;
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    const websiteRow = wrapper.findAll(".field").find((row) => row.text().includes("https://github.com"));
    await websiteRow!.trigger("contextmenu");
    expect(wrapper.findAll('[data-testid="context-menu"] button').map((button) => button.text())).toEqual(["Open website"]);
    await wrapper.find('[data-testid="context-menu-open-website"]').trigger("click");
    expect(api.openURL).toHaveBeenCalledWith("https://github.com");

    const recoveryRow = wrapper.findAll(".field").find((row) => row.text().includes("Recovery code"));
    await recoveryRow!.trigger("contextmenu");
    await wrapper.find('[data-testid="context-menu-reveal-recovery-code"]').trigger("click");
    expect(recoveryRow!.text()).toContain("abcd");
    wrapper.unmount();
  });

  it("offers the buttons of an SSH key's public key and a secure note's notes", async () => {
    store.selectedItem = createSSHKeyItem({ id: "key", title: "Deploy key", publicKey: "ssh-ed25519 AAAA deploy" });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    await wrapper.find('[data-testid="public-key"]').trigger("contextmenu");
    expect(wrapper.find('[data-testid="context-menu"]').attributes("aria-label")).toBe("Public key");
    expect(wrapper.findAll('[data-testid="context-menu"] button').map((button) => button.text())).toEqual(["Copy public key", "Copy secret reference for public key"]);
    await wrapper.find('[data-testid="context-menu-copy-public-key"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "key", "public key");
    wrapper.unmount();

    store.selectedItem = createSecureNoteItem({ id: "note", title: "Door code", notes: "4321" });
    const noteWrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    await noteWrapper.find(".notes").trigger("contextmenu");
    expect(noteWrapper.findAll('[data-testid="context-menu"] button').map((button) => button.text())).toEqual(["Copy notes", "Copy secret reference for notes"]);
    await noteWrapper.find('[data-testid="context-menu-copy-notes"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "note", "notes");
    noteWrapper.unmount();
  });

  it("keeps the system menu for selected text", async () => {
    store.selectedItem = githubDetail;
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    const usernameRow = wrapper.findAll(".field").find((row) => row.text().includes("alice"))!;
    window.getSelection()!.selectAllChildren(usernameRow.element);
    const event = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
    usernameRow.element.dispatchEvent(event);
    await flushPromises();
    expect(event.defaultPrevented).toBe(false);
    expect(wrapper.find('[data-testid="context-menu"]').exists()).toBe(false);
    window.getSelection()!.removeAllRanges();
    wrapper.unmount();
  });
});
