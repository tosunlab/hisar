import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createCommandLineStatus, createLauncherApp, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const allListener = vi.hoisted(() => new Map<string, () => void>());

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: () => void) => {
      allListener.set(name, listener);
      return () => allListener.delete(name);
    },
  },
}));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }),
  };
});

const { store } = await import("../store");
const CommandLineSection = (await import("./CommandLineSection.vue")).default;
const api = apiMock.current;

enableAutoUnmount(afterEach);

const installedStatus = createCommandLineStatus({
  installedPath: "/opt/homebrew/bin/hisar",
  allInstallation: [{ path: "/opt/homebrew/bin/hisar", kind: "this-app", target: "/Applications/Hisar.app/Contents/Helpers/hisar", isSymlink: true, description: "this app's command-line tool" }],
  pathCommandPath: "/opt/homebrew/bin/hisar",
  pathCommandTarget: "/Applications/Hisar.app/Contents/Helpers/hisar",
  isPathCommandFromThisApp: true,
});

beforeEach(() => {
  vi.clearAllMocks();
  allListener.clear();
  Object.assign(store, {
    status: { ...createStatus("unlocked"), biometricAvailable: true },
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    commandLineStatus: null,
    commandLineErrorMessage: "",
    allLauncherApp: [],
  });
  api.getCommandLineStatus.mockResolvedValue(createCommandLineStatus());
  api.listAllLauncherApp.mockResolvedValue([]);
});

