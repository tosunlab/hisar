import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import type { VaultSummary } from "../api";
import { createApiMock, createItem, createStatus } from "../testing";

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

const { store, lock, unlock, loadStatus } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const api = apiMock.current;

enableAutoUnmount(afterEach);

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 };
const workVault: VaultSummary = { id: "work", name: "Work", type: "shared", role: "editor", itemCount: 0 };
const github = createItem({ id: "github", title: "GitHub", password: "s3cret" });
const bank = createItem({ id: "bank", title: "Bank", username: "alice.bank" });

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: [personalVault, workVault],
    selectedVaultID: "personal",
    allItem: [bank, github],
    selectedItem: github,
    allFavorite: [],
    isFavoritesSelected: false,
    editorMode: "edit",
    editorDraft: null,
    discardDraftPrompt: null,
    searchQuery: "",
    selectedTag: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue([personalVault, workVault]);
  api.listAllItem.mockImplementation((vaultID: string) => Promise.resolve(vaultID === "personal" ? [bank, github] : []));
  api.getItem.mockImplementation((_vaultID: string, itemID: string) => Promise.resolve(itemID === "github" ? github : bank));
  api.listAllFavoriteItem.mockResolvedValue([]);
  api.generatePassword.mockResolvedValue("generated-password");
  api.lock.mockResolvedValue(createStatus("locked"));
  api.unlock.mockResolvedValue(createStatus("unlocked"));
});

function dialog(wrapper: ReturnType<typeof mount>) {
  return wrapper.find('[data-testid="discard-draft-dialog"]');
}

