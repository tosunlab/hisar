import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createShareLink, createShareLinkField, createStatus } from "../testing";

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
const api = apiMock.current;

const githubItem = createItem({ password: "k7#Vq2!mZp9@", fields: [{ name: "Recovery code", value: "a1b2-c3d4", concealed: true }] });
const allGitHubField = [
  createShareLinkField({ key: "username", label: "username", isDefault: true }),
  createShareLinkField({ key: "password", label: "password", isDefault: true, isConcealed: true }),
  createShareLinkField({ key: "urls", label: "website", isDefault: false }),
  createShareLinkField({ key: "field:0", label: "Recovery code", isDefault: false, isConcealed: true }),
];
const createdURL = "https://eu.tryhisar.com/s/AbCdEfGhIjKlMnOpQrStUv#-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_s";

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault,
    selectedVaultID: "personal",
    listVaultID: "personal",
    allItem: [githubItem],
    selectedItem: githubItem,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isSharingOpen: false,
    isImportOpen: false,
    allShareLink: [],
    isShareLinksOpen: false,
    shareLinkDialog: null,
  });
}

async function openItemDialog(): Promise<VueWrapper> {
  const wrapper = mount(MainView, { attachTo: document.body });
  await wrapper.find('[data-testid="share-link"]').trigger("click");
  await flushPromises();
  return wrapper;
}

function getCheckbox(wrapper: VueWrapper, key: string): HTMLInputElement {
  return wrapper.find(`[data-testid="share-field-${key}"]`).element as HTMLInputElement;
}

beforeEach(() => {
  vi.clearAllMocks();
  document.body.innerHTML = "";
  resetStore();
  api.listAllShareLinkField.mockResolvedValue(allGitHubField);
  api.listAllShareLink.mockResolvedValue([]);
});

