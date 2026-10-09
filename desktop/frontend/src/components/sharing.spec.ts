import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import type { VaultSummary } from "../api";
import { createApiMock, createDevice, createFamilyMember, createItem, createStatus, createVaultMember, testBob, testCarol } from "../testing";

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
const api = apiMock.current;

function createVaultList(familyRole: string): VaultSummary[] {
  return [
    { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0 },
    { id: "family", name: "Family", type: "shared", role: familyRole, itemCount: 1 },
  ];
}

function resetStore(familyRole = "owner"): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: createVaultList(familyRole),
    selectedVaultID: "family",
    allItem: [createItem()],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isSharingOpen: false,
    allVaultMember: [],
    allFamilyMember: [],
    allDevice: [],
  });
}

function mockMembers(familyRole = "owner"): void {
  api.listAllVaultMember.mockResolvedValue([
    createVaultMember({ role: familyRole }),
    createVaultMember({ ...testBob, role: "editor" }),
  ]);
  api.listAllMember.mockResolvedValue([
    createFamilyMember(),
    createFamilyMember({ ...testBob, role: "member" }),
    createFamilyMember({ ...testCarol, role: "member" }),
  ]);
}

async function openSharingPanel() {
  const wrapper = mount(MainView, { attachTo: document.body });
  await wrapper.find('[data-testid="share-vault"]').trigger("click");
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
  mockMembers();
});