describe("Leaving the item editor", () => {
  it("switches items at once when nothing changed", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('[role="option"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(false);
    expect(store.editorMode).toBe("view");
    expect(store.selectedItem?.id).toBe("bank");
  });

  it("asks before another item replaces a changed draft, and Keep editing keeps it", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('input[name="title"]').setValue("GitHub Enterprise");
    await wrapper.find('[role="option"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).find("h2").text()).toBe("Discard changes to “GitHub”?");
    expect(api.getItem).not.toHaveBeenCalled();
    await dialog(wrapper).find('[data-testid="keep-editing"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(false);
    expect(store.editorMode).toBe("edit");
    expect((wrapper.find('input[name="title"]').element as HTMLInputElement).value).toBe("GitHub Enterprise");
  });

  it("Discard drops the draft and finishes the navigation", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('input[name="title"]').setValue("GitHub Enterprise");
    await wrapper.find('[data-testid="vault-work"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(true);
    await dialog(wrapper).find('[data-testid="discard-draft"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(false);
    expect(store.editorMode).toBe("view");
    expect(store.selectedVaultID).toBe("work");
    expect(store.editorDraft).toBeNull();
  });

  it("asks when Cancel or ⌘N would close a changed draft", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('textarea[name="notes"]').setValue("recovery codes");
    await wrapper.get('[data-testid="editor-cancel"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(true);
    await dialog(wrapper).find('[data-testid="keep-editing"]').trigger("click");
    await flushPromises();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "n", metaKey: true }));
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(true);
    expect(store.editorMode).toBe("edit");
  });

  it("Esc uses the shared prompt, keeps the draft until Discard, and blocks ⌘N behind it", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.get('input[name="username"]').setValue("alice@work.example");
    await wrapper.get('input[name="username"]').trigger("keydown", { key: "Escape" });
    await flushPromises();
    expect(dialog(wrapper).find("h2").text()).toBe("Discard changes to “GitHub”?");
    await dialog(wrapper).get('[data-testid="keep-editing"]').trigger("keydown", { key: "n", metaKey: true });
    await flushPromises();
    expect(store.editorMode).toBe("edit");
    expect(dialog(wrapper).exists()).toBe(true);
    await dialog(wrapper).get('[data-testid="keep-editing"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(false);
    expect((wrapper.get('input[name="username"]').element as HTMLInputElement).value).toBe("alice@work.example");
    await wrapper.get('input[name="username"]').trigger("keydown", { key: "Escape" });
    await flushPromises();
    await dialog(wrapper).get('[data-testid="discard-draft"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(false);
    expect(store.editorMode).toBe("view");
    expect(store.editorDraft).toBeNull();
    expect(api.saveItem).not.toHaveBeenCalled();
  });

  it("names a new item by its typed title, or asks about the new item", async () => {
    store.selectedItem = null;
    store.editorMode = "create";
    const wrapper = mount(MainView);
    await wrapper.find('input[name="username"]').setValue("alice");
    await wrapper.find('[role="option"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).find("h2").text()).toBe("Discard the new item?");
    await dialog(wrapper).find('[data-testid="keep-editing"]').trigger("click");
    await wrapper.find('input[name="title"]').setValue("Stripe");
    await wrapper.find('[role="option"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).find("h2").text()).toBe("Discard changes to “Stripe”?");
  });

  it("keeps the draft through a lock and opens it again after the unlock", async () => {
    const wrapper = mount(MainView);
    await wrapper.find('input[name="title"]').setValue("GitHub Enterprise");
    await lock();
    await flushPromises();
    expect(store.editorMode).toBe("view");
    expect(wrapper.find(".item-editor").exists()).toBe(false);
    await unlock("password");
    await flushPromises();
    expect(store.editorMode).toBe("edit");
    expect(store.selectedItem?.id).toBe("github");
    expect((wrapper.find('input[name="title"]').element as HTMLInputElement).value).toBe("GitHub Enterprise");
    expect(store.editorDraft?.isDirty).toBe(true);
  });
});

describe("SSH creation drafts", () => {
  beforeEach(() => {
    store.selectedItem = null;
    store.editorMode = "create-ssh-key";
  });

  it.each(["generate", "import"])("guards Cancel and navigation and restores the %s draft after unlock", async (mode) => {
    const wrapper = mount(MainView);
    await wrapper.get('input[name="title"]').setValue("Production");
    await wrapper.get('input[value="rsa"]').setValue();
    if (mode === "import") {
      await wrapper.get('[role="tab"][aria-selected="false"]').trigger("click");
      await wrapper.get('textarea[name="privateKey"]').setValue("private-key-material");
      await wrapper.get('input[name="passphrase"]').setValue("key-passphrase");
    }
    const expected = { ...store.editorDraft!.sshKey };
    await wrapper.get(".ssh-key-form .ghost").trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(true);
    await dialog(wrapper).get('[data-testid="keep-editing"]').trigger("click");
    await wrapper.get('[data-testid="vault-work"]').trigger("click");
    await flushPromises();
    expect(dialog(wrapper).exists()).toBe(true);
    await dialog(wrapper).get('[data-testid="keep-editing"]').trigger("click");
    await lock();
    await flushPromises();
    expect(wrapper.find(".ssh-key-form").exists()).toBe(false);
    await unlock("password");
    await flushPromises();
    expect(store.editorDraft?.sshKey).toEqual(expected);
    expect(wrapper.get('input[name="title"]').element).toHaveProperty("value", "Production");
    await wrapper.get(".ssh-key-form .ghost").trigger("click");
    await flushPromises();
    await dialog(wrapper).get('[data-testid="discard-draft"]').trigger("click");
    await flushPromises();
    expect(store.editorDraft).toBeNull();
  });

  it.each(["generate", "import"])("clears a saved %s draft", async (mode) => {
    api.generateSSHKey.mockResolvedValue(createItem({ category: "ssh-key" }));
    api.importSSHKey.mockResolvedValue(createItem({ category: "ssh-key" }));
    const wrapper = mount(MainView);
    await wrapper.get('input[name="title"]').setValue("Production");
    if (mode === "import") {
      await wrapper.get('[role="tab"][aria-selected="false"]').trigger("click");
      await wrapper.get('textarea[name="privateKey"]').setValue("private-key-material");
      await wrapper.get('input[name="passphrase"]').setValue("key-passphrase");
    }
    await wrapper.get(".ssh-key-form").trigger("submit");
    await flushPromises();
    expect(store.editorDraft).toBeNull();
    expect(wrapper.find(".ssh-key-form").exists()).toBe(false);
  });

  it("forgets imported secrets on sign-out, including a locked draft", async () => {
    const wrapper = mount(MainView);
    await wrapper.get('[role="tab"][aria-selected="false"]').trigger("click");
    await wrapper.get('textarea[name="privateKey"]').setValue("private-key-material");
    await wrapper.get('input[name="passphrase"]').setValue("key-passphrase");
    await lock();
    api.getStatus.mockResolvedValue(createStatus("signed-out"));
    await loadStatus();
    await unlock("password");
    await flushPromises();
    expect(store.editorDraft).toBeNull();
    expect(store.editorMode).toBe("view");
  });
});
