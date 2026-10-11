import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import type { VaultSummary } from "../api";
import {
  createApiMock,
  createDevice,
  createItem,
  createPerson,
  createReceivedShareInvitation,
  createSentShareInvitation,
  createStatus,
  createVaultMember,
  testBob,
} from "../testing";

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
    { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0, recoverable: true, recoveryOptOut: false },
    { id: "family", name: "Family", type: "shared", role: familyRole, itemCount: 1, recoverable: true, recoveryOptOut: false },
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
    allPerson: [],
    shareInvitations: { received: [], sent: [], completionError: "" },
    allDevice: [],
  });
}

function mockMembers(familyRole = "owner"): void {
  api.listAllVaultMember.mockResolvedValue([
    createVaultMember({ role: familyRole }),
    createVaultMember({ ...testBob, role: "editor" }),
  ]);
  api.listAllPerson.mockResolvedValue([createPerson(), createPerson({ ...testBob, role: "member", isCurrentUser: false })]);
  api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [createSentShareInvitation()], completionError: "" });
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

  it("invites someone to the vault by email without listing other accounts", async () => {
    api.shareVault.mockResolvedValue(undefined);
    const wrapper = await openSharingPanel();
    expect(wrapper.find('[data-testid="new-member-select"]').exists()).toBe(false);
    expect(api.listAllPerson).not.toHaveBeenCalled();
    const sendButton = wrapper.find('[data-testid="send-share-invitation"]');
    expect(sendButton.attributes("disabled")).toBeDefined();

    await wrapper.find('[data-testid="new-member-email"]').setValue("carol@example.com");
    await wrapper.find('[data-testid="new-member-role"]').setValue("reader");
    await wrapper.find('[data-testid="add-member"]').trigger("submit");
    await flushPromises();

    expect(api.shareVault).toHaveBeenCalledWith("family", "carol@example.com", "reader");
    expect(api.listAllVaultMember).toHaveBeenCalledTimes(2);
    expect(store.noticeMessage).toBe("Invitation sent to carol@example.com");
    expect((wrapper.find('[data-testid="new-member-email"]').element as HTMLInputElement).value).toBe("");
    wrapper.unmount();
  });

  it("lists the vault's pending invitations and cancels one after confirmation", async () => {
    api.cancelShareInvitation.mockResolvedValue(undefined);
    const wrapper = await openSharingPanel();
    const invitation = wrapper.find('[data-testid="vault-invitation-invitation-to-dave"]');
    expect(invitation.text()).toContain("dave@example.com");
    expect(invitation.text()).toContain("Waiting for them to accept");
    expect(invitation.text()).toContain("Reader");

    await invitation.find('[data-testid="cancel-vault-invitation-invitation-to-dave"]').trigger("click");
    expect(api.cancelShareInvitation).not.toHaveBeenCalled();
    api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
    await wrapper.find('[data-testid="confirm-cancel-vault-invitation"]').trigger("click");
    await flushPromises();
    expect(api.cancelShareInvitation).toHaveBeenCalledWith("invitation-to-dave");
    expect(wrapper.find('[data-testid="vault-invitations"]').exists()).toBe(false);
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
    expect(detail.text()).toContain("Compare this code with Bob in person or over a channel you trust");
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
    expect(wrapper.find('[data-testid="vault-invitations"]').exists()).toBe(false);
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
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-recovery-panel"]').text()).toContain("“Personal” is private");
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

describe("Share invitation banner", () => {
  it("shows received invitations above the vaults and accepts or declines them", async () => {
    store.shareInvitations = { received: [createReceivedShareInvitation(), createReceivedShareInvitation({ id: "from-dave", invitedByName: "", invitedByEmail: "dave@example.com", role: "reader" })], sent: [], completionError: "" };
    api.acceptShareInvitation.mockResolvedValue(undefined);
    api.declineShareInvitation.mockResolvedValue(undefined);
    api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
    const wrapper = mount(MainView, { attachTo: document.body });
    const banner = wrapper.find('[data-testid="share-invitation-banner"]');
    expect(banner.text()).toContain("Carol shared a vault with you as editor");
    expect(banner.text()).toContain("dave@example.com shared a vault with you as reader");

    await wrapper.find('[data-testid="banner-decline-from-dave"]').trigger("click");
    await flushPromises();
    expect(api.declineShareInvitation).toHaveBeenCalledWith("from-dave");
    store.shareInvitations = { received: [createReceivedShareInvitation()], sent: [], completionError: "" };
    await flushPromises();
    await wrapper.find('[data-testid="banner-accept-invitation-from-carol"]').trigger("click");
    await flushPromises();
    expect(api.acceptShareInvitation).toHaveBeenCalledWith("invitation-from-carol");
    expect(wrapper.find('[data-testid="share-invitation-banner"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("is hidden without invitations", () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="share-invitation-banner"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Settings people and devices", () => {
  beforeEach(() => {
    api.listAllDevice.mockResolvedValue([
      createDevice(),
      createDevice({ id: "linux", name: "build-box", platform: "linux/cli", isCurrent: false, lastSeenAt: new Date(Date.now() - 3 * 3600_000).toISOString() }),
    ]);
  });

  it("lists the people you share with and their security codes", async () => {
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    const people = wrapper.find('[data-testid="people-section"]');
    expect(people.find('.section-label').text()).toBe("People");
    expect(people.find('[data-testid="person-role-ALICE1"]').text()).toBe("Admin");
    expect(people.find('[data-testid="person-role-BOB222"]').exists()).toBe(false);
    const bob = people.find('[data-testid="person-BOB222"]');
    expect(bob.text()).toContain("bob@example.com");
    expect(bob.find('[data-testid="person-security-code"]').text()).toBe("C0FF EE12 3456 789A");
    expect(people.find('[data-testid="person-ALICE1"]').text()).toContain("you");
    expect(people.find('[data-testid="people-empty-hint"]').exists()).toBe(false);
    expect(wrapper.text()).not.toMatch(/family member|in your family/i);
    wrapper.unmount();
  });

  it("explains how people appear when you share with nobody yet", async () => {
    api.listAllPerson.mockResolvedValue([createPerson()]);
    api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    expect(wrapper.find('[data-testid="people-empty-hint"]').text()).toContain("People appear here once you share a vault with them");
    expect(wrapper.find('[data-testid="sent-invitations"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("emails an invite for anyone, without showing a link", async () => {
    api.listAllPerson.mockResolvedValue([createPerson({ role: "member" })]);
    api.invitePerson.mockResolvedValue(undefined);
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    await wrapper.find('[data-testid="start-invite"]').trigger("click");
    expect(wrapper.find('[data-testid="invite-role"]').exists()).toBe(false);
    await wrapper.find('input[name="inviteEmail"]').setValue("dan@example.com");
    await wrapper.find('[data-testid="invite-form"]').trigger("submit");
    await flushPromises();

    expect(api.invitePerson).toHaveBeenCalledWith("dan@example.com");
    expect(store.noticeMessage).toBe("Invitation sent to dan@example.com");
    expect(wrapper.find('[data-testid="invite-form"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("/invite/");
    wrapper.unmount();
  });

  it("shows invitations you received and sent, and answers them", async () => {
    const received = createReceivedShareInvitation();
    const accepted = createSentShareInvitation({ id: "invitation-to-erin", email: "erin@example.com", state: "accepted", role: "editor" });
    api.listAllShareInvitation.mockResolvedValue({ received: [received], sent: [createSentShareInvitation(), accepted], completionError: "the signing key of erin@example.com changed" });
    api.acceptShareInvitation.mockResolvedValue(undefined);
    api.cancelShareInvitation.mockResolvedValue(undefined);
    const wrapper = mount(SettingsPanel);
    await flushPromises();

    const receivedRow = wrapper.find('[data-testid="received-invitation-invitation-from-carol"]');
    expect(receivedRow.text()).toContain("Carol shared a vault with you");
    expect(receivedRow.text()).toContain("Editor");
    const sent = wrapper.find('[data-testid="sent-invitations"]');
    expect(sent.find('[data-testid="sent-invitation-invitation-to-dave"]').text()).toContain("Family · Waiting for them to accept");
    expect(sent.find('[data-testid="sent-invitation-invitation-to-erin"]').text()).toContain("Accepted · finishing on your next sync");
    expect(wrapper.find('[data-testid="share-completion-error"]').text()).toContain("signing key of erin@example.com changed");

    await receivedRow.find('[data-testid="accept-invitation-invitation-from-carol"]').trigger("click");
    await flushPromises();
    expect(api.acceptShareInvitation).toHaveBeenCalledWith("invitation-from-carol");

    await wrapper.find('[data-testid="cancel-invitation-invitation-to-dave"]').trigger("click");
    await wrapper.find('[data-testid="confirm-cancel-invitation"]').trigger("click");
    await flushPromises();
    expect(api.cancelShareInvitation).toHaveBeenCalledWith("invitation-to-dave");
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
