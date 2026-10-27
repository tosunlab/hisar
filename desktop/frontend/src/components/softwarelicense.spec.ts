import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createImportPreview, createItem, createSoftwareLicenseItem, createStatus } from "../testing";
import { formatSoftwareLicenseSummary } from "../util";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
    createEmptySoftwareLicense: () => ({ product: "", version: "", licenseKey: "", licensedTo: "", email: "", purchaseDate: "", downloadUrl: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store, filterAllItem, hasShareableField } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const ImportPanel = (await import("./ImportPanel.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
});

describe("Creating a software license", () => {
  it("opens from the + menu with the license's fields and no login fields", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-software-license"]').trigger("click");
    expect(store.editorMode).toBe("create-software-license");
    const editor = wrapper.find('[data-testid="software-license-editor"]');
    for (const name of ["product", "version", "licenseKey", "licensedTo", "licenseEmail", "purchaseDate", "downloadUrl"]) {
      expect(editor.find(`input[name="${name}"]`).exists()).toBe(true);
    }
    expect(editor.find('input[name="licenseKey"]').attributes("type")).toBe("password");
    expect(editor.find('input[name="purchaseDate"]').attributes("type")).toBe("date");
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find('input[name="password"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("saves the license's values", async () => {
    store.editorMode = "create-software-license";
    const saved = createSoftwareLicenseItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Sketch");
    await wrapper.find('input[name="product"]').setValue("Sketch");
    await wrapper.find('input[name="version"]').setValue("100");
    await wrapper.find('input[name="licenseKey"]').setValue("TEST-LICENSE-KEY");
    await wrapper.find('input[name="licensedTo"]').setValue("Alice Example");
    await wrapper.find('input[name="licenseEmail"]').setValue("alice@example.com");
    await wrapper.find('input[name="purchaseDate"]').setValue("2024-03-09");
    await wrapper.find('input[name="downloadUrl"]').setValue("https://www.sketch.com/download");
    await wrapper.find('button[aria-label="Show license key"]').trigger("click");
    expect(wrapper.find('input[name="licenseKey"]').attributes("type")).toBe("text");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        category: "software-license",
        title: "Sketch",
        softwareLicense: {
          product: "Sketch",
          version: "100",
          licenseKey: "TEST-LICENSE-KEY",
          licensedTo: "Alice Example",
          email: "alice@example.com",
          purchaseDate: "2024-03-09",
          downloadUrl: "https://www.sketch.com/download",
        },
      }),
    );
    expect(store.editorMode).toBe("view");
  });

  it("keeps an imported purchase date that is not a date editable as text", () => {
    store.selectedItem = createSoftwareLicenseItem({ softwareLicense: { ...createSoftwareLicenseItem().softwareLicense!, purchaseDate: "03/09/2024" } });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    const purchaseDate = wrapper.find('input[name="purchaseDate"]');
    expect(purchaseDate.attributes("type")).toBe("text");
    expect((purchaseDate.element as HTMLInputElement).value).toBe("03/09/2024");
  });
});

describe("Viewing a software license", () => {
  it("masks the license key until revealed, copies it and opens the download URL", async () => {
    store.selectedItem = createSoftwareLicenseItem();
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    expect(wrapper.find('[data-testid="software-license-summary"]').text()).toBe("Version 100 · Alice Example");
    expect(wrapper.find('[data-testid="card-Product"]').text()).toBe("Sketch");
    expect(wrapper.find('[data-testid="card-License key"]').text()).not.toContain("TEST-LICENSE-KEY");
    expect(wrapper.text()).toContain("Seats");
    await wrapper.find('button[aria-label="Reveal License key"]').trigger("click");
    expect(wrapper.find('[data-testid="card-License key"]').text()).toBe("TEST-LICENSE-KEY");
    await wrapper.find('button[aria-label="Copy License key"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "sketch-1", "License key");
    await wrapper.find('button[aria-label="Copy secret reference for License key"]').trigger("click");
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "sketch-1", "License key");

    expect(wrapper.findAll('button[aria-label^="Open "]').map((button) => button.attributes("aria-label"))).toEqual(["Open Download URL"]);
    await wrapper.find('button[aria-label="Open Download URL"]').trigger("click");
    expect(api.openURL).toHaveBeenCalledWith("https://www.sketch.com/download");
  });

  it("opens only http and https download URLs", async () => {
    store.selectedItem = createSoftwareLicenseItem({ softwareLicense: { ...createSoftwareLicenseItem().softwareLicense!, downloadUrl: "file:///Applications" } });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    await wrapper.find('button[aria-label="Open Download URL"]').trigger("click");
    expect(api.openURL).not.toHaveBeenCalled();
    expect(store.errorMessage).toBe("Only http and https websites can be opened");
  });

  it("offers a share link for a license with only its values", () => {
    const license = createSoftwareLicenseItem({ notes: "", fields: [] });
    expect(hasShareableField(license)).toBe(true);
    store.selectedItem = license;
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: false, vaultName: "Personal", vaultType: "personal" } });
    expect(wrapper.find('[data-testid="share-link"]').exists()).toBe(true);
  });

  it("lists licenses with their version and owner and finds them by product, version, owner and email", () => {
    const summary = { product: "Sketch", version: "100", licenseKey: "", licensedTo: "Alice Example", email: "alice@example.com", purchaseDate: "", downloadUrl: "" };
    const allItem = [
      createSoftwareLicenseItem({ title: "Design app", softwareLicense: summary }),
      createSoftwareLicenseItem({ id: "editor-1", title: "Editor", softwareLicense: { ...summary, product: "Nova", version: "v11", licensedTo: "", email: "bob@example.com" } }),
      createItem({ username: "carol" }),
    ];
    store.allItem = allItem;
    const wrapper = mount(MainView);
    const allSubtitle = wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("Version 100 · Alice Example");
    expect(allSubtitle).toContain("v11 · bob@example.com");
    wrapper.unmount();
    expect(filterAllItem(allItem, "sketch").map((vaultItem) => vaultItem.title)).toEqual(["Design app"]);
    expect(filterAllItem(allItem, "v11").map((vaultItem) => vaultItem.title)).toEqual(["Editor"]);
    expect(filterAllItem(allItem, "alice ex").map((vaultItem) => vaultItem.title)).toEqual(["Design app"]);
    expect(filterAllItem(allItem, "bob@").map((vaultItem) => vaultItem.title)).toEqual(["Editor"]);
  });

  it("summarizes a license by what it has", () => {
    const empty = { product: "", version: "", licensedTo: "", email: "" };
    expect(formatSoftwareLicenseSummary({ ...empty, product: "Sketch" })).toBe("Sketch");
    expect(formatSoftwareLicenseSummary({ ...empty, email: "alice@example.com" })).toBe("alice@example.com");
    expect(formatSoftwareLicenseSummary(null)).toBe("Software license");
  });
});

describe("Importing software licenses", () => {
  it("counts them in the preview", () => {
    store.importPreview = createImportPreview({ itemCount: 2, loginCount: 1, softwareLicenseCount: 1 });
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-software-license"]');
    expect(count.text()).toContain("1");
    expect(count.text()).toContain("software license");
  });
});
