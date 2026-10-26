import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createApiMock, createSSHAgentStatus, createStatus } from "./testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const runtime = vi.hoisted(() => ({ listenerByName: new Map<string, (event: { data: unknown }) => unknown>(), emit: vi.fn() }));

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
    Emit: runtime.emit,
  },
}));

// The store reads which window it runs in when it loads, so the page is the Settings window's before the import.
window.history.replaceState(null, "", "/?window=settings");
const { store } = await import("./store");
const SettingsWindow = (await import("./SettingsWindow.vue")).default;
const api = apiMock.current;

let wrapper: VueWrapper | null = null;

async function openSettingsWindow(sectionID: string): Promise<VueWrapper> {
  wrapper = mount(SettingsWindow, { attachTo: document.body });
  await runtime.listenerByName.get("open-settings-window")?.({ data: sectionID });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  Object.assign(store, { status: createStatus("signed-out"), isSettingsOpen: false, errorMessage: "" });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue([]);
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

  it("hides the window when Settings closes", async () => {
    await openSettingsWindow("general");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(api.hideSettingsWindow).toHaveBeenCalled();
  });

  it("hands Import to the main window and closes", async () => {
    const settings = await openSettingsWindow("general");
    await settings.find('[data-testid="open-import"]').trigger("click");
    await flushPromises();
    expect(runtime.emit).toHaveBeenCalledWith("open-from-settings", "import");
    expect(store.isImportOpen).toBe(false);
    expect(api.hideSettingsWindow).toHaveBeenCalled();
  });

  it("stays hidden while Hisar is locked", async () => {
    api.getStatus.mockResolvedValue(createStatus("locked"));
    const settings = await openSettingsWindow("general");
    expect(settings.find("#settings-title").exists()).toBe(false);
    expect(api.hideSettingsWindow).toHaveBeenCalled();
  });
});
