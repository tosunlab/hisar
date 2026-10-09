import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  allTestVault,
  createApiMock,
  createDevice,
  createFamilyMember,
  createItem,
  createPendingRecovery,
  createRecoveryStatus,
  createStatus,
  createVaultMember,
  testBob,
  testCarol,
} from "./testing";

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

const storeModule = await import("./store");
const { store } = storeModule;
const api = apiMock.current;

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("signed-out"),
    allVault: [],
    selectedVaultID: "",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    pendingSecretKey: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
}

describe("store", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("signs in, loads vaults and selects the first vault", async () => {
    api.signIn.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue(allTestVault);
    api.listAllItem.mockResolvedValue([createItem({ id: "b", title: "zeta" }), createItem({ id: "a", title: "Alpha" })]);

    expect(await storeModule.signIn("https://hisar.test", "alice@example.com", "H1-KEY", "pw")).toBe(true);

    expect(api.signIn).toHaveBeenCalledWith("https://hisar.test", "alice@example.com", "H1-KEY", "pw");
    expect(store.status.state).toBe("unlocked");
    expect(store.selectedVaultID).toBe("personal");
    expect(store.allItem.map((vaultItem) => vaultItem.title)).toEqual(["Alpha", "zeta"]);
  });

  it("shows the error message when sign in fails", async () => {
    api.signIn.mockRejectedValue(new Error("incorrect email, master password or Secret Key"));
    expect(await storeModule.signIn("s", "e", "k", "p")).toBe(false);
    expect(store.errorMessage).toBe("incorrect email, master password or Secret Key");
    expect(store.status.state).toBe("signed-out");
    expect(store.isBusy).toBe(false);
  });

  it("keeps the Secret Key pending after sign up until confirmed", async () => {
    api.signUp.mockResolvedValue({ status: createStatus("unlocked"), secretKey: "H1-SECRET" });
    api.listAllVault.mockResolvedValue([allTestVault[0]]);
    api.listAllItem.mockResolvedValue([]);
    await storeModule.signUp("https://hisar.test/invite/x", "alice@example.com", "Alice", "long password");
    expect(store.pendingSecretKey).toBe("H1-SECRET");
    storeModule.confirmSecretKeySaved();
    expect(store.pendingSecretKey).toBe("");
  });

  it("drops all vault data when the backend reports it is locked", async () => {
    store.status = createStatus("unlocked");
    store.allItem = [createItem()];
    store.selectedItem = createItem({ password: "secret" });
    api.getItem.mockRejectedValue(new Error("Hisar is locked"));
    await storeModule.selectItem("item-1");
    expect(store.status.state).toBe("locked");
    expect(store.allItem).toEqual([]);
    expect(store.selectedItem).toBeNull();
  });

  it("handles the auto-lock event", () => {
    store.status = createStatus("unlocked");
    store.selectedItem = createItem({ password: "secret" });
    storeModule.handleLocked();
    expect(store.status.state).toBe("locked");
    expect(store.selectedItem).toBeNull();
  });

  it("filters items by title, username and URL", () => {
    const allItem = [
      createItem({ id: "1", title: "GitHub", username: "alice", urls: ["https://github.com"] }),
      createItem({ id: "2", title: "Bank", username: "family", urls: ["https://bank.example"] }),
    ];
    expect(storeModule.filterAllItem(allItem, "").length).toBe(2);
    expect(storeModule.filterAllItem(allItem, "git").map((vaultItem) => vaultItem.id)).toEqual(["1"]);
    expect(storeModule.filterAllItem(allItem, "FAMILY").map((vaultItem) => vaultItem.id)).toEqual(["2"]);
    expect(storeModule.filterAllItem(allItem, "bank.example").map((vaultItem) => vaultItem.id)).toEqual(["2"]);
  });

  it("saves an item, reloads the list and shows it", async () => {
    store.selectedVaultID = "personal";
    store.editorMode = "create";
    const saved = createItem({ id: "new", title: "New" });
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    expect(await storeModule.saveItem(createItem({ id: "", title: "New" }))).toBe(true);
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ title: "New" }));
    expect(store.selectedItem?.id).toBe("new");
    expect(store.editorMode).toBe("view");
    expect(store.noticeMessage).toBe("Saved");
  });

  it("deletes an item", async () => {
    store.selectedVaultID = "personal";
    store.allItem = [createItem({ id: "1" }), createItem({ id: "2" })];
    store.selectedItem = store.allItem[0];
    api.deleteItem.mockResolvedValue(undefined);
    await storeModule.deleteItem("1");
    expect(api.deleteItem).toHaveBeenCalledWith("personal", "1");
    expect(store.allItem.map((vaultItem) => vaultItem.id)).toEqual(["2"]);
    expect(store.selectedItem).toBeNull();
  });

  it("copies a field through the backend and tells the user it will be cleared", async () => {
    store.selectedVaultID = "personal";
    store.selectedItem = createItem({ id: "1", password: "secret" });
    api.copyField.mockResolvedValue(undefined);
    await storeModule.copyField("password", "Password");
    expect(api.copyField).toHaveBeenCalledWith("personal", "1", "password");
    expect(store.noticeMessage).toContain("clears in 90 seconds");
  });
});

