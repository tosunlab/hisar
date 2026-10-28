import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createEmergencyKit, createItem, createStatus } from "../testing";
import globalStyles from "../style.css?raw";

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
enableAutoUnmount(afterEach);

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("signed-out"),
    allVault: [],
    selectedVaultID: "personal",
    listVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    onboardingStep: "",
    pendingInvite: null,
    pendingRecovery: [],
    isRecoveryOpen: false,
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
  it("shows hosted consent only for the actual signup target", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.find('[data-testid="legal-links"]').exists()).toBe(false);
    await wrapper.get('[data-testid="use-different-server"]').trigger("click");
    await wrapper.get('input[name="server"]').setValue(" HTTPS://EU.TRYHISAR.COM:443/ ");
    await wrapper.get('input[name="server"]').trigger("blur");
    await flushPromises();
    expect(wrapper.find('[data-testid="legal-links"]').exists()).toBe(true);
    await wrapper.get('input[name="inviteCode"]').setValue("https://hisar.example.org/invite/abc123");
    expect(wrapper.find('[data-testid="legal-links"]').exists()).toBe(false);
    await wrapper.findAll("button").find(button => button.text() === "Remove invitation")!.trigger("click");
    await flushPromises();
    await wrapper.get('input[name="inviteCode"]').setValue("https://eu.tryhisar.com/invite/abc123");
    expect(wrapper.find('[data-testid="legal-links"]').exists()).toBe(true);
  });

  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
  });

  it("keeps manual entry without an unsupported kit action on Linux", () => {
    Object.defineProperty(navigator, "platform", { value: "Linux x86_64", configurable: true });
    try {
      const wrapper = mount(SignInView);
      expect(wrapper.find('[data-testid="open-emergency-kit"]').exists()).toBe(false);
      expect(wrapper.find('input[name="secretKey"]').exists()).toBe(true);
      wrapper.unmount();
    } finally {
      Object.defineProperty(navigator, "platform", { value: "MacIntel", configurable: true });
    }
  });

  it("fills only account fields from a local kit and requires an explicit sign in", async () => {
    api.openEmergencyKit.mockResolvedValue({ server: "https://kit.example", email: "kit@example.com", secretKey: "H1-IMPORTED" });
    const wrapper = mount(SignInView, { attachTo: document.body });
    await wrapper.get('input[name="password"]').setValue("do not keep this password");
    await wrapper.get('[data-testid="open-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(wrapper.get('[data-testid="imported-kit"]').text()).toContain("kit@example.com on kit.example");
    expect(wrapper.find('input[name="secretKey"]').exists()).toBe(false);
    expect((wrapper.get('input[name="password"]').element as HTMLInputElement).value).toBe("");
    expect(wrapper.get('[data-testid="sign-in-server"]').text()).toBe("Signs in to kit.example.");
    expect(document.activeElement).toBe(wrapper.get('input[name="password"]').element);
    expect(api.signIn).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("keeps manual details after cancelling the kit panel and displays a failed import", async () => {
    api.openEmergencyKit.mockResolvedValue(null);
    const wrapper = mount(SignInView);
    await wrapper.get('input[name="email"]').setValue("manual@example.com");
    await wrapper.get('[data-testid="open-emergency-kit"]').trigger("click");
    await flushPromises();
    expect((wrapper.get('input[name="email"]').element as HTMLInputElement).value).toBe("manual@example.com");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    api.openEmergencyKit.mockRejectedValue(new Error("This PDF does not contain a valid Hisar Emergency Kit QR code."));
    await wrapper.get('[data-testid="open-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("This PDF does not contain a valid Hisar Emergency Kit QR code.");
    expect(api.signIn).not.toHaveBeenCalled();
  });

  it.each(["", "alice@example.com"])("focuses the first empty sign-in field for remembered email %s", (email) => {
    store.status.email = email;
    const wrapper = mount(SignInView, { attachTo: document.body });
    expect(document.activeElement).toBe(wrapper.get(`input[name="${email ? "secretKey" : "email"}"]`).element);
    wrapper.unmount();
  });

  it("focuses the server when its disclosure opens", async () => {
    const wrapper = mount(SignInView, { attachTo: document.body });
    await wrapper.get('[data-testid="use-different-server"]').trigger("click");
    await flushPromises();
    expect(document.activeElement).toBe(wrapper.get('input[name="server"]').element);
    wrapper.unmount();
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
    await flushPromises();
    await wrapper.find('input[name="password"]').setValue("first password");
    await wrapper.find('input[name="repeatedPassword"]').setValue("other password");
    expect(wrapper.find('button[type="submit"]').attributes("disabled")).toBeUndefined();
    await wrapper.find("form").trigger("submit");
    expect(wrapper.text()).toContain("The master passwords don't match.");
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
    expect(wrapper.find("h1").text()).toBe("Welcome back");
    expect(wrapper.text()).toContain("Made for the terminal");
    expect(wrapper.text()).toContain("The hisar command-line tool reads passwords and one-time codes, and uses SSH keys through the agent");
    expect(wrapper.text()).toContain("Private keys never leave Hisar.");
    expect(wrapper.text()).not.toContain("!");
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.find('[data-testid="brand-title"]').text()).toBe(tagline);
    expect(wrapper.get("h1").text()).toBe("Create account");
    expect(wrapper.text()).toContain("Safe even if the server is breached");
  });

  it("opens lost-key help without offering a password reset", async () => {
    const wrapper = mount(SignInView);
    await wrapper.get('[data-testid="secret-key-note"]').trigger("click");
    expect(wrapper.find('[role="dialog"]').text()).toContain("Hisar can't reset it");
    expect(wrapper.findAll('[role="tab"]').map((tab) => tab.text())).toEqual(["Sign in", "Create account"]);
  });

  it("saves the recovery warning for the Secret Key screen", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.text()).toContain("Your master password and a Secret Key you get next. Neither is ever sent to the server.");
    expect(wrapper.text()).not.toMatch(/can't be recovered|Nobody can reset/);
    expect(wrapper.find('[data-testid="create-account-note"]').exists()).toBe(false);
    wrapper.unmount();
    store.status = { ...createStatus("unlocked"), emergencyKit: createEmergencyKit() };
    const secretKey = mount(SecretKeyView);
    expect(secretKey.findAll('[data-testid="master-password-warning"]')).toHaveLength(1);
    expect(secretKey.get('[data-testid="master-password-warning"]').text()).toBe("A forgotten master password can't be recovered by anyone, including Hisar.");
    expect(secretKey.text()).not.toMatch(/admin/i);
    secretKey.unmount();
  });

  it("hides the server behind a disclosure and signs in to the hosted server by default", async () => {
    store.status = { ...store.status, server: "" };
    api.signIn.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    const wrapper = mount(SignInView);
    await wrapper.findAll("button").find(button => button.text() === "I already have an account")!.trigger("click");
    expect(wrapper.find('input[name="server"]').exists()).toBe(false);
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="secretKey"]').setValue("H1-KEY");
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signIn).toHaveBeenCalledWith("eu.tryhisar.com", "alice@example.com", "H1-KEY", "master password");
  });

  it("shows the server field after choosing a different server", async () => {
    store.status = { ...store.status, server: "https://eu.tryhisar.com/" };
    const wrapper = mount(SignInView);
    const disclosure = wrapper.find('[data-testid="use-different-server"]');
    expect(disclosure.text()).toBe("Use a different server…");
    await disclosure.trigger("click");
    expect((wrapper.find('input[name="server"]').element as HTMLInputElement).value).toBe("eu.tryhisar.com");
    expect(wrapper.find('input[name="server"]').exists()).toBe(true);
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
    expect(wrapper.find("h1").text()).toBe("Welcome to Hisar");
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

  it("rates the new master password with the backend's rule and blocks only clearly weak ones", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    await flushPromises();
    const submitButton = () => wrapper.find('button[type="submit"]');
    const feedback = () => wrapper.find('[data-testid="password-feedback"]');
    expect(wrapper.find('[data-testid="strength-meter"]').exists()).toBe(false);

    await wrapper.find('input[name="password"]').setValue("hunter2");
    await flushPromises();
    expect(feedback().text()).toBe("Use at least 10 characters.");
    expect(submitButton().attributes("disabled")).toBeUndefined();

    await wrapper.find('input[name="password"]').setValue("password1234");
    await wrapper.find('input[name="repeatedPassword"]').setValue("password1234");
    await flushPromises();
    expect(api.checkMasterPassword).toHaveBeenCalledWith("password1234");
    expect(wrapper.find('[data-testid="strength-meter"]').text()).toContain("Weak");
    expect(feedback().text()).toBe("This password is too easy to guess. Add a few more words or characters.");
    expect(submitButton().attributes("disabled")).toBeUndefined();
    await wrapper.find("form").trigger("submit");
    expect(api.signUp).not.toHaveBeenCalled();

    await wrapper.find('input[name="password"]').setValue("orange cactus harbor");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor");
    await flushPromises();
    expect(wrapper.find('[data-testid="strength-meter"]').text()).toContain("Weak");
    expect(feedback().text()).toBe("You can use it, but another word or a few more characters make it much harder to guess.");
    expect(submitButton().attributes("disabled")).toBeUndefined();

    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lantern");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
    await flushPromises();
    expect(wrapper.find('[data-testid="strength-meter"]').text()).toContain("Fair");
    expect(feedback().text()).toBe("A few random words you can remember make a strong master password.");
    expect(submitButton().attributes("disabled")).toBeUndefined();

    api.checkMasterPassword.mockReturnValueOnce(new Promise(() => undefined));
    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lanterns");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lanterns");
    await flushPromises();
    await wrapper.find("form").trigger("submit");
    expect(api.signUp).not.toHaveBeenCalled();
  });

  it("turns a pasted invite link into its code and signs up with the link", async () => {
    api.signUp.mockResolvedValue({ secretKey: "H1-NEW", status: createStatus("unlocked") });
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    await flushPromises();
    const inviteInput = wrapper.find('input[name="inviteCode"]');
    await inviteInput.setValue("https://hisar.example.com/invite/abc_DEF-123");
    expect(wrapper.get('[data-testid="invitation-card"]').text()).toContain("hisar.example.com");
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
    expect(api.signUp).toHaveBeenCalledWith("https://hisar.test/invite/abc123", "alice@example.com", "Alice", "orange cactus harbor lantern");
    expect(api.signUpWithoutInvite).not.toHaveBeenCalled();
  });

  it("creates the account without an invite on the chosen server", async () => {
    api.signUpWithoutInvite.mockResolvedValue({ secretKey: "H1-NEW", status: createStatus("unlocked") });
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.find('input[name="inviteCode"]').attributes("required")).toBeUndefined();
    expect(wrapper.find('[data-testid="invite-hint"]').text()).toContain("This server doesn't say whether it needs an invitation.");
    expect(wrapper.find('[data-testid="create-account-server"]').text()).toBe("Creates your account on hisar.test.");
    expect(wrapper.find('input[name="server"]').exists()).toBe(false);
    await wrapper.find('[data-testid="use-different-server"]').trigger("click");
    expect(wrapper.find('input[name="server"]').exists()).toBe(true);
    await wrapper.find('input[name="server"]').setValue("hisar.example.org");
    await wrapper.get('input[name="server"]').trigger("blur");
    await flushPromises();
    await wrapper.find('input[name="name"]').setValue("Alice");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lantern");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signUpWithoutInvite).toHaveBeenCalledWith("hisar.example.org", "alice@example.com", "Alice", "orange cactus harbor lantern");
    expect(api.signUp).not.toHaveBeenCalled();
  });

  it("lets the invite link name the server instead of the server field", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    await wrapper.find('[data-testid="use-different-server"]').trigger("click");
    expect(wrapper.find('input[name="server"]').exists()).toBe(true);
    await wrapper.find('input[name="inviteCode"]').setValue("https://hisar.example.com/invite/abc_DEF-123");
    expect((wrapper.get('input[name="server"]').element as HTMLInputElement).value).toBe("hisar.example.com");
    expect(wrapper.get('[data-testid="create-account-server"]').text()).toContain("hisar.example.com");
    expect(wrapper.find('[data-testid="invite-hint"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="invite-server"]').text()).toBe("on hisar.example.com");
    await wrapper.findAll("button").find(button => button.text() === "Remove invitation")!.trigger("click");
    await flushPromises();
    expect(wrapper.find('input[name="inviteCode"]').exists()).toBe(true);
  });

  it("shows the server's answer when sign-ups are not open", async () => {
    api.signUpWithoutInvite.mockRejectedValue(new Error("Sign-ups are not open on this server. Ask an admin for an invitation."));
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    await wrapper.find('input[name="name"]').setValue("Alice");
    await wrapper.find('input[name="email"]').setValue("alice@example.com");
    await wrapper.find('input[name="password"]').setValue("orange cactus harbor lantern");
    await wrapper.find('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signUpWithoutInvite).toHaveBeenCalledWith("hisar.test", "alice@example.com", "Alice", "orange cactus harbor lantern");
    expect(wrapper.find('[role="alert"]').text()).toBe("Sign-ups on hisar.test need an invitation now. Ask someone with an account there to invite you.");
    expect(store.status.state).toBe("signed-out");
  });
});

