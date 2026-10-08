import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const lockedListener = vi.hoisted(() => ({ current: undefined as undefined | (() => void) }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", category: "", title: "", username: "", urls: [], password: "", notes: "", fields: [], publicKey: "", fingerprint: "", keyType: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: () => void) => {
      if (name === "locked") {
        lockedListener.current = listener;
      }
      return () => undefined;
    },
  },
}));

const { store } = await import("../store");
const App = (await import("../App.vue")).default;
const SignInView = (await import("./SignInView.vue")).default;
const SecretKeyView = (await import("./SecretKeyView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const PasswordGenerator = (await import("./PasswordGenerator.vue")).default;
const api = apiMock.current;

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("signed-out"),
    allVault: [],
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    pendingSecretKey: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
}

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("shows sign in when signed out and unlock when locked", async () => {
    api.getStatus.mockResolvedValue(createStatus("signed-out"));
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.text()).toContain("Sign in to use Hisar on this device");

    api.getStatus.mockResolvedValue(createStatus("locked"));
    store.status = createStatus("locked");
    await flushPromises();
    expect(wrapper.text()).toContain("Hisar is locked");
    wrapper.unmount();
  });

  it("switches to the unlock screen on the auto-lock event", async () => {
    api.getStatus.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(App);
    await flushPromises();
    expect(store.status.state).toBe("unlocked");
    lockedListener.current?.();
    await flushPromises();
    expect(wrapper.text()).toContain("Hisar is locked");
    wrapper.unmount();
  });
});

describe("SignInView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("submits the sign in form", async () => {
    api.signIn.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(SignInView);
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="secretKey"]').setValue("H1-KEY");
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signIn).toHaveBeenCalledWith("https://hisar.test", "alice@example.com", "H1-KEY", "master password");
  });

  it("blocks account creation when the passwords differ", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    await wrapper.find('input[name="password"]').setValue("first password");
    await wrapper.find('input[name="repeatedPassword"]').setValue("other password");
    expect(wrapper.text()).toContain("The passwords do not match.");
    expect(wrapper.find('button[type="submit"]').attributes("disabled")).toBeDefined();
    await wrapper.find("form").trigger("submit");
    expect(api.signUp).not.toHaveBeenCalled();
  });

  it("shows the error from the backend", async () => {
    store.errorMessage = "cannot reach the Hisar server; check your connection";
    const wrapper = mount(SignInView);
    expect(wrapper.find('[role="alert"]').text()).toContain("cannot reach the Hisar server");
  });
});

describe("SecretKeyView", () => {
  beforeEach(resetStore);

  it("only continues after the user confirms saving the Secret Key", async () => {
    store.pendingSecretKey = "H1-ABC123-SECRET";
    const wrapper = mount(SecretKeyView);
    expect(wrapper.find('[data-testid="secret-key"]').attributes("aria-label")).toBe("H1-ABC123-SECRET");
    const continueButton = wrapper.findAll("button").find((button) => button.text() === "Continue to Hisar")!;
    expect(continueButton.attributes("disabled")).toBeDefined();
    await wrapper.find('input[name="hasSaved"]').setValue(true);
    await continueButton.trigger("click");
    expect(store.pendingSecretKey).toBe("");
  });
});

describe("ItemDetailPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    store.selectedItem = createItem({ password: "hunter2hunter2", fields: [{ name: "PIN", value: "1234", concealed: true }] });
  });

  it("masks the password until revealed", async () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.text()).not.toContain("hunter2hunter2");
    expect(wrapper.text()).not.toContain("1234");
    await wrapper.find('button[aria-label="Reveal password"]').trigger("click");
    expect(wrapper.find('[data-testid="password-value"]').text()).toBe("hunter2hunter2");
  });

  it("copies through the backend instead of exposing the clipboard to the page", async () => {
    api.copyField.mockResolvedValue(undefined);
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    await wrapper.find('button[aria-label="Copy password"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "item-1", "password");
  });

  it("asks for confirmation before deleting", async () => {
    api.deleteItem.mockResolvedValue(undefined);
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    await wrapper.find('button[aria-label="Delete item"]').trigger("click");
    expect(api.deleteItem).not.toHaveBeenCalled();
    await wrapper.find('[data-testid="confirm-delete"]').trigger("click");
    await flushPromises();
    expect(api.deleteItem).toHaveBeenCalledWith("personal", "item-1");
  });

  it("hides edit and delete for read-only vaults", () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: false, vaultName: "Work", isShared: true } });
    expect(wrapper.findAll("button").map((button) => button.text())).not.toContain("Edit");
    expect(wrapper.find('button[aria-label="Delete item"]').exists()).toBe(false);
  });
});

describe("PasswordGenerator", () => {
  beforeEach(() => vi.clearAllMocks());

  it("generates on open, regenerates on option change and emits the chosen password", async () => {
    api.generatePassword.mockResolvedValueOnce("first-password").mockResolvedValue("second-password");
    const wrapper = mount(PasswordGenerator);
    await flushPromises();
    expect(wrapper.find('[data-testid="generated-password"]').text()).toBe("first-password");

    await wrapper.find('input[name="symbols"]').setValue(false);
    await flushPromises();
    expect(api.generatePassword).toHaveBeenLastCalledWith({ length: 24, digits: true, symbols: false });

    await wrapper.findAll("button").find((button) => button.text() === "Use password")!.trigger("click");
    expect(wrapper.emitted("use")?.[0]).toEqual(["second-password"]);
  });
});