describe("openWebsite", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("opens http(s) links and adds https to bare domains", async () => {
    await storeModule.openWebsite("https://github.com");
    await storeModule.openWebsite("netflix.com");
    expect(api.openURL).toHaveBeenNthCalledWith(1, "https://github.com");
    expect(api.openURL).toHaveBeenNthCalledWith(2, "https://netflix.com");
  });

  it("refuses other URL schemes", async () => {
    await storeModule.openWebsite("javascript:alert(1)");
    await storeModule.openWebsite("file:///etc/passwd");
    expect(api.openURL).not.toHaveBeenCalled();
    expect(store.errorMessage).toContain("Only http and https");
  });
});

describe("vault item counts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("updates the selected vault's count after saving and deleting", async () => {
    store.allVault = [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1, recoverable: true, recoveryOptOut: false }];
    store.selectedVaultID = "personal";
    store.allItem = [createItem({ id: "1" })];
    api.saveItem.mockResolvedValue(createItem({ id: "2" }));
    api.listAllItem.mockResolvedValue([createItem({ id: "1" }), createItem({ id: "2" })]);
    await storeModule.saveItem(createItem({ id: "", title: "New" }));
    expect(store.allVault[0].itemCount).toBe(2);

    api.deleteItem.mockResolvedValue(undefined);
    await storeModule.deleteItem("1");
    expect(store.allVault[0].itemCount).toBe(1);
  });
});

