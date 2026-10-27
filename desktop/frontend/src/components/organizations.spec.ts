import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { nextTick } from "vue";
import type { Organization, Organizations, VaultSummary, WaitingVaultKey } from "../api";
import {
  createApiMock,
  createItem,
  createOrganizationDetail,
  createOrganizationInvitation,
  createOrganizationMember,
  createOrganizationTeam,
  createStatus,
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

const { store, syncInBackground } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const DeleteAccountDialog = (await import("./DeleteAccountDialog.vue")).default;
const api = apiMock.current;

const ownerTakeBackRefusal = "Nobody who can open this vault would be its owner. Give owner access to a team whose members already have the vault first.";

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 };
const familyVault: VaultSummary = { id: "family", name: "Family", type: "shared", role: "owner", itemCount: 0 };
const deployVault: VaultSummary = { id: "deploy", name: "Deploy", type: "organization", role: "owner", itemCount: 2, organizationId: "acme" };
const docsVault: VaultSummary = { id: "docs", name: "Docs", type: "organization", role: "reader", itemCount: 1, organizationId: "acme" };
const labVault: VaultSummary = { id: "lab", name: "Lab", type: "organization", role: "editor", itemCount: 0, organizationId: "globex" };

const acmeSummary = { id: "acme", name: "Acme", role: "admin", memberCount: 2 };
const globexSummary = { id: "globex", name: "Globex", role: "member", memberCount: 3 };

function listOrganizations(overrides: Partial<Organizations> = {}): Organizations {
  return { organizations: [acmeSummary, globexSummary], invitations: [], ...overrides };
}

// globexDetail is Globex as its member Alice sees it: she is in Research, which has no vaults yet.
const globexDetail: Organization = createOrganizationDetail({
  id: "globex",
  name: "Globex",
  role: "member",
  isAdmin: false,
  teams: [createOrganizationTeam({ id: "research", name: "Research", vaults: [] })],
});

function getOrganizationDetail(organizationID: string, acme: Organization = createOrganizationDetail()): Organization {
  return organizationID === "acme" ? acme : globexDetail;
}

async function openNewVaultForm(): Promise<VueWrapper> {
  const wrapper = mount(MainView, { attachTo: document.body });
  await wrapper.find('[data-testid="new-vault"]').trigger("click");
  await flushPromises();
  return wrapper;
}

function listAllWhereChoice(wrapper: VueWrapper): string[] {
  return wrapper.findAll('[data-testid="new-vault-where"] option').map((option) => option.text());
}

function isCreateDisabled(wrapper: VueWrapper): boolean {
  return wrapper.find('[data-testid="create-vault"]').attributes("disabled") !== undefined;
}

function resetStore(allVault: VaultSummary[] = [personalVault, familyVault, deployVault, docsVault, labVault], selectedVaultID = "personal"): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault,
    selectedVaultID,
    listVaultID: selectedVaultID,
    allItem: [createItem()],
    selectedItem: null,
    allFavorite: [],
    isFavoritesSelected: false,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    isSharingOpen: false,
    allVaultMember: [],
    shareInvitations: { received: [], sent: [], completionError: "" },
    organizations: listOrganizations(),
    organizationByID: {},
    selectedOrganizationID: "",
    organizationsErrorMessage: "",
    waitingVaultKeys: { waiting: [], deliveryError: "", rotationError: "" },
    organizationAlert: null,
  });
}

async function openOrganizationInSettings(): Promise<VueWrapper> {
  const settings = mount(SettingsPanel, { attachTo: document.body });
  await flushPromises();
  await settings.find('[data-testid="settings-section-organizations"]').trigger("click");
  await settings.find('[data-testid="organization-acme"]').trigger("click");
  await flushPromises();
  return settings;
}

async function openSharingPanel(): Promise<VueWrapper> {
  const wrapper = mount(MainView, { attachTo: document.body });
  await wrapper.find('[data-testid="share-vault"]').trigger("click");
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
  localStorage.clear();
  document.body.innerHTML = "";
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue(store.allVault);
  api.listAllItem.mockResolvedValue([createItem()]);
  api.listAllOrganization.mockResolvedValue(listOrganizations());
  api.getOrganization.mockResolvedValue(createOrganizationDetail());
  api.listAllWaitingVaultKey.mockResolvedValue({ waiting: [], deliveryError: "", rotationError: "" });
  api.takeVaultFromTeam.mockResolvedValue(undefined);
});

