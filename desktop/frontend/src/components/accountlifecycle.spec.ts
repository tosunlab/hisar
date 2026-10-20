import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createStatus } from "../testing";
import type { AccountDeletionPreview } from "../api";

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

const { store } = await import("../store");
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const ChangeMasterPasswordDialog = (await import("./ChangeMasterPasswordDialog.vue")).default;
const ReplaceSecretKeyDialog = (await import("./ReplaceSecretKeyDialog.vue")).default;
const DeleteAccountDialog = (await import("./DeleteAccountDialog.vue")).default;
const SignInView = (await import("./SignInView.vue")).default;
const api = apiMock.current;

const strongPassword = "orange cactus harbor lantern";

function createPreview(overrides: Partial<AccountDeletionPreview> = {}): AccountDeletionPreview {
  return {
    email: "alice@example.com",
    ownedVaults: [
      { id: "personal", name: "Personal", type: "personal", role: "owner", otherMemberCount: 0 },
      { id: "family", name: "Family", type: "shared", role: "owner", otherMemberCount: 2 },
    ],
    memberVaults: [
      { id: "team", name: "Team", type: "shared", role: "owner", otherMemberCount: 0 },
      { id: "design", name: "Design", type: "shared", role: "reader", otherMemberCount: 0 },
    ],
    shareLinkCount: 1,
    sentInvitationCount: 2,
    receivedInvitationCount: 0,
    deviceCount: 3,
    organizations: [],
    ...overrides,
  };
}

async function submitForm(wrapper: ReturnType<typeof mount>): Promise<void> {
  await wrapper.find("form").trigger("submit");
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getSSHAgentStatus?.mockResolvedValue?.({});
  api.forgetNewAccountSecretKey.mockResolvedValue(undefined);
  Object.assign(store, { status: createStatus("unlocked"), errorMessage: "", noticeMessage: "", isBusy: false, pendingSecretKey: "" });
});

describe("Settings rows", () => {
  it("offers changing the master password and replacing the Secret Key under Security", async () => {
    window.localStorage.setItem("hisar.settings.selectedSection", "security");
    const wrapper = mount(SettingsPanel);
    expect(wrapper.find('[data-testid="change-master-password"]').text()).toBe("Change master password…");
    expect(wrapper.find('[data-testid="replace-secret-key"]').text()).toBe("Replace Secret Key…");
    await wrapper.find('[data-testid="change-master-password"]').trigger("click");
    expect(wrapper.find('[data-testid="change-master-password-dialog"]').exists()).toBe(true);
  });

  it("offers deleting the account under Account", async () => {
    window.localStorage.setItem("hisar.settings.selectedSection", "account");
    api.previewAccountDeletion.mockResolvedValue(createPreview());
    const wrapper = mount(SettingsPanel);
    expect(wrapper.find('[data-testid="delete-account"]').text()).toBe("Delete account…");
    await wrapper.find('[data-testid="delete-account"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="delete-account-consequences"]').exists()).toBe(true);
  });
});

describe("Change master password", () => {
  it("says what happens to other devices before anything is changed", () => {
    const wrapper = mount(ChangeMasterPasswordDialog);
    expect(wrapper.find('[data-testid="change-master-password-consequences"]').text()).toContain(
      "Your other devices will be signed out. Sign in on them with the new master password.",
    );
    expect(api.changeMasterPassword).not.toHaveBeenCalled();
  });

  it("refuses an easy or mismatched new password", async () => {
    const wrapper = mount(ChangeMasterPasswordDialog);
    await wrapper.find('input[name="currentPassword"]').setValue("current password");
    await wrapper.find('input[name="newPassword"]').setValue("password1234");
    await wrapper.find('input[name="repeatedPassword"]').setValue("password1234");
    expect(wrapper.find('[data-testid="new-password-feedback"]').text()).toBe("This password is too easy to guess. Add a few more words or characters.");
    expect(wrapper.find('[data-testid="confirm-change-master-password"]').attributes("disabled")).toBeDefined();
    await wrapper.find('input[name="newPassword"]').setValue(strongPassword);
    expect(wrapper.find('[data-testid="new-password-mismatch"]').exists()).toBe(true);
    await submitForm(wrapper);
    expect(api.changeMasterPassword).not.toHaveBeenCalled();
  });

  it("changes it and reports the signed-out devices", async () => {
    api.changeMasterPassword.mockResolvedValue({ status: createStatus("unlocked"), secretKey: "", signedOutDeviceCount: 2, keychainError: "", localError: "" });
    const wrapper = mount(ChangeMasterPasswordDialog);
    await wrapper.find('input[name="currentPassword"]').setValue("current password");
    await wrapper.find('input[name="newPassword"]').setValue(strongPassword);
    await wrapper.find('input[name="repeatedPassword"]').setValue(strongPassword);
    await submitForm(wrapper);
    expect(api.changeMasterPassword).toHaveBeenCalledWith("current password", strongPassword);
    expect(wrapper.find('[data-testid="change-master-password-done"]').text()).toContain("2 other devices were signed out.");
    await wrapper.find('[data-testid="change-master-password-close"]').trigger("click");
    expect(wrapper.emitted("close")).toHaveLength(1);
  });

  it("shows a wrong current password and keeps the form", async () => {
    api.changeMasterPassword.mockRejectedValue(new Error("the master password is not correct"));
    const wrapper = mount(ChangeMasterPasswordDialog);
    await wrapper.find('input[name="currentPassword"]').setValue("wrong");
    await wrapper.find('input[name="newPassword"]').setValue(strongPassword);
    await wrapper.find('input[name="repeatedPassword"]').setValue(strongPassword);
    await submitForm(wrapper);
    expect(wrapper.find('[role="alert"]').text()).toBe("the master password is not correct");
    expect((wrapper.find('input[name="currentPassword"]').element as HTMLInputElement).value).toBe("");
  });
});