describe("sharing, family and devices", () => {
  const bob = createVaultMember({ ...testBob, role: "editor" });

  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    Object.assign(store, { selectedVaultID: "family", isSharingOpen: false, allVaultMember: [], allFamilyMember: [], allDevice: [] });
    api.listAllVaultMember.mockResolvedValue([createVaultMember(), bob]);
    api.listAllMember.mockResolvedValue([createFamilyMember(), createFamilyMember({ ...testBob, role: "member" })]);
  });

  it("opens sharing and loads the vault and family members", async () => {
    await storeModule.openSharing();
    expect(store.isSharingOpen).toBe(true);
    expect(api.listAllVaultMember).toHaveBeenCalledWith("family");
    expect(store.allVaultMember.map((member) => member.accountId)).toEqual(["ALICE1", "BOB222"]);
    expect(store.allFamilyMember).toHaveLength(2);

    store.errorMessage = "old error";
    storeModule.closeSharing();
    expect(store.isSharingOpen).toBe(false);
    expect(store.errorMessage).toBe("");
  });

  it("shares, changes roles and reloads the members", async () => {
    api.shareVault.mockResolvedValue(undefined);
    expect(await storeModule.shareVault(createFamilyMember({ ...testCarol, role: "member" }), "reader")).toBe(true);
    expect(api.shareVault).toHaveBeenCalledWith("family", "carol@example.com", "reader");
    expect(api.listAllVaultMember).toHaveBeenCalledTimes(1);
    expect(store.noticeMessage).toBe("Shared with Carol");

    api.changeVaultMemberRole.mockResolvedValue(undefined);
    expect(await storeModule.changeVaultMemberRole(bob, "owner")).toBe(true);
    expect(api.changeVaultMemberRole).toHaveBeenCalledWith("family", "BOB222", "owner");
    expect(store.noticeMessage).toBe("Bob is now an owner");

    api.shareVault.mockRejectedValue(new Error("the signing key of carol@example.com changed since you last shared with them; stopping"));
    expect(await storeModule.shareVault(createFamilyMember({ ...testCarol }), "reader")).toBe(false);
    expect(store.errorMessage).toContain("signing key of carol@example.com changed");
  });

  it("reloads the items after removing a member because the vault was re-encrypted", async () => {
    store.selectedItem = createItem({ revision: "old" });
    api.removeVaultMember.mockResolvedValue(undefined);
    api.listAllItem.mockResolvedValue([createItem({ revision: "new" })]);
    expect(await storeModule.removeVaultMember(bob)).toBe(true);
    expect(api.removeVaultMember).toHaveBeenCalledWith("family", "BOB222");
    expect(store.allItem[0].revision).toBe("new");
    expect(store.selectedItem).toBeNull();
    expect(store.noticeMessage).toBe("Removed Bob and re-encrypted the vault");

    api.removeVaultMember.mockRejectedValue(new Error("only owners of this vault can share it, change roles or remove members"));
    expect(await storeModule.removeVaultMember(bob)).toBe(false);
    expect(store.errorMessage).toContain("only owners");
  });

  it("loads settings data, invites and revokes devices", async () => {
    api.listAllDevice.mockResolvedValue([createDevice(), createDevice({ id: "old", name: "Old laptop", isCurrent: false })]);
    await storeModule.loadSettingsData();
    expect(store.allFamilyMember).toHaveLength(2);
    expect(store.allDevice).toHaveLength(2);

    api.inviteMember.mockResolvedValue("https://hisar.test/invite/abc");
    expect(await storeModule.inviteMember("dan@example.com", "member")).toBe("https://hisar.test/invite/abc");
    api.inviteMember.mockRejectedValue(new Error("only family admins can invite people"));
    expect(await storeModule.inviteMember("dan@example.com", "member")).toBeUndefined();
    expect(store.errorMessage).toBe("only family admins can invite people");

    api.revokeDevice.mockResolvedValue(undefined);
    api.listAllDevice.mockResolvedValue([createDevice()]);
    expect(await storeModule.revokeDevice(store.allDevice[1])).toBe(true);
    expect(api.revokeDevice).toHaveBeenCalledWith("old");
    expect(store.allDevice).toHaveLength(1);
    expect(store.noticeMessage).toBe("Old laptop was signed out");
  });

  it("forgets sharing data when Hisar locks", async () => {
    await storeModule.openSharing();
    store.allDevice = [createDevice()];
    storeModule.handleLocked();
    expect(store.isSharingOpen).toBe(false);
    expect(store.allVaultMember).toEqual([]);
    expect(store.allFamilyMember).toEqual([]);
    expect(store.allDevice).toEqual([]);
  });
});

