import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createStatus, createUpdateStatus } from "../testing";
import type { InviteLink, UpdateRequirement } from "../api";
import { loadSelectedSectionID } from "../settingsSections";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const allListener = vi.hoisted(() => ({ current: new Map<string, (event: { data: unknown }) => void>() }));

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
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      allListener.current.set(name, listener);
      return () => undefined;
    },
  },
}));

const { store, unlock } = await import("../store");
const App = (await import("../App.vue")).default;
const api = apiMock.current;

enableAutoUnmount(afterEach);

const requirement: UpdateRequirement = { currentVersion: "0.1.5", minimumVersion: "0.1.6", updateURL: "https://tryhisar.com/download" };

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("signed-out"),
    allVault: [],
    allItem: [],
    selectedItem: null,
    onboardingStep: "",
    pendingInvite: null,
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  allListener.current.clear();
  resetStore();
  api.takePendingInvite.mockResolvedValue(null);
  api.getUpdateStatus.mockResolvedValue(createUpdateStatus());
});

describe("Update required", () => {
  it("blocks everything else while the server refuses this version", async () => {
    api.getStatus.mockResolvedValue({ ...createStatus("unlocked"), updateRequired: requirement });
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.find('[data-testid="update-required"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="update-required-versions"]').text()).toBe("This server needs Hisar 0.1.6 or later. This Mac has 0.1.5.");
    expect(wrapper.text()).toContain("Hisar needs an update");
    expect(wrapper.find(".sidebar").exists()).toBe(false);
    expect(api.listAllVault).not.toHaveBeenCalled();
  });

  it("shows the screen when the backend reports it while the app is open", async () => {
    api.getStatus.mockResolvedValue(createStatus("locked"));
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.find('[data-testid="update-required"]').exists()).toBe(false);
    allListener.current.get("update-required")?.({ data: requirement });
    await flushPromises();
    expect(wrapper.find('[data-testid="update-required"]').exists()).toBe(true);
  });

  it("updates through Sparkle when it is available", async () => {
    api.getStatus.mockResolvedValue({ ...createStatus("locked"), updateRequired: requirement });
    api.checkForUpdates.mockResolvedValue(createUpdateStatus());
    const wrapper = mount(App);
    await flushPromises();
    await wrapper.find('[data-testid="update-now"]').trigger("click");
    await flushPromises();
    expect(api.checkForUpdates).toHaveBeenCalledTimes(1);
    expect(api.openURL).not.toHaveBeenCalled();
  });

  it("opens the download page when Sparkle cannot update", async () => {
    api.getStatus.mockResolvedValue({ ...createStatus("signed-out"), updateRequired: { ...requirement, currentVersion: "" } });
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus({ isAvailable: false, isSignedIn: false }));
    api.openURL.mockResolvedValue(undefined);
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.find('[data-testid="update-required-versions"]').text()).toContain("This Mac has a development build.");
    await wrapper.find('[data-testid="update-now"]').trigger("click");
    await flushPromises();
    expect(api.checkForUpdates).not.toHaveBeenCalled();
    expect(api.openURL).toHaveBeenCalledWith("https://tryhisar.com/download");
  });

  it("goes back to the app once the server accepts this version again", async () => {
    api.getStatus.mockResolvedValue({ ...createStatus("locked"), updateRequired: requirement });
    api.checkMinimumVersion.mockResolvedValue(createStatus("locked"));
    const wrapper = mount(App);
    await flushPromises();
    await wrapper.find('[data-testid="check-again"]').trigger("click");
    await flushPromises();
    expect(api.checkMinimumVersion).toHaveBeenCalledTimes(1);
    expect(wrapper.find('[data-testid="update-required"]').exists()).toBe(false);
  });
});

// inviteLink is what core makes of a hisar://invite link; invitation and email come with an organization's or a share's.
function inviteLink(invite: string, extra: Partial<InviteLink> = {}): InviteLink {
  return { invite, server: "https://hisar.test", invitation: "", email: "", ...extra };
}

