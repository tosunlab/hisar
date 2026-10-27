import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
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
const UnlockView = (await import("./UnlockView.vue")).default;
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
  it("contains a nested recipient review and returns to the still-open sharing panel", async () => {
    api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [createSentShareInvitation({ id: "reviewed", requiresConfirmation: true, state: "accepted" })], completionError: "" });
    const wrapper = await openSharingPanel();
    const review = wrapper.find('[data-testid="review-share-reviewed"]');
    (review.element as HTMLElement).focus();
    await review.trigger("click");
    await flushPromises();
    const confirmation = wrapper.find('[data-testid="share-recipient-confirmation"]');
    const all = confirmation.findAll("button").map((button) => button.element);
    expect(all.map((button) => button.textContent)).toEqual(["Cancel", "Give access"]);
    expect(document.activeElement).toBe(all[0]);
    for (const shiftKey of [false, false, true, true]) {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", shiftKey, cancelable: true }));
      expect(all).toContain(document.activeElement);
    }
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(wrapper.find('[data-testid="share-recipient-confirmation"]').exists()).toBe(false);
    expect(store.isSharingOpen).toBe(true);
    expect(document.activeElement).toBe(review.element);
    wrapper.unmount();
  });

  it("lists the members with initials, role and a you tag", async () => {
    const wrapper = await openSharingPanel();
    expect(api.listAllVaultMember).toHaveBeenCalledWith("family");
    const panel = wrapper.find('[data-testid="vault-sharing-panel"]');
    expect(panel.text()).toContain("Share “Family”");
    expect(panel.find('[data-testid="sharing-member-count"]').text()).toBe("2 members");

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
    for (const role of panel.findAll("select.role-select")) {
      expect(role.attributes("title")).toBe("Readers view items, editors also change them, owners also share, rename and delete the vault.");
    }
    expect(panel.text()).not.toContain("Readers view items");
    expect(panel.text()).not.toContain("Editors can add and change items");
    store.allVaultMember = [store.allVaultMember[0]!];
    await flushPromises();
    expect(panel.find('[data-testid="sharing-member-count"]').text()).toBe("1 member");
    wrapper.unmount();
  });

  it("invites someone to the vault by email without listing other accounts", async () => {
    api.shareVault.mockResolvedValue(undefined);
    const wrapper = await openSharingPanel();
    expect(wrapper.find('[data-testid="new-member-select"]').exists()).toBe(false);
    expect(api.listAllPerson).not.toHaveBeenCalled();
    const sendButton = wrapper.find('[data-testid="send-share-invitation"]');
    expect(sendButton.attributes("disabled")).toBeDefined();
    expect(sendButton.text()).toBe("Share");
    expect(wrapper.find('[data-testid="add-member"]').text()).toContain("Invite by email");
    expect(wrapper.find('[data-testid="add-member"] .hint').text()).toBe("They get an email and accept in Hisar; without an account they create one first.");

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
    const privateBadge = wrapper.find('[data-testid="personal-vault-private"]');
    expect(privateBadge.element.tagName).toBe("SPAN");
    expect(privateBadge.attributes("title")).toBe("Personal vaults are private. Create a shared vault to share items with other people.");
    expect(wrapper.text()).not.toMatch(/recover/i);
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

describe("Inbox", () => {
  async function openInbox(wrapper: VueWrapper): Promise<void> {
    await wrapper.find('[data-testid="open-inbox"]').trigger("click");
    await flushPromises();
  }

  it("reviews the concrete recipient from the Inbox and sends only that displayed authorization", async () => {
    const invitation = createSentShareInvitation({ id: "new-recipient", state: "accepted", requiresConfirmation: true, recipientId: "NEW123", securityCode: "1111 2222 3333 4444", reviewToken: "exact-reviewed-context", email: "new@example.com", role: "reader" });
    store.shareInvitations = { received: [], sent: [invitation], completionError: "" };
    api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [invitation], completionError: "" });
    api.confirmShareRecipient.mockResolvedValue(undefined);
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="inbox-count"]').text()).toBe("1");
    expect(wrapper.find('[data-testid="review-share-new-recipient"]').exists()).toBe(false);
    await openInbox(wrapper);
    expect(wrapper.find('[data-testid="settings-page-people"]').isVisible()).toBe(true);
    expect(wrapper.find('[data-testid="people-invitation-badge"]').text()).toBe("1");
    expect(api.confirmShareRecipient).not.toHaveBeenCalled();
    await wrapper.find('[data-testid="review-share-new-recipient"]').trigger("click");
    const dialog = wrapper.find('[data-testid="share-recipient-confirmation"]');
    expect(dialog.text()).toContain("new@example.com");
    expect(dialog.text()).toContain("Family");
    expect(dialog.text()).toContain("Reader");
    expect(dialog.text()).toContain("NEW123");
    expect(dialog.text()).toContain("1111 2222 3333 4444");
    api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
    await wrapper.find('[data-testid="confirm-share-recipient"]').trigger("click");
    await flushPromises();
    expect(api.confirmShareRecipient).toHaveBeenCalledWith("new-recipient", "exact-reviewed-context");
    expect(wrapper.find('[data-testid="people-invitation-badge"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="open-inbox"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("counts received invitations on the Inbox, not above the vaults, and answers them in Settings → People", async () => {
    const allReceived = [createReceivedShareInvitation(), createReceivedShareInvitation({ id: "from-dave", invitedByName: "", invitedByEmail: "dave@example.com", role: "reader" })];
    store.shareInvitations = { received: allReceived, sent: [], completionError: "" };
    api.listAllShareInvitation.mockResolvedValueOnce({ received: allReceived, sent: [], completionError: "" }).mockResolvedValue({ received: [], sent: [], completionError: "" });
    api.acceptShareInvitation.mockResolvedValue(undefined);
    api.declineShareInvitation.mockResolvedValue(undefined);
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find(".sidebar").text()).not.toContain("shared a vault");
    expect(wrapper.find('[data-testid="inbox-count"]').text()).toBe("2");
    expect(wrapper.find('[data-testid="inbox-count"]').attributes("aria-label")).toBe("2 waiting for you");
    await openInbox(wrapper);
    expect(wrapper.find('[data-testid="received-invitations"]').text()).toContain("dave@example.com shared a vault with you");

    await wrapper.find('[data-testid="decline-invitation-from-dave"]').trigger("click");
    await flushPromises();
    expect(api.declineShareInvitation).toHaveBeenCalledWith("from-dave");
    store.shareInvitations = { received: [createReceivedShareInvitation()], sent: [], completionError: "" };
    await flushPromises();
    expect(wrapper.find('[data-testid="inbox-count"]').text()).toBe("1");
    await wrapper.find('[data-testid="accept-invitation-invitation-from-carol"]').trigger("click");
    await flushPromises();
    expect(api.acceptShareInvitation).toHaveBeenCalledWith("invitation-from-carol");
    expect(wrapper.find('[data-testid="open-inbox"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("is hidden when nothing is in it", () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="open-inbox"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("tells the recipient whose Hisar the share is waiting for and since when", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-11T10:00:00Z"));
    const shareInvitations = {
      received: [],
      sent: [],
      waiting: [
        { id: "from-carol", vaultId: "carol-vault", role: "editor", invitedByName: "Carol", invitedByEmail: "carol@example.com", acceptedAt: "2026-10-11T08:00:00Z" },
        { id: "from-dave", vaultId: "dave-vault", role: "reader", invitedByName: "", invitedByEmail: "dave@example.com", acceptedAt: "2026-10-11T09:55:00Z" },
      ],
      completionError: "",
    };
    store.shareInvitations = structuredClone(shareInvitations);
    api.listAllShareInvitation.mockResolvedValue(structuredClone(shareInvitations));
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="inbox-count"]').exists()).toBe(false);
    await openInbox(wrapper);
    expect(wrapper.find('[data-testid="waiting-invitation-from-carol"] .title-text').text()).toBe("Waiting for Carol's Hisar to finish sharing");
    expect(wrapper.find('[data-testid="waiting-invitation-from-carol"]').text()).toContain("Accepted 2 h ago");
    expect(wrapper.find('[data-testid="waiting-invitation-from-dave"]').text()).toContain("Waiting for dave@example.com's Hisar to finish sharing");
    expect(wrapper.find('[data-testid="waiting-invitation-from-dave"]').text()).toContain("Accepted 5 min ago");
    expect(wrapper.find('[data-testid="waiting-invitation-from-dave"] button').exists()).toBe(false);
    wrapper.unmount();
    vi.useRealTimers();
  });

  it("tells the owner when an accepted share could not be finished", async () => {
    const shareInvitations = {
      received: [],
      sent: [
        createSentShareInvitation({ id: "to-erin", email: "erin@example.com", recipientName: "Erin", state: "accepted", vaultName: "Work", acceptedAt: "2026-10-11T08:00:00Z" }),
        createSentShareInvitation({ id: "to-frank", email: "frank@example.com", recipientName: "Frank", state: "accepted", vaultName: "Family", acceptedAt: "2026-10-11T08:00:00Z" }),
        createSentShareInvitation(),
      ],
      completionError: "the signing key of erin@example.com changed since you last shared with them",
    };
    store.shareInvitations = structuredClone(shareInvitations);
    api.listAllShareInvitation.mockResolvedValue(structuredClone(shareInvitations));
    const wrapper = mount(MainView, { attachTo: document.body });
    await openInbox(wrapper);
    expect(wrapper.find('[data-testid="share-completion-error"]').text()).toBe(
      "Could not finish sharing: the signing key of erin@example.com changed since you last shared with them",
    );
    expect(wrapper.find('[data-testid="sent-invitation-to-frank"]').text()).toContain("Family · Accepted · Hisar is finishing sharing");
    wrapper.unmount();
  });
});

describe("Lock screen", () => {
  it("asks the owner to unlock to finish shares people accepted", async () => {
    store.status = createStatus("locked");
    api.listAllShareAwaitingUnlock.mockResolvedValue([
      { invitationId: "to-ada", recipientName: "Ada", recipientEmail: "ada@example.com", acceptedAt: "2026-10-11T08:00:00Z" },
      { invitationId: "to-bo", recipientName: "", recipientEmail: "bo@example.com", acceptedAt: "2026-10-11T08:00:00Z" },
    ]);
    const wrapper = mount(UnlockView, { attachTo: document.body });
    await flushPromises();
    const banner = wrapper.find('[data-testid="share-awaiting-unlock"]');
    expect(banner.findAll("li").map((row) => row.text())).toEqual([
      "Ada accepted your invitation to a vault — unlock to finish sharing",
      "bo@example.com accepted your invitation to a vault — unlock to finish sharing",
    ]);

    api.listAllShareAwaitingUnlock.mockResolvedValue([]);
    window.dispatchEvent(new Event("focus"));
    await flushPromises();
    expect(wrapper.find('[data-testid="share-awaiting-unlock"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows nothing when the server cannot be reached", async () => {
    store.status = createStatus("locked");
    store.allShareAwaitingUnlock = [];
    api.listAllShareAwaitingUnlock.mockRejectedValue(new Error("offline"));
    const wrapper = mount(UnlockView, { attachTo: document.body });
    await flushPromises();
    expect(wrapper.find('[data-testid="share-awaiting-unlock"]').exists()).toBe(false);
    expect(store.errorMessage).toBe("");
    wrapper.unmount();
  });

  it("forgets the banner when the lock screen goes away", async () => {
    store.status = createStatus("locked");
    api.listAllShareAwaitingUnlock.mockResolvedValue([{ invitationId: "to-ada", recipientName: "Ada", recipientEmail: "ada@example.com", acceptedAt: "" }]);
    const wrapper = mount(UnlockView, { attachTo: document.body });
    await flushPromises();
    expect(store.allShareAwaitingUnlock).toHaveLength(1);
    api.unlock.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(store.status.state).toBe("unlocked");
    wrapper.unmount();
    expect(store.allShareAwaitingUnlock).toHaveLength(0);
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
    await wrapper.find('[data-testid="settings-section-people"]').trigger("click");
    expect(wrapper.find("#settings-title").text()).toBe("People");
    expect(people.text()).not.toMatch(/admin/i);
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
    expect(wrapper.find('[data-testid="people-empty-hint"]').text()).toBe("Nobody yet. Share a vault to see the people you share with here.");
    expect(wrapper.find('[data-testid="sent-invitations"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("emails an invite for anyone, without showing a link", async () => {
    api.listAllPerson.mockResolvedValue([createPerson({ role: "member" })]);
    api.invitePerson.mockResolvedValue(undefined);
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    await wrapper.find('[data-testid="settings-section-people"]').trigger("click");
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
    expect(sent.find('[data-testid="sent-invitation-invitation-to-erin"]').text()).toContain("Accepted · Hisar is finishing sharing");
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
