import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import type { VaultSummary } from "../api";
import { createApiMock, createPerson, createItem, createPendingRecovery, createRecoveryStatus, createStatus, testBob } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", category: "", title: "", username: "", urls: [], password: "", notes: "", fields: [], publicKey: "", fingerprint: "", keyType: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const SignInView = (await import("./SignInView.vue")).default;
const SecretKeyView = (await import("./SecretKeyView.vue")).default;
const api = apiMock.current;

function createVaultList(overrides: Partial<VaultSummary> = {}): VaultSummary[] {
  return [
    { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1, recoverable: true, recoveryOptOut: false, ...overrides },
    { id: "family", name: "Family", type: "shared", role: "owner", itemCount: 0, recoverable: true, recoveryOptOut: false },
  ];
}

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: createVaultList(),
    selectedVaultID: "personal",
    allItem: [createItem()],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    pendingSecretKey: "",
    isRecoverySubmitted: false,
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isSharingOpen: false,
    isVaultRecoveryOpen: false,
    allPerson: [],
    allDevice: [],
    recoveryStatus: null,
    allPendingRecovery: [],
  });
}

function mockFamily(isAdmin = true): void {
  api.listAllPerson.mockResolvedValue([
    createPerson({ role: isAdmin ? "admin" : "member" }),
    createPerson({ ...testBob, role: "member" }),
  ]);
}

async function mountSettings() {
  const wrapper = mount(SettingsPanel, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
  mockFamily();
});

