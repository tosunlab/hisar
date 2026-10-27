import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type DOMWrapper, type VueWrapper } from "@vue/test-utils";
import { computeAccessibleName } from "dom-accessibility-api";
import { allTestVault, createApiMock, createItem, createSecureNoteItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const api = apiMock.current;

function mountDetail(): VueWrapper {
  return mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
}

function findRow(wrapper: VueWrapper, label: string): DOMWrapper<Element> {
  return wrapper.findAll("[data-copy-target]").find((row) => row.find(".field-label, .section-label").text().toLowerCase() === label.toLowerCase())!;
}

beforeEach(() => {
  vi.clearAllMocks();
  document.body.innerHTML = "";
  window.getSelection()?.removeAllRanges();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    listVaultID: "personal",
    selectedItem: createItem({
      id: "github",
      password: "hunter2hunter2",
      fields: [{ name: "Recovery code", value: "abcd-efgh", concealed: true }],
      hasOneTimePassword: true,
      notes: "Recovery codes are in the safe.",
    }),
    editorMode: "view",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
  api.getOneTimeCode.mockResolvedValue({ code: "482913", expiresAt: "2099-01-01T00:00:00Z", period: 30 });
});

describe("A click on a field row copies it", () => {
  it("names a plain field's copy target with its label and displayed value", () => {
    store.selectedItem = createItem({ username: "alper" });
    const row = findRow(mountDetail(), "username");
    expect(row.attributes("role")).toBe("button");
    expect(computeAccessibleName(row.element)).toMatch(/^Copy\s*Username\s*alper$/);
  });

  it("keeps concealed passwords and custom fields out of their copy target names", () => {
    const wrapper = mountDetail();
    for (const [label, secret] of [["password", "hunter2hunter2"], ["Recovery code", "abcd-efgh"]]) {
      const name = computeAccessibleName(findRow(wrapper, label).element);
      expect(name).toMatch(new RegExp(`^Copy\\s*${label}\\s*`, "i"));
      expect(name).toContain("••");
      expect(name).not.toContain(secret);
    }
  });

  it("reads a secret only after the user reveals it, and masks it again when hidden", async () => {
    const wrapper = mountDetail();
    await wrapper.get('button[aria-label="Reveal password"]').trigger("click");
    expect(computeAccessibleName(findRow(wrapper, "password").element)).toContain("hunter2hunter2");
    await wrapper.get('button[aria-label="Hide password"]').trigger("click");
    expect(computeAccessibleName(findRow(wrapper, "password").element)).not.toContain("hunter2hunter2");
  });

  it("copies the password from a click on its label, as its copy button does, without revealing it", async () => {
    const wrapper = mountDetail();
    const row = findRow(wrapper, "password");
    expect(row.attributes("tabindex")).toBe("0");
    await row.find(".field-label").trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledTimes(1);
    expect(api.copyField).toHaveBeenCalledWith("personal", "github", "password");
    expect(store.noticeMessage).toBe("Password copied. It clears in 90 seconds.");
    expect(wrapper.find('[data-testid="password-value"]').text()).not.toContain("hunter2hunter2");
  });

  it("copies every kind of row by its field name", async () => {
    const wrapper = mountDetail();
    await flushPromises();
    for (const [label, fieldName] of [
      ["username", "username"],
      ["Recovery code", "Recovery code"],
      ["one-time code", "one-time code"],
      ["notes", "notes"],
    ]) {
      await findRow(wrapper, label).trigger("click");
      await flushPromises();
      expect(api.copyField).toHaveBeenLastCalledWith("personal", "github", fieldName);
    }
    expect(wrapper.text()).not.toContain("abcd-efgh");
  });

  it("copies a website row's own link", async () => {
    store.selectedItem = createItem({ id: "github", urls: ["https://github.com", "https://gist.github.com"] });
    const wrapper = mountDetail();
    const allRow = wrapper.findAll("[data-copy-target]").filter((row) => row.find(".field-label, .section-label").text() === "Website");
    await allRow[1].trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith("https://gist.github.com");
    expect(store.noticeMessage).toBe("Website copied. It clears in 90 seconds.");
  });

  it("copies a secure note's notes from anywhere on the note", async () => {
    store.selectedItem = createSecureNoteItem();
    const wrapper = mountDetail();
    await wrapper.find('[data-testid="secure-note-notes"] .notes').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "note-1", "notes");
  });

  it("leaves the row's own buttons to their action, with no second copy", async () => {
    const wrapper = mountDetail();
    const row = findRow(wrapper, "password").element.parentElement!;
    (row.querySelector('button[aria-label="Reveal password"]') as HTMLButtonElement).click();
    (row.querySelector('[data-testid="copy-secret-reference"]') as HTMLButtonElement).click();
    await flushPromises();
    expect(wrapper.find('[data-testid="password-value"]').text()).toBe("hunter2hunter2");
    expect(api.copySecretReference).toHaveBeenCalledTimes(1);
    expect(api.copyField).not.toHaveBeenCalled();

    (row.querySelector('button[aria-label="Copy password"]') as HTMLButtonElement).click();
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledTimes(1);
  });

  it("does not copy when the click ends a text selection in the row", async () => {
    const wrapper = mountDetail();
    const row = findRow(wrapper, "username");
    const value = row.find(".field-value").element;
    const range = document.createRange();
    range.selectNodeContents(value);
    window.getSelection()!.addRange(range);
    await row.find(".field-value").trigger("click");
    await flushPromises();
    expect(api.copyField).not.toHaveBeenCalled();

    window.getSelection()!.removeAllRanges();
    await row.find(".field-value").trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "github", "username");
  });

  it("copies on Enter or Space while the row has focus, but not from a focused button inside it", async () => {
    const wrapper = mountDetail();
    const row = findRow(wrapper, "username");
    await row.trigger("keydown", { key: "Enter" });
    await row.trigger("keydown", { key: " " });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledTimes(2);

    await wrapper.get('button[aria-label="Copy username"]').trigger("keydown", { key: "Enter" });
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledTimes(2);
  });

  it("leaves rows with nothing to copy as they are", () => {
    store.selectedItem = createItem({
      id: "github",
      androidApps: [{ packageName: "com.github.android", certificateSha256: "14:6D" }],
    });
    const wrapper = mountDetail();
    expect(wrapper.find('[data-testid="android-app-row"]').attributes("role")).toBeUndefined();
  });

  it("keeps field actions outside the copy button, and still copies a click on row padding", async () => {
    const wrapper = mountDetail();
    await flushPromises();
    for (const target of wrapper.findAll("[data-copy-target]")) {
      expect(target.find("button, a, input, select, textarea").exists()).toBe(false);
      expect(target.element.parentElement?.querySelector(".field-actions")).not.toBeNull();
    }
    const row = findRow(wrapper, "password").element.parentElement!;
    expect(row.getAttribute("role")).toBeNull();
    row.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "github", "password");
  });
});
