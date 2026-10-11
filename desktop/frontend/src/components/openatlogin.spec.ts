import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createLoginItemStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
  };
});

const OpenAtLoginSection = (await import("./OpenAtLoginSection.vue")).default;
const api = apiMock.current;

function getSwitch(wrapper: ReturnType<typeof mount>): HTMLInputElement {
  return wrapper.find('input[name="openAtLogin"]').element as HTMLInputElement;
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus());
});

describe("OpenAtLoginSection", () => {
  it("shows the switch on and that Hisar opens at login", async () => {
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(api.getLoginItemStatus).toHaveBeenCalled();
    expect(wrapper.find(".section-label").text()).toBe("Startup");
    expect(wrapper.text()).toContain("Open Hisar at login");
    expect(getSwitch(wrapper).checked).toBe(true);
    expect(getSwitch(wrapper).disabled).toBe(false);
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("opens locked at login");
    expect(wrapper.find('[data-testid="open-login-items"]').exists()).toBe(false);
  });

  it("turns opening at login off and on", async () => {
    api.setOpenAtLogin.mockResolvedValue(createLoginItemStatus({ isOpenAtLogin: false, systemState: "not-registered" }));
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    await wrapper.find('input[name="openAtLogin"]').setValue(false);
    await flushPromises();
    expect(api.setOpenAtLogin).toHaveBeenCalledWith(false);
    expect(getSwitch(wrapper).checked).toBe(false);
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("Off.");
    expect(wrapper.find('[data-testid="add-login-item"]').exists()).toBe(false);

    api.setOpenAtLogin.mockResolvedValue(createLoginItemStatus());
    await wrapper.find('input[name="openAtLogin"]').setValue(true);
    await flushPromises();
    expect(api.setOpenAtLogin).toHaveBeenLastCalledWith(true);
    expect(getSwitch(wrapper).checked).toBe(true);
  });

  it("asks for approval in System Settings and opens Login Items", async () => {
    api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus({ systemState: "requires-approval" }));
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("System Settings → General → Login Items");
    await wrapper.find('[data-testid="open-login-items"]').trigger("click");
    expect(api.openLoginItemSettings).toHaveBeenCalledTimes(1);
  });

  it("adds Hisar again after it was removed from the login items", async () => {
    api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus({ systemState: "not-registered" }));
    api.setOpenAtLogin.mockResolvedValue(createLoginItemStatus());
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("no longer in your login items");
    await wrapper.find('[data-testid="add-login-item"]').trigger("click");
    await flushPromises();
    expect(api.setOpenAtLogin).toHaveBeenCalledWith(true);
    expect(wrapper.find('[data-testid="add-login-item"]').exists()).toBe(false);
  });

  it("shows a system error from registering", async () => {
    api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus({ isOpenAtLogin: false, systemState: "not-registered" }));
    api.setOpenAtLogin.mockResolvedValue(createLoginItemStatus({ systemState: "not-registered", errorMessage: "could not add Hisar to the login items: Operation not permitted" }));
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    await wrapper.find('input[name="openAtLogin"]').setValue(true);
    await flushPromises();
    expect(wrapper.find('[data-testid="login-item-error"]').text()).toContain("Operation not permitted");
  });

  it("disables the switch in a build that cannot open at login", async () => {
    api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus({ isAvailable: false, isOpenAtLogin: false, systemState: "unavailable", canOpenSystemSettings: false }));
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(getSwitch(wrapper).disabled).toBe(true);
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("Only the installed Hisar app");
  });

  it("shows a failed request", async () => {
    api.getLoginItemStatus.mockRejectedValue(new Error("the login item service is not available"));
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="login-item-error"]').text()).toBe("the login item service is not available");
  });
});