describe("share link dialog", () => {
  it("opens from the item view with the username and password chosen and 1 view for 7 days", async () => {
    const wrapper = await openItemDialog();
    expect(api.listAllShareLinkField).toHaveBeenCalledWith("personal", "item-1");
    const dialog = wrapper.find('[data-testid="share-link-dialog"]');
    expect(dialog.text()).toContain("Share “GitHub”");
    expect(dialog.text()).toContain("Recovery code");
    expect(getCheckbox(wrapper, "username").checked).toBe(true);
    expect(getCheckbox(wrapper, "password").checked).toBe(true);
    expect(getCheckbox(wrapper, "urls").checked).toBe(false);
    expect(getCheckbox(wrapper, "field:0").checked).toBe(false);
    expect((wrapper.find('[data-testid="share-expiry"]').element as HTMLSelectElement).value).toBe("604800");
    expect((wrapper.find('[data-testid="share-views"]').element as HTMLInputElement).value).toBe("1");
    expect(dialog.text()).toContain("never shared");
    wrapper.unmount();
  });

  it("refuses to create a link without a field or with a short passphrase", async () => {
    const wrapper = await openItemDialog();
    await wrapper.find('[data-testid="share-field-username"]').setValue(false);
    await wrapper.find('[data-testid="share-field-password"]').setValue(false);
    await wrapper.find("form").trigger("submit");
    expect(wrapper.find('[data-testid="share-link-validation"]').text()).toBe("Choose at least one field to share.");

    await wrapper.find('[data-testid="share-field-password"]').setValue(true);
    await wrapper.find('[data-testid="share-passphrase-required"]').setValue(true);
    await wrapper.find('[data-testid="share-passphrase"]').setValue("short");
    await wrapper.find("form").trigger("submit");
    expect(wrapper.find('[data-testid="share-link-validation"]').text()).toBe("Use a passphrase of at least 8 characters.");

    await wrapper.find('[data-testid="share-passphrase"]').setValue("orange-cactus");
    await wrapper.find('[data-testid="share-views"]').setValue(101);
    await wrapper.find("form").trigger("submit");
    expect(wrapper.find('[data-testid="share-link-validation"]').text()).toBe("Choose 1 to 100 views, or unlimited.");
    expect(api.createShareLink).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("checks both passphrase entries and offers labelled reveal controls", async () => {
    const wrapper = await openItemDialog();
    await wrapper.get('[data-testid="share-passphrase-required"]').setValue(true);
    await wrapper.get('[data-testid="share-passphrase"]').setValue("orange-cactus");
    await wrapper.get('[data-testid="share-passphrase-repeat"]').setValue("orange-cactuz");
    expect(wrapper.get("#share-passphrase-mismatch").text()).toBe("The passphrases don't match.");
    expect(wrapper.get('[data-testid="create-share-link"]').attributes("disabled")).toBeDefined();
    await wrapper.get("form").trigger("submit");
    expect(api.createShareLink).not.toHaveBeenCalled();
    await wrapper.get('button[aria-label="Show passphrase"]').trigger("click");
    await wrapper.get('button[aria-label="Show repeated passphrase"]').trigger("click");
    expect(wrapper.get('[data-testid="share-passphrase"]').attributes("type")).toBe("text");
    expect(wrapper.get('[data-testid="share-passphrase-repeat"]').attributes("type")).toBe("text");
    await wrapper.get('[data-testid="share-passphrase-repeat"]').setValue("orange-cactus");
    expect(wrapper.find("#share-passphrase-mismatch").exists()).toBe(false);
    expect(wrapper.get('[data-testid="create-share-link"]').attributes("disabled")).toBeUndefined();
    await wrapper.get('button[aria-label="Close"]').trigger("click");
    await wrapper.get('[data-testid="share-link"]').trigger("click");
    await flushPromises();
    await wrapper.get('[data-testid="share-passphrase-required"]').setValue(true);
    expect((wrapper.get('[data-testid="share-passphrase"]').element as HTMLInputElement).value).toBe("");
    expect((wrapper.get('[data-testid="share-passphrase-repeat"]').element as HTMLInputElement).value).toBe("");
    wrapper.unmount();
  });

  it("creates the link with the chosen fields and options, then shows it with Copy", async () => {
    api.createShareLink.mockResolvedValue(
      createShareLink({ id: "AbCdEfGhIjKlMnOpQrStUv", url: createdURL, fieldLabels: ["password", "Recovery code"], maxViews: 3, hasPassphrase: true }),
    );
    api.listAllShareLink.mockResolvedValue([createShareLink({ id: "AbCdEfGhIjKlMnOpQrStUv" })]);
    const wrapper = await openItemDialog();
    await wrapper.find('[data-testid="share-field-username"]').setValue(false);
    await wrapper.find('[data-testid="share-field-field:0"]').setValue(true);
    await wrapper.find('[data-testid="share-expiry"]').setValue("86400");
    await wrapper.find('[data-testid="share-views"]').setValue(3);
    await wrapper.find('[data-testid="share-passphrase-required"]').setValue(true);
    await wrapper.find('[data-testid="share-passphrase"]').setValue("orange-cactus-harbor");
    await wrapper.find('[data-testid="share-passphrase-repeat"]').setValue("orange-cactus-harbor");
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(api.createShareLink).toHaveBeenCalledWith({
      vaultID: "personal",
      itemID: "item-1",
      fieldKeys: ["password", "field:0"],
      title: "",
      text: "",
      expiresInSeconds: 86400,
      maxViews: 3,
      passphrase: "orange-cactus-harbor",
    });
    const created = wrapper.find('[data-testid="share-link-created"]');
    expect((wrapper.find('[data-testid="share-link-url"]').element as HTMLInputElement).value).toBe(createdURL);
    expect(created.text()).toContain("Password, Recovery code");
    expect(created.text()).toContain("3 views");
    expect(created.text()).toContain("Needs the passphrase");
    expect(store.allShareLink).toHaveLength(1);

    await wrapper.find('[data-testid="copy-share-link"]').trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith(createdURL);
    expect(store.noticeMessage).toBe("Link copied. It clears in 90 seconds.");
    wrapper.unmount();
  });

  it("sends 0 views for unlimited and no passphrase when it is not required", async () => {
    api.createShareLink.mockResolvedValue(createShareLink({ url: createdURL, maxViews: 0 }));
    const wrapper = await openItemDialog();
    await wrapper.find('[data-testid="share-views"]').setValue(37);
    await wrapper.find('[data-testid="share-unlimited-views"]').setValue(true);
    expect((wrapper.find('[data-testid="share-views"]').element as HTMLInputElement).disabled).toBe(true);
    expect((wrapper.find('[data-testid="share-views"]').element as HTMLInputElement).value).toBe("Unlimited");
    await wrapper.find('[data-testid="share-unlimited-views"]').setValue(false);
    expect((wrapper.find('[data-testid="share-views"]').element as HTMLInputElement).value).toBe("37");
    await wrapper.find('[data-testid="share-unlimited-views"]').setValue(true);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.createShareLink).toHaveBeenCalledWith(expect.objectContaining({ fieldKeys: ["username", "password"], maxViews: 0, passphrase: "", expiresInSeconds: 604800 }));
    expect(wrapper.find('[data-testid="share-link-created"]').text()).toContain("unlimited views");
    wrapper.unmount();
  });

  it("shares a piece of text from the Shared links panel, not the new item menu", async () => {
    api.createShareLink.mockResolvedValue(createShareLink({ url: createdURL, title: "Staging database URL", fieldLabels: ["text"], isText: true, itemID: "", vaultID: "" }));
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    expect(wrapper.find('[data-testid="new-import"]').exists()).toBe(true);
    expect(wrapper.find(".new-item-menu").text()).not.toContain("Share text");
    await wrapper.find('[data-testid="open-shared-links"]').trigger("click");
    await flushPromises();
    await wrapper.find('[data-testid="share-text-from-panel"]').trigger("click");
    expect(wrapper.find('[data-testid="share-link-dialog"]').text()).toContain("Share text");
    await wrapper.find("form").trigger("submit");
    expect(wrapper.find('[data-testid="share-link-validation"]').text()).toBe("Enter the text to share.");

    await wrapper.find('input[name="shareTextTitle"]').setValue("Staging database URL");
    await wrapper.find('[data-testid="share-text"]').setValue("postgres://staging.internal/app");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.createShareLink).toHaveBeenCalledWith(
      expect.objectContaining({ vaultID: "", itemID: "", fieldKeys: [], title: "Staging database URL", text: "postgres://staging.internal/app" }),
    );
    expect(wrapper.find('[data-testid="share-link-created"]').text()).toContain("Text");
    wrapper.unmount();
  });

  it("hides Share link for an item with nothing to share", async () => {
    const emptyItem = createItem({ username: "", urls: [] });
    Object.assign(store, { allItem: [emptyItem], selectedItem: emptyItem });
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="share-link"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("shared links", () => {
  const allShareLink = [
    createShareLink({ id: "link-github", fieldLabels: ["username", "password"], maxViews: 5, viewCount: 2, hasPassphrase: true }),
    createShareLink({ id: "link-other", title: "AWS Console", itemID: "aws", fieldLabels: ["password"], maxViews: 1 }),
    createShareLink({ id: "link-text", title: "Staging database URL", vaultID: "", itemID: "", fieldLabels: ["text"], isText: true, maxViews: 0, viewCount: 3 }),
  ];

  it("lists only the item's own links in the item view", () => {
    store.allShareLink = allShareLink;
    const wrapper = mount(MainView, { attachTo: document.body });
    const section = wrapper.find('[data-testid="item-share-links"]');
    const allRow = section.findAll('[data-testid="share-link-row"]');
    expect(allRow).toHaveLength(1);
    expect(allRow[0].text()).toContain("Username, password");
    expect(allRow[0].text()).toContain("2 of 5 views");
    expect(allRow[0].text()).toContain("Passphrase");
    expect(allRow[0].text()).toContain("Expires in 7 days");
    wrapper.unmount();
  });

  it("shows a failed link copy as an error notice while the overview stays open", async () => {
    api.listAllShareLink.mockResolvedValue(allShareLink);
    api.copyShareLink.mockRejectedValue(new Error("the clipboard is not available"));
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="open-shared-links"]').trigger("click");
    await flushPromises();
    await wrapper.find('button[aria-label="Copy link to AWS Console"]').trigger("click");
    await flushPromises();
    expect(store.noticeMessage).toBe("the clipboard is not available");
    expect(store.noticeKind).toBe("error");
    expect(store.errorMessage).toBe("");
    expect(wrapper.find('[data-testid="shared-links-panel"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("opens the overview from the sidebar, copies and revokes after confirming", async () => {
    api.listAllShareLink.mockResolvedValueOnce(allShareLink).mockResolvedValueOnce(allShareLink.slice(1));
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="open-shared-links"]').trigger("click");
    await flushPromises();
    const panel = wrapper.find('[data-testid="shared-links-panel"]');
    const allRow = panel.findAll('[data-testid="share-link-row"]');
    expect(allRow.map((row) => row.find(".share-link-title").text())).toEqual(["GitHub", "AWS Console", "Staging database URL"]);
    expect(allRow[1].text()).toContain("0 of 1 view");
    expect(allRow[2].text()).toContain("3 views · unlimited");
    expect(allRow[2].text()).toContain("Text");

    await allRow[1].find('button[aria-label="Copy link to AWS Console"]').trigger("click");
    await flushPromises();
    expect(api.copyShareLink).toHaveBeenCalledWith("link-other");

    await panel.find('[data-testid="revoke-share-link-link-github"]').trigger("click");
    expect(api.revokeShareLink).not.toHaveBeenCalled();
    await panel.find('[data-testid="confirm-revoke-share-link"]').trigger("click");
    await flushPromises();
    expect(api.revokeShareLink).toHaveBeenCalledWith("link-github");
    expect(store.allShareLink.map((shareLink) => shareLink.id)).toEqual(["link-other", "link-text"]);
    expect(store.noticeMessage).toBe("Link to GitHub revoked");
    wrapper.unmount();
  });
});
