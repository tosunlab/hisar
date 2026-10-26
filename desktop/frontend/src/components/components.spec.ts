import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createStatus } from "../testing";

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
const TouchIDOfferView = (await import("./TouchIDOfferView.vue")).default;
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
    isTouchIDOfferShown: false,
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
    expect(wrapper.text()).toContain("Sign in to Hisar on this Mac.");

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
    api.getStatus.mockResolvedValue(createStatus("locked"));
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
    expect(api.signIn).toHaveBeenCalledWith("hisar.test", "alice@example.com", "H1-KEY", "master password");
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

  it("keeps one tagline and shows the developer panel for sign in and the safety panel for account creation", async () => {
    const tagline = "Passwords, SSH keys and secrets for developers, end-to-end encrypted.";
    const wrapper = mount(SignInView);
    expect(wrapper.find('[data-testid="brand-title"]').text()).toBe(tagline);
    expect(wrapper.find("h1").exists()).toBe(false);
    expect(wrapper.text()).toContain("Made for the terminal");
    expect(wrapper.text()).toContain("The hisar command-line tool reads passwords and one-time codes, and uses SSH keys through the agent");
    expect(wrapper.text()).toContain("Private keys never leave Hisar.");
    expect(wrapper.text()).not.toContain("!");
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.find('[data-testid="brand-title"]').text()).toBe(tagline);
    expect(wrapper.get("h1").text()).toBe("Create your account");
    expect(wrapper.get(".subtitle").text()).toBe("Hisar is invite-only: paste the code or link you received.");
    expect(wrapper.text()).toContain("Safe even if the server is breached");
  });

  it("points to the Emergency Kit for a lost Secret Key and offers no account recovery", () => {
    const wrapper = mount(SignInView);
    expect(wrapper.find('[data-testid="secret-key-note"]').text()).toBe("Lost your Secret Key? It's on your Emergency Kit.");
    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text())).toEqual(["Sign in", "Create account"]);
    expect(wrapper.text()).not.toMatch(/recover|admin/i);
  });

  it("saves the recovery warning for the Secret Key screen", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.text()).toContain("Your master password and a Secret Key you get next. Neither is ever sent to the server.");
    expect(wrapper.text()).not.toMatch(/can't be recovered|Nobody can reset/);
    expect(wrapper.find('[data-testid="create-account-note"]').exists()).toBe(false);
    wrapper.unmount();
    store.pendingSecretKey = "H1-ABC123-SECRET";
    const secretKey = mount(SecretKeyView);
    expect(secretKey.findAll('[data-testid="master-password-warning"]')).toHaveLength(1);
    expect(secretKey.get('[data-testid="master-password-warning"]').text()).toBe("A forgotten master password can't be recovered by anyone, including Hisar.");
    secretKey.unmount();
  });

  it("hides the server behind a disclosure and signs in to the hosted server by default", async () => {
    store.status = { ...store.status, server: "" };
    api.signIn.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(SignInView);
    expect(wrapper.find('input[name="server"]').exists()).toBe(false);
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="secretKey"]').setValue("H1-KEY");
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signIn).toHaveBeenCalledWith("api.tryhisar.com", "alice@example.com", "H1-KEY", "master password");
  });

  it("shows the server field after choosing a different server", async () => {
    store.status = { ...store.status, server: "https://api.tryhisar.com/" };
    const wrapper = mount(SignInView);
    const disclosure = wrapper.find('[data-testid="use-different-server"]');
    expect(disclosure.text()).toBe("Use a different server…");
    await disclosure.trigger("click");
    expect((wrapper.find('input[name="server"]').element as HTMLInputElement).value).toBe("api.tryhisar.com");
    expect(wrapper.find('[data-testid="use-different-server"]').exists()).toBe(false);
  });

  it("keeps another server behind the disclosure too, naming it", async () => {
    const wrapper = mount(SignInView);
    expect(wrapper.find('input[name="server"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="sign-in-server"]').text()).toBe("Signs in to hisar.test.");
    await wrapper.find('[data-testid="use-different-server"]').trigger("click");
    expect((wrapper.find('input[name="server"]').element as HTMLInputElement).value).toBe("hisar.test");
  });

  it("signs back in to the server the Mac used before signing out", async () => {
    // A signed-out backend reports the last server and no email (core/app buildStatus).
    store.status = { ...createStatus("signed-out"), server: "http://127.0.0.1:8080", email: "" };
    api.signIn.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(SignInView);
    expect(wrapper.find('input[name="server"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="sign-in-server"]').text()).toBe("Signs in to http://127.0.0.1:8080.");
    expect((wrapper.find('input[name="email"]').element as HTMLInputElement).value).toBe("");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="secretKey"]').setValue("H1-KEY");
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signIn).toHaveBeenCalledWith("http://127.0.0.1:8080", "alice@example.com", "H1-KEY", "master password");
  });

  it("names the remembered server when the first status arrives after the view opened", async () => {
    store.status = { ...createStatus("signed-out"), server: "", email: "" };
    api.signIn.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(SignInView);
    expect(wrapper.find('[data-testid="sign-in-server"]').text()).toBe("Signs in to api.tryhisar.com.");
    store.status = { ...createStatus("signed-out"), server: "http://127.0.0.1:8080", email: "" };
    await flushPromises();
    expect(wrapper.find('[data-testid="sign-in-server"]').text()).toBe("Signs in to http://127.0.0.1:8080.");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="secretKey"]').setValue("H1-KEY");
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signIn).toHaveBeenCalledWith("http://127.0.0.1:8080", "alice@example.com", "H1-KEY", "master password");
  });

  it("rates the new master password and blocks only clearly weak ones", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    const submitButton = () => wrapper.find('button[type="submit"]');
    const feedback = () => wrapper.find('[data-testid="password-feedback"]');
    expect(wrapper.find('[data-testid="strength-meter"]').exists()).toBe(false);

    await wrapper.find('input[name="password"]').setValue("hunter2");
    expect(feedback().text()).toBe("Use at least 10 characters.");
    expect(submitButton().attributes("disabled")).toBeDefined();

    await wrapper.find('input[name="password"]').setValue("password1234");
    await wrapper.find('input[name="repeatedPassword"]').setValue("password1234");
    expect(wrapper.find('[data-testid="strength-meter"]').text()).toContain("Weak");
    expect(feedback().text()).toBe("This password is too easy to guess. Add a few more words or characters.");
    expect(submitButton().attributes("disabled")).toBeDefined();
    await wrapper.find("form").trigger("submit");
    expect(api.signUp).not.toHaveBeenCalled();

    await wrapper.find('input[name="password"]').setValue("orange cactus harbor");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor");
    expect(wrapper.find('[data-testid="strength-meter"]').text()).toContain("Weak");
    expect(feedback().text()).toBe("You can use it, but another word or a few more characters make it much harder to guess.");
    expect(submitButton().attributes("disabled")).toBeUndefined();

    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lantern");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
    expect(wrapper.find('[data-testid="strength-meter"]').text()).toContain("Fair");
    expect(feedback().text()).toBe("A few random words you can remember make a strong master password.");
    expect(submitButton().attributes("disabled")).toBeUndefined();
  });

  it("turns a pasted invite link into its code and signs up with the link", async () => {
    api.signUp.mockResolvedValue({ secretKey: "H1-NEW", status: createStatus("unlocked") });
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    const inviteInput = wrapper.find('input[name="inviteCode"]');
    await inviteInput.setValue("https://hisar.example.com/invite/abc_DEF-123");
    expect((inviteInput.element as HTMLInputElement).value).toBe("abc_DEF-123");
    await wrapper.find('input[name="name"]').setValue("Alice");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lantern");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signUp).toHaveBeenCalledWith("https://hisar.example.com/invite/abc_DEF-123", "alice@example.com", "Alice", "orange cactus harbor lantern");
  });

  it("signs up with a typed invite code", async () => {
    api.signUp.mockResolvedValue({ secretKey: "H1-NEW", status: createStatus("unlocked") });
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    await wrapper.find('input[name="inviteCode"]').setValue("abc123");
    await wrapper.find('input[name="name"]').setValue("Alice");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lantern");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signUp).toHaveBeenCalledWith("abc123", "alice@example.com", "Alice", "orange cactus harbor lantern");
  });
});