describe("account recovery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    Object.assign(store, {
      status: createStatus("unlocked"),
      allVault: structuredClone(allTestVault),
      recoveryStatus: null,
      allPendingRecovery: [],
      isRecoverySubmitted: false,
      isVaultRecoveryOpen: false,
    });
  });

  it("loads the pending recoveries only for admins once recovery is set up", async () => {
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isSetUp: false }));
    await storeModule.loadRecoveryData();
    expect(store.recoveryStatus?.isSetUp).toBe(false);
    expect(api.listAllPendingRecovery).not.toHaveBeenCalled();

    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isAdmin: false }));
    await storeModule.loadRecoveryData();
    expect(api.listAllPendingRecovery).not.toHaveBeenCalled();

    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    api.listAllPendingRecovery.mockResolvedValue([createPendingRecovery()]);
    await storeModule.loadRecoveryData();
    expect(store.allPendingRecovery.map((pending) => pending.accountId)).toEqual(["BOB222"]);
  });

  it("sets up recovery and reloads the vaults", async () => {
    api.setUpRecovery.mockResolvedValue(["Old vault"]);
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    api.listAllVault.mockResolvedValue([{ ...allTestVault[0], recoverable: true }]);
    expect(await storeModule.setUpRecovery()).toEqual(["Old vault"]);
    expect(store.recoveryStatus?.isSetUp).toBe(true);
    expect(store.allVault).toHaveLength(1);
    expect(store.noticeMessage).toBe("Account recovery is set up");

    api.setUpRecovery.mockRejectedValue(new Error("only family admins can manage account recovery"));
    expect(await storeModule.setUpRecovery()).toBeUndefined();
    expect(store.errorMessage).toContain("only family admins");
  });

  it("turns recovery off and on for a vault and keeps its item count", async () => {
    store.allVault[0].itemCount = 5;
    api.disableVaultRecovery.mockResolvedValue({ ...allTestVault[0], itemCount: 0, recoverable: false, recoveryOptOut: true });
    expect(await storeModule.setVaultRecovery("personal", false)).toBe(true);
    expect(api.disableVaultRecovery).toHaveBeenCalledWith("personal");
    expect(store.allVault[0]).toMatchObject({ itemCount: 5, recoverable: false, recoveryOptOut: true });
    expect(store.noticeMessage).toBe("Recovery is off for Personal");

    api.enableVaultRecovery.mockResolvedValue({ ...allTestVault[0], recoverable: true, recoveryOptOut: false });
    expect(await storeModule.setVaultRecovery("personal", true)).toBe(true);
    expect(store.allVault[0].recoverable).toBe(true);
    expect(store.noticeMessage).toBe("Personal can be recovered again");
  });

  it("starts and completes a recovery and reloads the family", async () => {
    api.startAccountRecovery.mockResolvedValue("https://hisar.test/recover/abc");
    expect(await storeModule.startAccountRecovery(createFamilyMember({ ...testBob, role: "member" }))).toBe("https://hisar.test/recover/abc");
    expect(api.startAccountRecovery).toHaveBeenCalledWith("BOB222");

    const pending = createPendingRecovery();
    store.allPendingRecovery = [pending];
    api.completeAccountRecovery.mockResolvedValue(["Bob's personal vault"]);
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    api.listAllPendingRecovery.mockResolvedValue([]);
    api.listAllMember.mockResolvedValue([createFamilyMember(), createFamilyMember({ ...testBob, role: "member" })]);
    expect(await storeModule.completeAccountRecovery(pending)).toEqual(["Bob's personal vault"]);
    expect(api.completeAccountRecovery).toHaveBeenCalledWith("BOB222");
    expect(store.allPendingRecovery).toEqual([]);
    expect(store.allFamilyMember).toHaveLength(2);
    expect(store.noticeMessage).toBe("Recovered Bob");

    api.completeAccountRecovery.mockRejectedValue(new Error("only the admin who set up recovery can complete it"));
    expect(await storeModule.completeAccountRecovery(pending)).toBeUndefined();
    expect(store.errorMessage).toContain("only the admin who set up recovery");
  });

  it("shows the new Secret Key after recovering an account while signed out", async () => {
    store.status = createStatus("signed-out");
    api.recoverAccount.mockResolvedValue({ status: { ...createStatus("signed-out"), email: "bob@example.com" }, secretKey: "H1-NEW" });
    expect(await storeModule.recoverAccount("https://hisar.test/recover/abc", "bob@example.com", "new master password")).toBe(true);
    expect(api.recoverAccount).toHaveBeenCalledWith("https://hisar.test/recover/abc", "bob@example.com", "new master password");
    expect(store.pendingSecretKey).toBe("H1-NEW");
    expect(store.isRecoverySubmitted).toBe(true);
    expect(store.status.email).toBe("bob@example.com");

    storeModule.confirmSecretKeySaved();
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
    api.signIn.mockResolvedValue(createStatus("signed-out"));
    await storeModule.signIn("https://hisar.test", "bob@example.com", "H1-NEW", "new master password");
    expect(store.isRecoverySubmitted).toBe(false);
  });

  it("forgets recovery data when Hisar locks", () => {
    Object.assign(store, { recoveryStatus: createRecoveryStatus(), allPendingRecovery: [createPendingRecovery()], isVaultRecoveryOpen: true });
    storeModule.handleLocked();
    expect(store.recoveryStatus).toBeNull();
    expect(store.allPendingRecovery).toEqual([]);
    expect(store.isVaultRecoveryOpen).toBe(false);
  });
});
