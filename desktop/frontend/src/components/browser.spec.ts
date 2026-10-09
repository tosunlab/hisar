import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createBrowserStatus, createStatus } from "../testing";

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

const { store, restoreBrowserPairing, showBrowserPairing } = await import("../store");
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
  it("shows the code from the pairing event and cancels it", async () => {
    const wrapper = mount(BrowserPairingModal);
    showBrowserPairing({ browser: "Chrome", code: "482913", expiresAt: new Date(Date.now() + 120_000).toISOString() });
    await flushPromises();
    api.getBrowserStatus.mockResolvedValue(createBrowserStatus({ pendingPairing: store.browserPairing }));
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
});

describe("BrowserExtensionSection", () => {
  it("lists registered and paired browsers and removes one after confirming", async () => {
    api.getBrowserStatus.mockResolvedValue(pairedStatus);
    api.removePairedBrowser.mockResolvedValue({ ...pairedStatus, allPairedBrowser: [] });
    const wrapper = mount(BrowserExtensionSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="registered-browsers"]').text()).toContain("Google Chrome");
    expect(wrapper.find(".tag.warning").attributes("title")).toBe("permission denied");
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