describe("SecretKeyView", () => {
  beforeEach(resetStore);

  it("says that a forgotten master password can't be recovered", () => {
    store.pendingSecretKey = "H1-ABC123-SECRET";
    const wrapper = mount(SecretKeyView);
    expect(wrapper.find("h1").text()).toBe("Save your Secret Key");
    expect(wrapper.find('[data-testid="master-password-warning"]').text()).toBe("A forgotten master password can't be recovered by anyone, including Hisar.");
    expect(wrapper.text()).not.toMatch(/admin/i);
  });

  it("only continues after the user confirms saving the Secret Key", async () => {
    store.pendingSecretKey = "H1-ABC123-SECRET";
    const wrapper = mount(SecretKeyView);
    expect(wrapper.find('[data-testid="secret-key"]').attributes("aria-label")).toBe("H1-ABC123-SECRET");
    const continueButton = wrapper.findAll("button").find((button) => button.text() === "Continue to Hisar")!;
    expect(continueButton.attributes("disabled")).toBeDefined();
    await wrapper.find('input[name="hasSaved"]').setValue(true);
    await continueButton.trigger("click");
    expect(store.pendingSecretKey).toBe("");
    expect(store.isTouchIDOfferShown).toBe(false);
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
  });

  it("offers Touch ID after the Secret Key when the Mac has it", async () => {
    store.pendingSecretKey = "H1-ABC123-SECRET";
    store.status = { ...createStatus("unlocked"), biometricAvailable: true };
    const wrapper = mount(SecretKeyView);
    await wrapper.find('input[name="hasSaved"]').setValue(true);
    await wrapper.findAll("button").find((button) => button.text() === "Continue to Hisar")!.trigger("click");
    expect(store.pendingSecretKey).toBe("");
    expect(store.isTouchIDOfferShown).toBe(true);
    expect(api.forgetNewAccountSecretKey).not.toHaveBeenCalled();
  });
});

