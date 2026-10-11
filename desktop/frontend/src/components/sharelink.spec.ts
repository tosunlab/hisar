import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createPlan, createShareLink, createShareLinkField, createStatus, proLimits } from "../testing";

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
const createdURL = "https://api.tryhisar.com/s/AbCdEfGhIjKlMnOpQrStUv#-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_s";

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault,
    selectedVaultID: "personal",
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
    plan: null,
    planLimit: null,
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
    expect(wrapper.find('[data-testid="share-link-validation"]').text()).toBe("Choose 1 to 100 views.");
    expect(api.createShareLink).not.toHaveBeenCalled();
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
    expect(store.noticeMessage).toBe("Link copied, clears in 90 seconds");
    wrapper.unmount();
  });

  it("sends one view by default and offers only the expiries the plan allows", async () => {
    api.getPlan.mockResolvedValue(createPlan());
    api.createShareLink.mockResolvedValue(createShareLink({ url: createdURL, maxViews: 1 }));
    const wrapper = await openItemDialog();
    expect(wrapper.find('[data-testid="share-unlimited-views"]').exists()).toBe(false);
    const allExpiryLabel = wrapper.findAll('[data-testid="share-expiry"] option').map((option) => option.text());
    expect(allExpiryLabel).toEqual(["1 hour", "1 day", "7 days"]);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.createShareLink).toHaveBeenCalledWith(expect.objectContaining({ fieldKeys: ["username", "password"], maxViews: 1, passphrase: "", expiresInSeconds: 604800 }));
    wrapper.unmount();
  });

  it("offers 30 days on Pro", async () => {
    api.getPlan.mockResolvedValue(createPlan({ plan: "pro", title: "Pro", limits: proLimits }));
    const wrapper = await openItemDialog();
    expect(wrapper.findAll('[data-testid="share-expiry"] option').map((option) => option.text())).toContain("30 days");
    wrapper.unmount();
  });

  it("offers every expiry when the plan is unknown", async () => {
    api.getPlan.mockRejectedValue(new Error("offline"));
    const wrapper = await openItemDialog();
    expect(wrapper.findAll('[data-testid="share-expiry"] option')).toHaveLength(4);
    wrapper.unmount();
  });

  it("shows the limit with one upgrade link on Free", async () => {
    const message = "Free includes 10 live share links. Revoke one to create another.";
    api.createShareLink.mockRejectedValue(Object.assign(new Error(message), { cause: { message, code: "plan_limit", kind: "live-share-links", limit: 10, current: 10, plan: "free" } }));
    const wrapper = await openItemDialog();
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(wrapper.find('[data-testid="share-link-dialog"] .error').text()).toBe(message);
    expect(wrapper.findAll('[data-testid="plan-limit-upgrade"]')).toHaveLength(1);
    await wrapper.find('[data-testid="plan-limit-upgrade"]').trigger("click");
    await flushPromises();
    expect(api.createCheckoutURL).toHaveBeenCalledWith("yearly");
    expect(api.openURL).toHaveBeenCalledWith("https://hisar.lemonsqueezy.com/checkout/custom/1");
    wrapper.unmount();
  });

  it("shows a limit on Pro without an upgrade link", async () => {
    const message = "Pro includes 200 live share links. Revoke one to create another.";
    api.createShareLink.mockRejectedValue(Object.assign(new Error(message), { cause: JSON.stringify({ message, code: "plan_limit", kind: "live-share-links", limit: 200, current: 200, plan: "pro" }) }));
    const wrapper = await openItemDialog();
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(wrapper.find('[data-testid="share-link-dialog"] .error').text()).toBe(message);
    expect(wrapper.find('[data-testid="plan-limit-notice"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shares a piece of text from the new item menu", async () => {
    api.createShareLink.mockResolvedValue(createShareLink({ url: createdURL, title: "Staging database URL", fieldLabels: ["text"], isText: true, itemID: "", vaultID: "" }));
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-share-text"]').trigger("click");
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
