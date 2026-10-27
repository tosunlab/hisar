import type { CredentialChangeResult, PendingRecoverySummary, SignUpResult, Status } from "./api";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  allTestVault,
  createApiMock,
  createDevice,
  createIdentityItem,
  createItem,
  createPerson,
  createReceivedShareInvitation,
  createSentShareInvitation,
  createStatus,
  createVaultMember,
  testBob,
} from "./testing";
import { loadSelectedSectionID } from "./settingsSections";

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
    listVaultID: "",
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

  it("creates an account without an invite on an open server", async () => {
    api.signUpWithoutInvite.mockResolvedValue({ status: createStatus("unlocked"), secretKey: "H1-SECRET" });
    api.listAllVault.mockResolvedValue([allTestVault[0]]);
    api.listAllItem.mockResolvedValue([]);
    expect(await storeModule.signUpWithoutInvite("hisar.test", "alice@example.com", "Alice", "long password")).toBe(true);
    expect(api.signUpWithoutInvite).toHaveBeenCalledWith("hisar.test", "alice@example.com", "Alice", "long password");
    expect(api.signUp).not.toHaveBeenCalled();
    expect(store.status.state).toBe("unlocked");
    expect(store.pendingSecretKey).toBe("H1-SECRET");
  });

  it("shows the server's answer when sign-ups without an invite are closed", async () => {
    api.signUpWithoutInvite.mockRejectedValue(new Error("Sign-ups are not open on this server. Ask an admin for an invitation."));
    expect(await storeModule.signUpWithoutInvite("hisar.test", "alice@example.com", "Alice", "long password")).toBe(false);
    expect(store.errorMessage).toBe("Sign-ups are not open on this server. Ask an admin for an invitation.");
    expect(store.status.state).toBe("signed-out");
    expect(store.isBusy).toBe(false);
  });

  it("delivers a committed key replacement despite a local cache warning", async () => {
    const result = {
      status: createStatus("unlocked"),
      secretKey: "H1-NEW-SECRET",
      signedOutDeviceCount: -1,
      keychainError: "",
      localError: "your credentials were changed, but this device's offline copy of the vaults could not be deleted",
    };
    api.replaceSecretKey.mockResolvedValue(result);
    expect(await storeModule.replaceSecretKey("master password")).toEqual(result);
    expect(store.status.state).toBe("unlocked");
    expect(store.errorMessage).toBe(result.localError);
    expect(store.isBusy).toBe(false);
  });

  it("reports a committed password change and its local warning together", async () => {
    const result = {
      status: createStatus("unlocked"),
      secretKey: "",
      signedOutDeviceCount: 0,
      keychainError: "",
      localError: "your credentials were changed, but the offline copy could not be refreshed",
    };
    api.changeMasterPassword.mockResolvedValue(result);
    expect(await storeModule.changeMasterPassword("old password", "new password")).toEqual(result);
    expect(store.errorMessage).toBe(result.localError);
  });

  it("hides the new account's Secret Key when Hisar locks before it was confirmed", async () => {
    api.signUp.mockResolvedValue({ status: createStatus("unlocked"), secretKey: "H1-SECRET" });
    api.listAllVault.mockResolvedValue([allTestVault[0]]);
    api.listAllItem.mockResolvedValue([]);
    await storeModule.signUp("https://hisar.test/invite/x", "alice@example.com", "Alice", "long password");
    api.getStatus.mockResolvedValue(createStatus("locked"));
    await storeModule.handleLocked();
    expect(store.pendingSecretKey).toBe("");
  });

  it("shows sign-in and the error when signing out could not reach the server", async () => {
    store.status = createStatus("unlocked");
    store.selectedItem = createItem({ password: "secret" });
    api.signOut.mockRejectedValue(new Error("signed out here, but Hisar could not reach the server to end this device's session there"));
    api.getStatus.mockResolvedValue(createStatus("signed-out"));
    await storeModule.signOut();
    expect(store.status.state).toBe("signed-out");
    expect(store.selectedItem).toBeNull();
    expect(store.errorMessage).toContain("could not reach the server");
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

  it("handles the auto-lock event", async () => {
    store.status = createStatus("unlocked");
    store.selectedItem = createItem({ password: "secret" });
    api.getStatus.mockResolvedValue(createStatus("locked"));
    await storeModule.handleLocked();
    expect(store.status.state).toBe("locked");
    expect(store.selectedItem).toBeNull();
  });

  it("shows sign-in with the reason when the server signed this device out", async () => {
    store.status = createStatus("unlocked");
    store.selectedItem = createItem({ password: "secret" });
    const reason = "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one.";
    api.getStatus.mockResolvedValue({ ...createStatus("signed-out"), email: "", signedOutReason: reason });
    await storeModule.handleLocked();
    expect(store.status.state).toBe("signed-out");
    expect(store.status.signedOutReason).toBe(reason);
    expect(store.selectedItem).toBeNull();
  });

  it("follows an unlock from the approval window", async () => {
    store.status = createStatus("locked");
    api.getStatus.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue(allTestVault);
    api.listAllItem.mockResolvedValue([]);
    await storeModule.handleUnlockedElsewhere();
    expect(store.status.state).toBe("unlocked");
    expect(store.allVault).toHaveLength(2);

    await storeModule.handleUnlockedElsewhere();
    expect(api.getStatus).toHaveBeenCalledTimes(1);
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

  it("filters items by tag, searches tags and lists the tags with their counts", () => {
    const allItem = [
      createItem({ id: "1", title: "GitHub", username: "octocat", tags: ["work", "dev"] }),
      createItem({ id: "2", title: "Bank", tags: ["Finance"] }),
      createItem({ id: "3", title: "Email", username: "alice", tags: ["work"] }),
      createItem({ id: "4", title: "Untagged" }),
    ];
    expect(storeModule.filterAllItem(allItem, "", "work").map((vaultItem) => vaultItem.id)).toEqual(["1", "3"]);
    expect(storeModule.filterAllItem(allItem, "alice", "work").map((vaultItem) => vaultItem.id)).toEqual(["3"]);
    expect(storeModule.filterAllItem(allItem, "fin").map((vaultItem) => vaultItem.id)).toEqual(["2"]);
    expect(storeModule.listAllTag(allItem)).toEqual([
      { name: "dev", itemCount: 1 },
      { name: "Finance", itemCount: 1 },
      { name: "work", itemCount: 2 },
    ]);
    storeModule.selectTag("work");
    expect(store.selectedTag).toBe("work");
    storeModule.selectTag("work");
    expect(store.selectedTag).toBe("");

    store.isFavoritesSelected = true;
    storeModule.selectTag("dev");
    expect(store.selectedTag).toBe("dev");
    expect(store.isFavoritesSelected).toBe(false);
  });

  it("clears the tag filter when a vault is selected or the last tagged item is deleted", async () => {
    store.selectedVaultID = "personal";
    store.listVaultID = "personal";
    store.allItem = [createItem({ id: "1", tags: ["work"] }), createItem({ id: "2", title: "Bank" })];
    store.selectedTag = "work";
    api.deleteItem.mockResolvedValue(undefined);
    await storeModule.deleteItem("1");
    expect(store.selectedTag).toBe("");

    store.selectedTag = "work";
    api.listAllItem.mockResolvedValue([]);
    await storeModule.selectVault("family");
    expect(store.selectedTag).toBe("");
  });

  it("finds an identity by the name and email the list shows", () => {
    const identity = { ...createIdentityItem().identity!, middleName: "M.", email: "" };
    const allItem = [
      createIdentityItem({ id: "passport", title: "Passport", identity }),
      createIdentityItem({ id: "work", title: "Work", identity: { ...identity, firstName: "", middleName: "", lastName: "", email: "bob@example.com" } }),
      createItem({ id: "login", title: "GitHub", username: "carol" }),
    ];
    expect(storeModule.filterAllItem(allItem, "alice").map((vaultItem) => vaultItem.id)).toEqual(["passport"]);
    expect(storeModule.filterAllItem(allItem, "Alice M. Example").map((vaultItem) => vaultItem.id)).toEqual(["passport"]);
    expect(storeModule.filterAllItem(allItem, "bob@example").map((vaultItem) => vaultItem.id)).toEqual(["work"]);
  });

  it("finds an identity by its address, where imports put a person's address", () => {
    const identity = createIdentityItem().identity!;
    const allItem = [
      createIdentityItem({ id: "alice", identity: { ...identity, street: "1 Main St", city: "Springfield", country: "US" } }),
      createIdentityItem({ id: "bob", title: "Bob", identity: { ...identity, firstName: "Bob", street: "", city: "Shelbyville", country: "TR" } }),
    ];
    expect(storeModule.filterAllItem(allItem, "springfield").map((vaultItem) => vaultItem.id)).toEqual(["alice"]);
    expect(storeModule.filterAllItem(allItem, "main st").map((vaultItem) => vaultItem.id)).toEqual(["alice"]);
    expect(storeModule.filterAllItem(allItem, "tr").map((vaultItem) => vaultItem.id)).toEqual(["bob"]);
  });

  it("saves an item, reloads the list and shows it", async () => {
    store.selectedVaultID = "personal";
    store.listVaultID = "personal";
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
    store.listVaultID = "personal";
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

describe("openSettings", () => {
  it("opens Settings at the section the app asks for and ignores an unknown one", async () => {
    resetStore();
    store.isSettingsOpen = false;
    await storeModule.openSettings("no-such-section");
    expect(store.isSettingsOpen).toBe(false);
    await storeModule.openSettings("developer");
    expect(store.isSettingsOpen).toBe(true);
    expect(loadSelectedSectionID()).toBe("developer");
  });

  it("opens the Settings window instead of the overlay where there is one, also while locked", async () => {
    resetStore();
    store.isSettingsOpen = false;
    store.status = createStatus("unlocked");
    api.showSettingsWindow.mockResolvedValueOnce(true).mockResolvedValueOnce(true);
    await storeModule.openSettings("security");
    expect(api.showSettingsWindow).toHaveBeenCalledWith("security");
    expect(store.isSettingsOpen).toBe(false);
    expect(loadSelectedSectionID()).toBe("security");

    store.status = createStatus("locked");
    await storeModule.openSettings("developer");
    expect(api.showSettingsWindow).toHaveBeenLastCalledWith("developer");
    expect(store.isSettingsOpen).toBe(false);
  });
});

describe("openFromSettings", () => {
  it("opens the pending recovery a Secret Key replacement in the Settings window left", async () => {
    const replacement: PendingRecoverySummary = {
      operationId: "replacement", kind: "secret-key", origin: "https://hisar.test", accountId: "account", email: "alice@example.com", name: "Alice",
      phase: "delivery-pending", kitSaved: false, manualUnlock: false, keychainProtected: true, revision: 1, problems: [],
    };
    resetStore();
    store.status = createStatus("unlocked");
    store.isSettingsOpen = true;
    store.pendingRecovery = [{ ...replacement, operationId: "earlier" }];
    store.isRecoveryOpen = false;
    api.listPendingRecovery.mockResolvedValueOnce([{ ...replacement, operationId: "earlier" }, replacement]);
    await storeModule.openFromSettings("recovery");
    expect(store.isSettingsOpen).toBe(false);
    expect(store.pendingRecovery.map((operation) => operation.operationId)).toEqual(["earlier", "replacement"]);
    expect(store.isRecoveryOpen).toBe(true);
  });
});

describe("vault item counts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("updates the selected vault's count after saving and deleting", async () => {
    store.allVault = [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 }];
    store.selectedVaultID = "personal";
    store.listVaultID = "personal";
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

describe("sharing, people and devices", () => {
  const bob = createVaultMember({ ...testBob, role: "editor" });
  const sentToDave = createSentShareInvitation();
  const fromCarol = createReceivedShareInvitation();

  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    Object.assign(store, { status: createStatus("unlocked"), selectedVaultID: "family", listVaultID: "family", isSharingOpen: false, allVaultMember: [], allPerson: [], allDevice: [] });
    api.listAllVaultMember.mockResolvedValue([createVaultMember(), bob]);
    api.listAllPerson.mockResolvedValue([createPerson(), createPerson({ ...testBob, role: "member", isCurrentUser: false })]);
    api.listAllShareInvitation.mockResolvedValue({ received: [fromCarol], sent: [sentToDave], completionError: "" });
  });

  it("opens sharing and loads the vault members and the invitations", async () => {
    await storeModule.openSharing();
    expect(store.isSharingOpen).toBe(true);
    expect(api.listAllVaultMember).toHaveBeenCalledWith("family");
    expect(store.allVaultMember.map((member) => member.accountId)).toEqual(["ALICE1", "BOB222"]);
    expect(storeModule.getAllSentShareInvitation("family")).toEqual([sentToDave]);
    expect(storeModule.getAllSentShareInvitation("work")).toEqual([]);
    expect(api.listAllPerson).not.toHaveBeenCalled();

    store.errorMessage = "old error";
    storeModule.closeSharing();
    expect(store.isSharingOpen).toBe(false);
    expect(store.errorMessage).toBe("");
  });

  it("sends a share invitation by email, changes roles and reloads the members", async () => {
    api.shareVault.mockResolvedValue(undefined);
    expect(await storeModule.shareVault(" Carol@Example.com ", "reader")).toBe(true);
    expect(api.shareVault).toHaveBeenCalledWith("family", " Carol@Example.com ", "reader");
    expect(api.listAllVaultMember).toHaveBeenCalledTimes(1);
    expect(store.noticeMessage).toBe("Invitation sent to carol@example.com");

    api.changeVaultMemberRole.mockResolvedValue(undefined);
    expect(await storeModule.changeVaultMemberRole(bob, "owner")).toBe(true);
    expect(api.changeVaultMemberRole).toHaveBeenCalledWith("family", "BOB222", "owner");
    expect(store.noticeMessage).toBe("Bob is now an owner");

    api.shareVault.mockRejectedValue(new Error("this person already has access to the vault"));
    expect(await storeModule.shareVault("bob@example.com", "reader")).toBe(false);
    expect(store.errorMessage).toBe("this person already has access to the vault");
  });

  it("accepts, declines and cancels share invitations", async () => {
    api.acceptShareInvitation.mockResolvedValue(undefined);
    expect(await storeModule.acceptShareInvitation(fromCarol)).toBe(true);
    expect(api.acceptShareInvitation).toHaveBeenCalledWith("invitation-from-carol");
    expect(store.noticeMessage).toBe("Accepted. The vault appears once Carol's Hisar finishes sharing");
    expect(store.shareInvitations.received).toEqual([fromCarol]);

    api.declineShareInvitation.mockResolvedValue(undefined);
    expect(await storeModule.declineShareInvitation(fromCarol)).toBe(true);
    expect(store.noticeMessage).toBe("Invitation declined");

    api.cancelShareInvitation.mockResolvedValue(undefined);
    expect(await storeModule.cancelShareInvitation(sentToDave)).toBe(true);
    expect(api.cancelShareInvitation).toHaveBeenCalledWith("invitation-to-dave");
    expect(store.noticeMessage).toBe("Invitation to dave@example.com cancelled");

    api.acceptShareInvitation.mockRejectedValue(new Error("this invitation no longer exists"));
    expect(await storeModule.acceptShareInvitation(fromCarol)).toBe(false);
    expect(store.errorMessage).toBe("this invitation no longer exists");
  });

  it("loads invitations with the vaults, quietly, so accepted shares complete", async () => {
    api.listAllVault.mockResolvedValue(structuredClone(allTestVault));
    api.listAllItem.mockResolvedValue([]);
    await storeModule.loadAllVault();
    expect(api.listAllShareInvitation).toHaveBeenCalled();
    expect(store.shareInvitations.received).toEqual([fromCarol]);

    api.listAllShareInvitation.mockRejectedValue(new Error("cannot reach the Hisar server"));
    await storeModule.loadShareInvitationsQuietly();
    expect(store.errorMessage).toBe("");
    expect(store.shareInvitations.received).toEqual([fromCarol]);
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

  it("loads settings data, invites by email and revokes devices", async () => {
    api.listAllDevice.mockResolvedValue([createDevice(), createDevice({ id: "old", name: "Old laptop", isCurrent: false })]);
    await storeModule.loadSettingsData();
    expect(store.allPerson).toHaveLength(2);
    expect(store.allDevice).toHaveLength(2);
    expect(store.shareInvitations.sent).toEqual([sentToDave]);

    api.invitePerson.mockResolvedValue(undefined);
    expect(await storeModule.invitePerson(" Dan@Example.com ")).toBe(true);
    expect(api.invitePerson).toHaveBeenCalledWith(" Dan@Example.com ");
    expect(store.noticeMessage).toBe("Invitation sent to dan@example.com");
    api.invitePerson.mockRejectedValue(new Error("this server cannot send email yet, so invitations cannot be sent"));
    expect(await storeModule.invitePerson("dan@example.com")).toBe(false);
    expect(store.errorMessage).toBe("this server cannot send email yet, so invitations cannot be sent");

    api.revokeDevice.mockResolvedValue(undefined);
    api.listAllDevice.mockResolvedValue([createDevice()]);
    expect(await storeModule.revokeDevice(store.allDevice[1])).toBe(true);
    expect(api.revokeDevice).toHaveBeenCalledWith("old");
    expect(store.allDevice).toHaveLength(1);
    expect(store.noticeMessage).toBe("Old laptop was signed out");
  });

  it("forgets sharing data when Hisar locks", async () => {
    await storeModule.openSharing();
    await storeModule.loadSettingsData();
    store.allDevice = [createDevice()];
    storeModule.handleLocked();
    expect(store.isSharingOpen).toBe(false);
    expect(store.allVaultMember).toEqual([]);
    expect(store.allPerson).toEqual([]);
    expect(store.shareInvitations).toEqual({ received: [], sent: [], completionError: "" });
    expect(store.allDevice).toEqual([]);
  });
});

it("judge refuses a sign-up Secret Key response that arrives after lock", async () => {
  resetStore();
  const { promise, resolve: finish } = Promise.withResolvers<SignUpResult>();
  api.signUp.mockImplementation(() => promise);
  api.listAllVault.mockResolvedValue([allTestVault[0]]);
  api.listAllItem.mockResolvedValue([]);
  const pending = storeModule.signUp("https://hisar.test/invite/x", "alice@example.com", "Alice", "long password");
  api.getStatus.mockResolvedValue(createStatus("locked"));
  await storeModule.handleLocked();
  finish({ status: createStatus("unlocked"), secretKey: "H1-SECRET", pendingOperationId: "", setupState: "", problems: [] });
  await pending;
  expect(store.pendingSecretKey).toBe("");
  expect(store.status.state).toBe("locked");
});

it("refuses a replacement Secret Key response that arrives after lock", async () => {
  resetStore();
  store.status = createStatus("unlocked");
  const { promise, resolve: finish } = Promise.withResolvers<CredentialChangeResult>();
  api.replaceSecretKey.mockImplementation(() => promise);
  const pending = storeModule.replaceSecretKey("password");
  api.getStatus.mockResolvedValue(createStatus("locked"));
  await storeModule.handleLocked();
  finish({ status: createStatus("unlocked"), secretKey: "H1-REPLACEMENT", signedOutDeviceCount: 1, keychainError: "", localError: "", pendingOperationId: "", setupState: "", problems: [] });
  expect(await pending).toBeUndefined();
  expect(store.pendingSecretKey).toBe("");
  expect(store.status.state).toBe("locked");
});

it("does not install a staged signup when lock occurs during journal refresh", async () => {
  resetStore();
  const journal = Promise.withResolvers<PendingRecoverySummary[]>();
  api.signUp.mockResolvedValue({
    status: createStatus("unlocked"), secretKey: "H1-SECRET", pendingOperationId: "pending",
    setupState: "delivery-pending", problems: ["The keychain could not save the key."],
  });
  api.listPendingRecovery.mockReturnValue(journal.promise);
  const pending = storeModule.signUp("https://hisar.test/invite/x", "alice@example.com", "Alice", "password");
  await Promise.resolve();
  api.getStatus.mockResolvedValue(createStatus("locked"));
  await storeModule.handleLocked();
  journal.resolve([]);
  expect(await pending).toBe(false);
  expect(store.status.state).toBe("locked");
  expect(store.pendingSecretKey).toBe("");
});

it("ignores a password unlock response that arrives after a newer lock", async () => {
  resetStore();
  store.status = createStatus("locked");
  const unlocked = Promise.withResolvers<Status>();
  api.unlock.mockReturnValue(unlocked.promise);
  const pending = storeModule.unlock("password");
  api.getStatus.mockResolvedValue(createStatus("locked"));
  await storeModule.handleLocked();
  unlocked.resolve(createStatus("unlocked"));
  expect(await pending).toBe(false);
  expect(store.status.state).toBe("locked");
});

it("discovers undelivered recovery on restart without automatically unlocking", async () => {
  resetStore();
  Object.assign(store, { pendingRecovery: [], isRecoveryOpen: false });
  api.getStatus.mockResolvedValue(createStatus("locked"));
  api.listPendingRecovery.mockResolvedValue([{
    operationId: "pending", kind: "signup", origin: "https://hisar.test", accountId: "account",
    email: "alice@example.com", name: "Alice", phase: "delivery-pending", kitSaved: false,
    manualUnlock: false, problems: [],
  }]);
  await storeModule.loadStatus();
  expect(store.isRecoveryOpen).toBe(true);
  expect(store.pendingRecovery[0].phase).toBe("delivery-pending");
  expect(store.status.state).toBe("locked");
  expect(api.resumePendingRecovery).not.toHaveBeenCalled();
});

it("keeps Finish later when the status is read again", async () => {
  resetStore();
  const pending = {
    operationId: "pending", kind: "signup", origin: "https://hisar.test", accountId: "account",
    email: "alice@example.com", name: "Alice", phase: "manual-unlock", revision: 2, kitSaved: true,
    keychainProtected: false, manualUnlock: true, problems: [],
  };
  Object.assign(store, { status: createStatus("locked"), pendingRecovery: [pending], isRecoveryOpen: false });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listPendingRecovery.mockResolvedValue([pending]);
  api.listAllVault.mockResolvedValue([allTestVault[0]]);
  api.listAllItem.mockResolvedValue([]);
  await storeModule.handleUnlockedElsewhere();
  expect(store.status.state).toBe("unlocked");
  expect(store.pendingRecovery).toHaveLength(1);
  expect(store.isRecoveryOpen).toBe(false);
});

it("keeps committed signup warnings visible after loading the unlocked account", async () => {
  resetStore();
  api.signUp.mockResolvedValue({
    status: createStatus("unlocked"), secretKey: "H1-NEW", pendingOperationId: "pending",
    setupState: "needs-finishing", problems: ["Recovery progress could not be saved."],
  });
  api.listPendingRecovery.mockResolvedValue([{
    operationId: "pending", kind: "signup", origin: "https://hisar.test", accountId: "account",
    email: "alice@example.com", name: "Alice", phase: "outcome-unknown", revision: 1,
    kitSaved: false, keychainProtected: true, manualUnlock: false, problems: [],
  }]);
  api.listAllVault.mockResolvedValue([allTestVault[0]]);
  api.listAllItem.mockResolvedValue([]);
  await storeModule.signUp("https://hisar.test/invite/x", "alice@example.com", "Alice", "password");
  expect(store.status.state).toBe("unlocked");
  expect(store.recoveryFactor?.secretKey).toBe("H1-NEW");
  expect(store.errorMessage).toContain("Recovery progress could not be saved.");
});
