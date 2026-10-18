import { setTimeout as sleep } from "node:timers/promises";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { allAppErrorCode } from "./errors";
import {
  answerHandshake,
  createSocketDirectory,
  startFakeApp,
  type Answer,
  type FakeApp,
  type SocketDirectory,
} from "./fake-app";
import { Hisar, HisarError, type LaunchSystem } from "./index";
import { resolveSocketPath } from "./launch";
import { allLauncherRequestType } from "./protocol";

const github = { vaultID: "v-personal", itemID: "i-github" };
const emptyList = { vaults: [], items: [] };

let directory: SocketDirectory;
let app: FakeApp | undefined;
let hisar: Hisar | undefined;

beforeEach(() => {
  directory = createSocketDirectory();
});

afterEach(async () => {
  vi.useRealTimers();
  hisar?.close();
  hisar = undefined;
  await app?.close();
  app = undefined;
  directory.remove();
});

async function startApp(answer: Answer, allFeature?: readonly string[]): Promise<FakeApp> {
  app = await startFakeApp(directory.socketPath, answerHandshake(answer, allFeature));
  return app;
}

function connect(launchSystem?: LaunchSystem): Hisar {
  hisar = new Hisar({ socketPath: directory.socketPath, clientVersion: "0.1.0", launchSystem });
  return hisar;
}

async function catchError(promise: Promise<unknown>): Promise<HisarError> {
  const error = await promise.then(
    () => undefined,
    (caught: unknown) => caught,
  );
  expect(error).toBeInstanceOf(HisarError);
  return error as HisarError;
}

interface FakeLaunchSystem extends LaunchSystem {
  openCount: number;
}

function createLaunchSystem(isAppRunning: boolean, openApp: () => Promise<void>): FakeLaunchSystem {
  const system: FakeLaunchSystem = {
    openCount: 0,
    openApp: async () => {
      system.openCount += 1;
      await openApp();
    },
    isAppRunning: async () => isAppRunning,
  };
  return system;
}

describe("features Hisar announces", () => {
  it("refuses a request Hisar did not announce with update-required, without sending it", async () => {
    const withoutCopy = allLauncherRequestType.filter((type) => type !== "launcher.items.copy");
    const fakeApp = await startApp((request) => ({ id: request.id, ok: true, result: emptyList }), withoutCopy);
    const client = connect();

    expect((await catchError(client.copy(github, "password"))).code).toBe("update-required");
    expect(await client.listItems()).toEqual({ allVault: [], allItem: [] });
    expect(fakeApp.getRequestTypes()).toEqual(["hello", "launcher.hello", "launcher.items.list"]);
  });

  it("needs an update to open or share an item when Hisar announces neither, and sends neither", async () => {
    const beforeOpenAndShare = allLauncherRequestType.filter(
      (type) => type !== "launcher.items.open" && type !== "launcher.items.share",
    );
    const fakeApp = await startApp((request) => ({ id: request.id, ok: true, result: emptyList }), beforeOpenAndShare);
    const client = connect();

    expect((await catchError(client.open(github))).code).toBe("update-required");
    expect((await catchError(client.open(github, { isEditing: true }))).code).toBe("update-required");
    const sharing = client.share(github, { allField: ["password"], expiresInSeconds: 3_600, maxViews: 1 });
    expect((await catchError(sharing)).code).toBe("update-required");
    expect(await client.listItems()).toEqual({ allVault: [], allItem: [] });
    expect(fakeApp.getRequestTypes()).toEqual(["hello", "launcher.hello", "launcher.items.list"]);
  });

  it("needs an update of a Hisar that knows no launchers, and sends it nothing after hello", async () => {
    const fakeApp = await startApp(() => undefined, ["favorites"]);

    expect((await catchError(connect().listItems())).code).toBe("update-required");
    expect(fakeApp.getRequestTypes()).toEqual(["hello"]);
  });
});

describe("errors Hisar answers with", () => {
  it("keeps each code of the contract and the app's sentence, and reads an unknown code as internal", async () => {
    const allCode = [...allAppErrorCode, "teapot"];
    let index = 0;
    await startApp((request) => ({
      id: request.id,
      ok: false,
      error: { code: allCode[index++], message: "Because." },
    }));
    const client = connect();

    const allError: HisarError[] = [];
    for (let count = 0; count < allCode.length; count += 1) allError.push(await catchError(client.listItems()));

    expect(allError.map((error) => error.code)).toEqual([...allAppErrorCode, "internal"]);
    expect(allError.every((error) => error.message === "Because.")).toBe(true);
  });
});

