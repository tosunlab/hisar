import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createCommandLineStatus, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

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

const installedStatus = createCommandLineStatus({
  installedPath: "/opt/homebrew/bin/hisar",
  allInstallation: [{ path: "/opt/homebrew/bin/hisar", kind: "this-app", target: "/Applications/Hisar.app/Contents/Helpers/hisar", isSymlink: true, description: "this app's command-line tool" }],
  pathCommandPath: "/opt/homebrew/bin/hisar",
  pathCommandTarget: "/Applications/Hisar.app/Contents/Helpers/hisar",
  isPathCommandFromThisApp: true,
});

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: { ...createStatus("unlocked"), biometricAvailable: true },
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    commandLineStatus: null,
    commandLineErrorMessage: "",
  });
  api.getCommandLineStatus.mockResolvedValue(createCommandLineStatus());
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

  it("shows the hisar the Linux package installed, with nothing to install or uninstall", async () => {
    api.getCommandLineStatus.mockResolvedValue(
      createCommandLineStatus({
        bundledPath: "/usr/bin/hisar",
        installedPath: "/usr/bin/hisar",
        pathCommandPath: "/usr/bin/hisar",
        pathCommandTarget: "/usr/bin/hisar",
        isPathCommandFromThisApp: true,
        isInstalledByPackage: true,
      }),
    );
    const wrapper = mount(CommandLineSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="command-line-state"]').text()).toBe("Installed with the Hisar package at /usr/bin/hisar");
    expect(wrapper.find('[data-testid="command-line-path"]').text()).toBe("hisar in your terminal is this app's.");
    expect(wrapper.find('[data-testid="install-command-line"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="uninstall-command-line"]').exists()).toBe(false);
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
});
