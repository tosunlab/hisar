import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createImportPreview, createImportProgress, createItem, createStatus } from "../testing";
import type { VaultSummary } from "../api";

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

const store = await import("../store");
const ImportPanel = (await import("./ImportPanel.vue")).default;
const MainView = (await import("./MainView.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const api = apiMock.current;

const allImportTestVault: VaultSummary[] = [
  ...allTestVault,
  { id: "work", name: "Work", type: "shared", role: "editor", itemCount: 0, recoverable: true, recoveryOptOut: false },
];
const allTestFormat = [
  { id: "1pux", name: "1Password export (.1pux)" },
  { id: "csv", name: "CSV (Chrome, Firefox, Safari, Edge and others)" },
];

function resetStore(): void {
  Object.assign(store.store, {
    status: createStatus("unlocked"),
    allVault: allImportTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isImportOpen: false,
    importStep: "choose",
    allImportFormat: [],
    importPreview: null,
    importProgress: null,
    isImportFileDeleted: false,
  });
}

function findButton(wrapper: ReturnType<typeof mount>, text: string) {
  const button = wrapper.findAll("button").find((candidate) => candidate.text().includes(text));
  if (!button) {
    throw new Error(`no button with text ${text}`);
  }
  return button;
}

async function openPreview(previewOverrides = {}): Promise<ReturnType<typeof mount>> {
  api.chooseImportFile.mockResolvedValue(createImportPreview(previewOverrides));
  await store.openImport();
  const wrapper = mount(ImportPanel);
  await flushPromises();
  await wrapper.find('[data-testid="import-choose"]').trigger("submit");
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
  api.listAllImportFormat.mockResolvedValue(allTestFormat);
  api.listAllVault.mockResolvedValue(allImportTestVault);
  api.listAllItem.mockResolvedValue([]);
  api.chooseImportFile.mockResolvedValue(createImportPreview());
});

describe("Import store", () => {
  it("opens over Settings and loads the formats once", async () => {
    store.store.isSettingsOpen = true;
    await store.openImport();
    expect(store.store.isSettingsOpen).toBe(false);
    expect(store.store.isImportOpen).toBe(true);
    expect(store.store.allImportFormat).toEqual(allTestFormat);
    store.closeImport();
    await store.openImport();
    expect(api.listAllImportFormat).toHaveBeenCalledTimes(1);
  });

  it("stays on the first step when the file dialog is cancelled", async () => {
    api.chooseImportFile.mockResolvedValue(createImportPreview({ fileName: "" }));
    await store.openImport();
    expect(await store.chooseImportFile("personal", "")).toBe(false);
    expect(store.store.importStep).toBe("choose");
    expect(store.store.errorMessage).toBe("");
  });

  it("saves batch by batch, then reloads the vault and reports the count", async () => {
    await store.openImport();
    await store.chooseImportFile("personal", "bitwarden-json");
    expect(api.chooseImportFile).toHaveBeenCalledWith("personal", "bitwarden-json");
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport
      .mockResolvedValueOnce(createImportProgress({ savedCount: 10 }))
      .mockResolvedValueOnce(createImportProgress({ savedCount: 11, isDone: true }));
    api.listAllItem.mockResolvedValue([createItem({ id: "b", title: "beta" }), createItem({ id: "a", title: "Alpha" })]);
    await store.startImport(true);
    expect(api.startImport).toHaveBeenCalledWith("personal", true);
    expect(api.continueImport).toHaveBeenCalledTimes(2);
    expect(store.store.importStep).toBe("done");
    expect(store.store.allItem.map((vaultItem) => vaultItem.title)).toEqual(["Alpha", "beta"]);
    expect(store.store.noticeMessage).toBe("Imported 11 items into Personal");
  });

  it("does not reload another vault's items after importing elsewhere", async () => {
    api.chooseImportFile.mockResolvedValue(createImportPreview({ vaultId: "work", vaultName: "Work" }));
    await store.openImport();
    await store.chooseImportFile("work", "");
    api.startImport.mockResolvedValue(createImportProgress({ vaultId: "work", vaultName: "Work" }));
    api.continueImport.mockResolvedValue(createImportProgress({ vaultId: "work", vaultName: "Work", savedCount: 1, totalCount: 1, isDone: true }));
    await store.startImport(false);
    expect(api.listAllVault).toHaveBeenCalled();
    expect(api.listAllItem).not.toHaveBeenCalled();
    expect(store.store.noticeMessage).toBe("Imported 1 item into Work");
  });

  it("pauses on an error and continues where it stopped", async () => {
    await store.openImport();
    await store.chooseImportFile("personal", "");
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport
      .mockResolvedValueOnce(createImportProgress({ savedCount: 10 }))
      .mockRejectedValueOnce(new Error("cannot reach the Hisar server; changes can be saved once you are back online"));
    await store.startImport(true);
    expect(store.store.importStep).toBe("saving");
    expect(store.store.importProgress?.savedCount).toBe(10);
    expect(store.store.errorMessage).toContain("cannot reach");

    api.continueImport.mockResolvedValueOnce(createImportProgress({ savedCount: 11, isDone: true }));
    await store.continueImport();
    expect(store.store.importStep).toBe("done");
    expect(store.store.errorMessage).toBe("");
  });

  it("goes back to the preview when the import cannot start", async () => {
    await store.openImport();
    await store.chooseImportFile("personal", "");
    api.startImport.mockRejectedValue(new Error("you can only read this vault; choose a vault you can edit"));
    await store.startImport(true);
    expect(store.store.importStep).toBe("preview");
    expect(api.continueImport).not.toHaveBeenCalled();
  });

  it("drops the parsed export when closed, but not while saving", async () => {
    await store.openImport();
    await store.chooseImportFile("personal", "");
    store.store.importStep = "saving";
    store.closeImport();
    expect(store.store.isImportOpen).toBe(true);
    store.store.importStep = "preview";
    store.closeImport();
    expect(api.cancelImport).toHaveBeenCalled();
    expect(store.store.isImportOpen).toBe(false);
    expect(store.store.importPreview).toBeNull();
  });

  it("forgets the import when Hisar locks", async () => {
    await store.openImport();
    await store.chooseImportFile("personal", "");
    store.handleLocked();
    expect(store.store.isImportOpen).toBe(false);
    expect(store.store.importPreview).toBeNull();
  });

  it("lists only vaults the user can edit", () => {
    expect(store.getAllEditableVault().map((vault) => vault.id)).toEqual(["personal", "work"]);
  });
});

describe("Import panel", () => {
  it("offers the editable vaults, starting with the selected one, and the formats", async () => {
    store.store.selectedVaultID = "work";
    await store.openImport();
    const wrapper = mount(ImportPanel);
    await flushPromises();
    const vaultSelect = wrapper.find('[data-testid="import-vault"]');
    expect(vaultSelect.findAll("option").map((option) => option.text())).toEqual(["Personal", "Work"]);
    expect((vaultSelect.element as HTMLSelectElement).value).toBe("work");
    expect(wrapper.find('[data-testid="import-format"]').findAll("option").map((option) => option.text())).toEqual([
      "Detect from the file",
      "1Password export (.1pux)",
      "CSV (Chrome, Firefox, Safari, Edge and others)",
    ]);
    await wrapper.find('[data-testid="import-format"]').setValue("csv");
    api.chooseImportFile.mockResolvedValue(createImportPreview({ fileName: "" }));
    await wrapper.find('[data-testid="import-choose"]').trigger("submit");
    expect(api.chooseImportFile).toHaveBeenCalledWith("work", "csv");
  });

  it("starts with the first editable vault when a read-only vault is selected", async () => {
    store.store.selectedVaultID = "family";
    await store.openImport();
    const wrapper = mount(ImportPanel);
    expect((wrapper.find('[data-testid="import-vault"]').element as HTMLSelectElement).value).toBe("personal");
  });

  it("previews counts, duplicates and skipped entries before saving", async () => {
    const wrapper = await openPreview();
    expect(wrapper.find('[data-testid="import-file-name"]').text()).toBe("bitwarden_export.json");
    expect(wrapper.text()).toContain("Bitwarden JSON (not encrypted)");
    expect(wrapper.find('[data-testid="import-count-login"]').text()).toContain("9");
    expect(wrapper.find('[data-testid="import-count-secure-note"]').text()).toContain("2");
    expect(wrapper.find('[data-testid="import-count-ssh-key"]').text()).toContain("1");
    expect(wrapper.find('[data-testid="import-count-ssh-key"]').text()).toContain("SSH key");
    expect(wrapper.find('[data-testid="import-duplicates"]').text()).toContain("Already in Personal · 1");
    expect(wrapper.find('[data-testid="import-duplicates"]').text()).toContain("alice · github.com");
    expect(wrapper.find('[data-testid="import-skipped"]').text()).toContain("Bitwarden cards are not supported");
    expect(api.startImport).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="start-import"]').text()).toBe("Import 11 items");

    await wrapper.find('[data-testid="skip-duplicates"]').setValue(false);
    expect(wrapper.find('[data-testid="start-import"]').text()).toBe("Import 12 items");
    api.startImport.mockResolvedValue(createImportProgress({ totalCount: 12 }));
    api.continueImport.mockReturnValue(new Promise(() => undefined));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(api.startImport).toHaveBeenCalledWith("personal", false);
    expect(wrapper.find('[data-testid="import-saving"]').text()).toContain("Saving 0 of 12 items to Personal");
    expect(wrapper.find('button[aria-label="Close"]').attributes("disabled")).toBeDefined();
  });

  it("hides the duplicate and skipped sections when there are none", async () => {
    const wrapper = await openPreview({ allDuplicate: [], allSkipped: [] });
    expect(wrapper.find('[data-testid="import-duplicates"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="import-skipped"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="start-import"]').text()).toBe("Import 12 items");
  });

  it("checks duplicates again when the vault changes", async () => {
    const wrapper = await openPreview();
    api.previewImport.mockResolvedValue(createImportPreview({ vaultId: "work", vaultName: "Work", allDuplicate: [] }));
    await wrapper.find('[data-testid="import-preview-vault"]').setValue("work");
    await flushPromises();
    expect(api.previewImport).toHaveBeenCalledWith("work");
    expect(wrapper.find('[data-testid="import-duplicates"]').exists()).toBe(false);
  });

  it("offers to delete the export file after the import", async () => {
    const wrapper = await openPreview();
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockResolvedValue(createImportProgress({ savedCount: 10, totalCount: 11, isDone: true, allFailed: [{ title: "Huge note", reason: "the item is too large" }] }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="import-result"]').text()).toBe("Imported 10 items into Personal");
    expect(wrapper.find('[data-testid="import-failed"]').text()).toContain("the item is too large");
    const deleteOffer = wrapper.find('[data-testid="delete-import-file-offer"]');
    expect(deleteOffer.text()).toContain("bitwarden_export.json holds all these passwords in plain text");
    expect(deleteOffer.text()).toContain("/Users/alice/Downloads/bitwarden_export.json");

    api.deleteImportFile.mockResolvedValue(undefined);
    await wrapper.find('[data-testid="delete-import-file"]').trigger("click");
    await flushPromises();
    expect(api.deleteImportFile).toHaveBeenCalled();
    expect(store.store.noticeMessage).toBe("Deleted bitwarden_export.json");
    expect(wrapper.find('[data-testid="delete-import-file-offer"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="import-file-deleted"]').exists()).toBe(true);
    await findButton(wrapper, "Done").trigger("click");
    expect(store.store.isImportOpen).toBe(false);
  });

  it("keeps the file when asked", async () => {
    const wrapper = await openPreview();
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockResolvedValue(createImportProgress({ savedCount: 11, isDone: true }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    await findButton(wrapper, "Keep file").trigger("click");
    expect(api.deleteImportFile).not.toHaveBeenCalled();
    expect(api.cancelImport).toHaveBeenCalled();
    expect(store.store.isImportOpen).toBe(false);
  });

  it("shows the error and a way to continue when saving stops", async () => {
    const wrapper = await openPreview();
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockRejectedValueOnce(new Error("cannot reach the Hisar server"));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toContain("cannot reach the Hisar server");
    api.continueImport.mockResolvedValueOnce(createImportProgress({ savedCount: 11, isDone: true }));
    await wrapper.find('[data-testid="continue-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="import-done"]').exists()).toBe(true);
  });
});

describe("Ways to open the import", () => {
  it("opens from Settings", async () => {
    store.store.isSettingsOpen = true;
    const wrapper = mount(SettingsPanel);
    await wrapper.find('[data-testid="open-import"]').trigger("click");
    await flushPromises();
    expect(store.store.isImportOpen).toBe(true);
    expect(store.store.isSettingsOpen).toBe(false);
  });

  it("opens from the new item menu", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-import"]').trigger("click");
    await flushPromises();
    expect(store.store.isImportOpen).toBe(true);
    expect(wrapper.find('[data-testid="import-panel"]').exists()).toBe(true);
    wrapper.unmount();
  });
});