describe("one connection", () => {
  it("sends the next request only after Hisar answered the one before", async () => {
    const { promise: release, resolve: answerList } = Promise.withResolvers<void>();
    const fakeApp = await startApp(async (request) => {
      if (request.type === "launcher.items.list") {
        await release;
        return { id: request.id, ok: true, result: emptyList };
      }
      return { id: request.id, ok: true, result: { password: "hunter2" } };
    });
    const client = connect();

    const listing = client.listItems();
    const reading = client.readPassword(github);
    await vi.waitFor(() => expect(fakeApp.getRequestTypes()).toContain("launcher.items.list"));
    await sleep(50);
    expect(fakeApp.getRequestTypes()).not.toContain("launcher.items.password");

    answerList();
    expect(await listing).toEqual({ allVault: [], allItem: [] });
    expect(await reading).toBe("hunter2");
    expect(fakeApp.getRequestTypes()).toEqual([
      "hello",
      "launcher.hello",
      "launcher.items.list",
      "launcher.items.password",
    ]);
  });

  it("gives up on a request Hisar does not answer in time, and connects again for the next one", async () => {
    let listCount = 0;
    const fakeApp = await startApp((request) => {
      listCount += 1;
      return listCount === 1 ? undefined : { id: request.id, ok: true, result: emptyList };
    });
    const client = connect();
    expect(await client.isApproved()).toBe(true);

    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const timingOut = catchError(client.listItems());
    await vi.waitFor(() => expect(fakeApp.getRequestTypes()).toContain("launcher.items.list"));
    await vi.advanceTimersByTimeAsync(149_000);
    expect(fakeApp.getRequestTypes()).toEqual(["hello", "launcher.hello", "launcher.items.list"]);
    await vi.advanceTimersByTimeAsync(1_000);
    expect((await timingOut).code).toBe("timeout");
    vi.useRealTimers();

    expect(await client.listItems()).toEqual({ allVault: [], allItem: [] });
    expect(fakeApp.getRequestTypes().slice(3)).toEqual(["hello", "launcher.hello", "launcher.items.list"]);
  });
});

describe("starting Hisar", () => {
  it("starts Hisar hidden when nothing listens on its socket, then waits for it", async () => {
    const launchSystem = createLaunchSystem(false, async () => {
      await startApp((request) => ({ id: request.id, ok: true, result: emptyList }));
    });

    expect(await connect(launchSystem).listItems()).toEqual({ allVault: [], allItem: [] });
    expect(launchSystem.openCount).toBe(1);
  });

  it("never starts Hisar to lock it", async () => {
    const launchSystem = createLaunchSystem(false, async () => undefined);

    expect((await catchError(connect(launchSystem).lock())).code).toBe("app-not-running");
    expect(launchSystem.openCount).toBe(0);
  });

  it("asks to turn on the integration when Hisar runs without its socket, and does not start it again", async () => {
    const launchSystem = createLaunchSystem(true, async () => undefined);

    expect((await catchError(connect(launchSystem).listItems())).code).toBe("integration-disabled");
    expect(launchSystem.openCount).toBe(0);
  });

  it("says Hisar is not installed when it cannot be started", async () => {
    const launchSystem = createLaunchSystem(false, async () => {
      throw new Error("Unable to find application with bundle identifier com.tryhisar.hisar");
    });

    expect((await catchError(connect(launchSystem).listItems())).code).toBe("app-not-installed");
  });
});

describe("resolveSocketPath", () => {
  it("uses ~/.hisar/cli.sock when the preference is empty and expands a leading ~/", () => {
    expect(resolveSocketPath(undefined, "/home/alice")).toBe("/home/alice/.hisar/cli.sock");
    expect(resolveSocketPath("  ", "/home/alice")).toBe("/home/alice/.hisar/cli.sock");
    expect(resolveSocketPath("~/.hisar/cli.sock", "/home/alice")).toBe("/home/alice/.hisar/cli.sock");
    expect(resolveSocketPath(" ~/dev/hisar/cli.sock ", "/home/alice")).toBe("/home/alice/dev/hisar/cli.sock");
    expect(resolveSocketPath("/tmp/hisar/cli.sock", "/home/alice")).toBe("/tmp/hisar/cli.sock");
  });
});