describe("Organization invitations", () => {
  it("counts an invitation on the Inbox and accepts it in Settings → Organizations", async () => {
    const invitation = createOrganizationInvitation();
    store.organizations = listOrganizations({ invitations: [invitation] });
    api.listAllOrganization.mockResolvedValueOnce(listOrganizations({ invitations: [invitation] }));
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find(".sidebar").text()).not.toContain("invited you");
    expect(wrapper.find('[data-testid="inbox-count"]').text()).toBe("1");
    await wrapper.find('[data-testid="open-inbox"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="settings-page-organizations"]').isVisible()).toBe(true);
    expect(wrapper.find('[data-testid="organization-invitation-invitation-to-globex"]').text()).toContain("Ada invited you to Globex as a member");

    await wrapper.find('[data-testid="accept-organization-invitation-invitation-to-globex"]').trigger("click");
    await flushPromises();

    expect(api.acceptOrganizationInvitation).toHaveBeenCalledWith("invitation-to-globex");
    expect(wrapper.find('[data-testid="open-inbox"]').exists()).toBe(false);
    expect(store.noticeMessage).toContain("You joined Globex");
    wrapper.unmount();
  });

  it("counts invitations on the Organizations section and accepts one there", async () => {
    store.organizations = listOrganizations({ organizations: [acmeSummary] });
    api.listAllOrganization.mockResolvedValue(listOrganizations({ organizations: [acmeSummary], invitations: [createOrganizationInvitation()] }));
    const settings = mount(SettingsPanel, { attachTo: document.body });
    await flushPromises();
    expect(settings.find('[data-testid="organizations-invitation-badge"]').text()).toBe("1");
    await settings.find('[data-testid="settings-section-organizations"]').trigger("click");
    const invitationRow = settings.find('[data-testid="organization-invitation-invitation-to-globex"]');
    expect(invitationRow.text()).toContain("ada@example.com");

    api.listAllOrganization.mockResolvedValue(listOrganizations());
    await invitationRow.find('[data-testid="accept-organization-invitation-invitation-to-globex"]').trigger("click");
    await flushPromises();

    expect(api.acceptOrganizationInvitation).toHaveBeenCalledWith("invitation-to-globex");
    expect(settings.find('[data-testid="organizations-invitation-badge"]').exists()).toBe(false);
    expect(settings.find('[data-testid="organization-globex"]').text()).toContain("Member · 3 members");
    settings.unmount();
  });
});