describe("CommandLineSection", () => {
  it("offers to install the command-line tool", async () => {
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-state"]').text()).toBe("Not installed");
    expect(wrapper.find('[data-testid="command-line-path"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="install-command-line"]').text()).toBe("Install command-line tool");
    expect((wrapper.find('input[name="commandLineIntegration"]').element as HTMLInputElement).checked).toBe(true);
    expect(wrapper.find('[data-testid="command-line-integration"]').text()).toContain("approve each terminal with Touch ID");

    api.installCommandLineTool.mockResolvedValue(installedStatus);
    await wrapper.find('[data-testid="install-command-line"]').trigger("click");
    await flushPromises();
    expect(api.installCommandLineTool).toHaveBeenCalledTimes(1);
    expect(store.noticeMessage).toBe("Installed the hisar command");
    expect(wrapper.find('[data-testid="command-line-state"]').text()).toBe("Installed at /opt/homebrew/bin/hisar");
    expect(wrapper.find('[data-testid="command-line-path"]').text()).toBe("hisar in your terminal is this app's.");
    expect(wrapper.find('[data-testid="uninstall-command-line"]').exists()).toBe(true);
  });

  it("uninstalls", async () => {
    api.getCommandLineStatus.mockResolvedValue(installedStatus);
    api.uninstallCommandLineTool.mockResolvedValue(createCommandLineStatus());
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    await wrapper.find('[data-testid="uninstall-command-line"]').trigger("click");
    await flushPromises();
    expect(api.uninstallCommandLineTool).toHaveBeenCalledTimes(1);
    expect(store.noticeMessage).toBe("Removed the hisar command");
    expect(wrapper.find('[data-testid="install-command-line"]').exists()).toBe(true);
  });

  it("shows a hisar that is not from Hisar and why installing failed", async () => {
    api.getCommandLineStatus.mockResolvedValue(
      createCommandLineStatus({
        allInstallation: [{ path: "/opt/homebrew/bin/hisar", kind: "other", target: "", isSymlink: false, description: "a file that is not from Hisar" }],
        pathCommandPath: "/opt/homebrew/bin/hisar",
        pathCommandTarget: "/opt/homebrew/Cellar/hisar/1.0/bin/hisar",
      }),
    );
    api.installCommandLineTool.mockRejectedValue(new Error("/opt/homebrew/bin/hisar is already a file that is not from Hisar; Hisar leaves it alone"));
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-conflict"]').text()).toContain("/opt/homebrew/bin/hisar is a file that is not from Hisar; Hisar leaves it alone.");
    expect(wrapper.find('[data-testid="command-line-path"]').text()).toContain("is /opt/homebrew/bin/hisar (/opt/homebrew/Cellar/hisar/1.0/bin/hisar), not this app's.");
    await wrapper.find('[data-testid="install-command-line"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-error"]').text()).toContain("leaves it alone");
    expect(store.errorMessage).toBe("");
  });

  it("shows the PATH hint for ~/.local/bin", async () => {
    api.getCommandLineStatus.mockResolvedValue(
      createCommandLineStatus({
        installedPath: "/Users/alice/.local/bin/hisar",
        pathHint: `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc`,
      }),
    );
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-path"]').text()).toBe("/Users/alice/.local/bin is not on your PATH yet. Add it with:");
    expect(wrapper.find('[data-testid="command-line-path-hint"]').text()).toBe(`echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc`);
  });

  it("asks for a new terminal when the PATH has the directory but the shell has not found hisar yet", async () => {
    api.getCommandLineStatus.mockResolvedValue(createCommandLineStatus({ installedPath: "/usr/local/bin/hisar" }));
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-path"]').text()).toBe("hisar is not on your PATH yet; open a new terminal.");
    expect(wrapper.find('[data-testid="command-line-path-hint"]').exists()).toBe(false);
  });

  it("has nothing to install in a development build", async () => {
    api.getCommandLineStatus.mockResolvedValue(createCommandLineStatus({ isAvailable: false }));
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-state"]').text()).toContain("development build has none");
    expect(wrapper.find('[data-testid="install-command-line"]').exists()).toBe(false);
  });

  it("turns the app integration off and on", async () => {
    api.setCommandLineIntegration.mockResolvedValue(createCommandLineStatus({ isIntegrationEnabled: false, isServing: false }));
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    await wrapper.find('input[name="commandLineIntegration"]').setValue(false);
    await flushPromises();
    expect(api.setCommandLineIntegration).toHaveBeenCalledWith(false);
    expect(store.noticeMessage).toBe("hisar signs in on its own now");
    expect((wrapper.find('input[name="commandLineIntegration"]').element as HTMLInputElement).checked).toBe(false);

    api.setCommandLineIntegration.mockResolvedValue(createCommandLineStatus());
    await wrapper.find('input[name="commandLineIntegration"]').setValue(true);
    await flushPromises();
    expect(api.setCommandLineIntegration).toHaveBeenLastCalledWith(true);
    expect(store.commandLineStatus?.isIntegrationEnabled).toBe(true);
  });

  it("explains a socket that could not be opened", async () => {
    api.getCommandLineStatus.mockResolvedValue(createCommandLineStatus({ isServing: false, errorMessage: "another program listens on /Users/alice/.hisar/cli.sock" }));
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-integration"]').text()).toBe("Not available: another program listens on /Users/alice/.hisar/cli.sock");
  });

  it("says when no app is approved as a launcher", async () => {
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-launchers"] strong').text()).toBe("Launchers");
    expect(wrapper.find('[data-testid="launcher-apps-empty"]').text()).toBe("No app is approved right now.");
    expect(wrapper.findAll('[data-testid="launcher-app"]')).toHaveLength(0);
  });

  it("shows a launcher approved while the section is open", async () => {
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="launcher-apps-empty"]').exists()).toBe(true);
    api.listAllLauncherApp.mockResolvedValue([createLauncherApp()]);
    allListener.get("ssh-approval-closed")?.();
    await flushPromises();
    expect(wrapper.findAll('[data-testid="launcher-app"] strong').map((name) => name.text())).toEqual(["Raycast"]);
    expect(wrapper.find('[data-testid="launcher-apps-empty"]').exists()).toBe(false);
  });

  it("clears the launchers when Hisar locks", async () => {
    api.listAllLauncherApp.mockResolvedValue([createLauncherApp()]);
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.findAll('[data-testid="launcher-app"]')).toHaveLength(1);
    api.listAllLauncherApp.mockResolvedValue([]);
    allListener.get("locked")?.();
    await flushPromises();
    expect(wrapper.find('[data-testid="launcher-apps-empty"]').exists()).toBe(true);
    expect(wrapper.findAll('[data-testid="launcher-app"]')).toHaveLength(0);
  });

  it("lists the apps approved as launchers and revokes one", async () => {
    const alfred = createLauncherApp({ name: "Alfred 5", path: "/Applications/Alfred 5.app", iconURL: "", lastUsedAt: new Date(Date.now() - 3 * 3_600_000).toISOString() });
    api.listAllLauncherApp.mockResolvedValue([createLauncherApp({ lastUsedAt: new Date().toISOString() }), alfred]);
    api.revokeLauncherApp.mockResolvedValue([alfred]);
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="launcher-apps-empty"]').exists()).toBe(false);
    const allRow = wrapper.findAll('[data-testid="launcher-app"]');
    expect(allRow.map((row) => [row.find("strong").text(), row.find("p").text()])).toEqual([
      ["Raycast", "Last used just now"],
      ["Alfred 5", "Last used 3 hours ago"],
    ]);
    expect(allRow[0].find("img").attributes("src")).toBe("data:image/png;base64,Raycast");

    await wrapper.find('[data-testid="revoke-launcher-Raycast"]').trigger("click");
    await flushPromises();
    expect(api.revokeLauncherApp).toHaveBeenCalledWith("/Applications/Raycast.app");
    expect(store.noticeMessage).toBe("Raycast asks again the next time it uses Hisar");
    expect(wrapper.findAll('[data-testid="launcher-app"] strong').map((name) => name.text())).toEqual(["Alfred 5"]);
  });

  it("keeps a launcher whose revoking failed and says why", async () => {
    api.listAllLauncherApp.mockResolvedValue([createLauncherApp()]);
    api.revokeLauncherApp.mockRejectedValue(new Error("Hisar could not reach the command-line socket"));
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    await wrapper.find('[data-testid="revoke-launcher-Raycast"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-error"]').text()).toBe("Hisar could not reach the command-line socket");
    expect(store.noticeMessage).toBe("");
    expect(wrapper.findAll('[data-testid="launcher-app"]')).toHaveLength(1);
  });

  it("ends every launcher approval when the integration is turned off", async () => {
    api.listAllLauncherApp.mockResolvedValue([createLauncherApp()]);
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.findAll('[data-testid="launcher-app"]')).toHaveLength(1);

    api.setCommandLineIntegration.mockResolvedValue(createCommandLineStatus({ isIntegrationEnabled: false, isServing: false }));
    api.listAllLauncherApp.mockResolvedValue([]);
    await wrapper.find('input[name="commandLineIntegration"]').setValue(false);
    await flushPromises();
    expect(wrapper.findAll('[data-testid="launcher-app"]')).toHaveLength(0);
    expect(wrapper.find('[data-testid="launcher-apps-empty"]').exists()).toBe(true);
  });
});
