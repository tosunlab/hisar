import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createBrowserStatus, createStatus } from "../testing";
import type { BrowserStatus } from "../api";

enableAutoUnmount(afterEach);
afterEach(() => document.body.replaceChildren());

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }),
  };
});

const { store, restoreBrowserPairing, showBrowserPairing, handleLocked } = await import("../store");
const BrowserPairingModal = (await import("./BrowserPairingModal.vue")).default;
const BrowserExtensionSection = (await import("./BrowserExtensionSection.vue")).default;
const api = apiMock.current;

const pairedStatus = createBrowserStatus({
  allInstallation: [
    { name: "Google Chrome", manifestPath: "/chrome.json", isInstalled: true, errorMessage: "" },
    { name: "Brave", manifestPath: "/brave.json", isInstalled: false, errorMessage: "permission denied" },
  ],
  allPairedBrowser: [{ instanceID: "chrome-1", browser: "Chrome", pairedAt: "2026-10-01T09:00:00Z", lastUsedAt: "2026-10-01T09:00:00Z" }],
});

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  Object.assign(store, {
    status: createStatus("unlocked"),
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    browserPairing: null,
    browserStatus: null,
  });
  api.getBrowserStatus.mockResolvedValue(createBrowserStatus());
});

describe("BrowserPairingModal", () => {
  it.each(["connected", "removed", "none"])("cancels on Escape and restores focus with a %s opener", async (openerState) => {
    const wrapper = mount(BrowserPairingModal, { attachTo: document.body });
    const fallback = document.createElement("button");
    fallback.dataset.testid = "open-favorites";
    document.body.append(fallback);
    const opener = document.createElement("button");
    if (openerState !== "none") {
      document.body.append(opener);
      opener.focus();
    }
    store.browserPairing = { browser: "Chrome", code: "482913", expiresAt: new Date(Date.now() + 120_000).toISOString() };
    await flushPromises();
    (wrapper.get('[data-testid="cancel-pairing"]').element as HTMLButtonElement).focus();
    if (openerState === "removed") opener.remove();
    const shortcut = vi.fn();
    window.addEventListener("keydown", shortcut);
    const escape = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
    window.dispatchEvent(escape);
    await flushPromises();
    window.removeEventListener("keydown", shortcut);
    expect(escape.defaultPrevented).toBe(true);
    expect(shortcut).not.toHaveBeenCalled();
    expect(api.cancelBrowserPairing).toHaveBeenCalledOnce();
    expect(store.browserPairing).toBeNull();
    expect(document.activeElement).toBe(openerState === "connected" ? opener : fallback);
  });

  it("shows the code from the pairing event and cancels it", async () => {
    const wrapper = mount(BrowserPairingModal);
    const pendingPairing = { browser: "Chrome", code: "482913", expiresAt: new Date(Date.now() + 120_000).toISOString() };
    api.getBrowserStatus.mockResolvedValue(createBrowserStatus({ pendingPairing }));
    showBrowserPairing({ browser: "Chrome", code: "482913", expiresAt: new Date(Date.now() + 120_000).toISOString() });
    await flushPromises();
    expect(wrapper.find('[data-testid="pairing-code"]').text().replace(/\s/g, "")).toBe("482913");
    expect(wrapper.text()).toContain("Pair Chrome with Hisar?");
    await wrapper.find('[data-testid="cancel-pairing"]').trigger("click");
    expect(api.cancelBrowserPairing).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="browser-pairing"]').exists()).toBe(false);
  });

  it("hides the code when the pairing event clears it", async () => {
    const wrapper = mount(BrowserPairingModal);
    store.browserPairing = { browser: "Arc", code: "111222", expiresAt: new Date(Date.now() + 60_000).toISOString() };
    await flushPromises();
    expect(wrapper.find('[data-testid="browser-pairing"]').exists()).toBe(true);
    showBrowserPairing(null);
    await flushPromises();
    expect(wrapper.find('[data-testid="browser-pairing"]').exists()).toBe(false);
  });

  it("restores a pairing that started before the window loaded", async () => {
    const pendingPairing = { browser: "Brave", code: "765432", expiresAt: new Date(Date.now() + 60_000).toISOString() };
    api.getBrowserStatus.mockResolvedValue(createBrowserStatus({ pendingPairing }));
    const wrapper = mount(BrowserPairingModal);
    await restoreBrowserPairing();
    await flushPromises();
    expect(wrapper.find('[data-testid="pairing-code"]').text().replace(/\s/g, "")).toBe("765432");
  });

  it("closes once the code expires", async () => {
    vi.useFakeTimers();
    store.browserPairing = { browser: "Arc", code: "111222", expiresAt: new Date(Date.now() + 2_000).toISOString() };
    const wrapper = mount(BrowserPairingModal);
    expect(wrapper.text()).toContain("Expires in 0:02");
    await vi.advanceTimersByTimeAsync(3_000);
    expect(wrapper.find('[data-testid="browser-pairing"]').exists()).toBe(false);
  });

  it("clears a pairing on lock and rejects a late restoration after unlock", async () => {
    const pendingPairing = { browser: "Chrome", code: "482913", expiresAt: new Date(Date.now() + 120_000).toISOString() };
    store.browserPairing = pendingPairing;
    store.browserStatus = createBrowserStatus({ pendingPairing });
    const pending = Promise.withResolvers<BrowserStatus>();
    api.getBrowserStatus.mockReturnValueOnce(pending.promise);
    const restoring = restoreBrowserPairing();
    api.getStatus.mockResolvedValue(createStatus("locked"));
    await handleLocked();
    expect(store.browserPairing).toBeNull();
    expect(store.browserStatus?.pendingPairing).toBeNull();
    store.status = createStatus("unlocked");
    pending.resolve(createBrowserStatus({ pendingPairing }));
    await restoring;
    expect(store.browserPairing).toBeNull();
  });

  it("does not restore a cleared pairing from a late Settings response", async () => {
    const pendingPairing = { browser: "Chrome", code: "482913", expiresAt: new Date(Date.now() + 120_000).toISOString() };
    const pending = Promise.withResolvers<BrowserStatus>();
    api.getBrowserStatus.mockReturnValueOnce(pending.promise);
    const restoring = restoreBrowserPairing();
    showBrowserPairing(null);
    pending.resolve(createBrowserStatus({ pendingPairing }));
    await restoring;
    expect(store.browserPairing).toBeNull();
    expect(store.browserStatus?.pendingPairing ?? null).toBeNull();
  });
});