describe("Organization screen", () => {
  it("expands security codes from member menus and separates members, teams and invitations", async () => {
    api.getOrganization.mockResolvedValue(
      createOrganizationDetail({
        members: [createOrganizationMember(), createOrganizationMember({ ...testBob, role: "member", isKeyChanged: true })],
        teams: [createOrganizationTeam({ vaults: [{ vaultId: "deploy", name: "Deploy", role: "owner", canOpen: true }, { vaultId: "secret", name: "", role: "reader", canOpen: false }] })],
      }),
    );
    const settings = await openOrganizationInSettings();
    expect(api.getOrganization).toHaveBeenCalledWith("acme");
    const bob = settings.find('[data-testid="organization-member-BOB222"]');
    expect(settings.find('[data-testid="organization-segment-members"]').attributes("aria-selected")).toBe("true");
    expect(settings.find('[data-testid="organization-teams"]').isVisible()).toBe(false);
    expect(settings.find('[data-testid="organization-invite"]').isVisible()).toBe(false);
    expect(bob.find('[data-testid="organization-member-security-code"]').exists()).toBe(false);
    await bob.find('[data-testid="organization-member-menu-BOB222"]').trigger("click");
    await settings.find('[data-testid="context-menu-organization-security-code"]').trigger("click");
    expect(bob.find('[data-testid="organization-member-security-code"]').text()).toBe(testBob.securityCode);
    expect(bob.find('[data-testid="organization-member-key-changed"]').exists()).toBe(true);
    await bob.find('[data-testid="organization-member-menu-BOB222"]').trigger("click");
    expect(settings.find('[data-testid="context-menu-organization-security-code"]').text()).toBe("Hide security code");
    await settings.find('[data-testid="context-menu-organization-security-code"]').trigger("click");
    expect(bob.find('[data-testid="organization-member-security-code"]').exists()).toBe(false);
    await settings.find('[data-testid="organization-segment-teams"]').trigger("click");
    expect(settings.find('[data-testid="organization-members"]').isVisible()).toBe(false);
    expect(settings.find('[data-testid="organization-teams"]').isVisible()).toBe(true);
    const allVaultName = settings.findAll('[data-testid="team-platform"] [data-testid="team-vault-name"]').map((name) => name.text());
    expect(allVaultName).toEqual(["Deploy", "A vault you can't open"]);
    expect(settings.find('[data-testid="team-vault-role-platform-deploy"]').attributes("title")).toBe("Readers view items, editors also change them, owners also share, rename and delete the vault.");
    await settings.find('[data-testid="organization-segment-invitations"]').trigger("click");
    expect(settings.find('[data-testid="organization-invite"]').isVisible()).toBe(true);
    expect(settings.find('[data-testid="organization-no-invitations"]').text()).toBe("No pending invitations.");
    settings.unmount();
  });

  it("gives admins the admin actions and hides them from members", async () => {
    const admin = await openOrganizationInSettings();
    for (const testID of ["rename-organization", "delete-organization"]) {
      expect(admin.find(`[data-testid="${testID}"]`).isVisible(), testID).toBe(true);
    }
    await admin.find('[data-testid="organization-member-menu-BOB222"]').trigger("click");
    expect(admin.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Show security code", "Make admin", "Remove from Acme…"]);
    await admin.find('[data-testid="context-menu-organization-role"]').trigger("click");
    await flushPromises();
    expect(api.setOrganizationMemberRole).toHaveBeenCalledWith("acme", "BOB222", "admin");
    await admin.find('[data-testid="organization-member-menu-ALICE1"]').trigger("click");
    expect(admin.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Show security code", "Make member"]);
    await admin.find('[data-testid="context-menu-organization-security-code"]').trigger("keydown", { key: "Escape" });
    expect(admin.find('[data-testid="context-menu"]').exists()).toBe(false);
    expect(document.activeElement).toBe(admin.find('[data-testid="organization-member-menu-ALICE1"]').element);
    await admin.find('[data-testid="organization-segment-teams"]').trigger("click");
    for (const testID of ["new-team", "remove-team-member-platform-BOB222", "take-back-platform-deploy"]) {
      expect(admin.find(`[data-testid="${testID}"]`).isVisible(), testID).toBe(true);
    }
    await admin.find('[data-testid="organization-team-menu-platform"]').trigger("click");
    expect(admin.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Rename…", "Give a vault…", "Delete team…"]);
    await admin.find('[data-testid="context-menu-rename-team"]').trigger("click");
    expect(admin.find('[data-testid="organization-dialog-rename-team"]').isVisible()).toBe(true);
    await admin.find('[data-testid="organization-dialog-rename-team"] button.ghost').trigger("click");
    await admin.find('[data-testid="organization-team-menu-platform"]').trigger("click");
    await admin.find('[data-testid="context-menu-delete-team"]').trigger("click");
    expect(admin.find('[data-testid="organization-dialog-delete-team"]').isVisible()).toBe(true);
    await admin.find('[data-testid="organization-dialog-delete-team"] button.ghost').trigger("click");
    await admin.find('[data-testid="organization-segment-invitations"]').trigger("click");
    expect(admin.find('[data-testid="organization-invite"]').isVisible()).toBe(true);
    admin.unmount();

    resetStore();
    api.getOrganization.mockResolvedValue(createOrganizationDetail({ role: "member", isAdmin: false, members: [createOrganizationMember({ role: "member" }), createOrganizationMember({ ...testBob, role: "admin" })] }));
    const member = await openOrganizationInSettings();
    expect(member.find('[data-testid="organization-name"]').text()).toBe("Acme");
    await member.find('[data-testid="organization-member-menu-BOB222"]').trigger("click");
    expect(member.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Show security code"]);
    await member.find('[data-testid="context-menu-organization-security-code"]').trigger("click");
    expect(member.find('[data-testid="organization-member-BOB222"] [data-testid="organization-member-security-code"]').text()).toBe(testBob.securityCode);
    expect(member.find('[data-testid="leave-organization"]').text()).toBe("Leave Acme…");
    await member.find('[data-testid="organization-segment-teams"]').trigger("click");
    for (const testID of ["rename-organization", "delete-organization", "new-team", "organization-team-menu-platform", "remove-team-member-platform-BOB222", "take-back-platform-deploy"]) {
      expect(member.find(`[data-testid="${testID}"]`).exists(), testID).toBe(false);
    }
    expect(member.find('[data-testid="organization-segment-invitations"]').exists()).toBe(false);
    expect(member.find('[data-testid="organization-invite"]').exists()).toBe(false);
    expect(member.find('[data-testid="organization-no-invitations"]').exists()).toBe(false);
    member.unmount();
  });

  it("keeps adding members and giving vaults reachable in a team's menu", async () => {
    api.getOrganization.mockResolvedValue(createOrganizationDetail({ teams: [createOrganizationTeam(), createOrganizationTeam({ id: "ops", name: "Ops", members: [], vaults: [] })] }));
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="organization-segment-teams"]').trigger("click");
    await settings.find('[data-testid="organization-team-menu-ops"]').trigger("click");
    expect(settings.findAll('[role="menuitem"]').map((action) => action.text())).toEqual(["Rename…", "Add member", "Give a vault…", "Delete team…"]);
    await settings.find('[data-testid="context-menu-add-team-member"]').trigger("click");
    await settings.find('[data-testid="team-add-member-select"]').setValue("BOB222");
    await settings.find('[data-testid="team-add-member-form"]').trigger("submit");
    await flushPromises();
    expect(api.addTeamMember).toHaveBeenCalledWith("acme", "ops", "BOB222");
    await settings.find('[data-testid="organization-team-menu-ops"]').trigger("click");
    await settings.find('[data-testid="context-menu-give-vault"]').trigger("click");
    expect(settings.find('[data-testid="organization-dialog-role"]').attributes("title")).toBe("Readers view items, editors also change them, owners also share, rename and delete the vault.");
    expect(settings.find('[data-testid="organization-dialog-give-vault"]').text()).not.toContain("Editors can add and change items");
    await settings.find('[data-testid="organization-dialog-role"]').setValue("reader");
    await settings.find('[data-testid="organization-dialog-give-vault"] form').trigger("submit");
    await flushPromises();
    expect(api.giveVaultToTeam).toHaveBeenCalledWith("acme", "ops", "deploy", "reader");
    settings.unmount();
  });

  it("supports keyboard navigation between organization segments", async () => {
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="organization-segment-members"]').trigger("keydown", { key: "ArrowRight" });
    expect(settings.find('[data-testid="organization-segment-teams"]').attributes("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(settings.find('[data-testid="organization-segment-teams"]').element);
    await settings.find('[data-testid="organization-segment-teams"]').trigger("keydown", { key: "End" });
    expect(settings.find('[data-testid="organization-segment-invitations"]').attributes("aria-selected")).toBe("true");
    await settings.find('[data-testid="organization-segment-invitations"]').trigger("keydown", { key: "Home" });
    expect(settings.find('[data-testid="organization-segment-members"]').attributes("aria-selected")).toBe("true");
    await settings.find('[data-testid="organization-segment-members"]').trigger("keydown", { key: "ArrowLeft" });
    expect(settings.find('[data-testid="organization-segment-invitations"]').attributes("aria-selected")).toBe("true");
    settings.unmount();
  });

  it("only offers Members and Teams to a non-admin, including keyboard navigation", async () => {
    api.getOrganization.mockResolvedValue(createOrganizationDetail({ role: "member", isAdmin: false }));
    const settings = await openOrganizationInSettings();
    expect(settings.find('[data-testid="organization-segment-invitations"]').exists()).toBe(false);
    expect(settings.find('[data-testid="organization-no-invitations"]').exists()).toBe(false);
    await settings.find('[data-testid="organization-segment-members"]').trigger("keydown", { key: "End" });
    expect(settings.find('[data-testid="organization-segment-teams"]').attributes("aria-selected")).toBe("true");
    await settings.find('[data-testid="organization-segment-teams"]').trigger("keydown", { key: "ArrowRight" });
    expect(settings.find('[data-testid="organization-segment-members"]').attributes("aria-selected")).toBe("true");
    settings.unmount();
  });

  it("cancels a sent invitation after confirmation in Invitations", async () => {
    api.getOrganization.mockResolvedValue(createOrganizationDetail({ invitations: [{ id: "dave", email: "dave@example.com", role: "member", createdAt: "2026-10-21T09:00:00Z", expiresAt: "2026-10-28T09:00:00Z" }] }));
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="organization-segment-invitations"]').trigger("click");
    await settings.find('[data-testid="cancel-organization-invitation-dave"]').trigger("click");
    expect(api.cancelOrganizationInvitation).not.toHaveBeenCalled();
    await settings.find('[data-testid="confirm-cancel-organization-invitation"]').trigger("click");
    await flushPromises();
    expect(api.cancelOrganizationInvitation).toHaveBeenCalledWith("acme", "dave");
    settings.unmount();
  });

  it("invites by email with a role and says the same for every address", async () => {
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="organization-segment-invitations"]').trigger("click");
    await settings.find('[data-testid="organization-invite-email"]').setValue("dave@example.com");
    await settings.find('[data-testid="organization-invite-role"]').setValue("admin");
    await settings.find('[data-testid="organization-invite"] form').trigger("submit");
    await flushPromises();
    expect(api.inviteToOrganization).toHaveBeenCalledWith("acme", "dave@example.com", "admin");
    expect(store.noticeMessage).toBe("Invitation sent to dave@example.com");
    settings.unmount();
  });

  it("names what goes before removing someone from the organization", async () => {
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="organization-member-menu-BOB222"]').trigger("click");
    await settings.find('[data-testid="context-menu-remove-organization-member"]').trigger("click");
    const dialog = settings.find('[data-testid="organization-dialog-remove-member"]');
    expect(dialog.text()).toContain("Bob Stone loses the vaults they have through Acme's teams.");
    expect(dialog.find('[data-testid="organization-dialog-vaults"]').text()).toContain("Deploy");
    expect(api.removeOrganizationMember).not.toHaveBeenCalled();

    await settings.find('[data-testid="organization-dialog-remove-member"] form').trigger("submit");
    await flushPromises();
    expect(api.removeOrganizationMember).toHaveBeenCalledWith("acme", "BOB222");
    expect(settings.find('[data-testid="organization-dialog-remove-member"]').exists()).toBe(false);
    settings.unmount();
  });

  it("shows the server's sentence in an alert when a change is refused", async () => {
    api.deleteOrganization.mockRejectedValue(new Error("the organization still has vaults; delete them first"));
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="delete-organization"]').trigger("click");
    await settings.find('[data-testid="organization-dialog-delete-organization"] form').trigger("submit");
    await flushPromises();
    expect(api.deleteOrganization).toHaveBeenCalledWith("acme");
    expect(store.organizationAlert).toEqual({ title: "Delete organization", message: "the organization still has vaults; delete them first" });
    expect(store.errorMessage).toBe("");

    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="organization-alert-message"]').text()).toBe("The organization still has vaults; delete them first");
    await wrapper.find('[data-testid="organization-alert-ok"]').trigger("click");
    expect(store.organizationAlert).toBeNull();
    wrapper.unmount();
    settings.unmount();
  });

  it("refreshes the open organization when the app reports a remote change", async () => {
    const settings = await openOrganizationInSettings();
    await settings.find('[data-testid="organization-segment-teams"]').trigger("click");
    store.isSettingsOpen = true;
    expect(settings.find('[data-testid="team-member-platform-CAROL3"]').exists()).toBe(false);

    api.getOrganization.mockResolvedValue(
      createOrganizationDetail({
        teams: [
          createOrganizationTeam({
            members: [...createOrganizationTeam().members, { accountId: "CAROL3", name: "Carol", email: "carol@example.com", isCurrentUser: false }],
          }),
        ],
      }),
    );
    await syncInBackground();
    await flushPromises();
    expect(settings.find('[data-testid="team-member-platform-CAROL3"]').exists()).toBe(true);
    settings.unmount();
  });
});

