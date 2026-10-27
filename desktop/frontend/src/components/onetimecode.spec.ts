import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { computeAccessibleName } from "dom-accessibility-api";
import { allTestVault, createApiMock, createItem, createServerItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createItem({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

const { store } = await import("../store");
const OneTimeCodeField = (await import("./OneTimeCodeField.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-09T12:00:10Z"));
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    selectedItem: createItem({ id: "github", hasOneTimePassword: true, oneTimePassword: "otpauth://totp/?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" }),
    editorMode: "view",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
});

afterEach(() => vi.useRealTimers());

describe("OneTimeCodeField", () => {
  it("shows the code with the seconds left and asks for the next one when it expires", async () => {
    api.getOneTimeCode
      .mockResolvedValueOnce({ code: "482913", expiresAt: "2026-10-09T12:00:30Z", period: 30 })
      .mockResolvedValueOnce({ code: "118204", expiresAt: "2026-10-09T12:01:00Z", period: 30 });
    const wrapper = mount(OneTimeCodeField, { props: { vaultID: "personal", itemID: "github" } });
    await flushPromises();
    expect(api.getOneTimeCode).toHaveBeenCalledWith("personal", "github");
    expect(wrapper.find('[data-testid="one-time-code-value"]').text()).toBe("482 913");
    expect(wrapper.find('[data-testid="one-time-code-seconds"]').text()).toBe("20s");

    await vi.advanceTimersByTimeAsync(15_000);
    expect(wrapper.find('[data-testid="one-time-code-seconds"]').text()).toBe("5s");
    expect(wrapper.find(".ring").classes()).toContain("ending");

    await vi.advanceTimersByTimeAsync(5_000);
    await flushPromises();
    expect(api.getOneTimeCode).toHaveBeenCalledTimes(2);
    expect(wrapper.find('[data-testid="one-time-code-value"]').text()).toBe("118 204");
    wrapper.unmount();
  });

  it("copies the code through the backend", async () => {
    api.getOneTimeCode.mockResolvedValue({ code: "482913", expiresAt: "2026-10-09T12:00:30Z", period: 30 });
    api.copyField.mockResolvedValue(undefined);
    const wrapper = mount(OneTimeCodeField, { props: { vaultID: "personal", itemID: "github" } });
    await flushPromises();
    await wrapper.find('button[aria-label="Copy one-time code"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "github", "one-time code");
    wrapper.unmount();
  });

  it("says so when the secret cannot be read", async () => {
    api.getOneTimeCode.mockRejectedValue(new Error("one-time password not found"));
    const wrapper = mount(OneTimeCodeField, { props: { vaultID: "personal", itemID: "github" } });
    await flushPromises();
    expect(wrapper.text()).toContain("The one-time code could not be read.");
    wrapper.unmount();
  });
});

describe("one-time password in items", () => {
  it("appears in the item detail only when the item has one", async () => {
    api.getOneTimeCode.mockResolvedValue({ code: "482913", expiresAt: "2026-10-09T12:00:30Z", period: 30 });
    const props = { canEdit: true, vaultName: "Personal", vaultType: "personal" };
    const wrapper = mount(ItemDetailPanel, { props });
    await flushPromises();
    expect(wrapper.find('[data-testid="one-time-code"]').exists()).toBe(true);
    store.selectedItem = createItem({ id: "plain" });
    await flushPromises();
    expect(wrapper.find('[data-testid="one-time-code"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("saves a pasted setup key from the editor", async () => {
    api.saveItem.mockResolvedValue(createItem({ id: "github", hasOneTimePassword: true }));
    api.listAllItem.mockResolvedValue([]);
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="oneTimePassword"]').setValue("jbsw y3dp ehpk 3pxp");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ id: "github", oneTimePassword: "jbsw y3dp ehpk 3pxp" }));
    wrapper.unmount();
  });

  it.each(["", "server"])("conceals a saved %s setup key until its independent reveal action, including reopening", async (category) => {
    store.editorMode = "edit";
    store.selectedItem = (category === "server" ? createServerItem : createItem)({ oneTimePassword: "otpauth://totp/?secret=JBSWY3DPEHPK3PXP" });
    let wrapper = mount(ItemEditor);
    const field = () => wrapper.get('input[name="oneTimePassword"]');
    expect(field().attributes("type")).toBe("password");
    expect(field().element).toHaveProperty("value", store.selectedItem.oneTimePassword);
    expect(computeAccessibleName(field().element)).toBe("One-time code setup key");
    await wrapper.get('[aria-label="Show setup key"]').trigger("click");
    expect(field().attributes("type")).toBe("text");
    expect(wrapper.get('input[name="password"]').attributes("type")).toBe("password");
    await wrapper.get('[aria-label="Hide setup key"]').trigger("click");
    expect(field().attributes("type")).toBe("password");
    await wrapper.get('[aria-label="Show setup key"]').trigger("click");
    wrapper.unmount();
    wrapper = mount(ItemEditor);
    expect(field().attributes("type")).toBe("password");
    wrapper.unmount();
  });
});