describe("TouchIDOfferView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    store.status = { ...createStatus("unlocked"), biometricAvailable: true };
    store.isTouchIDOfferShown = true;
  });

  it("turns on Touch ID in one click with the key kept from sign-up", async () => {
    api.enableNewAccountBiometricUnlock.mockResolvedValue({ ...createStatus("unlocked"), biometricAvailable: true, biometricEnabled: true });
    const wrapper = mount(TouchIDOfferView);
    expect(wrapper.find("h1").text()).toBe("Unlock with Touch ID?");
    await wrapper.find('[data-testid="accept-touch-id-offer"]').trigger("click");
    await flushPromises();
    expect(api.enableNewAccountBiometricUnlock).toHaveBeenCalledTimes(1);
    expect(api.enableBiometricUnlock).not.toHaveBeenCalled();
    expect(store.status.biometricEnabled).toBe(true);
    expect(store.isTouchIDOfferShown).toBe(false);
    expect(store.noticeMessage).toBe("Touch ID unlock is on");
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
  });

  it("stays on the offer without an error when Touch ID is cancelled", async () => {
    api.enableNewAccountBiometricUnlock.mockRejectedValue(new Error("Touch ID was cancelled"));
    const wrapper = mount(TouchIDOfferView);
    await wrapper.find('[data-testid="accept-touch-id-offer"]').trigger("click");
    await flushPromises();
    expect(store.isTouchIDOfferShown).toBe(true);
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(api.forgetNewAccountSecretKey).not.toHaveBeenCalled();
  });

  it("continues without Touch ID on Not now", async () => {
    const wrapper = mount(TouchIDOfferView);
    await wrapper.find('[data-testid="decline-touch-id-offer"]').trigger("click");
    expect(store.isTouchIDOfferShown).toBe(false);
    expect(api.enableNewAccountBiometricUnlock).not.toHaveBeenCalled();
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
  });

  it("is shown by the app between the Secret Key and the vaults", async () => {
    api.getStatus.mockResolvedValue({ ...createStatus("unlocked"), biometricAvailable: true });
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.find('[data-testid="touch-id-offer"]').exists()).toBe(true);
    wrapper.unmount();
  });
});

describe("ItemDetailPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    store.selectedItem = createItem({ password: "hunter2hunter2", fields: [{ name: "PIN", value: "1234", concealed: true }] });
  });

  it("masks the password until revealed", async () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    expect(wrapper.text()).not.toContain("hunter2hunter2");
    expect(wrapper.text()).not.toContain("1234");
    await wrapper.find('button[aria-label="Reveal password"]').trigger("click");
    expect(wrapper.find('[data-testid="password-value"]').text()).toBe("hunter2hunter2");
  });

  it("copies through the backend instead of exposing the clipboard to the page", async () => {
    api.copyField.mockResolvedValue(undefined);
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    await wrapper.find('button[aria-label="Copy password"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "item-1", "password");
  });

  it("deletes from the ⋯ menu through the one confirmation dialog", async () => {
    api.deleteItem.mockResolvedValue(undefined);
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    expect(wrapper.find('button[aria-label="Delete item"]').exists()).toBe(false);
    await wrapper.find('[data-testid="item-actions"]').trigger("click");
    expect(wrapper.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Duplicate", "Delete…"]);
    await wrapper.find('[data-testid="context-menu-delete"]').trigger("click");
    expect(api.deleteItem).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="confirm-delete-dialog"]').text()).toContain("Delete “GitHub”?");
    await wrapper.find('[data-testid="confirm-delete-item"]').trigger("click");
    await flushPromises();
    expect(api.deleteItem).toHaveBeenCalledWith("personal", "item-1");
    wrapper.unmount();
  });

  it("offers Move to… in the ⋯ menu when another vault takes items", async () => {
    store.allVault = [allTestVault[0], { id: "work", name: "Work", type: "shared", role: "editor", itemCount: 0 }];
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" }, attachTo: document.body });
    await wrapper.find('[data-testid="item-actions"]').trigger("click");
    expect(wrapper.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Duplicate", "Move to…", "Delete…"]);
    await wrapper.find('[data-testid="context-menu-move"]').trigger("click");
    expect(wrapper.find('[data-testid="move-item-dialog"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("hides edit and the ⋯ menu for read-only vaults", () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: false, vaultName: "Work", vaultType: "shared" } });
    expect(wrapper.findAll("button").map((button) => button.text())).not.toContain("Edit");
    expect(wrapper.find('[data-testid="item-actions"]').exists()).toBe(false);
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
