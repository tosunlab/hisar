import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createCommandLineApprovalEvent, createItem, createSSHKeyItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventListener = vi.hoisted(() => ({ allByName: {} as Record<string, (event: { data: unknown }) => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createItem({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      eventListener.allByName[name] = listener;
      return () => delete eventListener.allByName[name];
    },
  },
}));

const { store } = await import("../store");
const { approvalStore } = await import("../approval");
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const SSHApprovalWindow = (await import("./SSHApprovalWindow.vue")).default;
const api = apiMock.current;

const detailProperties = { canEdit: true, vaultName: "Personal", isShared: false };

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    selectedItem: createItem({
      id: "github",
      title: "GitHub",
      username: "alice",
      password: "github-password",
      fields: [{ name: "API token", value: "token-value", concealed: true }],
      hasOneTimePassword: true,
    }),
    editorMode: "view",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
  api.getOneTimeCode.mockResolvedValue({ code: "482913", expiresAt: "2099-01-01T00:00:00Z", period: 30 });
  Object.assign(approvalStore, { allApproval: [], failureMessage: "", isAuthorizing: false, isRemembered: true, isPasswordChosen: false, isExpanded: false });
  api.listAllPendingSSHApproval.mockResolvedValue([]);
});

describe("Copy secret reference", () => {
  it("sits next to the copy button of every field a reference can name", async () => {
    const wrapper = mount(ItemDetailPanel, { props: detailProperties });
    await flushPromises();
    const allLabel = wrapper.findAll('[data-testid="copy-secret-reference"]').map((button) => button.attributes("aria-label"));
    expect(allLabel).toEqual([
      "Copy secret reference for username",
      "Copy secret reference for password",
      "Copy secret reference for one-time code",
      "Copy secret reference for API token",
    ]);
    const passwordButton = wrapper.find('button[aria-label="Copy secret reference for password"]');
    expect(passwordButton.attributes("title")).toBe("Copy secret reference — use it in a terminal with hisar run or hisar read");
    expect(passwordButton.element.closest(".field-actions")?.querySelector('button[aria-label="Copy password"]')).not.toBeNull();
    wrapper.unmount();
  });

  it("copies the reference the backend builds and shows it", async () => {
    api.copySecretReference.mockResolvedValue("hisar://Personal/GitHub/API%20token");
    const wrapper = mount(ItemDetailPanel, { props: detailProperties });
    await flushPromises();
    await wrapper.find('button[aria-label="Copy secret reference for API token"]').trigger("click");
    await flushPromises();
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "github", "API token");
    expect(api.copyField).not.toHaveBeenCalled();
    expect(store.noticeMessage).toBe("Copied hisar://Personal/GitHub/API%20token — use it with hisar run or hisar read");
    wrapper.unmount();
  });

  it("asks for the one-time code by its field name", async () => {
    api.copySecretReference.mockResolvedValue("hisar://Personal/GitHub/otp");
    const wrapper = mount(ItemDetailPanel, { props: detailProperties });
    await flushPromises();
    await wrapper.find('button[aria-label="Copy secret reference for one-time code"]').trigger("click");
    await flushPromises();
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "github", "one-time code");
    wrapper.unmount();
  });

  it("shows why a field cannot be referenced", async () => {
    api.copySecretReference.mockRejectedValue(new Error('2 fields are labelled "API token"; rename one of them to reference it'));
    const wrapper = mount(ItemDetailPanel, { props: detailProperties });
    await flushPromises();
    await wrapper.find('button[aria-label="Copy secret reference for API token"]').trigger("click");
    await flushPromises();
    expect(store.errorMessage).toBe('2 fields are labelled "API token"; rename one of them to reference it');
    expect(store.noticeMessage).toBe("");
    wrapper.unmount();
  });

  it("offers the fingerprint and public key of an SSH key, never its private key", async () => {
    store.selectedItem = createSSHKeyItem();
    api.copySecretReference.mockResolvedValue("hisar://Personal/Laptop%20key/public%20key");
    const wrapper = mount(ItemDetailPanel, { props: detailProperties });
    await flushPromises();
    const allLabel = wrapper.findAll('[data-testid="copy-secret-reference"]').map((button) => button.attributes("aria-label"));
    expect(allLabel).toEqual(["Copy secret reference for fingerprint", "Copy secret reference for public key"]);
    expect(wrapper.text()).toContain("Private keyNever leaves Hisar; the SSH agent uses it");
    await wrapper.find('button[aria-label="Copy secret reference for public key"]').trigger("click");
    await flushPromises();
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "ssh-1", "public key");
    expect(wrapper.text()).not.toContain("PRIVATE KEY");
    wrapper.unmount();
  });
});

describe("Approval for hisar run", () => {
  it("shows the command and the secrets it reads", async () => {
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    eventListener.allByName["ssh-approval"]({
      data: createCommandLineApprovalEvent({
        appName: "Terminal",
        action: "run a command with 6 secrets",
        commandLine: "npm test",
        allSecretName: ["Personal › GitHub › password", "Personal › GitHub › API token", "Work › AWS › access key", "Work › AWS › secret key"],
        moreSecretCount: 2,
      }),
    });
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("hisar in Terminal wants to run a command with 6 secrets");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["npm test", "/dev/ttys003"]);
    const secretList = wrapper.find('[data-testid="approval-secrets"]');
    expect(secretList.find(".secret-heading").text()).toBe("Secrets");
    expect(secretList.findAll("li").map((item) => item.text())).toEqual([
      "Personal › GitHub › password",
      "Personal › GitHub › API token",
      "Work › AWS › access key",
      "Work › AWS › secret key",
      "and 2 more",
    ]);
    expect(wrapper.text()).toContain("10 minutes after this terminal’s last command");
    wrapper.unmount();
  });

  it("names a single secret for hisar read", async () => {
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    eventListener.allByName["ssh-approval"]({
      data: createCommandLineApprovalEvent({ appName: "Terminal", action: "read 1 secret", allSecretName: ["Personal › GitHub › one-time code"] }),
    });
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("hisar in Terminal wants to read 1 secret");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["/dev/ttys003"]);
    expect(wrapper.find(".secret-heading").text()).toBe("Secret");
    wrapper.unmount();
  });

  it("keeps other command-line approvals as they were", async () => {
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    eventListener.allByName["ssh-approval"]({ data: createCommandLineApprovalEvent({ appName: "Terminal", allSecretName: null }) });
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("hisar in Terminal wants to use Hisar");
    expect(wrapper.find('[data-testid="approval-secrets"]').exists()).toBe(false);
    wrapper.unmount();
  });
});