describe("Replace Secret Key", () => {
  const newSecretKey = "H1-ABC123-AAAAA-BBBBB-CCCCC-DDDDD-EEEEEE";

  it("warns that the old Emergency Kit stops working before asking for the master password", () => {
    const wrapper = mount(ReplaceSecretKeyDialog);
    expect(wrapper.find('[data-testid="replace-secret-key-consequences"]').text()).toContain("Your old Emergency Kit stops working.");
    expect(wrapper.find('[data-testid="confirm-replace-secret-key"]').attributes("disabled")).toBeDefined();
  });

  it("shows the new Secret Key and keeps the dialog open until the kit is saved", async () => {
    api.replaceSecretKey.mockResolvedValue({ status: createStatus("unlocked"), secretKey: newSecretKey, signedOutDeviceCount: 1, keychainError: "", localError: "" });
    api.saveNewAccountEmergencyKit.mockResolvedValue("/Users/alice/Hisar Emergency Kit.pdf");
    const wrapper = mount(ReplaceSecretKeyDialog);
    await wrapper.find('input[name="replaceSecretKeyPassword"]').setValue("master password");
    await submitForm(wrapper);
    expect(api.replaceSecretKey).toHaveBeenCalledWith("master password");
    expect(wrapper.find('[data-testid="new-secret-key"]').attributes("aria-label")).toBe(newSecretKey);
    expect(wrapper.find('[data-testid="replace-secret-key-done"]').text()).toContain("1 other device was signed out.");
    expect(wrapper.find('[data-testid="replace-secret-key-close"]').attributes("disabled")).toBeDefined();
    await wrapper.find('[data-testid="save-new-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(api.saveNewAccountEmergencyKit).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="new-emergency-kit-saved"]').exists()).toBe(true);
    await wrapper.find('[data-testid="replace-secret-key-close"]').trigger("click");
    await flushPromises();
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
    expect(wrapper.emitted("close")).toHaveLength(1);
  });

  it("shows the keychain error and lets the user confirm they wrote the key down", async () => {
    api.replaceSecretKey.mockResolvedValue({ status: createStatus("unlocked"), secretKey: newSecretKey, signedOutDeviceCount: 0, keychainError: "the new Secret Key could not be saved to the keychain", localError: "" });
    const wrapper = mount(ReplaceSecretKeyDialog);
    await wrapper.find('input[name="replaceSecretKeyPassword"]').setValue("master password");
    await submitForm(wrapper);
    expect(wrapper.find('[data-testid="keychain-error"]').exists()).toBe(true);
    await wrapper.find('input[name="hasConfirmedSaved"]').setValue(true);
    expect(wrapper.find('[data-testid="replace-secret-key-close"]').attributes("disabled")).toBeUndefined();
  });

  it("keeps the new key and Emergency Kit screen visible alongside a cache warning", async () => {
    const localError = "your credentials were changed, but this device's offline copy of the vaults could not be deleted";
    api.replaceSecretKey.mockResolvedValue({ status: createStatus("unlocked"), secretKey: newSecretKey, signedOutDeviceCount: -1, keychainError: "", localError });
    api.saveNewAccountEmergencyKit.mockResolvedValue("/Users/alice/Hisar Emergency Kit.pdf");
    const wrapper = mount(ReplaceSecretKeyDialog);
    await wrapper.find('input[name="replaceSecretKeyPassword"]').setValue("master password");
    await submitForm(wrapper);
    expect(wrapper.find('[data-testid="new-secret-key"]').attributes("aria-label")).toBe(newSecretKey);
    expect(wrapper.find('[data-testid="credential-local-error"]').text()).toBe(localError);
    expect(wrapper.find('[data-testid="replace-secret-key-done"]').text()).toContain("Any other devices were signed out");
    expect(wrapper.find("form").exists()).toBe(false);
    await wrapper.find('[data-testid="save-new-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="new-emergency-kit-saved"]').exists()).toBe(true);
  });
});

