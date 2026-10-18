import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createLoginItemStatus, createStatus } from "../testing";

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

const { store } = await import("../store");
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const OpenAtLoginSection = (await import("./OpenAtLoginSection.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  Object.defineProperty(navigator, "platform", { value: "Linux x86_64", configurable: true });
  store.status = { ...createStatus("unlocked"), biometricAvailable: false };
  api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus());
});

afterEach(() => {
  Object.defineProperty(navigator, "platform", { value: "MacIntel", configurable: true });
});

describe("Settings on Linux", () => {
  it("says this computer unlocks with the master password and never mentions a Mac", async () => {
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    expect(wrapper.text()).toContain("Unlocking with a fingerprint is not available on this computer yet");
    expect(wrapper.text()).toContain("Hisar locks once this computer has had no keyboard or mouse input");
    expect(wrapper.text()).not.toContain("Mac");
  });

  it("keeps Hisar in the system tray", async () => {
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("stays in the system tray");
  });
});
