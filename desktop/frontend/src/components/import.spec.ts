import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createImportPreview, createImportProgress, createItem, createStatus, createVaultMember } from "../testing";
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
  { id: "work", name: "Work", type: "shared", role: "editor", itemCount: 0 },
];
const allTestFormat = [
  { id: "1pux", name: "1Password export (.1pux)" },
  { id: "lastpass-csv", name: "LastPass CSV" },
  { id: "csv", name: "CSV (Chrome, Firefox, Safari, Edge and others)" },
];

function resetStore(): void {
  Object.assign(store.store, {
    status: createStatus("unlocked"),
    allVault: allImportTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    listVaultID: "personal",
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

// openPassphrase chooses an encrypted Hisar export, which stops on the passphrase step.
async function openPassphrase(previewOverrides = {}): Promise<ReturnType<typeof mount>> {
  api.chooseImportFile.mockResolvedValue(createImportPreview({ format: "hisar", formatName: "Hisar encrypted export", isPassphraseRequired: true, isSourceKept: true, allSkipped: [], allDuplicate: [], ...previewOverrides }));
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

  it("reloads the open vault after an import that sent items to several vaults", async () => {
    const groups = [
      { id: "private-source", name: "1Password Private", isPrivate: true, itemCount: 1, vaultId: "personal", vaultName: "Personal", members: [] },
      { id: "shared-source", name: "1Password Team", isPrivate: false, itemCount: 1, vaultId: "work", vaultName: "Work", members: [] },
    ];
    api.chooseImportFile.mockResolvedValue(createImportPreview({ vaultId: "", vaultName: "", groups }));
    await store.openImport();
    await store.chooseImportFile("personal", "1pux");
    api.startImport.mockResolvedValue(createImportProgress({ vaultId: "", vaultName: "the mapped vaults" }));
    api.continueImport.mockResolvedValue(createImportProgress({ vaultId: "", vaultName: "the mapped vaults", savedCount: 2, totalCount: 2, isDone: true }));
    api.listAllItem.mockResolvedValue([createItem({ id: "a", title: "Imported into Personal" })]);
    await store.startImport(false);
    expect(api.startImport).toHaveBeenCalledWith("", false);
    expect(api.listAllItem).toHaveBeenCalledWith("personal");
    expect(store.store.allItem.map((vaultItem) => vaultItem.title)).toEqual(["Imported into Personal"]);
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
  it("shows complete explanations for omissions, duplicates and save failures", async () => {
    const reason = "Bitwarden asked for the master password again before showing this item. Hisar shows and fills it while unlocked.";
    const duplicateDetail = "This login has a different password. Keep the source until you have checked which password you want to keep.";
    const wrapper = await openPreview({
      allSkipped: [{ title: "Long omission", reason }],
      allDuplicate: [{ title: "Long duplicate", detail: duplicateDetail }],
    });
    expect(wrapper.get('[data-testid="import-skipped"] .entry-list span').text()).toBe(reason);
    expect(wrapper.get('[data-testid="import-duplicates"] .entry-list span').text()).toBe(duplicateDetail);
    const failure = "This vault's access changed while importing. Keep the source file and preview it again to review who can read these items.";
    store.store.importProgress = createImportProgress({ isDone: true, allFailed: [{ title: "Failed item", reason: failure }] });
    store.store.importStep = "done";
    await flushPromises();
    expect(wrapper.get('[data-testid="import-failed"] .entry-list span').text()).toBe(failure);
  });

  it("retains the full completion report until Done, including late omissions and duplicate outcomes", async () => {
    const parserEntry = { title: "GitHub", reason: "Unsupported passkey extension data was left in the source." };
    const lateEntry = { title: "GitHub", reason: "The skipped duplicate has a different recovery code that remains only in the source." };
    const wrapper = await openPreview({
      allSkipped: [parserEntry], notice: "This export is a cached partial copy.", exportedFrom: "alice@example.com", exportedAt: "2026-10-20T12:00:00Z",
    });
    const duplicateOutcomes = [
      { title: "GitHub", reason: "Added 1 new passkey to the existing login in Hisar." },
      { title: "Slack", reason: "Identical duplicate already present in Hisar." },
      { title: "Docker", reason: "Included the duplicate as another item." },
    ];
    api.startImport.mockResolvedValue(createImportProgress({
      isDone: true, savedCount: 2, allSkipped: [parserEntry, lateEntry], allDuplicateOutcomes: duplicateOutcomes,
      allFailed: [{ title: "Figma", reason: "The destination is no longer writable." }], leftBehindCount: 3, isSourceKept: true,
    }));
    await wrapper.get('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.get('[data-testid="import-result-skipped"]').text()).toContain(parserEntry.reason);
    expect(wrapper.get('[data-testid="import-result-skipped"]').text()).toContain(lateEntry.reason);
    for (const outcome of duplicateOutcomes) expect(wrapper.get('[data-testid="import-result-duplicates"]').text()).toContain(outcome.reason);
    expect(wrapper.get('[data-testid="import-failed"]').text()).toContain("The destination is no longer writable.");
    expect(wrapper.get('[data-testid="import-result-reviewed-duplicates"]').text()).toContain("alice · github.com");
    expect(wrapper.get('[data-testid="import-result-notice"]').text()).toContain("cached partial copy");
    expect(wrapper.get('[data-testid="import-result-exported-from"]').text()).toContain("alice@example.com");
    expect(wrapper.find('[data-testid="delete-import-file-offer"]').exists()).toBe(false);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.get(".overlay").trigger("click");
    expect(store.store.importStep).toBe("done");
    expect(store.store.importPreview).not.toBeNull();
    await findButton(wrapper, "Done").trigger("click");
    expect(store.store.importPreview).toBeNull();
    expect(store.store.importProgress).toBeNull();
  });

  it("offers editable vaults and starts with Personal even when a shared vault is selected", async () => {
    store.store.selectedVaultID = "work";
    await store.openImport();
    const wrapper = mount(ImportPanel);
    await flushPromises();
    const vaultSelect = wrapper.find('[data-testid="import-vault"]');
    expect(vaultSelect.findAll("option").map((option) => option.text())).toEqual(["Personal", "Work"]);
    expect((vaultSelect.element as HTMLSelectElement).value).toBe("personal");
    expect(wrapper.find('[data-testid="import-format"]').findAll("option").map((option) => option.text())).toEqual([
      "Detect from the file",
      "1Password export (.1pux)",
      "LastPass CSV",
      "CSV (Chrome, Firefox, Safari, Edge and others)",
    ]);
    expect(wrapper.text()).toContain("From a Hisar export, 1Password, Bitwarden, LastPass, or a CSV file from your browser");
    expect(wrapper.get("#import-title").text()).toBe("Import items");
    expect(wrapper.find(".how-to").text()).toContain("HisarExport from any of your devices (Settings → Import & Export on a Mac, Settings → Account → Export data… on an iPhone or Android phone, or hisar export).");
    expect(wrapper.find(".how-to").text()).toContain("LastPassAccount options → Advanced → Export, then save the file as CSV.");
    expect(wrapper.find(".how-to").text()).toContain("1Password desktop .1pux exports do not include passkeys. Keep 1Password until you have moved your passkeys separately.");
    await wrapper.find('[data-testid="import-format"]').setValue("csv");
    api.chooseImportFile.mockResolvedValue(createImportPreview({ fileName: "" }));
    await wrapper.find('[data-testid="import-choose"]').trigger("submit");
    expect(api.chooseImportFile).toHaveBeenCalledWith("personal", "csv");
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
    expect(wrapper.findAll('[data-testid^="import-count-"]').map((entry) => entry.text())).toEqual(["9 logins", "2 secure notes", "1 SSH key"]);
    expect(wrapper.get('[data-testid="import-category-counts"]').text()).toBe("Before skipping duplicates: 12 items. 9 logins · 2 secure notes · 1 SSH key");
    expect(wrapper.get('[data-testid="import-headline"]').text()).toBe("11 items to import into Personal");
    expect(wrapper.find('[data-testid="import-duplicates"]').text()).toContain("Already in Personal · 1");
    expect(wrapper.find('[data-testid="import-duplicates"]').text()).toContain("alice · github.com");
    expect(wrapper.find('[data-testid="import-skipped"]').text()).toContain("Bitwarden cards are not supported");
    expect(api.startImport).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="start-import"]').text()).toBe("Import 11 items");

    await wrapper.find('[data-testid="skip-duplicates"]').setValue(false);
    expect(wrapper.find('[data-testid="start-import"]').text()).toBe("Import 12 items");
    expect(wrapper.get('[data-testid="import-headline"]').text()).toBe("12 items to import into Personal");
    expect(wrapper.get('[data-testid="import-category-counts"]').text()).toBe("Before skipping duplicates: 12 items. 9 logins · 2 secure notes · 1 SSH key");
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
    expect(wrapper.get('[data-testid="import-headline"]').text()).toBe("12 items to import into Personal");
    expect(wrapper.get('[data-testid="import-category-counts"]').text()).toBe("Before skipping duplicates: 12 items. 9 logins · 2 secure notes · 1 SSH key");
  });

  it("shows a singular headline and omits empty categories", async () => {
    const wrapper = await openPreview({ itemCount: 1, loginCount: 1, secureNoteCount: 0, sshKeyCount: 0, allDuplicate: [] });
    expect(wrapper.get('[data-testid="import-headline"]').text()).toBe("1 item to import into Personal");
    expect(wrapper.get('[data-testid="import-category-counts"]').text()).toBe("Before skipping duplicates: 1 item. 1 login");
    expect(wrapper.find('[data-testid="import-count-secure-note"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="import-count-ssh-key"]').exists()).toBe(false);
  });

  it("checks duplicates again when the vault changes", async () => {
    const wrapper = await openPreview();
    api.previewImport.mockResolvedValue(createImportPreview({ vaultId: "work", vaultName: "Work", allDuplicate: [] }));
    await wrapper.find('[data-testid="import-preview-vault"]').setValue("work");
    await flushPromises();
    expect(api.previewImport).toHaveBeenCalledWith("work");
    expect(wrapper.find('[data-testid="import-duplicates"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="import-headline"]').text()).toBe("12 items to import into Work");
  });

  it("maps each source group independently and shows the destination members", async () => {
    const groups = [
      { id: "private-source", name: "1Password Private", isPrivate: true, itemCount: 1, vaultId: "personal", vaultName: "Personal", members: [] },
      { id: "shared-source", name: "1Password Team", isPrivate: false, itemCount: 1, vaultId: "work", vaultName: "Work", members: [createVaultMember({ name: "Ada", email: "ada@example.com", isCurrentUser: false })] },
    ];
    const wrapper = await openPreview({ groups });
    const rows = wrapper.findAll('[data-testid="import-source-group"]');
    expect(rows).toHaveLength(2);
    expect(rows[0].text()).toContain("Only you can access Personal");
    expect(rows[1].text()).toContain("ada@example.com");
    expect(rows[1].text()).toContain("Shared destination");
    api.mapImportGroup.mockResolvedValue(createImportPreview({ groups }));
    await rows[1].find("select").setValue("personal");
    await flushPromises();
    expect(api.mapImportGroup).toHaveBeenCalledWith("shared-source", "personal");
  });

  it("shows the 1PUX passkey warning in the preview without opening instructions", async () => {
    const wrapper = await openPreview({ format: "1pux" });
    expect(wrapper.find('[data-testid="import-passkey-warning"]').text()).toContain("1Password desktop .1pux exports do not include passkeys");
    expect(wrapper.find('[data-testid="import-passkey-warning"]').text()).toContain("Keep 1Password");
  });

  it("does not show a 1PUX passkey warning for other import formats", async () => {
    const wrapper = await openPreview({ format: "bitwarden-json" });
    expect(wrapper.find('[data-testid="import-passkey-warning"]').exists()).toBe(false);
  });

  it("offers to delete the export file after the import", async () => {
    const wrapper = await openPreview({ allSkipped: [] });
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockResolvedValue(createImportProgress({ savedCount: 11, totalCount: 11, isDone: true }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="import-result"]').text()).toBe("Imported 11 items into Personal");
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


  it.each(["failed", "omitted", "different-password duplicate"])("keeps the source when entries were %s", async (kind) => {
    const wrapper = await openPreview({ allSkipped: kind === "omitted" ? [{ title: "Login", reason: "its attachment was not imported" }] : [] });
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockResolvedValue(createImportProgress({
      savedCount: 10,
      totalCount: 11,
      isDone: true,
      leftBehindCount: 1,
      allFailed: kind === "failed" ? [{ title: "Huge note", reason: "the item is too large" }] : [],
    }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="delete-import-file"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("Keep the source file");
  });

  it("offers deletion when only exact in-file repeats were skipped", async () => {
    const wrapper = await openPreview({ allSkipped: [{ title: "Login", reason: "the same entry appears earlier in this file" }] });
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockResolvedValue(createImportProgress({ savedCount: 11, totalCount: 11, isDone: true, leftBehindCount: 0 }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="delete-import-file"]').exists()).toBe(true);
    expect(wrapper.text()).not.toContain("Review the entries that were not imported");
  });
  it("keeps the file when asked", async () => {
    const wrapper = await openPreview({ allSkipped: [] });
    api.startImport.mockResolvedValue(createImportProgress());
    api.continueImport.mockResolvedValue(createImportProgress({ savedCount: 11, isDone: true }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    await findButton(wrapper, "Done, keep file").trigger("click");
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

describe("Importing a Hisar export", () => {
  it("asks for the passphrase of an encrypted Hisar export", async () => {
    const wrapper = await openPassphrase();
    expect(store.store.importStep).toBe("passphrase");
    expect(wrapper.find('[data-testid="import-passphrase"]').text()).toContain("Enter the export passphrase");
    expect(wrapper.find('[data-testid="import-passphrase"]').text()).toContain("This export is encrypted. Enter the passphrase it was exported with.");
    expect(wrapper.find('[data-testid="import-preview"]').exists()).toBe(false);
  });

  it("keeps asking with the error and clears the field when the passphrase is wrong", async () => {
    const wrapper = await openPassphrase();
    api.openImportFile.mockRejectedValueOnce(new Error("that passphrase doesn't open this export"));
    await wrapper.find('[data-testid="import-export-passphrase"]').setValue("wrong one");
    await wrapper.find('[data-testid="import-passphrase"]').trigger("submit");
    await flushPromises();
    expect(store.store.importStep).toBe("passphrase");
    expect(wrapper.find('[role="alert"]').text()).toContain("that passphrase doesn't open this export");
    expect((wrapper.find('[data-testid="import-export-passphrase"]').element as HTMLInputElement).value).toBe("");
  });

  it("shows the preview with the notice and the exported-from line when the passphrase opens it", async () => {
    const wrapper = await openPassphrase();
    api.openImportFile.mockResolvedValue(createImportPreview({
      format: "hisar",
      formatName: "Hisar encrypted export",
      isSourceKept: true,
      notice: "Cached copy. Changes since this device's completed syncs may be absent.",
      exportedFrom: "alice@example.com",
      exportedAt: "2026-01-02T03:04:05Z",
      allSkipped: [],
      allDuplicate: [],
    }));
    await wrapper.find('[data-testid="import-export-passphrase"]').setValue("orange cactus harbor");
    await wrapper.find('[data-testid="import-passphrase"]').trigger("submit");
    await flushPromises();
    expect(api.openImportFile).toHaveBeenCalledWith("orange cactus harbor");
    expect(store.store.importStep).toBe("preview");
    expect(wrapper.find('[data-testid="import-notice"]').text()).toContain("Cached copy");
    expect(wrapper.find('[data-testid="import-exported-from"]').text()).toContain("Exported from alice@example.com on");
  });

  it("keeps the source and offers no deletion after importing a kept export", async () => {
    const wrapper = await openPreview({ format: "hisar", formatName: "Hisar export", isSourceKept: true, allSkipped: [], allDuplicate: [] });
    api.startImport.mockResolvedValue(createImportProgress({ isSourceKept: true }));
    api.continueImport.mockResolvedValue(createImportProgress({ savedCount: 12, totalCount: 12, isDone: true, isSourceKept: true }));
    await wrapper.find('[data-testid="start-import"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="import-result"]').text()).toBe("Imported 12 items into Personal");
    expect(wrapper.find('[data-testid="delete-import-file-offer"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="import-source-kept"]').text()).toContain("Hisar keeps this file: it is encrypted, or a copy that may not hold everything.");
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