describe("Organization vaults in the vault list", () => {
  it("groups organization vaults under their organization after the other vaults", () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    const allOwnVaultID = wrapper.findAll('[data-testid="own-vaults"] .vault').map((vault) => vault.attributes("data-testid"));
    expect(allOwnVaultID).toEqual(["vault-personal", "vault-family"]);
    const acme = wrapper.find('[data-testid="organization-vaults-acme"]');
    expect(acme.find(".section-label").text()).toBe("Acme");
    expect(acme.findAll(".vault").map((vault) => vault.attributes("data-testid"))).toEqual(["vault-deploy", "vault-docs"]);
    expect(wrapper.find('[data-testid="organization-vaults-globex"] [data-testid="vault-lab"]').exists()).toBe(true);
    const allGroupID = wrapper.findAll(".vault-group").map((group) => group.attributes("data-testid"));
    expect(allGroupID).toEqual(["own-vaults", "organization-vaults-acme", "organization-vaults-globex"]);
    wrapper.unmount();
  });

  it("keeps a reader's organization vault read-only", async () => {
    resetStore(undefined, "docs");
    store.selectedItem = createItem();
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('button[aria-label="New item"]').attributes("disabled")).toBeDefined();
    expect(wrapper.findAll("button").map((button) => button.text())).not.toContain("Edit");
    wrapper.unmount();
  });

  it("creates an organization vault for the team picked in Where", async () => {
    api.getOrganization.mockImplementation((organizationID: string) =>
      Promise.resolve(organizationID === "acme" ? createOrganizationDetail() : createOrganizationDetail({ id: "globex", name: "Globex", teams: [createOrganizationTeam({ id: "research", name: "Research", members: [] })] })),
    );
    const created: VaultSummary = { id: "release", name: "Release", type: "organization", role: "owner", itemCount: 0, organizationId: "acme" };
    api.createOrganizationVault.mockResolvedValue(created);
    api.listAllVault.mockResolvedValue([...store.allVault, created]);
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="new-vault"]').trigger("click");
    await flushPromises();

    const allChoice = wrapper.findAll('[data-testid="new-vault-where"] option').map((option) => option.text());
    expect(allChoice).toEqual(["Your own vault (share it later)", "Acme › Platform (team vault)"]);
    await wrapper.find('input[name="vaultName"]').setValue("Release");
    await wrapper.find('[data-testid="new-vault-where"]').setValue("acme/platform");
    expect(wrapper.find('[data-testid="new-vault-where-note"]').text()).toContain("Platform's members get this vault");
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();

    expect(api.createOrganizationVault).toHaveBeenCalledWith("acme", "platform", "Release");
    expect(api.createVault).not.toHaveBeenCalled();
    expect(store.selectedVaultID).toBe("release");
    wrapper.unmount();
  });

  it("creates the user's own vault when Where stays on Your own vault", async () => {
    api.createVault.mockResolvedValue({ ...familyVault, id: "notes", name: "Notes" });
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="new-vault"]').trigger("click");
    await flushPromises();
    await wrapper.find('input[name="vaultName"]').setValue("Notes");
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();
    expect(api.createVault).toHaveBeenCalledWith("Notes");
    expect(api.createOrganizationVault).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("never turns a team Where no longer lists into a shared vault", async () => {
    api.getOrganization.mockImplementation((organizationID: string) => Promise.resolve(getOrganizationDetail(organizationID)));
    const wrapper = await openNewVaultForm();
    await wrapper.find('input[name="vaultName"]').setValue("Release");
    await wrapper.find('[data-testid="new-vault-where"]').setValue("acme/platform");
    expect(isCreateDisabled(wrapper)).toBe(false);

    api.listAllOrganization.mockResolvedValue(listOrganizations({ organizations: [globexSummary] }));
    await syncInBackground();
    await flushPromises();
    const where = wrapper.find('[data-testid="new-vault-where"]');
    expect(listAllWhereChoice(wrapper)).toEqual(["Your own vault (share it later)", "Globex › Research (team vault)"]);
    expect((where.element as HTMLSelectElement).selectedIndex).toBe(-1);
    expect(wrapper.find('[data-testid="new-vault-where-note"]').exists()).toBe(false);
    expect(isCreateDisabled(wrapper)).toBe(true);
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();
    expect(api.createVault).not.toHaveBeenCalled();
    expect(api.createOrganizationVault).not.toHaveBeenCalled();

    api.createVault.mockResolvedValue({ ...familyVault, id: "release", name: "Release" });
    await where.setValue("");
    expect(isCreateDisabled(wrapper)).toBe(false);
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();
    expect(api.createVault).toHaveBeenCalledWith("Release");
    expect(api.createOrganizationVault).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("keeps Create off while Where loads the teams, and reloads them on a remote change", async () => {
    let finishLoading = (): void => undefined;
    const loading = new Promise<void>((resolve) => (finishLoading = resolve));
    api.getOrganization.mockImplementation(async (organizationID: string) => {
      await loading;
      return getOrganizationDetail(organizationID);
    });
    const wrapper = await openNewVaultForm();
    await wrapper.find('input[name="vaultName"]').setValue("Release");
    expect(wrapper.find('[data-testid="new-vault-where"]').exists()).toBe(false);
    expect(isCreateDisabled(wrapper)).toBe(true);
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();
    expect(api.createVault).not.toHaveBeenCalled();

    finishLoading();
    await flushPromises();
    expect(listAllWhereChoice(wrapper)).toEqual(["Your own vault (share it later)", "Acme › Platform (team vault)", "Globex › Research (team vault)"]);
    expect(isCreateDisabled(wrapper)).toBe(false);
    await wrapper.find('[data-testid="new-vault-where"]').setValue("acme/platform");

    // On another Mac an admin moves Alice from Platform to the new team Ops; Settings and the sharing panel stay closed.
    const platformWithoutAlice = createOrganizationTeam({ members: [{ accountId: testBob.accountId, name: testBob.name, email: testBob.email, isCurrentUser: false }] });
    const ops = createOrganizationTeam({ id: "ops", name: "Ops", vaults: [] });
    api.getOrganization.mockImplementation((organizationID: string) => Promise.resolve(getOrganizationDetail(organizationID, createOrganizationDetail({ teams: [platformWithoutAlice, ops] }))));
    await syncInBackground();
    await flushPromises();
    expect(store.isSettingsOpen || store.isSharingOpen).toBe(false);
    expect(listAllWhereChoice(wrapper)).toEqual(["Your own vault (share it later)", "Acme › Ops (team vault)", "Globex › Research (team vault)"]);
    expect(isCreateDisabled(wrapper)).toBe(true);
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();
    expect(api.createVault).not.toHaveBeenCalled();
    expect(api.createOrganizationVault).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("keeps Share on an organization vault without calling its items shared", async () => {
    resetStore(undefined, "deploy");
    store.selectedItem = createItem();
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="share-vault"]').exists()).toBe(true);
    const vaultBadge = wrapper.find('[data-testid="item-vault"]');
    expect(vaultBadge.text()).toBe("Deploy");
    expect(vaultBadge.findComponent({ name: "AppIcon" }).props("name")).toBe("users");
    expect(wrapper.find('[data-testid="item-shared"]').exists()).toBe(false);

    store.selectedVaultID = "family";
    store.listVaultID = "family";
    await nextTick();
    expect(wrapper.find('[data-testid="share-vault"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="item-shared"]').text()).toBe("Shared");
    wrapper.unmount();
  });

  it("says per organization which vaults wait for a key in the Inbox, and what Hisar could not do above the vaults", async () => {
    const metrics: WaitingVaultKey = { vaultId: "metrics", organizationId: "acme", organizationName: "Acme", role: "editor" };
    const billing: WaitingVaultKey = { vaultId: "billing", organizationId: "acme", organizationName: "Acme", role: "reader" };
    const notes: WaitingVaultKey = { vaultId: "notes", organizationId: "globex", organizationName: "Globex", role: "editor" };
    const deliveryError = "dave@example.com: their signing key changed";
    const rotationError = "Deploy: the signing key of bob@example.com changed since you last shared with them; stopping";
    api.listAllWaitingVaultKey.mockResolvedValue({ waiting: [metrics, billing, notes], deliveryError, rotationError });
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="organization-banner"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="open-inbox"]').exists()).toBe(false);

    await syncInBackground();
    await flushPromises();
    expect(wrapper.find('[data-testid="organization-banner-delivery-error"]').text()).toBe(`Hisar couldn't give every vault key: ${deliveryError}`);
    expect(wrapper.find('[data-testid="organization-banner-delivery-error"] button').attributes("aria-expanded")).toBe("false");
    await wrapper.find('[data-testid="organization-banner-delivery-error"] button').trigger("click");
    expect(wrapper.find('[data-testid="organization-banner-delivery-error"] button').attributes("aria-expanded")).toBe("true");
    expect(wrapper.find('[data-testid="organization-banner-rotation-error"]').text()).toBe(`Hisar couldn't re-encrypt every vault that needs a new key: ${rotationError}`);
    expect(wrapper.find('[data-testid="inbox-count"]').exists()).toBe(false);
    await wrapper.find('[data-testid="open-inbox"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="settings-page-organizations"]').isVisible()).toBe(true);
    expect(wrapper.find('[data-testid="waiting-vault-keys"]').text()).toContain("Vault keys on their way");
    expect(wrapper.findAll('[data-testid^="organization-waiting-"]').map((line) => line.text())).toEqual([
      "Waiting for the owners of 2 vaults in Acme to give you their keys",
      "Waiting for an owner of a vault in Globex to give you its key",
    ]);

    // Metrics arrives though the response still lists it, Billing still waits, Globex's vault waits no more, and the rotation is done.
    const metricsVault: VaultSummary = { id: "metrics", name: "Metrics", type: "organization", role: "editor", itemCount: 0, organizationId: "acme" };
    api.listAllVault.mockResolvedValue([...store.allVault, metricsVault]);
    api.listAllWaitingVaultKey.mockResolvedValue({ waiting: [metrics, billing], deliveryError: "", rotationError: "" });
    await syncInBackground();
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-metrics"]').exists()).toBe(true);
    expect(wrapper.findAll('[data-testid^="organization-waiting-"]').map((line) => line.text())).toEqual(["Waiting for an owner of a vault in Acme to give you its key"]);
    expect(wrapper.find('[data-testid="organization-waiting-globex"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="organization-banner-delivery-error"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="organization-banner-rotation-error"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Organization vault sharing panel", () => {
  it("shows the teams that have the vault instead of Invite by email", async () => {
    resetStore(undefined, "deploy");
    api.getOrganization.mockResolvedValue(
      createOrganizationDetail({ teams: [createOrganizationTeam(), createOrganizationTeam({ id: "ops", name: "Ops", members: [], vaults: [] })] }),
    );
    const wrapper = await openSharingPanel();
    expect(api.getOrganization).toHaveBeenCalledWith("acme");
    expect(api.listAllVaultMember).not.toHaveBeenCalled();
    const panel = wrapper.find('[data-testid="vault-sharing-panel"]');
    expect(panel.find('[data-testid="sharing-organization"]').text()).toBe("Shared through teams in Acme");
    expect(panel.find('[data-testid="add-member"]').exists()).toBe(false);
    const platform = panel.find('[data-testid="vault-team-platform"]');
    expect(platform.text()).toContain("Alice (you), Bob Stone");
    expect((platform.find('[data-testid="vault-team-role-platform"]').element as HTMLSelectElement).value).toBe("owner");
    expect(panel.find('[data-testid="vault-team-ops"]').exists()).toBe(false);

    await panel.find('[data-testid="give-to-team"]').trigger("click");
    for (const role of panel.findAll("select.role-select")) {
      expect(role.attributes("title")).toBe("Readers view items, editors also change them, owners also share, rename and delete the vault.");
    }
    expect(panel.text()).not.toContain("Editors can add and change items");
    await panel.find('[data-testid="give-to-team-role"]').setValue("reader");
    await panel.find('[data-testid="give-to-team-form"]').trigger("submit");
    await flushPromises();
    expect(api.giveVaultToTeam).toHaveBeenCalledWith("acme", "ops", "deploy", "reader");
    wrapper.unmount();
  });

  it("asks members to talk to an admin", async () => {
    resetStore(undefined, "docs");
    api.getOrganization.mockResolvedValue(createOrganizationDetail({ role: "member", isAdmin: false, teams: [createOrganizationTeam({ vaults: [{ vaultId: "docs", name: "Docs", role: "reader", canOpen: true }] })] }));
    const wrapper = await openSharingPanel();
    expect(wrapper.find('[data-testid="sharing-organization-read-only"]').text()).toBe("Ask an admin of Acme to change who has this vault.");
    expect(wrapper.find('[data-testid="vault-team-role-platform"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="give-to-team"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="vault-team-platform"]').text()).toContain("Reader");
    wrapper.unmount();
  });

  it("shows the owner rule in an alert when taking back the last owning team", async () => {
    resetStore(undefined, "deploy");
    api.takeVaultFromTeam.mockRejectedValue(new Error(ownerTakeBackRefusal));
    const wrapper = await openSharingPanel();
    await wrapper.find('[data-testid="take-back-from-platform"]').trigger("click");
    await flushPromises();
    expect(api.takeVaultFromTeam).toHaveBeenCalledWith("acme", "platform", "deploy");
    expect(wrapper.find('[data-testid="organization-alert"]').text()).toContain("Take back");
    expect(wrapper.find('[data-testid="organization-alert-message"]').text()).toBe(ownerTakeBackRefusal);
    expect(wrapper.find('[data-testid="vault-team-platform"]').exists()).toBe(true);
    wrapper.unmount();
  });
});

describe("Account deletion", () => {
  it("says what happens to each organization", async () => {
    api.previewAccountDeletion.mockResolvedValue({
      email: "alice@example.com",
      planVersion: "plan",
      ownedVaults: [],
      memberVaults: [],
      shareLinkCount: 0,
      sentInvitationCount: 0,
      receivedInvitationCount: 0,
      deviceCount: 1,
      organizations: [
        { id: "solo", name: "Solo", isDeleted: true, newAdminName: "" },
        { id: "acme", name: "Acme", isDeleted: false, newAdminName: "Ada" },
        { id: "globex", name: "Globex", isDeleted: false, newAdminName: "" },
      ],
    });
    const dialog = mount(DeleteAccountDialog, { attachTo: document.body });
    await flushPromises();
    expect(dialog.findAll('[data-testid="delete-account-organization"]').map((line) => line.text())).toEqual([
      "Solo is deleted with its teams, vaults, and items.",
      "Ada becomes an admin of Acme.",
      "You leave Globex.",
    ]);
    dialog.unmount();
  });
});
