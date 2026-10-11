import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import {
  createAgentApprovalEvent,
  createAgentRevealApprovalEvent,
  createAgentRunApprovalEvent,
  createApiMock,
  createApprovalEvent,
  createCommandLineApprovalEvent,
  createForwardedApprovalEvent,
  createSigningApprovalEvent,
} from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventListener = vi.hoisted(() => ({ allByName: {} as Record<string, (event: { data: unknown }) => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return { api: apiMock.current, getErrorMessage: (error: unknown) => String(error) };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      eventListener.allByName[name] = listener;
      return () => delete eventListener.allByName[name];
    },
  },
}));

const { approvalStore } = await import("../approval");
const SSHApprovalWindow = (await import("./SSHApprovalWindow.vue")).default;
const api = apiMock.current;

function sendEvent(name: string, data: unknown): void {
  eventListener.allByName[name]({ data });
}

function pressKey(key: string): void {
  window.dispatchEvent(new KeyboardEvent("keydown", { key }));
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(approvalStore, { allApproval: [], failureMessage: "", isAuthorizing: false, isRemembered: true, isPasswordChosen: false, isExpanded: false });
  api.listAllPendingSSHApproval.mockResolvedValue([]);
});

describe("SSHApprovalWindow", () => {
  it("says who wants to do what where, and starts Touch ID", async () => {
    api.listAllPendingSSHApproval.mockResolvedValue([createApprovalEvent({ appIconURL: "data:image/png;base64,AA==" })]);
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Terminal wants to push to git.tryhisar.com");
    expect(wrapper.find(".subtitle").text()).toBe("with your SSH key “Laptop key”");
    expect(wrapper.find('[data-testid="app-icon"]').attributes("src")).toBe("data:image/png;base64,AA==");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["git push origin main", "~/src/core · git.tryhisar.com:22"]);
    expect(wrapper.find(".remember").text()).toBe("Don’t ask again for Terminal for 1 hour");
    expect((wrapper.find('input[name="isRemembered"]').element as HTMLInputElement).checked).toBe(true);
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toContain("Touch ID to approve");
    expect(wrapper.find('[data-testid="use-password"]').text()).toBe("or use your master password");
    expect(wrapper.find('[data-testid="approval-forwarded"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="approval-signing-badge"]').exists()).toBe(false);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, true);
    expect(api.setApprovalTouchIDFrame).toHaveBeenCalled();
    expect(api.fitSSHApprovalWindow).toHaveBeenCalledWith(false, expect.any(Number));
    wrapper.unmount();
  });

  it("follows the checkbox and resets it for the next request", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    await wrapper.find('input[name="isRemembered"]').setValue(false);
    expect(api.setSSHApprovalRemembered).toHaveBeenCalledWith("request-1", false);
    expect(approvalStore.isRemembered).toBe(false);
    sendEvent("ssh-approval-closed", "request-1");
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    expect(api.clearApprovalTouchIDFrame).toHaveBeenCalled();
    expect(approvalStore.isRemembered).toBe(true);
    wrapper.unmount();
  });

  it("offers Try again after Touch ID fails", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    sendEvent("ssh-approval-failed", { requestID: "request-1", message: "Touch ID did not confirm it's you. Try again, or deny." });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toContain("Touch ID did not confirm");
    expect(wrapper.find('[data-testid="approval-touch-id"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Try again");
    await wrapper.find('[data-testid="authorize-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(2);
    expect(wrapper.find('[data-testid="approval-touch-id"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("denies with the button or Escape and moves to the next request", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-2", appName: "Visual Studio Code", target: "github.com" }));
    await flushPromises();
    expect(wrapper.text()).toContain("1 more waiting");
    await wrapper.find('[data-testid="deny-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", false, false);
    expect(wrapper.find("h1").text()).toBe("Visual Studio Code wants to push to github.com");
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-2", true, true);
    pressKey("Escape");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-2", false, false);
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("waits for Approve on Macs without Touch ID", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent({ isTouchIDApproval: false }));
    await flushPromises();
    expect(api.respondSSHApproval).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Approve");
    expect(wrapper.find('[data-testid="use-password"]').exists()).toBe(false);
    pressKey("Enter");
    await flushPromises();
    pressKey("Enter");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(1);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, true);
    wrapper.unmount();
  });

  it("switches from Touch ID to the master password", async () => {
    const wrapper = mount(SSHApprovalWindow, { attachTo: document.body });
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    await wrapper.find('[data-testid="use-password"]').trigger("click");
    await flushPromises();
    expect(api.cancelApprovalTouchID).toHaveBeenCalled();
    expect(document.activeElement).toBe(wrapper.find('[data-testid="approval-password"]').element);
    sendEvent("ssh-approval-failed", { requestID: "request-1", message: "Touch ID did not confirm it's you. Try again, or deny." });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="submit-password"]').text()).toBe("Approve");
    await wrapper.find('[data-testid="approval-password"]').setValue("correct horse battery");
    pressKey("Enter");
    await flushPromises();
    expect(api.respondApprovalWithPassword).toHaveBeenCalledWith("request-1", "correct horse battery", true);
    sendEvent("ssh-approval-failed", { requestID: "request-1", message: "Incorrect master password." });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toBe("Incorrect master password.");
    wrapper.unmount();
  });

  it("unlocks a locked Hisar with Touch ID", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent({ isUnlockRequired: true, keyTitle: "" }));
    await flushPromises();
    expect(wrapper.find(".subtitle").text()).toBe("Unlock Hisar to use your SSH keys");
    expect(wrapper.find('[data-testid="approval-unlock-note"]').text()).toContain("approving also unlocks it");
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toContain("Touch ID to unlock and approve");
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, true);
    wrapper.unmount();
  });

  it("unlocks a locked Hisar with the master password", async () => {
    const wrapper = mount(SSHApprovalWindow, { attachTo: document.body });
    sendEvent("ssh-approval", createApprovalEvent({ isUnlockRequired: true, isPasswordUnlock: true }));
    await flushPromises();
    expect(api.respondSSHApproval).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(wrapper.find('[data-testid="approval-password"]').element);
    expect(wrapper.find('[data-testid="submit-password"]').text()).toBe("Unlock");
    expect(wrapper.find('[data-testid="submit-password"]').attributes("disabled")).toBeDefined();
    await wrapper.find('[data-testid="approval-password"]').setValue("correct horse battery");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.respondApprovalWithPassword).toHaveBeenCalledWith("request-1", "correct horse battery", true);
    expect((wrapper.find('[data-testid="approval-password"]').element as HTMLInputElement).value).toBe("");
    wrapper.unmount();
  });

  it("asks for every commit signature with the signing badge", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createSigningApprovalEvent());
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-signing-badge"]').text()).toBe("Sign a commit");
    expect(wrapper.find("h1").text()).toBe("Terminal wants to sign a commit");
    expect(wrapper.find(".subtitle").text()).toBe("with your SSH key “Signing key”");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["“Fix the login redirect”", "~/src/core · feature/login"]);
    expect(wrapper.find('input[name="isRemembered"]').exists()).toBe(false);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("signing-1", true, false);
    wrapper.unmount();
  });

  it("shows a signed tag, or the command when the commit is unknown", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createSigningApprovalEvent({ purpose: "sign-tag", action: "sign the tag", target: "v1.4.0", commitSubject: "Release 1.4.0" }));
    sendEvent("ssh-approval", createSigningApprovalEvent({ requestID: "signing-2", commitSubject: "", branch: "" }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-signing-badge"]').text()).toBe("Sign a tag");
    expect(wrapper.find("h1").text()).toBe("Terminal wants to sign the tag v1.4.0");
    await wrapper.find('[data-testid="deny-approval"]').trigger("click");
    await flushPromises();
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["git commit -S -m 'Fix the login redirect'", "~/src/core"]);
    wrapper.unmount();
  });

  it("warns about forwarded requests, allows them once and denies on Return", async () => {
    const wrapper = mount(SSHApprovalWindow, { attachTo: document.body });
    sendEvent("ssh-approval", createForwardedApprovalEvent());
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-forwarded"]').text()).toBe(
      "Forwarded from another computer — this request comes from build.example.com through agent forwarding, not from this Mac",
    );
    expect(wrapper.find("h1").text()).toBe("build.example.com wants to sign in to github.com");
    expect(wrapper.find('input[name="isRemembered"]').exists()).toBe(false);
    expect(api.respondSSHApproval).not.toHaveBeenCalled();
    const denyButton = wrapper.find('[data-testid="deny-approval"]');
    expect(denyButton.classes()).toContain("primary");
    expect(document.activeElement).toBe(denyButton.element);
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Allow once");
    await wrapper.find('[data-testid="authorize-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("forwarded-1", true, false);

    sendEvent("ssh-approval", createForwardedApprovalEvent({ requestID: "forwarded-2" }));
    sendEvent("ssh-approval-closed", "forwarded-1");
    await flushPromises();
    pressKey("Enter");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("forwarded-2", false, false);
    wrapper.unmount();
  });

  it("shows the compact card and expands it to the full dialog", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent({ isCompact: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-compact-title"]').text()).toBe("Terminal · Push");
    expect(wrapper.find(".compact-subtitle").text()).toBe("git.tryhisar.com with “Laptop key”");
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toBe("Touch ID to allow for 1 hour");
    expect(wrapper.find("h1").exists()).toBe(false);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, true);
    expect(api.fitSSHApprovalWindow).toHaveBeenLastCalledWith(true, expect.any(Number));
    await wrapper.find('[data-testid="ssh-approval"]').trigger("click");
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Terminal wants to push to git.tryhisar.com");
    expect(api.fitSSHApprovalWindow).toHaveBeenLastCalledWith(false, expect.any(Number));
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it("says the iPhone can approve too, and closes when the phone answered", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent({ isPhoneApproval: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toContain("Touch ID to approve");
    expect(wrapper.find('[data-testid="approval-phone"]').text()).toBe("or approve on your iPhone");
    sendEvent("ssh-approval-closed", "request-1");
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    expect(approvalStore.isAuthorizing).toBe(false);

    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-2", isPhoneApproval: true, isTouchIDApproval: false }));
    await flushPromises();
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Approve");
    expect(wrapper.find('[data-testid="approval-phone"]').text()).toBe("or approve on your iPhone");
    sendEvent("ssh-approval-closed", "request-2");
    await flushPromises();

    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-3", isPhoneApproval: true, isCompact: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-phone"]').text()).toBe("or approve on your iPhone");
    sendEvent("ssh-approval-closed", "request-3");
    await flushPromises();

    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-4" }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-phone"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("denies from the compact card without expanding it", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createSigningApprovalEvent({ isCompact: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-compact-title"]').text()).toBe("Terminal · Sign a commit");
    expect(wrapper.find(".compact-subtitle").text()).toBe("~/src/core with “Signing key”");
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toBe("Touch ID to sign");
    await wrapper.find('[data-testid="deny-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("signing-1", false, false);
    expect(approvalStore.isExpanded).toBe(false);
    wrapper.unmount();
  });

  it("asks for the command-line tool in a terminal and starts Touch ID", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createCommandLineApprovalEvent());
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("hisar in Terminal wants to use Hisar");
    expect(wrapper.find(".subtitle").text()).toBe("from the terminal session ttys003");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["/Applications/Hisar.app/Contents/Helpers/hisar", "/dev/ttys003"]);
    expect(wrapper.find('input[name="isRemembered"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("10 minutes after this terminal’s last command");
    expect(api.respondSSHApproval).toHaveBeenCalledWith("command-line-1", true, false);
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toContain("Touch ID to approve");
    wrapper.unmount();
  });

  it("unlocks a locked Hisar for the command-line tool", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createCommandLineApprovalEvent({ isUnlockRequired: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-unlock-note"]').text()).toContain("unlocking it also approves this terminal");
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toContain("Touch ID to unlock and approve");
    expect(wrapper.find('[data-testid="approval-password"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("unlocks a locked Hisar for the command-line tool with the master password", async () => {
    const wrapper = mount(SSHApprovalWindow, { attachTo: document.body });
    sendEvent("ssh-approval", createCommandLineApprovalEvent({ isUnlockRequired: true, isPasswordUnlock: true }));
    await flushPromises();
    expect(api.respondSSHApproval).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="approval-password"]').attributes("placeholder")).toBe("Master password");
    await wrapper.find('[data-testid="approval-password"]').setValue("wrong password");
    pressKey("Enter");
    await flushPromises();
    expect(api.respondApprovalWithPassword).toHaveBeenCalledWith("command-line-1", "wrong password", false);
    sendEvent("ssh-approval-failed", { requestID: "command-line-1", message: "Incorrect master password." });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toBe("Incorrect master password.");
    wrapper.unmount();
  });

  it("names the coding agent and where it runs when it connects", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createAgentApprovalEvent());
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("Claude Code wants to use Hisar");
    expect(wrapper.find(".subtitle").text()).toBe("via hisar mcp in Terminal");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["/opt/homebrew/bin/hisar", "/dev/ttys003"]);
    expect(wrapper.find('input[name="isRemembered"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="approval-agent-note"]').text()).toBe(
      "Lets Claude Code list and search the vaults available to agents until 10 minutes after its last request, or until Hisar locks. Using a secret always asks.",
    );
    expect(api.respondSSHApproval).toHaveBeenCalledWith("agent-1", true, false);
    wrapper.unmount();
  });

  it("shows the command, the directory and the secrets of a run, with remembering off by default", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createAgentRunApprovalEvent({ appName: "" }));
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("Claude Code wants to run a command with 2 secrets");
    expect(wrapper.find(".subtitle").text()).toBe("via hisar mcp");
    expect(wrapper.findAll(".detail-line").map((line) => line.text())).toEqual(["npm run deploy -- --env staging", "~/src/billing"]);
    expect(wrapper.findAll('[data-testid="approval-secrets"] li').map((line) => line.text())).toEqual(["Work › Stripe › api key", "Work › Postgres staging › password"]);
    expect(wrapper.find(".remember").text()).toBe("Allow these secrets for this agent for 1 hour");
    expect((wrapper.find('input[name="isRemembered"]').element as HTMLInputElement).checked).toBe(false);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("agent-run-1", true, false);
    await wrapper.find('input[name="isRemembered"]').setValue(true);
    expect(api.setSSHApprovalRemembered).toHaveBeenCalledWith("agent-run-1", true);
    expect(wrapper.find('[data-testid="approval-agent-note"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("warns that a revealed secret becomes visible to the agent", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    sendEvent("ssh-approval", createAgentRevealApprovalEvent({ isConnectionApproval: true }));
    await flushPromises();
    sendEvent("ssh-approval-closed", "request-1");
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("Claude Code wants to read “Stripe › api key”");
    expect(wrapper.find('[data-testid="approval-warning"]').text()).toBe("The value will be visible to Claude Code and can end up in its transcript and logs.");
    expect(wrapper.find(".remember").text()).toBe("Allow this item for 1 hour");
    expect((wrapper.find('input[name="isRemembered"]').element as HTMLInputElement).checked).toBe(false);
    expect(approvalStore.isRemembered).toBe(false);
    expect(wrapper.find('[data-testid="approval-agent-note"]').text()).toBe("Approving also lets Claude Code list and search the vaults available to agents.");
    wrapper.unmount();
  });

  it("unlocks a locked Hisar for a coding agent", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createAgentRevealApprovalEvent({ isUnlockRequired: true, isPasswordUnlock: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-unlock-note"]').text()).toBe("Hisar is locked; unlocking it also approves this request.");
    await wrapper.find('[data-testid="approval-password"]').setValue("correct horse");
    await wrapper.find(".password-row").trigger("submit");
    await flushPromises();
    expect(api.respondApprovalWithPassword).toHaveBeenCalledWith("agent-reveal-1", "correct horse", false);
    wrapper.unmount();
  });
});

describe("SSHApprovalWindow app attribution", () => {
  const allSigningChain = [
    { name: "ssh-keygen", commandLine: "/usr/bin/ssh-keygen -Y sign -n git -f /Users/developer/.ssh/signing.pub /tmp/.git_signing_buffer_tmpX" },
    { name: "git", commandLine: "git commit -S -m 'Fix the login redirect'" },
    { name: "zsh", commandLine: "-zsh" },
  ];

  function createITermSigningEvent(overrides = {}) {
    return createSigningApprovalEvent({
      appName: "iTerm2",
      appIconURL: "data:image/png;base64,iTerm",
      appPath: "/Applications/iTerm.app",
      appSignature: "developer",
      appSigner: "Signed by George Nachman (H7V7XYVQ7D)",
      allChainProcess: allSigningChain,
      ...overrides,
    });
  }

  it("names the app behind a commit signature, with its icon, chain and signature", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createITermSigningEvent());
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("iTerm2 wants to sign a commit");
    expect(wrapper.find('[data-testid="app-icon"]').attributes("src")).toBe("data:image/png;base64,iTerm");
    const chain = wrapper.find('[data-testid="approval-chain"]');
    expect(chain.text()).toBe("ssh-keygen ← git ← zsh");
    expect(chain.attributes("title")).toBe(allSigningChain.map((process) => process.commandLine).join("\n"));
    expect(wrapper.find('[data-testid="approval-signer"]').text()).toBe("Signed by George Nachman (H7V7XYVQ7D)");
    expect(wrapper.find('[data-testid="approval-signer"]').classes()).not.toContain("is-unverified");
    expect(wrapper.find('[data-testid="approval-app-warning"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("names Visual Studio Code for its git extension", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent(
      "ssh-approval",
      createITermSigningEvent({ appName: "Visual Studio Code", appSigner: "Signed by Microsoft Corporation (UBF8T346G9)", allChainProcess: allSigningChain.slice(0, 2) }),
    );
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Visual Studio Code wants to sign a commit");
    expect(wrapper.find('[data-testid="approval-chain"]').text()).toBe("ssh-keygen ← git");
    expect(wrapper.find('[data-testid="approval-signer"]').text()).toBe("Signed by Microsoft Corporation (UBF8T346G9)");
    wrapper.unmount();
  });

  it("warns about an app that is not signed, in the dialog and the compact card", async () => {
    const appWarning = "iTerm2 is not signed, so macOS cannot confirm who made it. Approve only if you started this yourself.";
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createITermSigningEvent({ appSignature: "unsigned", appSigner: "Not signed", appWarning }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-signer"]').text()).toBe("Not signed");
    expect(wrapper.find('[data-testid="approval-signer"]').classes()).toContain("is-unverified");
    expect(wrapper.find('[data-testid="approval-app-warning"]').text()).toBe(appWarning);
    sendEvent("ssh-approval-closed", "signing-1");
    sendEvent("ssh-approval", createITermSigningEvent({ requestID: "compact-1", isCompact: true, appSignature: "invalid", appSigner: "Signature not valid", appWarning }));
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-compact-title"]').text()).toBe("iTerm2 · Sign a commit");
    expect(wrapper.find('[data-testid="approval-app-warning"]').text()).toBe("Signature not valid — check this is the app you expect");
    expect(wrapper.find('[data-testid="ssh-approval"]').attributes("title")).toBe("Show details\nssh-keygen ← git ← zsh");
    wrapper.unmount();
  });

  it("says what stands in for an app when there is none", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent({ requesterKind: "background", appName: "A background process", appSignature: "", appSigner: "", appPath: "" }));
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("A background process wants to push to git.tryhisar.com");
    expect(wrapper.find('[data-testid="app-icon"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="approval-signer"]').exists()).toBe(false);
    expect(wrapper.find(".remember").text()).toBe("Don’t ask again for ssh for 1 hour");
    sendEvent("ssh-approval-closed", "request-1");
    sendEvent("ssh-approval", createCommandLineApprovalEvent({ requesterKind: "ssh-session", appName: "An SSH session from 192.0.2.10", appSigner: "" }));
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("hisar in an SSH session from 192.0.2.10 wants to use Hisar");
    wrapper.unmount();
  });

  it("names the app that creates a share link", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent(
      "ssh-approval",
      createCommandLineApprovalEvent({
        action: "create a share link for “GitHub”",
        commandLine: "Expires in 7 days · 1 view",
        allSecretName: ["username", "password"],
        appName: "iTerm2",
        appIconURL: "data:image/png;base64,iTerm",
        appSigner: "Signed by George Nachman (H7V7XYVQ7D)",
        allChainProcess: [
          { name: "hisar", commandLine: "hisar share-link GitHub" },
          { name: "zsh", commandLine: "-zsh" },
        ],
      }),
    );
    await flushPromises();
    expect(wrapper.find("#approval-title").text()).toBe("hisar in iTerm2 wants to create a share link for “GitHub”");
    expect(wrapper.find('[data-testid="app-icon"]').attributes("src")).toBe("data:image/png;base64,iTerm");
    expect(wrapper.find('[data-testid="approval-chain"]').text()).toBe("hisar ← zsh");
    expect(wrapper.find('[data-testid="approval-signer"]').text()).toBe("Signed by George Nachman (H7V7XYVQ7D)");
    wrapper.unmount();
  });

  it("leaves the local chain out of a forwarded request", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createForwardedApprovalEvent());
    await flushPromises();
    expect(wrapper.find('[data-testid="approval-requester"]').exists()).toBe(false);
    wrapper.unmount();
  });
});