describe("TouchIDOfferView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStore();
    store.status = { ...createStatus("unlocked"), biometricAvailable: true };
    store.onboardingStep = "biometric";
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
    expect(store.onboardingStep).toBe("");
    expect(store.noticeMessage).toBe("Touch ID unlock is on");
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
  });

  it("stays on the offer without an error when Touch ID is cancelled", async () => {
    api.enableNewAccountBiometricUnlock.mockRejectedValue(new Error("Touch ID was cancelled"));
    const wrapper = mount(TouchIDOfferView);
    await wrapper.find('[data-testid="accept-touch-id-offer"]').trigger("click");
    await flushPromises();
    expect(store.onboardingStep).toBe("biometric");
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(api.forgetNewAccountSecretKey).not.toHaveBeenCalled();
  });

  it("continues without Touch ID on Not now", async () => {
    const wrapper = mount(TouchIDOfferView);
    await wrapper.find('[data-testid="decline-touch-id-offer"]').trigger("click");
    expect(store.onboardingStep).toBe("");
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

  it("defines a visible keyboard-focus outline for the native Length slider", async () => {
    api.generatePassword.mockResolvedValue("generated-password");
    const style = document.createElement("style");
    style.textContent = globalStyles;
    document.head.append(style);
    const wrapper = mount(PasswordGenerator);
    await flushPromises();
    expect(wrapper.get('input[name="length"]').attributes("type")).toBe("range");
    const rule = [...style.sheet!.cssRules].find((rule) => (rule as CSSStyleRule).selectorText?.replaceAll('"', "") === "input[type=range]:focus-visible") as CSSStyleRule;
    expect(rule.style.outline).toBe("2px solid var(--color-focus)");
    expect(rule.style.getPropertyValue("outline-offset")).toBe("2px");
    wrapper.unmount();
    style.remove();
  });

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

  it.each([8, 64])("generates at the supported length endpoint %i", async (length) => {
    api.generatePassword.mockResolvedValue("generated-password");
    const wrapper = mount(PasswordGenerator);
    await flushPromises();
    expect(api.generatePassword).toHaveBeenLastCalledWith({ length: 24, digits: true, symbols: true });
    const slider = wrapper.find('input[name="length"]');
    expect(slider.attributes("min")).toBe("8");
    expect(slider.attributes("max")).toBe("64");
    await slider.setValue(length);
    await flushPromises();
    expect(api.generatePassword).toHaveBeenLastCalledWith({ length, digits: true, symbols: true });
    wrapper.unmount();
  });

  it("keeps the same options in standalone mode and copies instead of selecting", async () => {
    api.generatePassword.mockResolvedValue("standalone-password");
    const wrapper = mount(PasswordGenerator, { props: { presentation: "standalone" } });
    await flushPromises();
    await wrapper.get('input[name="length"]').setValue(32);
    await wrapper.get('input[name="digits"]').setValue(false);
    await wrapper.get('input[name="symbols"]').setValue(false);
    await flushPromises();
    expect(api.generatePassword).toHaveBeenLastCalledWith({ length: 32, digits: false, symbols: false });
    expect(wrapper.text()).not.toContain("Use password");
    expect(wrapper.text()).not.toContain("Cancel");
    await wrapper.findAll("button").find((button) => button.text() === "Copy")!.trigger("click");
    expect(wrapper.emitted("copy")?.[0]).toEqual(["standalone-password"]);
    expect(wrapper.emitted("use")).toBeUndefined();
    await wrapper.findAll("button").find((button) => button.text() === "Close")!.trigger("click");
    expect(wrapper.emitted("close")).toHaveLength(1);
    wrapper.unmount();
  });
});
