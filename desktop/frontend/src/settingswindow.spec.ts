import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { allTestVault, createApiMock, createSSHAgentStatus, createStatus } from "./testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const runtime = vi.hoisted(() => ({ listenerByName: new Map<string, (event: { data: unknown }) => unknown>() }));

vi.mock("./api", async () => {
  const { createApiMock: createMock } = await import("./testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => unknown) => {
      runtime.listenerByName.set(name, listener);
      return () => runtime.listenerByName.delete(name);
    },
  },
}));

// The store reads which window it runs in when it loads, so the page is the Settings window's before the import.
window.history.replaceState(null, "", "/?window=settings");
const { openSettings, showNotice, store } = await import("./store");
const SettingsWindow = (await import("./SettingsWindow.vue")).default;
const api = apiMock.current;

let wrapper: VueWrapper | null = null;

// openSettingsWindow loads the page as Go shows it: the section was asked for before the page had loaded.
async function openSettingsWindow(sectionID: string): Promise<VueWrapper> {
  api.takeSettingsWindowRequest.mockResolvedValueOnce(sectionID);
  wrapper = mount(SettingsWindow, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  Object.assign(store, { status: createStatus("signed-out"), isSettingsOpen: false, isRecoveryOpen: false, pendingRecovery: [], errorMessage: "", noticeMessage: "", allVault: [] });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue(allTestVault.map((vault) => ({ ...vault })));
  api.getSSHAgentStatus.mockResolvedValue(createSSHAgentStatus());
  api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
  api.listAllPerson.mockResolvedValue([]);
});

afterEach(() => {
  wrapper?.unmount();
  wrapper = null;
});

describe("Settings window", () => {
  it("loads the account and opens at the section asked for, filling the window", async () => {
    const settings = await openSettingsWindow("security");
    expect(api.getStatus).toHaveBeenCalled();
    expect(settings.find("#settings-title").text()).toBe("Security");
    expect(settings.find(".overlay").classes()).toContain("window");
    expect(settings.find(".settings").attributes("role")).toBeUndefined();
    expect(settings.find('button[aria-label="Close settings"]').exists()).toBe(false);
    expect(api.hideSettingsWindow).not.toHaveBeenCalled();
  });

  it("opens a section asked for while the page was reloading, and one asked for once it is open", async () => {
    const settings = await openSettingsWindow("security");
    expect(settings.find("#settings-title").text()).toBe("Security");

    api.takeSettingsWindowRequest.mockResolvedValueOnce("developer");
    await runtime.listenerByName.get("open-settings-window")?.({ data: undefined });
    await flushPromises();
    expect(settings.find("#settings-title").text()).toBe("Developer tools");
  });

  it("stays empty when nothing asked for it", async () => {
    wrapper = mount(SettingsWindow, { attachTo: document.body });
    await flushPromises();
    expect(wrapper.find("#settings-title").exists()).toBe(false);
    expect(api.getStatus).not.toHaveBeenCalled();
  });

  it("lists the vaults for Coding agents without loading items or taking removed-vault notices", async () => {
    const settings = await openSettingsWindow("developer");
    await runtime.listenerByName.get("remote-change")?.({ data: null });
    await flushPromises();
    expect(settings.find(`[data-testid="agent-vault-${allTestVault[0].id}"]`).exists()).toBe(true);
    expect(api.listAllVault).toHaveBeenCalledTimes(2);
    expect(api.listAllItem).not.toHaveBeenCalled();
    expect(api.listAllRemovedVault).not.toHaveBeenCalled();
    expect(api.listPendingRecovery).not.toHaveBeenCalled();
  });

  it("hides the window when Settings closes", async () => {
    await openSettingsWindow("general");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(api.hideSettingsWindow).toHaveBeenCalled();
  });

  it("hands Import to the main window", async () => {
    const settings = await openSettingsWindow("general");
    await settings.find('[data-testid="open-import"]').trigger("click");
    await flushPromises();
    expect(api.openInMainWindow).toHaveBeenCalledWith("import");
    expect(store.isImportOpen).toBe(false);
  });

  it("hands People's Share vault to the main window, which knows the selected vault", async () => {
    const settings = await openSettingsWindow("people");
    await settings.find('[data-testid="people-share-vault"]').trigger("click");
    await flushPromises();
    expect(api.openInMainWindow).toHaveBeenCalledWith("sharing");
    expect(store.isSharingOpen).toBe(false);
  });

  it("hands the pending recovery of a Secret Key replacement to the main window", async () => {
    api.replaceSecretKey.mockResolvedValue({
      status: createStatus("unlocked"),
      secretKey: "H1-NEW-SECRET",
      signedOutDeviceCount: 1,
      keychainError: "",
      localError: "",
      pendingOperationId: "replacement",
      setupState: "awaiting-kit",
      problems: ["The keychain is unavailable. No credential change was sent."],
    });
    const settings = await openSettingsWindow("security");
    await settings.find('[data-testid="replace-secret-key"]').trigger("click");
    await settings.find('input[name="replaceSecretKeyPassword"]').setValue("master password");
    await settings.find('[data-testid="confirm-replace-secret-key"]').trigger("submit");
    await flushPromises();
    expect(api.openInMainWindow).toHaveBeenCalledWith("recovery");
    expect(api.listPendingRecovery).not.toHaveBeenCalled();
    expect(store.isRecoveryOpen).toBe(false);
    expect(store.recoveryFactor).toBeNull();
    expect(settings.find('[data-testid="replace-secret-key-done"]').exists()).toBe(false);
  });

  it("locks on ⌘L, also over a dialog", async () => {
    const settings = await openSettingsWindow("security");
    await settings.find('[data-testid="replace-secret-key"]').trigger("click");
    const shortcut = new KeyboardEvent("keydown", { key: "l", metaKey: true, cancelable: true });
    window.dispatchEvent(shortcut);
    await flushPromises();
    expect(shortcut.defaultPrevented).toBe(true);
    expect(api.lock).toHaveBeenCalled();
  });

  it("shows a failed copy's notice as an error over Settings", async () => {
    const settings = await openSettingsWindow("general");
    showNotice("The clipboard is not available.", "error");
    await flushPromises();
    const notice = settings.find('[data-testid="notice"]');
    expect(notice.text()).toBe("The clipboard is not available.");
    expect(notice.classes()).toContain("notice-error");
    expect(notice.attributes("role")).toBe("alert");
    expect(settings.find(".overlay").element.contains(notice.element)).toBe(false);
  });

  it("stays hidden while Hisar is locked", async () => {
    api.getStatus.mockResolvedValue(createStatus("locked"));
    const settings = await openSettingsWindow("general");
    expect(settings.find("#settings-title").exists()).toBe(false);
    expect(api.hideSettingsWindow).toHaveBeenCalled();
  });

  it("is asked for while Hisar is locked, never opened as an overlay", async () => {
    store.status = createStatus("locked");
    api.showSettingsWindow.mockResolvedValueOnce(true);
    await openSettings("developer");
    expect(api.showSettingsWindow).toHaveBeenCalledWith("developer");
    expect(store.isSettingsOpen).toBe(false);
  });
});
