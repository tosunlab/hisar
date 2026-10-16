import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createAPICredentialItem, createApiMock, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
    createEmptyAPICredential: () => ({ service: "", keyId: "", secret: "", host: "", expiry: "", expiryWarning: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store, filterAllItem } = await import("../store");
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

describe("Creating an API credential", () => {
  it("opens from the + menu with the credential's fields and no login fields", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-api-credential"]').trigger("click");
    expect(store.editorMode).toBe("create-api-credential");
    const editor = wrapper.find('[data-testid="api-credential-editor"]');
    for (const name of ["service", "keyId", "secret", "host", "expiry"]) {
      expect(editor.find(`input[name="${name}"]`).exists()).toBe(true);
    }
    expect(editor.find('input[name="secret"]').attributes("type")).toBe("password");
    expect(editor.find('input[name="expiry"]').attributes("type")).toBe("date");
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find('input[name="password"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("saves the credential's values", async () => {
    store.editorMode = "create-api-credential";
    const saved = createAPICredentialItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Stripe");
    await wrapper.find('input[name="service"]').setValue("Stripe");
    await wrapper.find('input[name="keyId"]').setValue("key_1");
    await wrapper.find('input[name="secret"]').setValue("sk_live_123");
    await wrapper.find('input[name="host"]').setValue("api.stripe.com");
    await wrapper.find('input[name="expiry"]').setValue("2031-01-01");
    await wrapper.find('button[aria-label="Show secret"]').trigger("click");
    expect(wrapper.find('input[name="secret"]').attributes("type")).toBe("text");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        category: "api-credential",
        title: "Stripe",
        apiCredential: expect.objectContaining({ service: "Stripe", keyId: "key_1", secret: "sk_live_123", host: "api.stripe.com", expiry: "2031-01-01" }),
      }),
    );
    expect(store.editorMode).toBe("view");
  });

  it("updates the expired warning as the expiry is edited", async () => {
    store.selectedItem = createAPICredentialItem({
      apiCredential: { ...createAPICredentialItem().apiCredential!, expiry: "2020-01-01", expiryWarning: "This credential has expired" },
    });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    const warning = () => wrapper.find('[data-testid="api-credential-expiry-warning"]');
    expect(warning().text()).toBe("This credential has expired");
    await wrapper.find('input[name="expiry"]').setValue("2999-01-01");
    expect(warning().exists()).toBe(false);
    await wrapper.find('input[name="expiry"]').setValue("2020-06-01");
    expect(warning().text()).toBe("This credential has expired");
    await wrapper.find('input[name="expiry"]').setValue("");
    expect(warning().exists()).toBe(false);
  });

  it("keeps the saved warning for an expiry in another form until it changes", async () => {
    store.selectedItem = createAPICredentialItem({
      apiCredential: { ...createAPICredentialItem().apiCredential!, expiry: "2020/01/01", expiryWarning: "This credential has expired" },
    });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    expect(wrapper.find('[data-testid="api-credential-expiry-warning"]').text()).toBe("This credential has expired");
    await wrapper.find('input[name="expiry"]').setValue("never");
    expect(wrapper.find('[data-testid="api-credential-expiry-warning"]').exists()).toBe(false);
  });

  it("keeps an imported expiry that is not a date editable as text", () => {
    store.selectedItem = createAPICredentialItem({ apiCredential: { ...createAPICredentialItem().apiCredential!, expiry: "never" } });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    const expiry = wrapper.find('input[name="expiry"]');
    expect(expiry.attributes("type")).toBe("text");
    expect((expiry.element as HTMLInputElement).value).toBe("never");
  });
});