describe("Invite links", () => {
  it("opens Create account with the invite from a hisar:// link the app was launched with", async () => {
    api.getStatus.mockResolvedValue(createStatus("signed-out"));
    api.takePendingInvite.mockResolvedValue(inviteLink("https://hisar.example.com/invite/abc_DEF-123", { server: "https://hisar.example.com" }));
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Create account");
    expect(wrapper.find('[data-testid="invitation-card"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="invite-server"]').text()).toBe("on hisar.example.com");
  });

  it("fills in an invite that arrives while the app is open", async () => {
    api.getStatus.mockResolvedValue(createStatus("signed-out"));
    api.signUp.mockResolvedValue({ status: createStatus("unlocked"), secretKey: "H1-TEST" });
    const wrapper = mount(App);
    await flushPromises();
    allListener.current.get("invite-link")?.({ data: inviteLink("xyz789", { server: "https://eu.tryhisar.com" }) });
    await flushPromises();
    expect(wrapper.find('[data-testid="invitation-card"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="invite-server"]').text()).toBe("on eu.tryhisar.com");
    await wrapper.find('input[name="name"]').setValue("Ada");
    await wrapper.find('input[name="email"]').setValue("ada@example.com");
    await wrapper.find('input[name="password"]').setValue("correct horse battery staple");
    await wrapper.find('input[name="repeatedPassword"]').setValue("correct horse battery staple");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.signUp).toHaveBeenCalledWith("https://eu.tryhisar.com/invite/xyz789", "ada@example.com", "Ada", "correct horse battery staple");
  });

  it("says to sign out first when someone is signed in", async () => {
    api.getStatus.mockResolvedValue(createStatus("locked"));
    api.takePendingInvite.mockResolvedValue(inviteLink("abc"));
    mount(App);
    await flushPromises();
    expect(store.pendingInvite).toBeNull();
    expect(store.noticeMessage).toBe("You're signed in as alice@example.com. Sign out to create an account with this invite.");
  });

  it("shows an organization invitation in Settings to the account it is for", async () => {
    api.getStatus.mockResolvedValue(createStatus("unlocked"));
    mount(App);
    await flushPromises();
    allListener.current.get("invite-link")?.({ data: inviteLink("abc", { invitation: "organization", email: "alice@example.com" }) });
    await flushPromises();
    expect(store.isSettingsOpen).toBe(true);
    expect(loadSelectedSectionID()).toBe("organizations");
    expect(store.pendingInvite).toBeNull();
  });

  it("shows a share invitation in Settings → People after the unlock", async () => {
    api.getStatus.mockResolvedValue(createStatus("locked"));
    api.takePendingInvite.mockResolvedValue(inviteLink("abc", { invitation: "share", email: "alice@example.com" }));
    api.unlock.mockResolvedValue(createStatus("unlocked"));
    mount(App);
    await flushPromises();
    expect(store.isSettingsOpen).toBe(false);
    expect(store.noticeMessage).toBe("Unlock Hisar to see the invitation.");
    await unlock("master password");
    await flushPromises();
    expect(store.isSettingsOpen).toBe(true);
    expect(loadSelectedSectionID()).toBe("people");
  });

  it("names who an invitation is for when another account is signed in", async () => {
    api.getStatus.mockResolvedValue(createStatus("unlocked"));
    mount(App);
    await flushPromises();
    allListener.current.get("invite-link")?.({ data: inviteLink("abc", { invitation: "organization", email: "bob@example.com" }) });
    await flushPromises();
    expect(store.isSettingsOpen).toBe(false);
    expect(store.noticeMessage).toBe("This invitation is for bob@example.com on hisar.test. You're signed in as alice@example.com.");
    allListener.current.get("invite-link")?.({ data: inviteLink("abc", { invitation: "organization", email: "alice@example.com", server: "https://other.example.com" }) });
    await flushPromises();
    expect(store.isSettingsOpen).toBe(false);
    expect(store.noticeMessage).toBe("This invitation is for alice@example.com on other.example.com. You're signed in as alice@example.com.");
  });
});