describe("Vault sharing panel", () => {
  it("lists the members with initials, role and a you tag", async () => {
    const wrapper = await openSharingPanel();
    expect(api.listAllVaultMember).toHaveBeenCalledWith("family");
    const panel = wrapper.find('[data-testid="vault-sharing-panel"]');
    expect(panel.text()).toContain("Share “Family”");
    expect(panel.text()).toContain("2 members");

    const own = panel.find('[data-testid="vault-member-ALICE1"]');
    expect(own.text()).toContain("you");
    expect(own.text()).toContain("Owner");
    expect(own.find("select").exists()).toBe(false);

    const bob = panel.find('[data-testid="vault-member-BOB222"]');
    expect(bob.text()).toContain("BS");
    expect(bob.text()).toContain("Bob Stone");
    expect(bob.text()).toContain("bob@example.com");
    expect(bob.text()).not.toContain("you");
    expect((bob.find('[data-testid="role-select-BOB222"]').element as HTMLSelectElement).value).toBe("editor");
    wrapper.unmount();
  });

  it("shares with a family member who is not in the vault yet", async () => {
    api.shareVault.mockResolvedValue(undefined);
    const wrapper = await openSharingPanel();
    const allOption = wrapper.findAll('[data-testid="new-member-select"] option').map((option) => option.text());
    expect(allOption.some((text) => text.includes("Carol"))).toBe(true);
    expect(allOption.some((text) => text.includes("Bob"))).toBe(false);

    await wrapper.find('[data-testid="new-member-select"]').setValue("CAROL3");
    await wrapper.find('[data-testid="new-member-role"]').setValue("reader");
    await wrapper.find('[data-testid="add-member"]').trigger("submit");
    await flushPromises();

    expect(api.shareVault).toHaveBeenCalledWith("family", "carol@example.com", "reader");
    expect(api.listAllVaultMember).toHaveBeenCalledTimes(2);
    expect(store.noticeMessage).toBe("Shared with Carol");
    wrapper.unmount();
  });

  it("changes a member's role", async () => {
    api.changeVaultMemberRole.mockResolvedValue(undefined);
    const wrapper = await openSharingPanel();
    await wrapper.find('[data-testid="role-select-BOB222"]').setValue("reader");
    await flushPromises();
    expect(api.changeVaultMemberRole).toHaveBeenCalledWith("family", "BOB222", "reader");
    expect(store.noticeMessage).toBe("Bob is now a reader");
    wrapper.unmount();
  });

  it("asks for confirmation before removing a member", async () => {
    api.removeVaultMember.mockResolvedValue(undefined);
    api.listAllItem.mockResolvedValue([createItem({ revision: "rotated" })]);
    const wrapper = await openSharingPanel();
    await wrapper.find('[data-testid="remove-member-BOB222"]').trigger("click");
    const confirmation = wrapper.find('[data-testid="remove-confirmation"]');
    expect(confirmation.text()).toContain("Removing Bob re-encrypts this vault with a new key");
    expect(api.removeVaultMember).not.toHaveBeenCalled();

    await confirmation.find("button.ghost").trigger("click");
    expect(wrapper.find('[data-testid="remove-confirmation"]').exists()).toBe(false);

    await wrapper.find('[data-testid="remove-member-BOB222"]').trigger("click");
    await wrapper.find('[data-testid="confirm-remove-member"]').trigger("click");
    await flushPromises();
    expect(api.removeVaultMember).toHaveBeenCalledWith("family", "BOB222");
    expect(api.listAllItem).toHaveBeenCalledWith("family");
    expect(store.allItem[0].revision).toBe("rotated");
    expect(wrapper.find('[data-testid="remove-confirmation"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("keeps the confirmation and shows the error when removing fails", async () => {
    api.removeVaultMember.mockRejectedValue(new Error("the signing key of bob@example.com changed since you last shared with them; stopping"));
    const wrapper = await openSharingPanel();
    await wrapper.find('[data-testid="remove-member-BOB222"]').trigger("click");
    await wrapper.find('[data-testid="confirm-remove-member"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-sharing-panel"] [role="alert"]').text()).toContain("signing key of bob@example.com changed");
    expect(wrapper.find('[data-testid="remove-confirmation"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("shows the security code when a member row is expanded", async () => {
    const wrapper = await openSharingPanel();
    expect(wrapper.find('[data-testid="security-code-detail"]').exists()).toBe(false);
    const toggle = wrapper.find('[data-testid="security-code-toggle-BOB222"]');
    await toggle.trigger("click");
    const detail = wrapper.find('[data-testid="security-code-detail"]');
    expect(detail.find('[data-testid="security-code"]').text()).toBe("C0FF EE12 3456 789A");
    expect(detail.text()).toContain("Compare this code with Bob in person to be sure the server isn't intercepting");
    expect(toggle.attributes("aria-expanded")).toBe("true");
    await toggle.trigger("click");
    expect(wrapper.find('[data-testid="security-code-detail"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("is read-only for members who are not owners", async () => {
    resetStore("reader");
    mockMembers("reader");
    const wrapper = await openSharingPanel();
    expect(wrapper.text()).toContain("Bob Stone");
    expect(wrapper.find('[data-testid="vault-sharing-panel"] select').exists()).toBe(false);
    expect(wrapper.find('[data-testid="remove-member-BOB222"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="add-member"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="sharing-read-only"]').text()).toContain("You are a reader in this vault. Only its owners can change who has access.");
    await wrapper.find('[data-testid="security-code-toggle-BOB222"]').trigger("click");
    expect(wrapper.find('[data-testid="security-code"]').text()).toBe("C0FF EE12 3456 789A");
    wrapper.unmount();
  });

  it("explains that personal vaults are private instead of offering to share", async () => {
    store.selectedVaultID = "personal";
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="share-vault"]').exists()).toBe(false);
    const privateButton = wrapper.find('[data-testid="personal-vault-private"]');
    expect(privateButton.attributes("title")).toContain("Personal vaults are private");
    await privateButton.trigger("click");
    expect(store.noticeMessage).toContain("Personal vaults are private");
    expect(store.isSharingOpen).toBe(false);
    wrapper.unmount();
  });

  it("closes with Escape", async () => {
    const wrapper = await openSharingPanel();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(store.isSharingOpen).toBe(false);
    expect(wrapper.find('[data-testid="vault-sharing-panel"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Settings family and devices", () => {
  beforeEach(() => {
    api.listAllDevice.mockResolvedValue([
      createDevice(),
      createDevice({ id: "linux", name: "build-box", platform: "linux/cli", isCurrent: false, lastSeenAt: new Date(Date.now() - 3 * 3600_000).toISOString() }),
    ]);
  });

  it("lists family members with role badges and security codes", async () => {
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    const family = wrapper.find('[data-testid="family-section"]');
    expect(family.find('[data-testid="family-role-ALICE1"]').text()).toBe("Admin");
    expect(family.find('[data-testid="family-role-BOB222"]').text()).toBe("Member");
    const bob = family.find('[data-testid="family-member-BOB222"]');
    expect(bob.text()).toContain("bob@example.com");
    expect(bob.find('[data-testid="family-security-code"]').text()).toBe("C0FF EE12 3456 789A");
    expect(family.find('[data-testid="family-member-ALICE1"]').text()).toContain("you");
    wrapper.unmount();
  });

  it("lets admins create an invite link and copy it", async () => {
    api.inviteMember.mockResolvedValue("https://hisar.test/invite/abc123");
    api.copyText.mockResolvedValue(undefined);
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    await wrapper.find('[data-testid="start-invite"]').trigger("click");
    await wrapper.find('input[name="inviteEmail"]').setValue("dan@example.com");
    await wrapper.find('[data-testid="invite-role"]').setValue("admin");
    await wrapper.find('[data-testid="invite-form"]').trigger("submit");
    await flushPromises();

    expect(api.inviteMember).toHaveBeenCalledWith("dan@example.com", "admin");
    const result = wrapper.find('[data-testid="invite-result"]');
    expect(result.find('[data-testid="invite-link"]').text()).toBe("https://hisar.test/invite/abc123");
    expect(result.text()).toContain("valid for 7 days");
    expect(result.text()).toContain("dan@example.com");
    expect(wrapper.find('[data-testid="invite-form"]').exists()).toBe(false);

    await result.find('[data-testid="copy-invite-link"]').trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith("https://hisar.test/invite/abc123");
    wrapper.unmount();
  });

  it("hides inviting from members who are not admins", async () => {
    api.listAllMember.mockResolvedValue([createFamilyMember({ role: "member" }), createFamilyMember({ ...testBob, role: "admin" })]);
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    expect(wrapper.find('[data-testid="family-member-BOB222"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="start-invite"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("lists devices and revokes another device after confirmation", async () => {
    api.revokeDevice.mockResolvedValue(undefined);
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    const devices = wrapper.find('[data-testid="devices-section"]');
    const current = devices.find('[data-testid="device-device-1"]');
    expect(current.text()).toMatch(/This (Mac|device)/);
    expect(current.text()).toContain("macOS app · Active now");
    expect(current.find("button").exists()).toBe(false);
    expect(devices.find('[data-testid="device-linux"]').text()).toContain("Linux command line · Last seen 3 hours ago");

    await devices.find('[data-testid="revoke-device-linux"]').trigger("click");
    expect(api.revokeDevice).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("build-box will be signed out and needs the master password and Secret Key to sign in again.");
    api.listAllDevice.mockResolvedValue([createDevice()]);
    await wrapper.find('[data-testid="confirm-revoke-device"]').trigger("click");
    await flushPromises();
    expect(api.revokeDevice).toHaveBeenCalledWith("linux");
    expect(wrapper.find('[data-testid="device-linux"]').exists()).toBe(false);
    expect(store.noticeMessage).toBe("build-box was signed out");
    wrapper.unmount();
  });
});
