import { beforeEach, describe, expect, it, vi } from "vitest";
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