describe("Account recovery in Settings", () => {
  it("lets an admin set up recovery after explaining the trade-off", async () => {
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isSetUp: false, isCreatedByCurrentUser: false, createdByName: "" }));
    const wrapper = await mountSettings();
    const section = wrapper.find('[data-testid="recovery-section"]');
    expect(section.find('[data-testid="recovery-state"]').text()).toBe("Off");
    expect(section.text()).toContain("their vaults are lost");
    expect(wrapper.find('[data-testid="start-recovery-BOB222"]').exists()).toBe(false);

    await section.find('[data-testid="start-recovery-setup"]').trigger("click");
    const confirmation = wrapper.find('[data-testid="recovery-setup-confirmation"]');
    expect(confirmation.text()).toContain("can open every recoverable vault");
    expect(confirmation.text()).toContain("lost if they lose access");
    expect(api.setUpRecovery).not.toHaveBeenCalled();

    api.setUpRecovery.mockResolvedValue(["Old notes"]);
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    api.listAllVault.mockResolvedValue(createVaultList());
    await wrapper.find('[data-testid="confirm-recovery-setup"]').trigger("click");
    await flushPromises();
    expect(api.setUpRecovery).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="recovery-setup-confirmation"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="recovery-state"]').text()).toBe("On");
    expect(wrapper.find('[data-testid="recovery-failed-vaults"]').text()).toContain("Old notes");
    expect(wrapper.find('[data-testid="no-pending-recovery"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("starts a recovery from a family member and shows the link", async () => {
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    const wrapper = await mountSettings();
    expect(wrapper.find('[data-testid="start-recovery-ALICE1"]').exists()).toBe(false);

    await wrapper.find('[data-testid="start-recovery-BOB222"]').trigger("click");
    expect(wrapper.find('[data-testid="start-recovery-confirmation"]').text()).toContain("keeps working until you complete the recovery");
    api.startAccountRecovery.mockResolvedValue("https://hisar.test/recover/token");
    await wrapper.find('[data-testid="confirm-start-recovery"]').trigger("click");
    await flushPromises();
    expect(api.startAccountRecovery).toHaveBeenCalledWith("BOB222");
    const result = wrapper.find('[data-testid="recovery-link-result"]');
    expect(result.find('[data-testid="recovery-link"]').text()).toBe("https://hisar.test/recover/token");
    expect(result.text()).toContain("valid for 7 days");

    api.copyText.mockResolvedValue(undefined);
    await result.find('[data-testid="copy-recovery-link"]').trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith("https://hisar.test/recover/token");
    wrapper.unmount();
  });

  it("completes a pending recovery after an inline confirmation and lists the lost vaults", async () => {
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    api.listAllPendingRecovery.mockResolvedValue([createPendingRecovery()]);
    const wrapper = await mountSettings();
    const row = wrapper.find('[data-testid="pending-recovery-BOB222"]');
    expect(row.text()).toContain("Bob Stone");
    expect(row.text()).toContain("Asked 3 hours ago");
    expect(row.text()).toContain("2 of 3 vaults can be recovered");

    await row.find('[data-testid="complete-recovery-BOB222"]').trigger("click");
    const confirmation = wrapper.find('[data-testid="complete-recovery-confirmation"]');
    expect(confirmation.text()).toContain("devices are signed out");
    expect(confirmation.text()).toContain("1 vault cannot be recovered");
    expect(api.completeAccountRecovery).not.toHaveBeenCalled();

    api.completeAccountRecovery.mockResolvedValue(["Bob's personal vault"]);
    api.listAllPendingRecovery.mockResolvedValue([]);
    await confirmation.find('[data-testid="confirm-complete-recovery"]').trigger("click");
    await flushPromises();
    expect(api.completeAccountRecovery).toHaveBeenCalledWith("BOB222");
    const result = wrapper.find('[data-testid="recovery-result"]');
    expect(result.text()).toContain("Bob can now sign in");
    expect(result.find('[data-testid="unrecoverable-vaults"]').text()).toBe("Bob's personal vault");
    expect(wrapper.find('[data-testid="pending-recovery-BOB222"]').exists()).toBe(false);
    expect(api.listAllPerson).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it("only lets the admin who set up recovery complete it", async () => {
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isCreatedByCurrentUser: false, createdByName: "Elif" }));
    api.listAllPendingRecovery.mockResolvedValue([createPendingRecovery({ unrecoverableVaultCount: 0 })]);
    const wrapper = await mountSettings();
    expect(wrapper.find('[data-testid="recovery-description"]').text()).toContain("only Elif can complete them");
    const completeButton = wrapper.find('[data-testid="complete-recovery-BOB222"]');
    expect(completeButton.attributes("disabled")).toBeDefined();
    expect(completeButton.attributes("title")).toContain("Only Elif");
    expect(wrapper.find('[data-testid="pending-recovery-BOB222"]').text()).toContain("All 2 of their vaults can be recovered");
    wrapper.unmount();
  });

  it("shows members whether their vaults are recoverable and turns on the missing ones", async () => {
    mockFamily(false);
    store.allVault = createVaultList({ recoverable: false });
    store.allVault.push({ id: "old", name: "Old notes", type: "shared", role: "owner", itemCount: 0, recoverable: false, recoveryOptOut: false });
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isAdmin: false, isCreatedByCurrentUser: false, createdByName: "Elif" }));
    const wrapper = await mountSettings();
    expect(wrapper.find('[data-testid="recovery-description"]').text()).toContain("Elif can help you back in");
    expect(wrapper.find('[data-testid="start-recovery-setup"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="start-recovery-BOB222"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="no-pending-recovery"]').exists()).toBe(false);
    expect(api.listAllPendingRecovery).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="vault-recovery-family"]').text()).toContain("Recoverable");
    expect(wrapper.find('[data-testid="vault-recovery-old"]').text()).toContain("Not recoverable yet");

    api.enableRecoveryForAllVault.mockResolvedValue([]);
    api.listAllVault.mockResolvedValue(createVaultList());
    await wrapper.find('[data-testid="enable-recovery-all"]').trigger("click");
    await flushPromises();
    expect(api.enableRecoveryForAllVault).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="enable-recovery-all"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="vault-recovery-personal"]').text()).toContain("Recoverable");
    wrapper.unmount();
  });

  it("tells members when recovery is not set up", async () => {
    mockFamily(false);
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isSetUp: false, isAdmin: false, isCreatedByCurrentUser: false, createdByName: "" }));
    const wrapper = await mountSettings();
    expect(wrapper.find('[data-testid="recovery-description"]').text()).toContain("Ask your admin to set up recovery");
    expect(wrapper.find('[data-testid="recovery-vault-list"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Personal vault recovery", () => {
  async function openPanel() {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="personal-vault-private"]').trigger("click");
    await flushPromises();
    return wrapper;
  }

  it("warns that the vault is lost before turning recovery off", async () => {
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isCreatedByCurrentUser: false, createdByName: "Elif" }));
    const wrapper = await openPanel();
    expect(wrapper.find('[data-testid="vault-recovery-state"]').text()).toContain("Elif can recover this vault");

    await wrapper.find('[data-testid="disable-vault-recovery"]').trigger("click");
    const confirmation = wrapper.find('[data-testid="disable-vault-recovery-confirmation"]');
    expect(confirmation.text()).toContain("everything in it is lost");
    expect(confirmation.text()).toContain("Nobody, including your admin, can get it back");
    expect(api.disableVaultRecovery).not.toHaveBeenCalled();

    api.disableVaultRecovery.mockResolvedValue({ ...createVaultList()[0], recoverable: false, recoveryOptOut: true });
    await confirmation.find('[data-testid="confirm-disable-vault-recovery"]').trigger("click");
    await flushPromises();
    expect(api.disableVaultRecovery).toHaveBeenCalledWith("personal");
    expect(wrapper.find('[data-testid="vault-recovery-state"]').text()).toContain("Recovery is off");
    expect(wrapper.find('[data-testid="vault-recovery-off"]').exists()).toBe(true);
    expect(store.noticeMessage).toBe("Recovery is off for Personal");
    wrapper.unmount();
  });

  it("turns recovery back on for an opted-out vault", async () => {
    store.allVault = createVaultList({ recoverable: false, recoveryOptOut: true });
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus());
    const wrapper = await openPanel();
    expect(wrapper.find('[data-testid="vault-recovery-state"]').text()).toContain("its items are lost for good");
    expect(wrapper.find('[data-testid="disable-vault-recovery"]').exists()).toBe(false);

    api.enableVaultRecovery.mockResolvedValue(createVaultList()[0]);
    await wrapper.find('[data-testid="enable-vault-recovery"]').trigger("click");
    await flushPromises();
    expect(api.enableVaultRecovery).toHaveBeenCalledWith("personal");
    expect(wrapper.find('[data-testid="vault-recovery-state"]').text()).toContain("nobody can recover your own account");
    expect(wrapper.find('[data-testid="vault-recovery-off"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("offers nothing to turn on before recovery is set up, and closes with Escape", async () => {
    store.allVault = createVaultList({ recoverable: false });
    api.getRecoveryStatus.mockResolvedValue(createRecoveryStatus({ isSetUp: false }));
    const wrapper = await openPanel();
    expect(wrapper.find('[data-testid="vault-recovery-state"]').text()).toContain("has not set up account recovery");
    expect(wrapper.find('[data-testid="enable-vault-recovery"]').exists()).toBe(false);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-recovery-panel"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Recovering an account from the sign-in screen", () => {
  beforeEach(() => {
    store.status = { ...createStatus("signed-out"), server: "", email: "" };
  });

  it("submits the recovery link, email and new master password", async () => {
    const wrapper = mount(SignInView);
    await wrapper.find('[data-testid="open-recover-account"]').trigger("click");
    const form = wrapper.find('[data-testid="recover-account-form"]');
    expect(wrapper.find("h1").text()).toBe("Recover your account");
    await form.find('input[name="recoveryLink"]').setValue("https://hisar.test/recover/token");
    await form.find('input[name="email"]').setValue("bob@example.com");
    await form.find('input[name="password"]').setValue("correct horse battery staple");
    expect(form.find('[data-testid="strength-meter"]').exists()).toBe(true);
    await form.find('input[name="repeatedPassword"]').setValue("something else");
    expect(form.text()).toContain("The passwords do not match");
    await form.trigger("submit");
    expect(api.recoverAccount).not.toHaveBeenCalled();

    await form.find('input[name="repeatedPassword"]').setValue("correct horse battery staple");
    api.recoverAccount.mockResolvedValue({ status: { ...createStatus("signed-out"), server: "https://hisar.test", email: "bob@example.com" }, secretKey: "H1-NEW-KEY" });
    await form.trigger("submit");
    await flushPromises();
    expect(api.recoverAccount).toHaveBeenCalledWith("https://hisar.test/recover/token", "bob@example.com", "correct horse battery staple");
    expect(store.pendingSecretKey).toBe("H1-NEW-KEY");
    expect(store.isRecoverySubmitted).toBe(true);
    wrapper.unmount();
  });

  it("shows the error for an invalid link and goes back to sign in", async () => {
    const wrapper = mount(SignInView);
    await wrapper.find('[data-testid="open-recover-account"]').trigger("click");
    const form = wrapper.find('[data-testid="recover-account-form"]');
    await form.find('input[name="recoveryLink"]').setValue("https://hisar.test/recover/used");
    await form.find('input[name="email"]').setValue("bob@example.com");
    await form.find('input[name="password"]').setValue("correct horse battery staple");
    await form.find('input[name="repeatedPassword"]').setValue("correct horse battery staple");
    api.recoverAccount.mockRejectedValue(new Error("this recovery link is invalid, expired or already used, or it was sent to a different email"));
    await form.trigger("submit");
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toContain("recovery link is invalid");
    await wrapper.find('[data-testid="back-to-sign-in"]').trigger("click");
    expect(wrapper.find("h1").text()).toBe("Welcome back");
    expect(store.errorMessage).toBe("");
    wrapper.unmount();
  });

  it("shows the new Secret Key with the Emergency Kit, then explains when to sign in", async () => {
    Object.assign(store, { pendingSecretKey: "H1-NEW-KEY", isRecoverySubmitted: true, status: { ...createStatus("signed-out"), server: "https://hisar.test", email: "bob@example.com" } });
    const secretKeyView = mount(SecretKeyView);
    expect(secretKeyView.find("h1").text()).toBe("Save your new Secret Key");
    expect(secretKeyView.find('[data-testid="recovery-secret-key-note"]').text()).toContain("Your admin now completes the recovery");
    api.saveNewAccountEmergencyKit.mockResolvedValue("/Users/bob/Hisar Emergency Kit.pdf");
    await secretKeyView.find('[data-testid="save-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(api.saveNewAccountEmergencyKit).toHaveBeenCalled();
    await secretKeyView.find('input[name="hasSaved"]').setValue(true);
    await secretKeyView.findAll("button").find((button) => button.text() === "Continue to sign in")!.trigger("click");
    expect(store.pendingSecretKey).toBe("");
    secretKeyView.unmount();

    const signInView = mount(SignInView);
    expect(signInView.find('[data-testid="recovery-submitted"]').text()).toContain("Once your admin completes it");
    expect((signInView.find('input[name="email"]').element as HTMLInputElement).value).toBe("bob@example.com");
    expect((signInView.find('input[name="server"]').element as HTMLInputElement).value).toBe("hisar.test");
    signInView.unmount();
  });
});
