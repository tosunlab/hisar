import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import type { Organizations, VaultSummary } from "../api";
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

function resetStore(allVault: VaultSummary[] = [personalVault, familyVault, deployVault, docsVault, labVault], selectedVaultID = "personal"): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault,
    selectedVaultID,
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
  it("accepts an invitation from the banner above the vaults", async () => {
    const invitation = createOrganizationInvitation();
    store.organizations = listOrganizations({ invitations: [invitation] });
    const wrapper = mount(MainView, { attachTo: document.body });
    const banner = wrapper.find('[data-testid="organization-banner-invitation-invitation-to-globex"]');
    expect(banner.text()).toContain("Ada invited you to Globex as a member");

    api.listAllOrganization.mockResolvedValue(listOrganizations());
    await banner.find('[data-testid="organization-banner-accept-invitation-to-globex"]').trigger("click");
    await flushPromises();

    expect(api.acceptOrganizationInvitation).toHaveBeenCalledWith("invitation-to-globex");
    expect(wrapper.find('[data-testid="organization-banner-invitation-invitation-to-globex"]').exists()).toBe(false);
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
  it("lists members with security codes, teams and the vaults this Mac can't open", async () => {
    api.getOrganization.mockResolvedValue(
      createOrganizationDetail({
        members: [createOrganizationMember(), createOrganizationMember({ ...testBob, role: "member", isKeyChanged: true })],
        teams: [createOrganizationTeam({ vaults: [{ vaultId: "deploy", name: "Deploy", role: "owner", canOpen: true }, { vaultId: "secret", name: "", role: "reader", canOpen: false }] })],
      }),
    );
    const settings = await openOrganizationInSettings();
    expect(api.getOrganization).toHaveBeenCalledWith("acme");
    const bob = settings.find('[data-testid="organization-member-BOB222"]');
    expect(bob.find('[data-testid="organization-member-security-code"]').text()).toBe(testBob.securityCode);
    expect(bob.find('[data-testid="organization-member-key-changed"]').exists()).toBe(true);
    const allVaultName = settings.findAll('[data-testid="team-platform"] [data-testid="team-vault-name"]').map((name) => name.text());
    expect(allVaultName).toEqual(["Deploy", "A vault you can't open"]);
    settings.unmount();
  });

  it("gives admins the admin actions and hides them from members", async () => {
    const admin = await openOrganizationInSettings();
    for (const testID of ["organization-invite", "rename-organization", "delete-organization", "new-team", "rename-team-platform", "delete-team-platform", "toggle-organization-role-BOB222", "remove-organization-member-BOB222", "remove-team-member-platform-BOB222", "take-back-platform-deploy"]) {
      expect(admin.find(`[data-testid="${testID}"]`).exists(), testID).toBe(true);
    }
    admin.unmount();

    resetStore();
    api.getOrganization.mockResolvedValue(createOrganizationDetail({ role: "member", isAdmin: false, members: [createOrganizationMember({ role: "member" }), createOrganizationMember({ ...testBob, role: "admin" })] }));
    const member = await openOrganizationInSettings();
    expect(member.find('[data-testid="organization-name"]').text()).toBe("Acme");
    for (const testID of ["organization-invite", "rename-organization", "delete-organization", "new-team", "rename-team-platform", "delete-team-platform", "toggle-organization-role-BOB222", "remove-organization-member-BOB222", "remove-team-member-platform-BOB222", "take-back-platform-deploy", "give-vault-platform"]) {
      expect(member.find(`[data-testid="${testID}"]`).exists(), testID).toBe(false);
    }
    expect(member.find('[data-testid="leave-organization"]').text()).toBe("Leave Acme…");
    expect(member.find('[data-testid="organization-member-BOB222"] [data-testid="organization-member-security-code"]').exists()).toBe(true);
    member.unmount();
  });

  it("invites by email with a role and says the same for every address", async () => {
    const settings = await openOrganizationInSettings();
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
    await settings.find('[data-testid="remove-organization-member-BOB222"]').trigger("click");
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
    expect(allChoice).toEqual(["Shared vault", "Acme › Platform"]);
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

  it("creates a shared vault when Where stays on Shared vault", async () => {
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

  it("says it waits for a key until the vault arrives, and shows what Hisar could not do", async () => {
    const waitingVault: VaultSummary = { id: "metrics", name: "Metrics", type: "organization", role: "editor", itemCount: 0, organizationId: "acme" };
    store.waitingVaultKeys = {
      waiting: [{ vaultId: "metrics", organizationId: "acme", organizationName: "Acme", role: "editor" }],
      deliveryError: "dave@example.com: their signing key changed",
      rotationError: "",
    };
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="organization-banner-waiting-acme"]').text()).toBe("Waiting for an owner of a vault in Acme to give you its key");
    expect(wrapper.find('[data-testid="organization-banner-delivery-error"]').text()).toContain("dave@example.com: their signing key changed");

    api.listAllVault.mockResolvedValue([...store.allVault, waitingVault]);
    await syncInBackground();
    await flushPromises();
    expect(wrapper.find('[data-testid="vault-metrics"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="organization-banner-waiting-acme"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="organization-banner-delivery-error"]').exists()).toBe(false);
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
      "Solo is deleted with your account.",
      "Ada becomes an admin of Acme.",
      "You leave Globex.",
    ]);
    dialog.unmount();
  });
});