describe("Delete account", () => {
  it("lists what is deleted and who loses access", async () => {
    api.previewAccountDeletion.mockResolvedValue(createPreview());
    const wrapper = mount(DeleteAccountDialog);
    await flushPromises();
    expect(wrapper.find('[data-testid="delete-account-export"]').text()).toContain("Export your items first in Settings → Export.");
    const consequences = wrapper.find('[data-testid="delete-account-consequences"]').text();
    expect(consequences).toContain("Deleted with their items: Personal.");
    expect(wrapper.find('[data-testid="delete-account-owned-shared-vault"]').text()).toContain("Family is deleted with its items: you are its only owner, so 2 people lose access");
    const allMemberText = wrapper.findAll('[data-testid="delete-account-member-vault"]').map((element) => element.text());
    expect(allMemberText).toEqual(["Team: you co-own it; it stays with its other owners.", "Design: you are a reader; your access is removed."]);
    expect(consequences).toContain("1 share link stops working.");
    expect(consequences).toContain("2 invitations are removed.");
    expect(consequences).toContain("3 devices are signed out, including this Mac.");
  });

  it("needs the master password and the typed email", async () => {
    api.previewAccountDeletion.mockResolvedValue(createPreview());
    api.deleteAccount.mockResolvedValue(createStatus("signed-out"));
    const wrapper = mount(DeleteAccountDialog);
    await flushPromises();
    const deleteButton = () => wrapper.find('[data-testid="confirm-delete-account"]');
    await wrapper.find('input[name="deleteAccountPassword"]').setValue("master password");
    await wrapper.find('input[name="deleteAccountEmail"]').setValue("bob@example.com");
    expect(deleteButton().attributes("disabled")).toBeDefined();
    await submitForm(wrapper);
    expect(api.deleteAccount).not.toHaveBeenCalled();
    await wrapper.find('input[name="deleteAccountEmail"]').setValue(" Alice@Example.com ");
    expect(deleteButton().attributes("disabled")).toBeUndefined();
    await submitForm(wrapper);
    expect(api.deleteAccount).toHaveBeenCalledWith("master password", expect.stringMatching(/^ ?Alice@Example\.com ?$/));
    expect(store.status.state).toBe("signed-out");
    expect(wrapper.emitted("close")).toHaveLength(1);
  });

  it("shows a wrong master password without closing", async () => {
    api.previewAccountDeletion.mockResolvedValue(createPreview({ ownedVaults: [], memberVaults: [], shareLinkCount: 0, sentInvitationCount: 0, deviceCount: 1 }));
    api.deleteAccount.mockRejectedValue(new Error("the master password is not correct"));
    const wrapper = mount(DeleteAccountDialog);
    await flushPromises();
    await wrapper.find('input[name="deleteAccountPassword"]').setValue("wrong");
    await wrapper.find('input[name="deleteAccountEmail"]').setValue("alice@example.com");
    await submitForm(wrapper);
    expect(wrapper.find('[role="alert"]').text()).toBe("the master password is not correct");
    expect(wrapper.emitted("close")).toBeUndefined();
    expect(store.status.state).toBe("unlocked");
  });
});

describe("Signed out by the server", () => {
  it("explains why on the sign-in screen and can be dismissed", async () => {
    const reason = "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one.";
    store.status = { ...createStatus("signed-out"), signedOutReason: reason };
    api.dismissSignedOutReason.mockResolvedValue({ ...createStatus("signed-out"), signedOutReason: "" });
    const wrapper = mount(SignInView);
    expect(wrapper.find('[data-testid="signed-out-reason"]').text()).toContain(reason);
    await wrapper.find('[data-testid="dismiss-signed-out-reason"]').trigger("click");
    await flushPromises();
    expect(api.dismissSignedOutReason).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="signed-out-reason"]').exists()).toBe(false);
  });
});