describe("BrowserExtensionSection", () => {
  it("keeps failed registration details keyboard-accessible without calling them ready", async () => {
    api.getBrowserStatus.mockResolvedValue(createBrowserStatus({ allInstallation: [pairedStatus.allInstallation[1]] }));
    const wrapper = mount(BrowserExtensionSection, { attachTo: document.body });
    await flushPromises();
    expect(wrapper.find('[data-testid="registered-browsers"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("No supported browser found");
    const field = wrapper.get('[data-testid="browser-manifest-path"]').element as HTMLInputElement;
    field.focus();
    expect(document.activeElement).toBe(field);
    expect(field.readOnly).toBe(true);
    expect(field.getAttribute("aria-label")).toBe("Brave manifest location");
    field.select();
    expect(field.value.slice(field.selectionStart ?? 0, field.selectionEnd ?? 0)).toBe("/brave.json");
    expect(wrapper.get('[data-testid="browser-registration-error"]').isVisible()).toBe(true);
    wrapper.unmount();
  });

  it("lists registered and paired browsers and removes one after confirming", async () => {
    api.getBrowserStatus.mockResolvedValue(pairedStatus);
    api.removePairedBrowser.mockResolvedValue({ ...pairedStatus, allPairedBrowser: [] });
    const wrapper = mount(BrowserExtensionSection);
    await flushPromises();
    expect(wrapper.get(".setting strong").text()).toBe("Browser extension");
    expect(wrapper.text()).toContain("Works in Chrome, Firefox, Brave, Edge, Arc, Dia, Vivaldi and Opera.");
    expect(wrapper.find('[data-testid="registered-browsers"]').text()).toContain("Google Chrome");
    expect(wrapper.get('[data-testid="registered-browsers"]').text()).not.toContain("Brave");
    const failure = wrapper.get('[data-testid="browser-registration-failure"]');
    expect(failure.text()).toContain("Brave registration failed");
    expect(failure.get('[data-testid="browser-registration-error"]').text()).toBe("permission denied");
    expect(failure.text()).toContain("Check that Hisar can write to this location.");
    expect(failure.text()).toContain("Quit Hisar Completely from the menu bar, then reopen it to retry registration.");
    expect((failure.get('[data-testid="browser-manifest-path"]').element as HTMLInputElement).value).toBe("/brave.json");
    expect(wrapper.findAll('[data-testid="paired-browser"]')).toHaveLength(1);
    await wrapper.find('[data-testid="remove-browser"]').trigger("click");
    expect(api.removePairedBrowser).not.toHaveBeenCalled();
    await wrapper.find('[data-testid="confirm-remove-browser"]').trigger("click");
    await flushPromises();
    expect(api.removePairedBrowser).toHaveBeenCalledWith("chrome-1");
    expect(wrapper.find('[data-testid="no-paired-browser"]').exists()).toBe(true);
    expect(store.noticeMessage).toContain("Browser removed");
  });

  it("explains what to do when no browser is found or paired", async () => {
    const wrapper = mount(BrowserExtensionSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="registered-browsers"]').text()).toContain("No supported browser found");
    expect(wrapper.find('[data-testid="no-paired-browser"]').text()).toContain("pair it");
  });

  it("switches exact host matching", async () => {
    api.setExactHostMatch.mockResolvedValue(createBrowserStatus({ isExactHostMatch: true }));
    const wrapper = mount(BrowserExtensionSection);
    await flushPromises();
    await wrapper.find('input[name="exactHostMatch"]').setValue(true);
    await flushPromises();
    expect(api.setExactHostMatch).toHaveBeenCalledWith(true);
    expect(store.browserStatus?.isExactHostMatch).toBe(true);
  });
});