describe("Viewing an API credential", () => {
  it("masks the secret until revealed and offers each field as a secret reference", async () => {
    store.selectedItem = createAPICredentialItem();
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="api-credential-summary"]').text()).toBe("Stripe · api.stripe.com");
    expect(wrapper.find('[data-testid="card-Key ID"]').text()).toBe("key_1");
    expect(wrapper.find('[data-testid="card-Secret"]').text()).not.toContain("sk_live_123");
    expect(wrapper.text()).toContain("Scope");
    await wrapper.find('button[aria-label="Reveal Secret"]').trigger("click");
    expect(wrapper.find('[data-testid="card-Secret"]').text()).toBe("sk_live_123");
    await wrapper.find('button[aria-label="Copy Secret"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "stripe-1", "Secret");
    await wrapper.find('button[aria-label="Copy secret reference for Secret"]').trigger("click");
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "stripe-1", "Secret");
  });

  it("warns under an expiry that has passed", () => {
    store.selectedItem = createAPICredentialItem({
      apiCredential: { ...createAPICredentialItem().apiCredential!, expiry: "2020-01-01", expiryWarning: "This credential has expired" },
    });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="card-warning-Expiry"]').text()).toBe("This credential has expired");
  });

  it("offers a share link for a credential with only its values", () => {
    store.selectedItem = createAPICredentialItem({ notes: "", fields: [] });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: false, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="share-link"]').exists()).toBe(true);
  });

  it("lists credentials with their service and host and finds them by either", () => {
    const summary = { service: "Stripe", keyId: "", secret: "", host: "api.stripe.com", expiry: "", expiryWarning: "" };
    const allItem = [
      createAPICredentialItem({ title: "Billing key", apiCredential: summary }),
      createAPICredentialItem({ id: "github-1", title: "GitHub token", apiCredential: { ...summary, service: "", keyId: "ghp_key_1", host: "api.github.com" } }),
      createItem({ username: "carol" }),
    ];
    store.allItem = allItem;
    const wrapper = mount(MainView);
    const allSubtitle = wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("Stripe · api.stripe.com");
    expect(allSubtitle).toContain("api.github.com");
    wrapper.unmount();
    expect(filterAllItem(allItem, "stripe").map((vaultItem) => vaultItem.title)).toEqual(["Billing key"]);
    expect(filterAllItem(allItem, "api.github").map((vaultItem) => vaultItem.title)).toEqual(["GitHub token"]);
    expect(filterAllItem(allItem, "ghp_key").map((vaultItem) => vaultItem.title)).toEqual(["GitHub token"]);
  });
});

describe("Importing API credentials", () => {
  it("counts them in the preview", () => {
    store.importPreview = {
      fileName: "export.1pux",
      filePath: "/tmp/export.1pux",
      format: "1pux",
      formatName: "1Password export (.1pux)",
      vaultId: "personal",
      vaultName: "Personal",
      itemCount: 3,
      loginCount: 2,
      secureNoteCount: 0,
      sshKeyCount: 0,
      creditCardCount: 0,
      identityCount: 0,
      apiCredentialCount: 1,
      addressCount: 0,
      softwareLicenseCount: 0,
      serverCount: 0,
      allSkipped: [],
      allDuplicate: [],
    };
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-api-credential"]');
    expect(count.text()).toContain("1");
    expect(count.text()).toContain("API credential");
  });

  it("tells duplicates with the same title apart by their key IDs", () => {
    store.importPreview = {
      fileName: "export.1pux",
      filePath: "/tmp/export.1pux",
      format: "1pux",
      formatName: "1Password export (.1pux)",
      vaultId: "personal",
      vaultName: "Personal",
      itemCount: 2,
      loginCount: 0,
      secureNoteCount: 0,
      sshKeyCount: 0,
      creditCardCount: 0,
      identityCount: 0,
      apiCredentialCount: 2,
      addressCount: 0,
      softwareLicenseCount: 0,
      serverCount: 0,
      allSkipped: [],
      allDuplicate: [
        { title: "Stripe", username: "", url: "", detail: "key_1" },
        { title: "Stripe", username: "", url: "", detail: "key_2" },
      ],
    };
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const allLine = wrapper.findAll('[data-testid="import-duplicates"] li').map((line) => [line.find("strong").text(), line.find(".muted").text()]);
    expect(allLine).toEqual([
      ["Stripe", "key_1"],
      ["Stripe", "key_2"],
    ]);
    expect(wrapper.find('[data-testid="import-duplicates"]').text()).toContain("Skip items that are already in the vault");
  });
});
